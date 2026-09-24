package session

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestCleanMessageReplacesSpecialCharacters(t *testing.T) {
	got := cleanMessage("hi\n\t\"ok\"\\\x00go", 80)
	want := "hi ok go"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestCleanMessageCollapsesWhitespace(t *testing.T) {
	got := cleanMessage("  hi \n \t  world  ", 80)
	want := "hi world"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestCleanMessageKeepsSingleQuotes(t *testing.T) {
	got := cleanMessage("don't stop", 80)
	want := "don't stop"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestCleanMessageTruncatesToEightyUnicodeCharacters(t *testing.T) {
	got := cleanMessage(strings.Repeat("字", 81), 80)
	want := strings.Repeat("字", 80) + "...[81]"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestFormatTableUsesFiveColumns(t *testing.T) {
	loc := time.Local
	s := Session{
		Dir:           "/repo/app",
		SessionID:     "session-1",
		File:          "/claude/project/session-1.jsonl",
		CreateTime:    time.Date(2026, 5, 18, 9, 10, 11, 0, loc),
		LastTime:      time.Date(2026, 5, 18, 12, 13, 14, 0, loc),
		FirstMsg:      "hello\n\"first\" message",
		UserMsgAmount: 5,
	}

	var out bytes.Buffer
	if err := FormatTable(&out, []Session{s}, false); err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("%-11s%-21s%-11s%-19s%s\n", "SessionId", "Title", "MsgAmount", "CreateTime", "LastTime") +
		fmt.Sprintf("%-11s%-21s%-11s%-19s%s\n", "session-1", "hello first message", "5", "20260518_09:10:11", "20260518_12:13:14")
	if out.String() != want {
		t.Fatalf("got %q want %q", out.String(), want)
	}
}

func TestFormatTableShowsDirectory(t *testing.T) {
	when := time.Date(2026, 5, 18, 9, 10, 11, 0, time.Local)
	s := Session{SessionID: "session-1", Title: "hello", Dir: "/repo/app",
		CreateTime: when, LastTime: when, UserMsgAmount: 1}
	var out bytes.Buffer
	if err := FormatTable(&out, []Session{s}, true); err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("%-11s%-7s%-11s%-19s%-19s%s\n", "SessionId", "Title", "MsgAmount", "CreateTime", "LastTime", "Directory") +
		fmt.Sprintf("%-11s%-7s%-11s%-19s%-19s%s\n", "session-1", "hello", "1", "20260518_09:10:11", "20260518_09:10:11", "/repo/app")
	if out.String() != want {
		t.Fatalf("got %q want %q", out.String(), want)
	}
	out.Reset()
	if err := FormatTable(&out, nil, true); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "SessionId  Title  MsgAmount  CreateTime  LastTime  Directory\n" {
		t.Fatalf("empty table = %q", got)
	}
	out.Reset()
	s.Dir = "/repo/odd\tname\n"
	if err := FormatTable(&out, []Session{s}, true); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.HasSuffix(got, "/repo/odd\\tname\\n\n") {
		t.Fatalf("directory must stay on one line: %q", got)
	}
}

func TestFormatTableUsesNativeTitleAndOmitsMessages(t *testing.T) {
	loc := time.Local
	s := Session{
		Dir:           "/repo/app",
		SessionID:     "solo",
		File:          "/path/solo.jsonl",
		CreateTime:    time.Date(2026, 5, 19, 6, 3, 37, 0, loc),
		LastTime:      time.Date(2026, 5, 19, 6, 3, 37, 0, loc),
		FirstMsg:      "only question",
		Title:         "  Native\n\"session\" title  ",
		UserMsgAmount: 1,
	}

	var out bytes.Buffer
	if err := FormatTable(&out, []Session{s}, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Native session title  1") ||
		strings.Contains(out.String(), "only question") || strings.Contains(out.String(), s.File) {
		t.Fatalf("unexpected table: %q", out.String())
	}
}

func TestFormatTableEmptyAndEscapedID(t *testing.T) {
	var out bytes.Buffer
	if err := FormatTable(&out, nil, false); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "SessionId  Title  MsgAmount  CreateTime  LastTime\n"; got != want {
		t.Fatalf("empty table = %q, want %q", got, want)
	}
	out.Reset()
	if err := FormatTable(&out, []Session{{SessionID: "bad\tid\n\"x\"", Title: "ok"}}, false); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 2 || !strings.Contains(lines[1], `bad\tid\n\"x\"  ok`) {
		t.Fatalf("escaped table = %q", out.String())
	}
}

func TestFormatTitleFallbackAndUnicodeLimit(t *testing.T) {
	if got := formatTitle(Session{Title: " \n", FirstMsg: "first user prompt"}); got != "first user prompt" {
		t.Fatalf("fallback = %q", got)
	}
	if got := formatTitle(Session{}); got != "未命名" {
		t.Fatalf("unnamed = %q", got)
	}
	if got := formatTitle(Session{Title: "一二三四五六七八九十" + strings.Repeat("字", 71)}); got != "一二三四五六七八九十"+strings.Repeat("字", 70)+"...[81]" {
		t.Fatalf("truncated title = %q", got)
	}
}
