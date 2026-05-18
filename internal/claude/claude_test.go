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
	if !got.LastTime.Equal(wantLast) || got.LastMsg != "last" {
		t.Fatalf("last user = (%s, %q), want (%s, %q)", got.LastTime, got.LastMsg, wantLast, "last")
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
	if !sessions[0].LastTime.Equal(wantLast) || sessions[0].LastMsg != "last human question" {
		t.Fatalf("last user = (%s, %q), want (%s, %q)", sessions[0].LastTime, sessions[0].LastMsg, wantLast, "last human question")
	}
}

func TestScanSkipsEmptyStringUserContent(t *testing.T) {
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
	if len(sessions) != 0 {
		t.Fatalf("Scan() returned %d sessions, want 0", len(sessions))
	}
}

func TestScanSkipsFilesWithoutUserMessages(t *testing.T) {
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
	if len(sessions) != 0 {
		t.Fatalf("Scan() returned %d sessions, want 0", len(sessions))
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
	if sessions[0].FirstMsg != longMessage || sessions[0].LastMsg != longMessage {
		t.Fatalf("message lengths = (%d, %d), want %d", len(sessions[0].FirstMsg), len(sessions[0].LastMsg), len(longMessage))
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
	if sessions[0].FirstMsg != "valid" || sessions[0].LastMsg != "valid" {
		t.Fatalf("messages = (%q, %q), want valid", sessions[0].FirstMsg, sessions[0].LastMsg)
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
		"info:skip file sessionId=skipped reason=no-user-message",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("logs missing %q in:\n%s", want, joined)
		}
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
