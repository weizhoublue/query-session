package session

import (
	"testing"
	"time"
)

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse time: %v", err)
	}
	return parsed
}

func TestParseDayRangeRejectsStartAfterEnd(t *testing.T) {
	_, _, err := ParseDayRange("20260519", "20260518", time.Local)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseDayRangeUsesLocalCalendarDayForEnd(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	_, end, err := ParseDayRange("20260308", "20260308", loc)
	if err != nil {
		t.Fatalf("parse range: %v", err)
	}

	want := time.Date(2026, 3, 9, 0, 0, 0, 0, loc).Add(-time.Nanosecond)
	if !end.Equal(want) {
		t.Fatalf("end = %s want %s", end, want)
	}
}

func TestFilterExactCurrentDirWhenProjectEmpty(t *testing.T) {
	sessions := []Session{
		{Dir: "/repo/a", CreateTime: mustTime(t, "2026-05-18T01:00:00Z")},
		{Dir: "/repo/b", CreateTime: mustTime(t, "2026-05-18T02:00:00Z")},
	}

	start, end, err := ParseDayRange("20260518", "20260518", time.UTC)
	if err != nil {
		t.Fatalf("parse range: %v", err)
	}

	got, err := Filter(sessions, FilterOptions{
		CurrentDir: "/repo/a",
		Start:      start,
		End:        end,
	})
	if err != nil {
		t.Fatalf("filter: %v", err)
	}
	if len(got) != 1 || got[0].Dir != "/repo/a" {
		t.Fatalf("unexpected result: %#v", got)
	}
}

func TestFilterProjectRegexCaseInsensitive(t *testing.T) {
	sessions := []Session{
		{Dir: "/Users/me/Foo", CreateTime: mustTime(t, "2026-05-18T01:00:00Z")},
		{Dir: "/Users/me/bar", CreateTime: mustTime(t, "2026-05-18T02:00:00Z")},
	}

	start, end, err := ParseDayRange("20260518", "20260518", time.UTC)
	if err != nil {
		t.Fatalf("parse range: %v", err)
	}

	got, err := Filter(sessions, FilterOptions{
		ProjectPattern: "foo",
		CurrentDir:     "/not-used",
		Start:          start,
		End:            end,
	})
	if err != nil {
		t.Fatalf("filter: %v", err)
	}
	if len(got) != 1 || got[0].Dir != "/Users/me/Foo" {
		t.Fatalf("unexpected result: %#v", got)
	}
}

func TestLatestUsesCreateTime(t *testing.T) {
	sessions := []Session{
		{SessionID: "old", CreateTime: mustTime(t, "2026-05-18T01:00:00Z")},
		{SessionID: "new", CreateTime: mustTime(t, "2026-05-18T03:00:00Z")},
		{SessionID: "middle", CreateTime: mustTime(t, "2026-05-18T02:00:00Z")},
	}

	got := LatestByCreateTime(sessions)
	if got == nil || got.SessionID != "new" {
		t.Fatalf("unexpected latest: %#v", got)
	}
}

func TestSortByDirThenCreateTime(t *testing.T) {
	sessions := []Session{
		{SessionID: "b2", Dir: "/b", CreateTime: mustTime(t, "2026-05-18T02:00:00Z")},
		{SessionID: "a2", Dir: "/a", CreateTime: mustTime(t, "2026-05-18T02:00:00Z")},
		{SessionID: "a1", Dir: "/a", CreateTime: mustTime(t, "2026-05-18T01:00:00Z")},
	}

	SortSessions(sessions)
	got := []string{sessions[0].SessionID, sessions[1].SessionID, sessions[2].SessionID}
	want := []string{"a1", "a2", "b2"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}
