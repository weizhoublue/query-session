package session

import (
	"testing"
	"time"
)

func TestCleanMessageSummaryReplacesSpecialCharacters(t *testing.T) {
	got := CleanMessageSummary("hi\n\t\"ok\"\\\x00go")
	want := "hi ok go"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestCleanMessageSummaryCollapsesWhitespace(t *testing.T) {
	got := CleanMessageSummary("  hi \n \t  world  ")
	want := "hi world"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestCleanMessageSummaryKeepsSingleQuotes(t *testing.T) {
	got := CleanMessageSummary("don't stop")
	want := "don't stop"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestCleanMessageSummaryTruncatesToTwentyUnicodeCharacters(t *testing.T) {
	got := CleanMessageSummary("一二三四五六七八九十一二三四五六七八九十一")
	want := "一二三四五六七八九十一二三四五六七八九十"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestFormatLineUsesCompleteFixedFormat(t *testing.T) {
	loc := time.Local
	s := Session{
		Dir:        "/repo/app",
		SessionID:  "session-1",
		File:       "/claude/project/session-1.jsonl",
		CreateTime: time.Date(2026, 5, 18, 9, 10, 11, 0, loc),
		LastTime:   time.Date(2026, 5, 18, 12, 13, 14, 0, loc),
		FirstMsg:   "hello\n\"first\" message",
		LastMsg:    "last\tmessage\\done",
	}

	got := FormatLine(s)
	want := `dir=/repo/app sessionId=session-1 createTime=20260518_09:10:11 lastTime=20260518_12:13:14 file=/claude/project/session-1.jsonl firstMsg="hello first message" lastMsg="last message done"`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestFormatLineLastMsgEmptyWhenSameAsFirst(t *testing.T) {
	loc := time.Local
	s := Session{
		Dir:        "/repo/app",
		SessionID:  "solo",
		File:       "/path/solo.jsonl",
		CreateTime: time.Date(2026, 5, 19, 6, 3, 37, 0, loc),
		LastTime:   time.Date(2026, 5, 19, 6, 3, 37, 0, loc),
		FirstMsg:   "only question",
		LastMsg:    "only question",
	}

	got := FormatLine(s)
	want := `dir=/repo/app sessionId=solo createTime=20260519_06:03:37 lastTime=20260519_06:03:37 file=/path/solo.jsonl firstMsg="only question" lastMsg=""`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
