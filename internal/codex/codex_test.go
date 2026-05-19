package codex

import (
	"os"
	"path/filepath"
	"testing"
	"encoding/json"
	"time"
)

func TestExtractUserContentSingleInputText(t *testing.T) {
	raw, _ := json.Marshal([]map[string]string{{"type": "input_text", "text": " hello world "}})
	got := extractUserContent(raw)
	if got != "hello world" {
		t.Fatalf("got %q, want %q", got, "hello world")
	}
}

func TestExtractUserContentMultiplePartsReturnsEmpty(t *testing.T) {
	raw, _ := json.Marshal([]map[string]string{
		{"type": "input_text", "text": "first"},
		{"type": "input_text", "text": "second"},
	})
	got := extractUserContent(raw)
	if got != "" {
		t.Fatalf("got %q, want empty (multi-part not supported)", got)
	}
}

func TestExtractUserContentWrongTypeReturnsEmpty(t *testing.T) {
	raw, _ := json.Marshal([]map[string]string{{"type": "text", "text": "hello"}})
	got := extractUserContent(raw)
	if got != "" {
		t.Fatalf("got %q, want empty (type not input_text)", got)
	}
}

func TestExtractUserContentEmptyTextReturnsEmpty(t *testing.T) {
	raw, _ := json.Marshal([]map[string]string{{"type": "input_text", "text": "  "}})
	got := extractUserContent(raw)
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestExtractUserContentPlainStringReturnsEmpty(t *testing.T) {
	raw := json.RawMessage(`"plain string"`)
	got := extractUserContent(raw)
	if got != "" {
		t.Fatalf("got %q, want empty (string not supported)", got)
	}
}

func TestExtractUserContentEmptyArrayReturnsEmpty(t *testing.T) {
	raw := json.RawMessage(`[]`)
	got := extractUserContent(raw)
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestScanExtractsCodexSessionFromPayloadID(t *testing.T) {
	root := t.TempDir()
	dayDir := filepath.Join(root, "2026", "05", "18")
	if err := os.MkdirAll(dayDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	jsonl := `{"timestamp":"2026-05-18T00:00:00Z","payload":{"id":"sid-from-payload","cwd":"/repo/a"}}
{"timestamp":"2026-05-18T01:00:00Z","payload":{"role":"user","content":[{"type":"input_text","text":"first question"}]}}
{"timestamp":"2026-05-18T02:00:00Z","payload":{"role":"assistant","content":"ignored"}}
{"timestamp":"2026-05-18T03:00:00Z","payload":{"role":"user","content":[{"type":"input_text","text":"last question"}]}}
`
	filePath := filepath.Join(dayDir, "x.jsonl")
	if err := os.WriteFile(filePath, []byte(jsonl), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	start := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 18, 23, 59, 59, 0, time.UTC)
	got, err := Scan(root, start, end, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d sessions", len(got))
	}
	if got[0].SessionID != "sid-from-payload" {
		t.Fatalf("session id = %q", got[0].SessionID)
	}
	if got[0].Dir != "/repo/a" {
		t.Fatalf("dir = %q", got[0].Dir)
	}
	if got[0].FirstMsg != "first question" || got[0].LastMsg != "last question" {
		t.Fatalf("messages = %q / %q", got[0].FirstMsg, got[0].LastMsg)
	}
	if got[0].UserMsgAmount != 2 {
		t.Fatalf("UserMsgAmount = %d, want 2", got[0].UserMsgAmount)
	}
	if got[0].File != filePath {
		t.Fatalf("file = %q, want %q", got[0].File, filePath)
	}
}

func TestScanFallbackToFilenameForSessionID(t *testing.T) {
	root := t.TempDir()
	dayDir := filepath.Join(root, "2026", "05", "18")
	if err := os.MkdirAll(dayDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	jsonl := `{"timestamp":"2026-05-18T01:00:00Z","payload":{"role":"user","content":[{"type":"input_text","text":"hello"}]}}
`
	if err := os.WriteFile(filepath.Join(dayDir, "my-session-id.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	start := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 18, 23, 59, 59, 0, time.UTC)
	got, err := Scan(root, start, end, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d sessions", len(got))
	}
	if got[0].SessionID != "my-session-id" {
		t.Fatalf("session id = %q, want my-session-id", got[0].SessionID)
	}
}

func TestScanSkipsCodexFileWithoutUserMessages(t *testing.T) {
	root := t.TempDir()
	dayDir := filepath.Join(root, "2026", "05", "18")
	if err := os.MkdirAll(dayDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	jsonl := `{"timestamp":"2026-05-18T01:00:00Z","payload":{"role":"assistant","content":"only assistant"}}
`
	if err := os.WriteFile(filepath.Join(dayDir, "x.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	start := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 18, 23, 59, 59, 0, time.UTC)
	got, err := Scan(root, start, end, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no sessions, got %d", len(got))
	}
}

func TestScanSkipsMultiMemberContentArray(t *testing.T) {
	root := t.TempDir()
	dayDir := filepath.Join(root, "2026", "05", "18")
	if err := os.MkdirAll(dayDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	jsonl := `{"timestamp":"2026-05-18T01:00:00Z","payload":{"id":"sid","role":"user","content":[{"type":"input_text","text":"AGENTS.md instructions"},{"type":"input_text","text":"environment context"}]}}
`
	if err := os.WriteFile(filepath.Join(dayDir, "x.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	start := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 18, 23, 59, 59, 0, time.UTC)
	got, err := Scan(root, start, end, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no sessions (multi-member content skipped), got %d", len(got))
	}
}

func TestScanSkipsUserMessageWithEmptyInputText(t *testing.T) {
	root := t.TempDir()
	dayDir := filepath.Join(root, "2026", "05", "18")
	if err := os.MkdirAll(dayDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	jsonl := `{"timestamp":"2026-05-18T01:00:00Z","payload":{"id":"sid","role":"user","content":[{"type":"input_text","text":"  "}]}}
`
	if err := os.WriteFile(filepath.Join(dayDir, "x.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	start := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 18, 23, 59, 59, 0, time.UTC)
	got, err := Scan(root, start, end, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no sessions, got %d", len(got))
	}
}

func TestScanSkipsInvalidJSONLines(t *testing.T) {
	root := t.TempDir()
	dayDir := filepath.Join(root, "2026", "05", "18")
	if err := os.MkdirAll(dayDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	jsonl := `not valid json
{"timestamp":"2026-05-18T01:00:00Z","payload":{"id":"sid","role":"user","content":[{"type":"input_text","text":"hello"}]}}
`
	if err := os.WriteFile(filepath.Join(dayDir, "x.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	start := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 18, 23, 59, 59, 0, time.UTC)
	got, err := Scan(root, start, end, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1 (invalid JSON line skipped)", len(got))
	}
}

func TestScanOnlyScansMatchingDateDirs(t *testing.T) {
	root := t.TempDir()
	dayDir := filepath.Join(root, "2026", "05", "18")
	if err := os.MkdirAll(dayDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	otherDir := filepath.Join(root, "2026", "05", "19")
	if err := os.MkdirAll(otherDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	jsonl := `{"timestamp":"2026-05-18T01:00:00Z","payload":{"id":"sid","role":"user","content":[{"type":"input_text","text":"hello"}]}}
`
	if err := os.WriteFile(filepath.Join(dayDir, "a.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(otherDir, "b.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	start := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 18, 23, 59, 59, 0, time.UTC)
	got, err := Scan(root, start, end, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1 (only 5/18)", len(got))
	}
}

func TestDayStartPreservesTimezone(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	got := dayStart(time.Date(2026, 5, 18, 14, 30, 0, 0, loc))
	want := time.Date(2026, 5, 18, 0, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Fatalf("dayStart = %s, want %s", got, want)
	}
}

func TestScanSkipsSubAgentSession(t *testing.T) {
	root := t.TempDir()
	dayDir := filepath.Join(root, "2026", "05", "18")
	if err := os.MkdirAll(dayDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	jsonl := `{"timestamp":"2026-05-18T00:00:00Z","type":"session_meta","payload":{"id":"sub-id","cwd":"/repo/a","source":{"subagent":{"thread_spawn":{"parent_thread_id":"parent-id"}}}}}
{"timestamp":"2026-05-18T01:00:00Z","payload":{"role":"user","content":[{"type":"input_text","text":"hello"}]}}
`
	if err := os.WriteFile(filepath.Join(dayDir, "x.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	start := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 18, 23, 59, 59, 0, time.UTC)
	got, err := Scan(root, start, end, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no sessions (sub-agent skipped), got %d", len(got))
	}
}
