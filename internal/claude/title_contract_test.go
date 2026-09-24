package claude

import (
	"path/filepath"
	"testing"
)

func TestScanSkipsAITitleOnlyWithoutTrustedTime(t *testing.T) {
	root := t.TempDir()
	fsRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(fsRoot, "repo"))
	project := filepath.Join(root, "repo")
	mustMkdirAll(t, project)
	mustWriteFile(t, filepath.Join(project, "title-only.jsonl"), `{"type":"ai-title","aiTitle":"Named but no time"}`+"\n")

	got, err := Scan(root, fsRoot, nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("Scan = (%+v, %v), want no timestampless session", got, err)
	}
}

func TestScanIncludesZeroMessageAITitleWithTrustedTime(t *testing.T) {
	root := t.TempDir()
	fsRoot := t.TempDir()
	mustMkdirAll(t, filepath.Join(fsRoot, "repo"))
	project := filepath.Join(root, "repo")
	mustMkdirAll(t, project)
	mustWriteFile(t, filepath.Join(project, "named.jsonl"),
		assistantLine("2026-05-18T09:00:00Z", "not a user input")+
			`{"type":"ai-title","aiTitle":"Native title"}`+"\n")

	got, err := Scan(root, fsRoot, nil)
	if err != nil || len(got) != 1 || got[0].Title != "Native title" ||
		got[0].UserMsgAmount != 0 || got[0].CreateTime.IsZero() ||
		!got[0].LastTime.Equal(got[0].CreateTime) {
		t.Fatalf("Scan = (%+v, %v), want named zero-message session", got, err)
	}
}
