package cursor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanSkipsZeroMessageStoreWithoutCreationTime(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "chat", "session")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	injectedWorkspace := []byte(`{"role":"user","content":"<user_info>Workspace Path: /repo/a</user_info>"}`)
	createTestStoreDB(t, dir, storeMeta{Name: "Native title"}, []struct {
		id   string
		data []byte
	}{{"workspace", injectedWorkspace}})

	got, err := Scan(root, nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("Scan = (%+v, %v), want zero-message store without time skipped", got, err)
	}
}
