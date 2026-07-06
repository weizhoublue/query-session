package main

import (
	"bytes"
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
