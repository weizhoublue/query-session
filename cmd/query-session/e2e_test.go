package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

type cliE2EFixture struct {
	binary, home, copilotHome, workspace, other string
}

func newCLIE2EFixture(t *testing.T) cliE2EFixture {
	t.Helper()
	root := t.TempDir()
	fixture := cliE2EFixture{
		binary:      filepath.Join(root, "query-session"),
		home:        filepath.Join(root, "home"),
		copilotHome: filepath.Join(root, "copilot-config"),
		workspace:   filepath.Join(root, "repo"),
		other:       filepath.Join(root, "elsewhere"),
	}
	for _, dir := range []string{fixture.home, fixture.workspace, fixture.other} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	build := exec.Command("go", "build", "-o", fixture.binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}
	return fixture
}

func writeE2EFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (f cliE2EFixture) run(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(f.binary, args...)
	cmd.Dir = f.workspace
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "HOME=") && !strings.HasPrefix(value, "COPILOT_HOME=") {
			cmd.Env = append(cmd.Env, value)
		}
	}
	cmd.Env = append(cmd.Env, "HOME="+f.home, "COPILOT_HOME="+f.copilotHome)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if err == nil {
		return stdout.String(), stderr.String(), 0
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("run CLI: %v", err)
	}
	return stdout.String(), stderr.String(), exitErr.ExitCode()
}

func makeCopilotE2EJournal(t *testing.T, f cliE2EFixture, id, cwd string, started time.Time, events, metadata string) string {
	t.Helper()
	base := filepath.Join(f.copilotHome, "session-state", id)
	path := filepath.Join(base, "events.jsonl")
	header := fmt.Sprintf(`{"type":"session.start","timestamp":%q,"data":{"sessionId":%q,"context":{"cwd":%q}}}`+"\n",
		started.Format(time.RFC3339Nano), id, cwd)
	writeE2EFile(t, path, header+events)
	if metadata != "" {
		writeE2EFile(t, filepath.Join(base, "workspace.yaml"), metadata)
	}
	return path
}

func makeCursorE2EStore(t *testing.T, f cliE2EFixture, id, title string, created time.Time, query string) {
	t.Helper()
	path := filepath.Join(f.home, ".cursor", "chats", "chat", id, "store.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE meta (key TEXT, value BLOB); CREATE TABLE blobs (id TEXT, data BLOB);`); err != nil {
		t.Fatal(err)
	}
	meta, err := json.Marshal(struct {
		AgentID   string `json:"agentId"`
		CreatedAt int64  `json:"createdAt"`
		Name      string `json:"name"`
	}{id, created.UnixMilli(), title})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO meta(key,value) VALUES('0',?)`, meta); err != nil {
		t.Fatal(err)
	}
	injection := fmt.Sprintf(`{"role":"user","content":%q}`, "<user_info>\nWorkspace Path: "+f.workspace+"\n</user_info>")
	if _, err := db.Exec(`INSERT INTO blobs(id,data) VALUES('workspace',?)`, []byte(injection)); err != nil {
		t.Fatal(err)
	}
	if query != "" {
		content := fmt.Sprintf(`{"role":"user","content":%q}`, "<user_query>"+query+"</user_query>")
		if _, err := db.Exec(`INSERT INTO blobs(id,data) VALUES('query',?)`, []byte(content)); err != nil {
			t.Fatal(err)
		}
	}
}

func assertE2ELines(t *testing.T, stdout string, count int) []string {
	t.Helper()
	parts := strings.SplitN(stdout, "\n\n", 2)
	if len(parts) != 2 || !strings.HasPrefix(parts[0], "provider: ") ||
		!strings.Contains(parts[0], "\nsession limit/matched/output: ") {
		t.Fatalf("missing report summary: %q", stdout)
	}
	table := strings.Split(strings.TrimSuffix(parts[1], "\n"), "\n")
	if len(table) == 0 || !regexp.MustCompile(`^SessionId {2,}Title {2,}MsgAmount {2,}CreateTime {2,}LastTime$`).MatchString(table[0]) {
		t.Fatalf("missing table header: %q", stdout)
	}
	lines := table[1:]
	if len(lines) != count {
		t.Fatalf("got %d lines, want %d: %q", len(lines), count, stdout)
	}
	pattern := regexp.MustCompile(`^\S+ {2,}.+? {2,}\d+ {2,}\d{8}_\d\d:\d\d:\d\d {2,}\d{8}_\d\d:\d\d:\d\d$`)
	for _, line := range lines {
		if !pattern.MatchString(line) {
			t.Fatalf("invalid five-column output: %q", line)
		}
	}
	return lines
}

func TestCLIBinaryEndToEnd(t *testing.T) {
	f := newCLIE2EFixture(t)
	may18 := time.Date(2026, 5, 18, 12, 0, 0, 0, time.Local)
	may19 := may18.AddDate(0, 0, 1)
	today := time.Now().In(time.Local)
	recent := time.Date(today.Year(), today.Month(), today.Day(), 12, 0, 0, 0, time.Local)

	writeE2EFile(t, filepath.Join(f.home, ".codex", "sessions", "2026", "05", "18", "named.jsonl"),
		fmt.Sprintf(`{"type":"session_meta","timestamp":%q,"payload":{"id":"codex-named","cwd":%q}}`+"\n"+
			`{"timestamp":%q,"payload":{"role":"user","content":[{"type":"input_text","text":"Codex first prompt"}]}}`+"\n",
			may18.Format(time.RFC3339Nano), f.workspace, may18.Add(time.Minute).Format(time.RFC3339Nano)))
	writeE2EFile(t, filepath.Join(f.home, ".codex", "sessions", "2026", "05", "18", "unnamed.jsonl"),
		fmt.Sprintf(`{"type":"session_meta","timestamp":%q,"payload":{"id":"codex-unnamed","cwd":%q}}`+"\n",
			may18.Add(time.Hour).Format(time.RFC3339Nano), f.workspace))

	encoded := strings.ReplaceAll(f.workspace, string(filepath.Separator), "-")
	writeE2EFile(t, filepath.Join(f.home, ".claude", "projects", encoded, "claude-named.jsonl"),
		fmt.Sprintf(`{"timestamp":%q,"message":{"role":"user","content":"Claude first prompt"}}`+"\n"+
			`{"type":"ai-title","aiTitle":"Claude native title"}`+"\n", may18.Format(time.RFC3339Nano)))
	writeE2EFile(t, filepath.Join(f.home, ".claude", "projects", encoded, "claude-unnamed.jsonl"),
		fmt.Sprintf(`{"timestamp":%q,"message":{"role":"assistant","content":"reply"}}`+"\n",
			may18.Add(time.Hour).Format(time.RFC3339Nano)))

	makeCursorE2EStore(t, f, "cursor-named", "Cursor native title", may18, "Cursor first prompt")
	makeCursorE2EStore(t, f, "cursor-unnamed", "", may19, "")

	old := makeCopilotE2EJournal(t, f, "copilot-old", f.workspace, may18,
		fmt.Sprintf(`{"type":"user.message","timestamp":%q,"data":{"content":"first Copilot prompt"}}`+"\n"+
			`{"type":"user.message","timestamp":%q,"data":{"content":"follow-up"}}`+"\n",
			may18.Add(5*time.Minute).Format(time.RFC3339Nano), may18.Add(10*time.Minute).Format(time.RFC3339Nano)),
		"name: 'Fix: #1'\n")
	makeCopilotE2EJournal(t, f, "copilot-fallback", f.workspace, may18.Add(time.Hour),
		fmt.Sprintf(`{"type":"user.message","timestamp":%q,"data":{"content":"Fallback question"}}`+"\n",
			may18.Add(time.Hour+time.Minute).Format(time.RFC3339Nano)), "")
	makeCopilotE2EJournal(t, f, "copilot-unnamed", f.workspace, recent, "", "id: copilot-unnamed\n")
	makeCopilotE2EJournal(t, f, "copilot-other", f.other, may18,
		`{"type":"user.message",broken}`+"\n", "name: [invalid yaml\n")

	t.Run("all providers and native title precedence", func(t *testing.T) {
		for _, tc := range []struct {
			provider string
			count    int
			titles   []string
		}{
			{"claude", 2, []string{"Claude native title", "未命名"}},
			{"codex", 2, []string{"Codex first prompt", "未命名"}},
			{"cursor", 2, []string{"Cursor native title", "未命名"}},
			{"copilot", 3, []string{"Fix: #1", "Fallback question", "未命名"}},
		} {
			t.Run(tc.provider, func(t *testing.T) {
				args := []string{"-t", tc.provider, "-n", "0"}
				if tc.provider == "claude" {
					args = append(args, "-p", ".*")
				}
				stdout, stderr, code := f.run(t, args...)
				if code != 0 {
					t.Fatalf("exit=%d, stderr=%q", code, stderr)
				}
				lines := assertE2ELines(t, stdout, tc.count)
				if !strings.Contains(stdout, fmt.Sprintf("session limit/matched/output: 0/%d/%d\n", tc.count, tc.count)) || stderr != "" {
					t.Fatalf("report = %q, stderr = %q", stdout, stderr)
				}
				for i, title := range tc.titles {
					if !strings.Contains(lines[i], "  "+title+"  ") {
						t.Fatalf("line %d does not contain title %q: %s", i, title, lines[i])
					}
				}
				if tc.provider == "claude" {
					zeroTime := may18.Add(time.Hour).Format("20060102_15:04:05")
					if strings.Count(lines[1], zeroTime) != 2 || !strings.Contains(lines[1], "  0  ") {
						t.Fatalf("Claude zero-message session time or count incorrect: %s", lines[1])
					}
				}
			})
		}
	})

	t.Run("Copilot time and output shape", func(t *testing.T) {
		stdout, _, code := f.run(t, "-t", "copilot", "-n", "0")
		if code != 0 {
			t.Fatalf("exit=%d", code)
		}
		lines := assertE2ELines(t, stdout, 3)
		if !strings.HasPrefix(lines[0], "copilot-old  ") ||
			!strings.Contains(lines[0], "  Fix: #1  ") || !strings.Contains(lines[0], "  2  ") ||
			!strings.Contains(lines[0], may18.Add(5*time.Minute).Format("20060102_15:04:05")) ||
			!strings.HasSuffix(lines[0], may18.Add(10*time.Minute).Format("20060102_15:04:05")) {
			t.Fatalf("first line = %q; journal = %s", lines[0], old)
		}
		wantRecentTime := recent.Format("20060102_15:04:05")
		if strings.Count(lines[2], wantRecentTime) != 2 || !strings.Contains(lines[2], "  0  ") {
			t.Fatalf("zero-message session time or count incorrect: %s", lines[2])
		}
	})

	t.Run("default provider and top one", func(t *testing.T) {
		stdout, stderr, code := f.run(t, "-n", "1")
		lines := assertE2ELines(t, stdout, 1)
		if code != 0 || stderr != "" || !strings.HasPrefix(stdout, "provider: copilot\n") ||
			!strings.HasPrefix(lines[0], "copilot-unnamed  ") {
			t.Fatalf("default = (code=%d, stdout=%q, stderr=%q)", code, stdout, stderr)
		}
		stdout, _, code = f.run(t, "-t", "codex", "-n", "1")
		if code != 0 || !strings.HasPrefix(assertE2ELines(t, stdout, 1)[0], "codex-unnamed  ") {
			t.Fatalf("explicit Codex top one = (code=%d, stdout=%q)", code, stdout)
		}
	})

	t.Run("date and last-days filtering", func(t *testing.T) {
		day := may18.Format("20060102")
		stdout, stderr, code := f.run(t, "-t", "copilot", "-n", "0", "-s", day, "-e", day)
		lines := assertE2ELines(t, stdout, 2)
		if code != 0 || stderr != "" || !strings.HasPrefix(lines[0], "copilot-old  ") ||
			!strings.HasPrefix(lines[1], "copilot-fallback  ") ||
			!strings.Contains(stdout, "session limit/matched/output: 0/2/2\n") {
			t.Fatalf("date range = (code=%d, stdout=%q, stderr=%q)", code, stdout, stderr)
		}
		stdout, _, code = f.run(t, "-t", "copilot", "-l", "2", "-n", "0")
		if code != 0 || !strings.HasPrefix(assertE2ELines(t, stdout, 1)[0], "copilot-unnamed  ") {
			t.Fatalf("last two days = (code=%d, stdout=%q)", code, stdout)
		}
	})

	t.Run("project regex and exclusion", func(t *testing.T) {
		stdout, _, code := f.run(t, "-t", "copilot", "-p", strings.ToUpper(filepath.Base(f.workspace)), "-n", "0")
		if code != 0 || len(assertE2ELines(t, stdout, 3)) != 3 {
			t.Fatalf("case-insensitive project = (code=%d, stdout=%q)", code, stdout)
		}
		stdout, stderr, code := f.run(t, "-t", "copilot", "-p", ".*", "-x", strings.ToUpper(filepath.Base(f.other)), "-n", "0")
		if code != 0 || len(assertE2ELines(t, stdout, 3)) != 3 ||
			!strings.Contains(stdout, "exclude: "+strings.ToUpper(filepath.Base(f.other))) || stderr != "" {
			t.Fatalf("exclude before reading invalid body = (code=%d, stdout=%q, stderr=%q)", code, stdout, stderr)
		}
		stdout, stderr, code = f.run(t, "-t", "copilot", "-x", strings.ToUpper(filepath.Base(f.workspace)))
		if code != 0 || len(assertE2ELines(t, stdout, 0)) != 0 ||
			!strings.Contains(stdout, "session limit/matched/output: 10/0/0\n") || stderr != "" {
			t.Fatalf("excluded current project = (code=%d, stdout=%q, stderr=%q)", code, stdout, stderr)
		}
	})

	t.Run("invalid input and storage errors", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			args []string
			code int
			err  string
		}{
			{"unknown provider", []string{"-t", "other-provider"}, 1, "unknown provider"},
			{"missing flag value", []string{"--type"}, 2, "flag needs an argument"},
			{"invalid project pattern", []string{"-t", "copilot", "-p", "["}, 1, "error parsing regexp"},
			{"invalid exclusion pattern", []string{"-t", "copilot", "-x", "["}, 1, "error parsing regexp"},
			{"conflicting dates", []string{"-t", "copilot", "-l", "2", "-s", "20260518"}, 1, "--last conflicts"},
			{"invalid day", []string{"-t", "copilot", "-s", "invalid"}, 1, "invalid start-day"},
			{"selected malformed metadata", []string{"-t", "copilot", "-p", ".*"}, 1, "workspace.yaml"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				stdout, stderr, code := f.run(t, tc.args...)
				if code != tc.code || stdout != "" || !strings.HasPrefix(stderr, "[error] ") ||
					!strings.Contains(stderr, tc.err) {
					t.Fatalf("CLI = (code=%d, stdout=%q, stderr=%q)", code, stdout, stderr)
				}
			})
		}

		t.Run("bad journal header cannot be silently filtered", func(t *testing.T) {
			writeE2EFile(t, filepath.Join(f.copilotHome, "session-state", "bad-header", "events.jsonl"),
				`{"type":"session.start","timestamp":"invalid"}`+"\n")
			stdout, stderr, code := f.run(t, "-t", "copilot")
			if code != 1 || stdout != "" || !strings.Contains(stderr, "[error] ") ||
				!strings.Contains(stderr, filepath.Join("bad-header", "events.jsonl")) {
				t.Fatalf("bad header = (code=%d, stdout=%q, stderr=%q)", code, stdout, stderr)
			}
			stdout, stderr, code = f.run(t, "-t", "copilot", "-p", "[")
			if code != 1 || stdout != "" || !strings.Contains(stderr, "error parsing regexp") ||
				strings.Contains(stderr, filepath.Join("bad-header", "events.jsonl")) {
				t.Fatalf("regex must fail before scanning: (code=%d, stdout=%q, stderr=%q)", code, stdout, stderr)
			}
		})
	})
}
