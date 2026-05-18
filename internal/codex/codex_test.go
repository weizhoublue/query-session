package codex

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
