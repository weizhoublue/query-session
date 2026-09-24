package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDecodeProjectDirUsesLongestExistingPrefix(t *testing.T) {
	fsRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(fsRoot, "Users", "me", "project"))

	got := DecodeProjectDir("Users-me-project-new-dir", fsRoot)
	want := filepath.Join(fsRoot, "Users", "me", "project", "new-dir")
	if got != want {
		t.Fatalf("DecodeProjectDir() = %q, want %q", got, want)
	}
}

func TestDecodeProjectDirHandlesHiddenDirectorySegment(t *testing.T) {
	fsRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(fsRoot, "Users", "me", ".hermes", "project"))

	got := DecodeProjectDir("Users-me--hermes-project", fsRoot)
	want := filepath.Join(fsRoot, "Users", "me", ".hermes", "project")
	if got != want {
		t.Fatalf("DecodeProjectDir() = %q, want %q", got, want)
	}
}

func TestDecodeProjectDirPreservesRawSuffixWhenHiddenSegmentMissing(t *testing.T) {
	fsRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(fsRoot, "Users", "me"))

	got := DecodeProjectDir("Users-me--missing-project", fsRoot)
	want := filepath.Join(fsRoot, "Users", "me", "--missing-project")
	if got != want {
		t.Fatalf("DecodeProjectDir() = %q, want %q", got, want)
	}
}

func TestDecodeProjectDirStopsWhenCandidatePrefixIsFile(t *testing.T) {
	fsRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(fsRoot, "Users"))
	mustWriteFile(t, filepath.Join(fsRoot, "Users", "me"), "")

	got := DecodeProjectDir("Users-me-project", fsRoot)
	want := filepath.Join(fsRoot, "Users", "me-project")
	if got != want {
		t.Fatalf("DecodeProjectDir() = %q, want %q", got, want)
	}
}

func TestScanIgnoresSessionSubdirectories(t *testing.T) {
	projectsRoot := t.TempDir()
	fsRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(fsRoot, "repo"))

	projectDir := filepath.Join(projectsRoot, "repo")
	mustMkdirAll(t, filepath.Join(projectDir, "ignored-session"))
	mustWriteFile(t, filepath.Join(projectDir, "kept.jsonl"), userLine("2026-05-18T10:00:00Z", "kept"))
	mustWriteFile(t, filepath.Join(projectDir, "ignored-session", "ignored.jsonl"), userLine("2026-05-18T11:00:00Z", "ignored"))

	sessions, err := Scan(projectsRoot, fsRoot, nil)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("Scan() returned %d sessions, want 1", len(sessions))
	}
	if sessions[0].SessionID != "kept" {
		t.Fatalf("SessionID = %q, want kept", sessions[0].SessionID)
	}
	if sessions[0].File != filepath.Join(projectDir, "kept.jsonl") {
		t.Fatalf("File = %q, want kept jsonl path", sessions[0].File)
	}
}

func TestScanUsesFirstAndLastUserMessages(t *testing.T) {
	projectsRoot := t.TempDir()
	fsRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(fsRoot, "repo"))

	projectDir := filepath.Join(projectsRoot, "repo")
	mustMkdirAll(t, projectDir)
	mustWriteFile(t, filepath.Join(projectDir, "abc.jsonl"),
		assistantLine("2026-05-18T09:00:00Z", "ignore")+
			userLine("2026-05-18T10:00:00Z", "first")+
			userLine("2026-05-18T12:30:00Z", "last"),
	)

	sessions, err := Scan(projectsRoot, fsRoot, nil)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("Scan() returned %d sessions, want 1", len(sessions))
	}

	wantCreate := time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)
	wantLast := time.Date(2026, 5, 18, 12, 30, 0, 0, time.UTC)
	got := sessions[0]
	wantFile := filepath.Join(projectDir, "abc.jsonl")
	if got.SessionID != "abc" || got.Dir != filepath.Join(fsRoot, "repo") || got.File != wantFile {
		t.Fatalf("session identity = (%q, %q, %q), want (%q, %q, %q)", got.SessionID, got.Dir, got.File, "abc", filepath.Join(fsRoot, "repo"), wantFile)
	}
	if !got.CreateTime.Equal(wantCreate) || got.FirstMsg != "first" {
		t.Fatalf("first user = (%s, %q), want (%s, %q)", got.CreateTime, got.FirstMsg, wantCreate, "first")
	}
	if !got.LastTime.Equal(wantLast) {
		t.Fatalf("last user time = %s, want %s", got.LastTime, wantLast)
	}
	if got.UserMsgAmount != 2 {
		t.Fatalf("UserMsgAmount = %d, want 2", got.UserMsgAmount)
	}
}

func TestScanSkipsNonStringUserContentWhenChoosingLastMessage(t *testing.T) {
	projectsRoot := t.TempDir()
	fsRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(fsRoot, "repo"))

	projectDir := filepath.Join(projectsRoot, "repo")
	mustMkdirAll(t, projectDir)
	mustWriteFile(t, filepath.Join(projectDir, "tool-result.jsonl"),
		userLine("2026-05-18T10:00:00Z", "first")+
			userLine("2026-05-18T10:10:00Z", "last human question")+
			`{"timestamp":"2026-05-18T10:10:30Z","message":{"role":"user","content":[{"type":"text","text":"array user text"}]}}`+"\n"+
			`{"timestamp":"2026-05-18T10:11:00Z","message":{"role":"user","content":[{"tool_use_id":"call_1","type":"tool_result","content":[{"type":"text","text":"tool output"}]}]}}`+"\n",
	)

	sessions, err := Scan(projectsRoot, fsRoot, nil)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("Scan() returned %d sessions, want 1", len(sessions))
	}
	wantLast := time.Date(2026, 5, 18, 10, 10, 0, 0, time.UTC)
	if !sessions[0].LastTime.Equal(wantLast) {
		t.Fatalf("last user time = %s, want %s", sessions[0].LastTime, wantLast)
	}
}

func TestScanIncludesEmptyStringSessionWithReliableTime(t *testing.T) {
	projectsRoot := t.TempDir()
	fsRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(fsRoot, "repo"))

	projectDir := filepath.Join(projectsRoot, "repo")
	mustMkdirAll(t, projectDir)
	mustWriteFile(t, filepath.Join(projectDir, "empty-string.jsonl"),
		`{"timestamp":"2026-05-18T10:00:00Z","message":{"role":"user","content":"  "}}`+"\n",
	)

	sessions, err := Scan(projectsRoot, fsRoot, nil)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(sessions) != 1 || sessions[0].UserMsgAmount != 0 ||
		!sessions[0].CreateTime.Equal(sessions[0].LastTime) {
		t.Fatalf("Scan() = %+v, want unnamed session with matching timestamps", sessions)
	}
}

func TestScanIncludesFilesWithoutUserMessagesWhenDirectoryExists(t *testing.T) {
	projectsRoot := t.TempDir()
	fsRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(fsRoot, "repo"))

	projectDir := filepath.Join(projectsRoot, "repo")
	mustMkdirAll(t, projectDir)
	mustWriteFile(t, filepath.Join(projectDir, "empty.jsonl"), assistantLine("2026-05-18T09:00:00Z", "ignore"))

	sessions, err := Scan(projectsRoot, fsRoot, nil)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(sessions) != 1 || sessions[0].UserMsgAmount != 0 ||
		sessions[0].CreateTime.IsZero() || !sessions[0].LastTime.Equal(sessions[0].CreateTime) {
		t.Fatalf("Scan() = %+v, want zero-message session with valid time", sessions)
	}
}

func TestScanUsesLatestNativeAITitle(t *testing.T) {
	projectsRoot := t.TempDir()
	fsRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(fsRoot, "repo"))
	projectDir := filepath.Join(projectsRoot, "repo")
	mustMkdirAll(t, projectDir)
	mustWriteFile(t, filepath.Join(projectDir, "titled.jsonl"),
		`{"type":"ai-title","aiTitle":"Early title"}`+"\n"+
			userLine("2026-05-18T10:00:00Z", "actual prompt")+
			`{"type":"ai-title","aiTitle":"Final title"}`+"\n")
	got, err := Scan(projectsRoot, fsRoot, nil)
	if err != nil || len(got) != 1 || got[0].Title != "Final title" {
		t.Fatalf("Scan = (%+v, %v), want latest AI title", got, err)
	}
}

func TestScanSkipsZeroMessageSessionWithoutVerifiedDirectory(t *testing.T) {
	projectsRoot := t.TempDir()
	fsRoot := t.TempDir()
	projectDir := filepath.Join(projectsRoot, "missing")
	mustMkdirAll(t, projectDir)
	mustWriteFile(t, filepath.Join(projectDir, "empty.jsonl"), assistantLine("2026-05-18T09:00:00Z", "ignored"))
	got, err := Scan(projectsRoot, fsRoot, nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("Scan = (%+v, %v), want skipped missing directory", got, err)
	}
}

func TestScanParsesUserMessageLongerThanDefaultScannerBuffer(t *testing.T) {
	projectsRoot := t.TempDir()
	fsRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(fsRoot, "repo"))

	longMessage := strings.Repeat("x", 70*1024)
	projectDir := filepath.Join(projectsRoot, "repo")
	mustMkdirAll(t, projectDir)
	mustWriteFile(t, filepath.Join(projectDir, "long.jsonl"), userLine("2026-05-18T10:00:00Z", longMessage))

	sessions, err := Scan(projectsRoot, fsRoot, nil)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("Scan() returned %d sessions, want 1", len(sessions))
	}
	if sessions[0].FirstMsg != longMessage {
		t.Fatalf("message length = %d, want %d", len(sessions[0].FirstMsg), len(longMessage))
	}
}

func TestScanSkipsInvalidJSONLinesWithoutFailing(t *testing.T) {
	projectsRoot := t.TempDir()
	fsRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(fsRoot, "repo"))

	var logs []string
	projectDir := filepath.Join(projectsRoot, "repo")
	mustMkdirAll(t, projectDir)
	mustWriteFile(t, filepath.Join(projectDir, "mixed.jsonl"),
		"{invalid json}\n"+
			`{"timestamp":"bad","message":{"role":"user","content":"bad time"}}`+"\n"+
			userLine("2026-05-18T10:00:00Z", "valid"),
	)

	sessions, err := Scan(projectsRoot, fsRoot, func(level, message string) {
		logs = append(logs, level+":"+message)
	})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("Scan() returned %d sessions, want 1", len(sessions))
	}
	if sessions[0].FirstMsg != "valid" {
		t.Fatalf("first message = %q, want valid", sessions[0].FirstMsg)
	}
	errorLogs := 0
	for _, log := range logs {
		if strings.HasPrefix(log, "error:") {
			errorLogs++
		}
	}
	if errorLogs != 2 {
		t.Fatalf("logged %d error messages, want 2: %#v", errorLogs, logs)
	}
}

func TestScanLogsProjectsFilesAndSessionOutcomes(t *testing.T) {
	projectsRoot := t.TempDir()
	fsRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(fsRoot, "repo"))

	var logs []string
	projectDir := filepath.Join(projectsRoot, "repo")
	mustMkdirAll(t, projectDir)
	mustWriteFile(t, filepath.Join(projectDir, "matched.jsonl"), userLine("2026-05-18T10:00:00Z", "valid"))
	mustWriteFile(t, filepath.Join(projectDir, "skipped.jsonl"), assistantLine("2026-05-18T11:00:00Z", "ignored"))

	_, err := Scan(projectsRoot, fsRoot, func(level, message string) {
		logs = append(logs, level+":"+message)
	})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	joined := strings.Join(logs, "\n")
	for _, want := range []string{
		"info:scan project encoded=repo",
		"info:scan file sessionId=matched",
		"info:parsed sessionId=matched",
		"info:scan file sessionId=skipped",
		"info:parsed sessionId=skipped",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("logs missing %q in:\n%s", want, joined)
		}
	}
}

func TestMessageStringPlainString(t *testing.T) {
	got := messageString([]byte(`"hello world"`))
	if got != "hello world" {
		t.Fatalf("got %q, want %q", got, "hello world")
	}
}

func TestMessageStringListContentReturnsEmpty(t *testing.T) {
	got := messageString([]byte(`[{"type":"text","text":"hello"}]`))
	if got != "" {
		t.Fatalf("got %q, want empty (list content not supported)", got)
	}
}

func TestMessageStringNullReturnsEmpty(t *testing.T) {
	got := messageString([]byte(`null`))
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestMessageStringEmptyArrayReturnsEmpty(t *testing.T) {
	got := messageString([]byte(`[]`))
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func userLine(timestamp, content string) string {
	return `{"timestamp":"` + timestamp + `","message":{"role":"user","content":"` + content + `"}}` + "\n"
}

func assistantLine(timestamp, content string) string {
	return `{"timestamp":"` + timestamp + `","message":{"role":"assistant","content":"` + content + `"}}` + "\n"
}
