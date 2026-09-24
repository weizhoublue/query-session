package copilot

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScanKeepsInitialWorkspaceAcrossResume(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "initial")
	body := `{"type":"user.message","timestamp":"2026-05-18T10:00:00Z","data":{"content":"Original question"}}` + "\n" +
		`{"type":"session.resume","timestamp":"2026-05-19T10:00:00Z","data":{"context":{"cwd":"/different-directory"}}}` + "\n" +
		`{"type":"user.message","timestamp":"2026-05-19T11:00:00Z","data":{"content":"Follow-up question"}}` + "\n"
	makeJournal(t, root, "sid", cwd, body, "")

	got, err := Scan(root, matcherFor(t, cwd), nil)
	if err != nil || len(got) != 1 {
		t.Fatalf("Scan = (%+v, %v), want one session", got, err)
	}
	s := got[0]
	if s.Dir != cwd || s.FirstMsg != "Original question" || s.Title != "" || s.UserMsgAmount != 2 ||
		s.CreateTime.Format(time.RFC3339) != "2026-05-18T10:00:00Z" ||
		s.LastTime.Format(time.RFC3339) != "2026-05-19T11:00:00Z" {
		t.Fatalf("resumed session = %+v", s)
	}
}

func TestScanLogsMalformedUserEventWithoutDroppingLaterMessage(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "repo")
	makeJournal(t, root, "sid", cwd,
		`{"type":"user.message","timestamp":"bad","data":`+"\n"+
			`{"type":"user.message","timestamp":"2026-05-18T11:00:00Z","data":{"content":"kept"}}`+"\n", "")
	var logs []string
	got, err := Scan(root, matcherFor(t, cwd), func(level, message string) {
		logs = append(logs, level+":"+message)
	})
	if err != nil || len(got) != 1 || got[0].FirstMsg != "kept" || got[0].UserMsgAmount != 1 ||
		len(logs) != 1 || !strings.Contains(logs[0], "invalid event") {
		t.Fatalf("Scan = (%+v, %v), logs = %v", got, err, logs)
	}
}

func TestScanIgnoresNestedJournals(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "repo")
	parent := makeJournal(t, root, "parent", cwd, "", "")
	nested := filepath.Join(filepath.Dir(parent), "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "events.jsonl"), []byte("invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Scan(root, matcherFor(t, cwd), nil)
	if err != nil || len(got) != 1 || got[0].SessionID != "parent" {
		t.Fatalf("Scan = (%+v, %v), want parent only", got, err)
	}
}

func TestReadLineRejectsUnterminatedOversizeRecord(t *testing.T) {
	_, err := readLine(bufio.NewReader(strings.NewReader("123456")), 5)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("readLine error = %v, want limit error", err)
	}
}
