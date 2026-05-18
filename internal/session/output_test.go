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

func TestCleanMessageSummaryTruncatesToTenUnicodeCharacters(t *testing.T) {
	got := CleanMessageSummary("你好世界一二三四五六七")
	want := "你好世界一二三四五六"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestFormatLineUsesCompleteFixedFormat(t *testing.T) {
	loc := time.FixedZone("test", 8*60*60)
	s := Session{
		Dir:        "/repo/app",
		SessionID:  "session-1",
		CreateTime: time.Date(2026, 5, 18, 9, 10, 11, 0, loc),
		LastTime:   time.Date(2026, 5, 18, 12, 13, 14, 0, loc),
		FirstMsg:   "hello\n\"first\" message",
		LastMsg:    "last\tmessage\\done",
	}

	got := FormatLine(s)
	want := `dir=/repo/app sessionId=session-1 createTime=20260518_09:10:11 lastTime=20260518_12:13:14 firstMsg="hello firs" lastMsg="last messa"`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
