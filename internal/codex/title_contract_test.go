package codex

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScanSkipsZeroMessageSubagentAndUntrustedMetadata(t *testing.T) {
	root := t.TempDir()
	day := filepath.Join(root, "2026", "05", "18")
	if err := os.MkdirAll(day, 0o755); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"subagent": `{"type":"session_meta","timestamp":"2026-05-18T09:00:00Z","payload":{"id":"child","cwd":"/repo","source":{"subagent":{"thread_spawn":{"parent_thread_id":"parent"}}}}}`,
		"no-time":  `{"type":"session_meta","timestamp":"invalid","payload":{"id":"no-time","cwd":"/repo"}}`,
		"no-dir":   `{"type":"session_meta","timestamp":"2026-05-18T09:00:00Z","payload":{"id":"no-dir"}}`,
	}
	for name, line := range cases {
		if err := os.WriteFile(filepath.Join(day, name+".jsonl"), []byte(line+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	start := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	got, err := Scan(root, start, start.Add(24*time.Hour-time.Nanosecond), nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("Scan = (%+v, %v), want all untrusted sessions excluded", got, err)
	}
}
