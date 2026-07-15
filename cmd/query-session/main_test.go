package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		`provider: claude, codex, or cursor (default "codex")`,
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

	for _, provider := range []string{"claude", "codex", "cursor"} {
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
