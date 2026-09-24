package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunUnknownProviderReturnsErrorWithoutWritingStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code, err := run([]string{"-t", "nope"}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if err == nil || err.Error() != "unknown provider: nope" {
		t.Fatalf("err = %v, want unknown provider", err)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunBadFlagReturnsErrorWithoutFlagOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code, err := run([]string{"--bad"}, &stdout, &stderr)

	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if err == nil || !strings.Contains(err.Error(), "flag provided but not defined") {
		t.Fatalf("err = %v, want bad flag error", err)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunUnexpectedArgumentsReturnsError(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code, err := run([]string{"t", "codex", "-l", "5", "-n", "4"}, &stdout, &stderr)

	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if err == nil || err.Error() != `unexpected arguments: "t" "codex" "-l" "5" "-n" "4"` {
		t.Fatalf("err = %v, want unexpected arguments error", err)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunHelpCombinesShortAndLongFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code, err := run([]string{"--help"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"-d / --debug",
		"-e / --end-day string",
		"-l / --last int",
		"cover past N days including today",
		"-n / --number int",
		"print top N sessions by createTime",
		"-p / --project string",
		"-s / --start-day string",
		"-t / --type string",
		`provider: claude, codex, cursor, or copilot (default "copilot")`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("help output missing %q in:\n%s", want, out)
		}
	}
}

func TestRunLongTypeFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code, err := run([]string{"--type", "nope"}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if err == nil || err.Error() != "unknown provider: nope" {
		t.Fatalf("err = %v, want unknown provider", err)
	}
}

func TestRunShortTypeFlagRejectsUnknownProvider(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code, err := run([]string{"-t", "nope"}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if err == nil || err.Error() != "unknown provider: nope" {
		t.Fatalf("err = %v, want unknown provider", err)
	}
}

func TestRunVersionFlags(t *testing.T) {
	for _, flagName := range []string{"-v", "--version"} {
		t.Run(flagName, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			code, err := run([]string{flagName}, &stdout, &stderr)

			if code != 0 {
				t.Fatalf("code = %d, want 0", code)
			}
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if stdout.String() != version+"\n" {
				t.Fatalf("stdout = %q, want version", stdout.String())
			}
			if stderr.String() != "" {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
		})
	}
}

func TestRunMissingFlagValueReturnsParseError(t *testing.T) {
	for _, flagName := range []string{"-t", "--type", "-n", "--number", "-l", "--last", "-p", "--project", "-x", "--exclude", "-s", "--start-day", "-e", "--end-day"} {
		t.Run(flagName, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			code, err := run([]string{flagName}, &stdout, &stderr)

			if code != 2 {
				t.Fatalf("code = %d, want 2", code)
			}
			if err == nil || !strings.Contains(err.Error(), "flag needs an argument") {
				t.Fatalf("err = %v, want missing argument error", err)
			}
		})
	}
}

func TestRunInvalidIntegerFlagReturnsParseError(t *testing.T) {
	for _, flagName := range []string{"-n", "--number", "-l", "--last"} {
		t.Run(flagName, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			code, err := run([]string{flagName, "not-an-int"}, &stdout, &stderr)

			if code != 2 {
				t.Fatalf("code = %d, want 2", code)
			}
			if err == nil || !strings.Contains(err.Error(), "invalid value") {
				t.Fatalf("err = %v, want invalid value error", err)
			}
		})
	}
}

func TestRunLastConflictsWithStartDay(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code, err := run([]string{"-l", "3", "-s", "20260101"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if err == nil || !strings.Contains(err.Error(), "--last conflicts with") {
		t.Fatalf("err = %v, want conflict error", err)
	}
}

func TestRunLastConflictsWithEndDay(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code, err := run([]string{"-l", "3", "-e", "20260101"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if err == nil || !strings.Contains(err.Error(), "--last conflicts with") {
		t.Fatalf("err = %v, want conflict error", err)
	}
}

func TestRunInvalidDateReturnsError(t *testing.T) {
	for _, args := range [][]string{
		{"-s", "not-a-date"},
		{"--start-day", "not-a-date"},
		{"-e", "not-a-date"},
		{"--end-day", "not-a-date"},
	} {
		t.Run(strings.Join(args, "-"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			code, err := run(args, &stdout, &stderr)

			if code != 1 {
				t.Fatalf("code = %d, want 1", code)
			}
			if err == nil || !strings.Contains(err.Error(), "invalid") {
				t.Fatalf("err = %v, want invalid date error", err)
			}
		})
	}
}

func TestRunAcceptsValidQueryFlags(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".claude", "projects"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("COPILOT_HOME", "")
	for _, provider := range []string{"claude", "codex", "cursor", "copilot"} {
		t.Run(provider, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			code, err := run([]string{
				"-t", provider,
				"-n", "4",
				"-l", "5",
				"-p", ".*",
				"-x", "never-matches",
				"-d",
			}, &stdout, &stderr)

			if code != 0 {
				t.Fatalf("code = %d, want 0; err = %v", code, err)
			}

			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if !strings.Contains(stderr.String(), "scanning") {
				t.Fatalf("stderr = %q, want debug scan log", stderr.String())
			}
		})
	}
}

func TestRunCopilotExcludesUnnamedZeroMessageSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("COPILOT_HOME", filepath.Join(home, ".copilot"))
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id, dir, body, name string
	}{
		{"titled", cwd, `{"type":"user.message","timestamp":"2026-05-18T12:00:00Z","data":{"content":"first prompt"}}` + "\n", "name: 'Native summary'\n"},
		{"unnamed", cwd, "", "id: unnamed\n"},
		{"other", filepath.Join(home, "other"), `{"type":"user.message",broken}` + "\n", ""},
	} {
		dir := filepath.Join(home, ".copilot", "session-state", tc.id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		header := fmt.Sprintf(`{"type":"session.start","timestamp":"2026-05-18T10:00:00Z","data":{"sessionId":%q,"context":{"cwd":%q}}}`+"\n", tc.id, tc.dir)
		if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte(header+tc.body), 0o644); err != nil {
			t.Fatal(err)
		}
		if tc.name != "" {
			if err := os.WriteFile(filepath.Join(dir, "workspace.yaml"), []byte(tc.name), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	var stdout, stderr bytes.Buffer
	code, err := run([]string{"-t", "copilot", "-n", "0"}, &stdout, &stderr)
	if code != 0 || err != nil {
		t.Fatalf("run = (%d, %v)", code, err)
	}
	lines := sessionRows(t, stdout.String())
	if len(lines) != 1 || !strings.Contains(lines[0], "  Native summary  ") ||
		strings.Contains(stdout.String(), "unnamed  ") {
		t.Fatalf("unexpected output: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "session limit/matched/output: 0/1/1\n\n") || stderr.Len() != 0 {
		t.Fatalf("report = %q, stderr = %q", stdout.String(), stderr.String())
	}

	stdout.Reset()
	code, err = run([]string{"-t", "copilot", "-n", "1"}, &stdout, &stderr)
	if code != 0 || err != nil || stderr.Len() != 0 {
		t.Fatalf("run top one = (%d, %v), stderr = %q", code, err, stderr.String())
	}
	created := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC).Local().Format("20060102_15:04:05")
	want := fmt.Sprintf("provider: copilot\ndirectory: %s\ntime range: all\nsession limit/matched/output: 1/1/1\n\n", cwd) +
		fmt.Sprintf("%-11s%-16s%-11s%-19s%s\n", "SessionId", "Title", "MsgAmount", "CreateTime", "LastTime") +
		fmt.Sprintf("%-11s%-16s%-11s%-19s%s\n", "titled", "Native summary", "1", created, created)
	if stdout.String() != want {
		t.Fatalf("top one output = %q, want %q", stdout.String(), want)
	}
}

func TestRunCopilotRejectsInvalidRegexBeforeOpeningJournal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("COPILOT_HOME", filepath.Join(home, ".copilot"))
	dir := filepath.Join(home, ".copilot", "session-state", "broken")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte("invalid header"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code, err := run([]string{"-t", "copilot", "-p", "["}, &stdout, &stderr)
	if code != 1 || err == nil || !strings.Contains(err.Error(), "error parsing regexp") {
		t.Fatalf("run = (%d, %v), want regex error before journal read", code, err)
	}
}

func TestRunInvalidProjectAndExcludePatternsReturnErrors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	for _, args := range [][]string{
		{"-t", "codex", "-p", "["},
		{"-t", "codex", "-x", "["},
	} {
		t.Run(strings.Join(args, "-"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			code, err := run(args, &stdout, &stderr)

			if code != 1 {
				t.Fatalf("code = %d, want 1", code)
			}
			if err == nil || !strings.Contains(err.Error(), "error parsing regexp") {
				t.Fatalf("err = %v, want regexp error", err)
			}
		})
	}
}

func TestRunStartDayMustNotBeLaterThanEndDay(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code, err := run([]string{"-s", "20260102", "-e", "20260101"}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if err == nil || err.Error() != "start-day must not be later than end-day" {
		t.Fatalf("err = %v, want date range error", err)
	}
}

func TestRunNegativeNumberReturnsError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code, err := run([]string{"-n", "-1"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if err == nil || !strings.Contains(err.Error(), "--number must be") {
		t.Fatalf("err = %v, want number error", err)
	}
}

func TestRunNegativeLastReturnsError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code, err := run([]string{"-l", "-1"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if err == nil || !strings.Contains(err.Error(), "--last must be") {
		t.Fatalf("err = %v, want last error", err)
	}
}

func TestRunDefaultsToAllDatesAndTenSessions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("COPILOT_HOME", "")
	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	writeCopilotSessions(t, home, currentDir, 11)

	var stdout, stderr bytes.Buffer
	code, err := run(nil, &stdout, &stderr)
	if code != 0 || err != nil {
		t.Fatalf("run() = (%d, %v), want (0, nil)", code, err)
	}
	lines := sessionRows(t, stdout.String())
	if got := len(lines); got != 10 {
		t.Fatalf("printed sessions = %d, want 10; output:\n%s", got, stdout.String())
	}
	if !strings.Contains(stdout.String(), "session-11  ") {
		t.Fatalf("output missing newest session:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "session-01  ") {
		t.Fatalf("output contains oldest session:\n%s", stdout.String())
	}
	wantSummary := fmt.Sprintf("provider: copilot\ndirectory: %s\ntime range: all\nsession limit/matched/output: 10/11/10\n\n", currentDir)
	if !strings.HasPrefix(stdout.String(), wantSummary) || stderr.Len() != 0 {
		t.Fatalf("report = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestRunNumberZeroReturnsAllDatesAndAllSessions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	writeCodexSessions(t, home, currentDir, 11)

	var stdout, stderr bytes.Buffer
	code, err := run([]string{"-t", "codex", "-n", "0"}, &stdout, &stderr)
	if code != 0 || err != nil {
		t.Fatalf("run() = (%d, %v), want (0, nil)", code, err)
	}
	if got := len(sessionRows(t, stdout.String())); got != 11 {
		t.Fatalf("printed sessions = %d, want 11; output:\n%s", got, stdout.String())
	}
	if !strings.Contains(stdout.String(), "session limit/matched/output: 0/11/11\n\n") || stderr.Len() != 0 {
		t.Fatalf("report = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestRunExplicitDateRangeStillFiltersSessions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	writeCodexSessions(t, home, currentDir, 11)
	today := time.Now().In(time.Local).Format("20060102")

	var stdout, stderr bytes.Buffer
	code, err := run([]string{"-t", "codex", "-s", today, "-e", today, "-n", "0"}, &stdout, &stderr)
	if code != 0 || err != nil {
		t.Fatalf("run() = (%d, %v), want (0, nil)", code, err)
	}
	if got := len(sessionRows(t, stdout.String())); got != 1 {
		t.Fatalf("printed sessions = %d, want 1; output:\n%s", got, stdout.String())
	}
	if !strings.Contains(stdout.String(), "time range: "+today+".."+today+"\n") || stderr.Len() != 0 {
		t.Fatalf("report = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestRunPrintsSummaryWhenNoSessionsMatch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("COPILOT_HOME", "")
	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code, err := run(nil, &stdout, &stderr)
	if code != 0 || err != nil {
		t.Fatalf("run() = (%d, %v), want (0, nil)", code, err)
	}
	want := fmt.Sprintf("provider: copilot\ndirectory: %s\ntime range: all\nsession limit/matched/output: 10/0/0\n\n", currentDir) +
		"SessionId  Title  MsgAmount  CreateTime  LastTime\n"
	if stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("report = %q, want %q; stderr = %q", stdout.String(), want, stderr.String())
	}
}

func TestRunSummaryShowsExplicitFilters(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("COPILOT_HOME", "")

	var stdout, stderr bytes.Buffer
	code, err := run([]string{"-l", "3", "-n", "0", "-p", "project-regexp", "-x", "excluded-regexp"}, &stdout, &stderr)
	if code != 0 || err != nil {
		t.Fatalf("run() = (%d, %v), want (0, nil)", code, err)
	}
	want := "provider: copilot\ndirectory: project-regexp\nexclude: excluded-regexp\ntime range: last 3 days\nsession limit/matched/output: 0/0/0\n\n" +
		"SessionId  Title  MsgAmount  CreateTime  LastTime\n"
	if stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("report = %q, want %q; stderr = %q", stdout.String(), want, stderr.String())
	}
}

func sessionRows(t *testing.T, report string) []string {
	t.Helper()
	parts := strings.SplitN(report, "\n\n", 2)
	if len(parts) != 2 {
		t.Fatalf("report missing table separator: %q", report)
	}
	lines := strings.Split(strings.TrimSuffix(parts[1], "\n"), "\n")
	if len(lines) == 0 || strings.Join(strings.Fields(lines[0]), " ") != "SessionId Title MsgAmount CreateTime LastTime" {
		t.Fatalf("report missing table header: %q", report)
	}
	return lines[1:]
}

func writeCodexSessions(t *testing.T, home, cwd string, count int) {
	t.Helper()
	today := time.Now().In(time.Local)
	for i := 1; i <= count; i++ {
		created := today.AddDate(0, 0, -(count - i)).Add(time.Hour)
		dayDir := filepath.Join(home, ".codex", "sessions", created.Format("2006"), created.Format("01"), created.Format("02"))
		if err := os.MkdirAll(dayDir, 0o755); err != nil {
			t.Fatal(err)
		}
		content := fmt.Sprintf(`{"timestamp":%q,"payload":{"id":"session-%02d","cwd":%q,"role":"user","content":[{"type":"input_text","text":"message"}]}}`+"\n", created.Format(time.RFC3339Nano), i, cwd)
		if err := os.WriteFile(filepath.Join(dayDir, fmt.Sprintf("session-%02d.jsonl", i)), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func writeCopilotSessions(t *testing.T, home, cwd string, count int) {
	t.Helper()
	today := time.Now().In(time.Local)
	for i := 1; i <= count; i++ {
		created := today.AddDate(0, 0, -(count - i)).Add(time.Hour)
		id := fmt.Sprintf("session-%02d", i)
		path := filepath.Join(home, ".copilot", "session-state", id, "events.jsonl")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		content := fmt.Sprintf(`{"type":"session.start","timestamp":%q,"data":{"sessionId":%q,"context":{"cwd":%q}}}`+"\n"+
			`{"type":"user.message","timestamp":%q,"data":{"content":"message"}}`+"\n",
			created.Format(time.RFC3339Nano), id, cwd, created.Add(time.Minute).Format(time.RFC3339Nano))
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
