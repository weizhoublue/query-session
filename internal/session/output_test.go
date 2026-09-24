package session

import (
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

func TestFormatLineUsesCompleteFixedFormat(t *testing.T) {
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

	got := FormatLine(s)
	want := `dir=/repo/app sessionId=session-1 createTime=20260518_09:10:11 lastTime=20260518_12:13:14 file=/claude/project/session-1.jsonl userMsgAmount=5 title="hello first message"`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestFormatLineUsesNativeTitleAndOmitsMessages(t *testing.T) {
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

	got := FormatLine(s)
	want := `dir=/repo/app sessionId=solo createTime=20260519_06:03:37 lastTime=20260519_06:03:37 file=/path/solo.jsonl userMsgAmount=1 title="Native session title"`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
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
