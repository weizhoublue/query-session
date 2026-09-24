package copilot

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"query-session/internal/session"
)

const startTime = "2026-05-18T09:00:00Z"

func makeJournal(t *testing.T, root, id, cwd, body, metadata string) string {
	t.Helper()
	dir := filepath.Join(root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	head := fmt.Sprintf(`{"type":"session.start","timestamp":%q,"data":{"sessionId":%q,"context":{"cwd":%q}}}`+"\n", startTime, id, cwd)
	path := filepath.Join(dir, "events.jsonl")
	if err := os.WriteFile(path, []byte(head+body), 0o600); err != nil {
		t.Fatal(err)
	}
	if metadata != "" {
		if err := os.WriteFile(filepath.Join(dir, "workspace.yaml"), []byte(metadata), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func matcherFor(t *testing.T, cwd string) *session.DirMatcher {
	t.Helper()
	m, err := session.NewDirMatcher(session.FilterOptions{CurrentDir: cwd})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestScanExtractsNativeTitleAndValidUserMessages(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "repo")
	body := `{"type":"assistant.message","timestamp":"2026-05-18T10:00:00Z","data":{"content":"ignored"}}` + "\n" +
		`{"type":"user.message","timestamp":"2026-05-18T10:30:00Z","data":{"content":"first request"}}` + "\n" +
		`{"type":"user.message","timestamp":"2026-05-18T10:31:00Z","data":{"content":"  "}}` + "\n" +
		`{"type": "user.message", "timestamp":"2026-05-18T11:00:00Z","data":{"content":"last request"}}` + "\n"
	path := makeJournal(t, root, "sid", cwd, body, "name: 'Review: #42'\nuser_named: false\n")
	got, err := Scan(root, matcherFor(t, cwd), nil)
	if err != nil || len(got) != 1 {
		t.Fatalf("Scan = (%v, %v), want one session", got, err)
	}
	s := got[0]
	if s.SessionID != "sid" || s.Dir != cwd || s.File != path || s.UserMsgAmount != 2 ||
		s.FirstMsg != "first request" || s.Title != "Review: #42" {
		t.Fatalf("session = %+v", s)
	}
	if s.CreateTime.UTC().Format(time.RFC3339) != "2026-05-18T10:30:00Z" ||
		s.LastTime.UTC().Format(time.RFC3339) != "2026-05-18T11:00:00Z" {
		t.Fatalf("times = %s, %s", s.CreateTime, s.LastTime)
	}
	var out bytes.Buffer
	if err := session.FormatTable(&out, got); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "  Review: #42  ") {
		t.Fatalf("output = %s", out.String())
	}
}

func TestScanIncludesUnnamedSessionWithoutMessages(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "repo")
	makeJournal(t, root, "unnamed", cwd, "", "id: unnamed\n")
	got, err := Scan(root, matcherFor(t, cwd), nil)
	if err != nil || len(got) != 1 {
		t.Fatalf("Scan = (%v, %v), want one session", got, err)
	}
	var out bytes.Buffer
	if err := session.FormatTable(&out, got); err != nil {
		t.Fatal(err)
	}
	if got[0].UserMsgAmount != 0 || !got[0].CreateTime.Equal(got[0].LastTime) ||
		!strings.Contains(out.String(), "未命名") {
		t.Fatalf("unnamed session = %+v", got[0])
	}
}

func TestScanReadsQuotedAndUnquotedWorkspaceTitles(t *testing.T) {
	for _, tc := range []struct {
		name, yaml, want string
	}{
		{"double quoted", "name: \"Review: #42\"\n", "Review: #42"},
		{"single quoted", "name: 'Review: #42'\n", "Review: #42"},
		{"plain", "name: Normal title\n", "Normal title"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			cwd := filepath.Join(root, "repo")
			makeJournal(t, root, "sid", cwd, "", tc.yaml)
			got, err := Scan(root, matcherFor(t, cwd), nil)
			if err != nil || len(got) != 1 || got[0].Title != tc.want {
				t.Fatalf("Scan = (%+v, %v), want %q", got, err, tc.want)
			}
		})
	}
}

func TestScanPrefiltersByInitialWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "repo")
	makeJournal(t, root, "other", filepath.Join(root, "elsewhere"), `{"type":"user.message",BAD}`+"\n", "name: [bad yaml\n")
	if got, err := Scan(root, matcherFor(t, cwd), nil); err != nil || len(got) != 0 {
		t.Fatalf("excluded journal should not read body or metadata: (%v, %v)", got, err)
	}
}

func TestScanSkipsInvalidUserTimestampWithDiagnostic(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "repo")
	makeJournal(t, root, "sid", cwd,
		`{"type":"user.message","timestamp":"invalid","data":{"content":"not counted"}}`+"\n", "")
	var logs []string
	got, err := Scan(root, matcherFor(t, cwd), func(level, message string) {
		logs = append(logs, level+":"+message)
	})
	if err != nil || len(got) != 1 || got[0].UserMsgAmount != 0 ||
		!got[0].CreateTime.Equal(got[0].LastTime) ||
		len(logs) != 1 || !strings.Contains(logs[0], "invalid user timestamp") {
		t.Fatalf("Scan = (%+v, %v), diagnostics = %v", got, err, logs)
	}
}

func TestScanRejectsInvalidHeader(t *testing.T) {
	for _, tc := range []struct {
		name   string
		header string
	}{
		{"malformed", `{invalid`},
		{"wrong id", `{"type":"session.start","timestamp":"2026-05-18T09:00:00Z","data":{"sessionId":"other","context":{"cwd":"/repo"}}}`},
		{"relative cwd", `{"type":"session.start","timestamp":"2026-05-18T09:00:00Z","data":{"sessionId":"sid","context":{"cwd":"repo"}}}`},
		{"bad time", `{"type":"session.start","timestamp":"invalid","data":{"sessionId":"sid","context":{"cwd":"/repo"}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "sid")
			if err := os.Mkdir(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte(tc.header+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := Scan(root, matcherFor(t, "/not-repo"), nil)
			if err == nil || !strings.Contains(err.Error(), "events.jsonl") {
				t.Fatalf("Scan error = %v, want journal path", err)
			}
		})
	}
}

func TestScanRejectsMalformedMetadata(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "repo")
	makeJournal(t, root, "sid", cwd, "", "name: [bad yaml\n")
	_, err := Scan(root, matcherFor(t, cwd), nil)
	if err == nil || !strings.Contains(err.Error(), "workspace.yaml") {
		t.Fatalf("error = %v, want metadata path", err)
	}
}

func TestScanReadsLineLargerThanDefaultScannerLimit(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "repo")
	text := strings.Repeat("x", 10<<20)
	makeJournal(t, root, "large", cwd,
		fmt.Sprintf(`{"type":"user.message","timestamp":"2026-05-18T10:00:00Z","data":{"content":%q}}`+"\n", text), "")
	got, err := Scan(root, matcherFor(t, cwd), nil)
	if err != nil || len(got) != 1 || len(got[0].FirstMsg) != len(text) {
		t.Fatalf("large journal = (%d sessions, %v)", len(got), err)
	}
}

func TestReadLineRejectsOversizeRecord(t *testing.T) {
	_, err := readLine(bufio.NewReader(strings.NewReader("123456\n")), 5)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("readLine error = %v, want size limit", err)
	}
}

func TestScanMissingRootReturnsNoSessions(t *testing.T) {
	got, err := Scan(filepath.Join(t.TempDir(), "missing"), nil, nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("Scan = (%v, %v), want empty", got, err)
	}
}
