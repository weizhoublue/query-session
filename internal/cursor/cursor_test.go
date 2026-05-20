package cursor

import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestExtractUserQueryText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"array part", `{"role":"user","content":[{"type":"text","text":"<user_query>\nhello\n</user_query>"}]}`, "hello"},
		{"plain string", `{"role":"user","content":"<user_query>fix bug</user_query>"}`, "fix bug"},
		{"no tag", `{"role":"user","content":"plain text"}`, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractUserQueryText([]byte(tc.in))
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestIsValidUserQueryMessage(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want bool
	}{
		{
			name: "context injection",
			data: []byte(`{"role":"user","providerOptions":{"cursor":{"requestContextCompleteness":{"rules":true}}},"content":"<user_info>...</user_info>"}`),
			want: false,
		},
		{
			name: "real user query",
			data: []byte(`{"role":"user","providerOptions":{"cursor":{"requestId":"x"}},"content":[{"type":"text","text":"<user_query>\nhi\n</user_query>"}]}`),
			want: true,
		},
		{
			name: "null requestContextCompleteness",
			data: []byte(`{"role":"user","providerOptions":{"cursor":{"requestContextCompleteness":null}},"content":"<user_query>ok</user_query>"}`),
			want: true,
		},
		{
			name: "invalid json",
			data: []byte(`not json`),
			want: false,
		},
		{
			name: "assistant role",
			data: []byte(`{"role":"assistant","content":"<user_query>hi</user_query>"}`),
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isValidUserQueryMessage(tc.data)
			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestFileURIToPath(t *testing.T) {
	if got := fileURIToPath("file:///repo/from-protobuf"); got != "/repo/from-protobuf" {
		t.Fatalf("got %q", got)
	}
	if got := fileURIToPath("https://example.com/x"); got != "" {
		t.Fatalf("got %q want empty", got)
	}
}

func createTestStoreDB(t *testing.T, dir string, meta storeMeta, blobs []struct {
	id   string
	data []byte
}) string {
	t.Helper()
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal meta: %v", err)
	}
	return createTestStoreDBWithMetaValue(t, dir, []byte(hex.EncodeToString(metaJSON)), blobs)
}

func createTestStoreDBWithMetaValue(t *testing.T, dir string, metaValue []byte, blobs []struct {
	id   string
	data []byte
}) string {
	t.Helper()
	path := filepath.Join(dir, "store.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE meta (key TEXT, value BLOB);
CREATE TABLE blobs (id TEXT, data BLOB);`)
	if err != nil {
		t.Fatalf("schema: %v", err)
	}

	if metaValue != nil {
		if _, err := db.Exec(`INSERT INTO meta(key,value) VALUES('0',?)`, metaValue); err != nil {
			t.Fatalf("meta: %v", err)
		}
	}
	for _, b := range blobs {
		if _, err := db.Exec(`INSERT INTO blobs(id,data) VALUES(?,?)`, b.id, b.data); err != nil {
			t.Fatalf("blob: %v", err)
		}
	}
	return path
}

func appendVarint(buf []byte, x uint64) []byte {
	for x >= 0x80 {
		buf = append(buf, byte(x)|0x80)
		x >>= 7
	}
	return append(buf, byte(x))
}

func encodeProtobufStringField(fieldNum int, value string) []byte {
	var buf []byte
	buf = appendVarint(buf, uint64(fieldNum<<3|2))
	buf = appendVarint(buf, uint64(len(value)))
	return append(buf, value...)
}

func TestScanExtractsCursorSession(t *testing.T) {
	root := t.TempDir()
	chatID := "chat-abc"
	sessionID := "sess-1111-2222-3333-4444"
	sessionDir := filepath.Join(root, chatID, sessionID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	createdMs := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC).UnixMilli()
	injectionBlob := []byte(`{"role":"user","providerOptions":{"cursor":{"requestContextCompleteness":{"rules":true}}},"content":"<user_info>\nWorkspace Path: /repo/test-project\n</user_info>"}`)
	firstUser := []byte(`{"role":"user","providerOptions":{"cursor":{"requestId":"a"}},"content":[{"type":"text","text":"<user_query>\nfirst question\n</user_query>"}]}`)
	lastUser := []byte(`{"role":"user","providerOptions":{"cursor":{"requestId":"b"}},"content":[{"type":"text","text":"<user_query>\nlast question\n</user_query>"}]}`)

	dbPath := createTestStoreDB(t, sessionDir, storeMeta{
		AgentID:   sessionID,
		CreatedAt: createdMs,
	}, []struct {
		id   string
		data []byte
	}{
		{"blob-inject", injectionBlob},
		{"blob-first", firstUser},
		{"blob-last", lastUser},
	})

	future := time.Date(2026, 5, 20, 18, 0, 0, 0, time.UTC)
	if err := os.Chtimes(dbPath, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	got, err := Scan(root, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d sessions", len(got))
	}
	s := got[0]
	if s.SessionID != sessionID {
		t.Fatalf("sessionId=%q", s.SessionID)
	}
	if s.Dir != "/repo/test-project" {
		t.Fatalf("dir=%q", s.Dir)
	}
	if s.File != dbPath {
		t.Fatalf("file=%q", s.File)
	}
	if s.FirstMsg != "first question" || s.LastMsg != "last question" {
		t.Fatalf("msgs=%q / %q", s.FirstMsg, s.LastMsg)
	}
	if s.UserMsgAmount != 2 {
		t.Fatalf("userMsgAmount=%d", s.UserMsgAmount)
	}
	wantCreate := time.UnixMilli(createdMs).Local()
	if !s.CreateTime.Equal(wantCreate) {
		t.Fatalf("createTime=%v want %v", s.CreateTime, wantCreate)
	}
	if !s.LastTime.Equal(future.Local()) {
		t.Fatalf("lastTime=%v want %v", s.LastTime, future.Local())
	}
}

func TestScanSkipsStoreWithoutUserQuery(t *testing.T) {
	root := t.TempDir()
	sessionDir := filepath.Join(root, "c", "s")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	onlyInjection := []byte(`{"role":"user","providerOptions":{"cursor":{"requestContextCompleteness":{}}},"content":"<user_info>only</user_info>"}`)
	createTestStoreDB(t, sessionDir, storeMeta{AgentID: "s", CreatedAt: 1}, []struct {
		id   string
		data []byte
	}{{"b1", onlyInjection}})

	got, err := Scan(root, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want 0 got %d", len(got))
	}
}

func TestScanResolvesWorkspaceFromProtobufField9(t *testing.T) {
	root := t.TempDir()
	sessionID := "sess-proto-ws"
	sessionDir := filepath.Join(root, "chat-1", sessionID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	rootBlobID := "root-blob-proto"
	rootBlob := encodeProtobufStringField(9, "file:///repo/from-protobuf")
	userBlob := []byte(`{"role":"user","content":"<user_query>only query</user_query>"}`)

	createTestStoreDB(t, sessionDir, storeMeta{
		AgentID:          sessionID,
		CreatedAt:        1,
		LatestRootBlobID: rootBlobID,
	}, []struct {
		id   string
		data []byte
	}{
		{rootBlobID, rootBlob},
		{"user-1", userBlob},
	})

	got, err := Scan(root, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d sessions", len(got))
	}
	if got[0].Dir != "/repo/from-protobuf" {
		t.Fatalf("dir=%q want protobuf workspace", got[0].Dir)
	}
}

func TestScanFallbackSessionIDFromDirectory(t *testing.T) {
	root := t.TempDir()
	sessionID := "dir-session-id-1234"
	sessionDir := filepath.Join(root, "chat-x", sessionID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	userBlob := []byte(`{"role":"user","content":"<user_query>hi</user_query>"}`)
	createTestStoreDB(t, sessionDir, storeMeta{CreatedAt: 1}, []struct {
		id   string
		data []byte
	}{{"u1", userBlob}})

	got, err := Scan(root, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got) != 1 || got[0].SessionID != sessionID {
		t.Fatalf("sessionId=%q want %q", got[0].SessionID, sessionID)
	}
}

func TestScanSkipsInvalidMeta(t *testing.T) {
	root := t.TempDir()
	userBlob := []byte(`{"role":"user","content":"<user_query>hi</user_query>"}`)

	t.Run("missing meta row", func(t *testing.T) {
		sessionDir := filepath.Join(root, "c1", "s1")
		if err := os.MkdirAll(sessionDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		createTestStoreDBWithMetaValue(t, sessionDir, nil, []struct {
			id   string
			data []byte
		}{{"b", userBlob}})
	})

	t.Run("invalid hex meta", func(t *testing.T) {
		sessionDir := filepath.Join(root, "c2", "s2")
		if err := os.MkdirAll(sessionDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		createTestStoreDBWithMetaValue(t, sessionDir, []byte("not-hex-json"), []struct {
			id   string
			data []byte
		}{{"b", userBlob}})
	})

	t.Run("invalid json after hex", func(t *testing.T) {
		sessionDir := filepath.Join(root, "c3", "s3")
		if err := os.MkdirAll(sessionDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		createTestStoreDBWithMetaValue(t, sessionDir, []byte(hex.EncodeToString([]byte("not-json"))), []struct {
			id   string
			data []byte
		}{{"b", userBlob}})
	})

	got, err := Scan(root, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want 0 sessions for invalid meta, got %d", len(got))
	}
}

func TestLoadStoreMetaPlainJSON(t *testing.T) {
	root := t.TempDir()
	sessionDir := filepath.Join(root, "chat", "sess-plain-meta")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	plainMeta, _ := json.Marshal(storeMeta{AgentID: "plain-id", CreatedAt: 99})
	userBlob := []byte(`{"role":"user","content":"<user_query>q</user_query>"}`)
	createTestStoreDBWithMetaValue(t, sessionDir, plainMeta, []struct {
		id   string
		data []byte
	}{{"u", userBlob}})

	got, err := Scan(root, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got) != 1 || got[0].SessionID != "plain-id" {
		t.Fatalf("sessionId=%q", got[0].SessionID)
	}
}

func TestScanDirEmptyWhenWorkspaceUnresolved(t *testing.T) {
	root := t.TempDir()
	sessionDir := filepath.Join(root, "c", "s-no-ws")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	userBlob := []byte(`{"role":"user","content":"<user_query>question</user_query>"}`)
	createTestStoreDB(t, sessionDir, storeMeta{AgentID: "s-no-ws", CreatedAt: 1}, []struct {
		id   string
		data []byte
	}{{"u", userBlob}})

	got, err := Scan(root, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d sessions", len(got))
	}
	if got[0].Dir != "" {
		t.Fatalf("dir=%q want empty", got[0].Dir)
	}
	if got[0].FirstMsg != "question" {
		t.Fatalf("firstMsg=%q", got[0].FirstMsg)
	}
}

func TestScanMultipleStores(t *testing.T) {
	root := t.TempDir()
	userBlob := []byte(`{"role":"user","content":"<user_query>x</user_query>"}`)

	for _, spec := range []struct {
		chat, session, wantID string
	}{
		{"chat-a", "session-a", "session-a"},
		{"chat-b", "session-b", "session-b"},
	} {
		sessionDir := filepath.Join(root, spec.chat, spec.session)
		if err := os.MkdirAll(sessionDir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		createTestStoreDB(t, sessionDir, storeMeta{AgentID: spec.wantID, CreatedAt: 1}, []struct {
			id   string
			data []byte
		}{{"u", userBlob}})
	}

	got, err := Scan(root, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d sessions want 2", len(got))
	}
	ids := map[string]bool{got[0].SessionID: true, got[1].SessionID: true}
	if !ids["session-a"] || !ids["session-b"] {
		t.Fatalf("session ids=%q %q", got[0].SessionID, got[1].SessionID)
	}
}

func TestCollectUserQueriesSkipsUnextractableTag(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "store.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE blobs (id TEXT, data BLOB);
INSERT INTO blobs(id,data) VALUES('1', ?)`,
		[]byte(`{"role":"user","content":"<user_query>unclosed`))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	first, last, count, err := collectUserQueries(db)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if count != 0 || first != "" || last != "" {
		t.Fatalf("count=%d first=%q last=%q want empty", count, first, last)
	}
}

func TestScanIgnoresChatRootStoreDB(t *testing.T) {
	root := t.TempDir()
	chatDir := filepath.Join(root, "only-chat")
	if err := os.MkdirAll(chatDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	userBlob := []byte(`{"role":"user","content":"<user_query>hi</user_query>"}`)
	createTestStoreDB(t, chatDir, storeMeta{AgentID: "x", CreatedAt: 1}, []struct {
		id   string
		data []byte
	}{{"b", userBlob}})

	got, err := Scan(root, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("chat-root store.db should not match, got %d", len(got))
	}
}
