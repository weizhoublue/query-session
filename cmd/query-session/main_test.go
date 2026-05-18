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
		"-l / --last",
		"print latest session only (default false)",
		"-p / --project string",
		"-s / --start-day string",
		"-t / --type string",
		`provider: claude or codex (default "claude")`,
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
