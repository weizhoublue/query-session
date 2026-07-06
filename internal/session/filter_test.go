package session

import (
	"strings"
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

func TestParseDayRangeSameDayUTC(t *testing.T) {
	start, end, err := ParseDayRange("20260518", "20260518", time.UTC)
	if err != nil {
		t.Fatalf("parse range: %v", err)
	}
	wantStart := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 5, 18, 23, 59, 59, 999999999, time.UTC)
	if !start.Equal(wantStart) || !end.Equal(wantEnd) {
		t.Fatalf("start=%s end=%s, want %s %s", start, end, wantStart, wantEnd)
	}
}

func TestParseDayRangeMultiDayRange(t *testing.T) {
	start, end, err := ParseDayRange("20260518", "20260520", time.UTC)
	if err != nil {
		t.Fatalf("parse range: %v", err)
	}
	wantStart := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 5, 20, 23, 59, 59, 999999999, time.UTC)
	if !start.Equal(wantStart) || !end.Equal(wantEnd) {
		t.Fatalf("start=%s end=%s, want %s %s", start, end, wantStart, wantEnd)
	}
}

func TestFilterDateBoundaryInclusion(t *testing.T) {
	start, end, err := ParseDayRange("20260518", "20260518", time.UTC)
	if err != nil {
		t.Fatalf("parse range: %v", err)
	}

	sessions := []Session{
		{SessionID: "at-start", Dir: "/a", CreateTime: start},
		{SessionID: "at-end", Dir: "/a", CreateTime: end},
		{SessionID: "before", Dir: "/a", CreateTime: start.Add(-time.Nanosecond)},
		{SessionID: "after", Dir: "/a", CreateTime: end.Add(time.Nanosecond)},
	}

	got, err := Filter(sessions, FilterOptions{
		CurrentDir: "/a",
		Start:      start,
		End:        end,
	})
	if err != nil {
		t.Fatalf("filter: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d sessions, want 2 (at-start and at-end included)", len(got))
	}
	ids := make(map[string]bool)
	for _, s := range got {
		ids[s.SessionID] = true
	}
	for _, want := range []string{"at-start", "at-end"} {
		if !ids[want] {
			t.Fatalf("missing session %q", want)
		}
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

func TestFilterExcludeTakesPrecedence(t *testing.T) {
	sessions := []Session{
		{Dir: "/repo/foo", CreateTime: mustTime(t, "2026-05-18T01:00:00Z")},
		{Dir: "/repo/foobar", CreateTime: mustTime(t, "2026-05-18T02:00:00Z")},
		{Dir: "/repo/bar", CreateTime: mustTime(t, "2026-05-18T03:00:00Z")},
	}

	start, end, err := ParseDayRange("20260518", "20260518", time.UTC)
	if err != nil {
		t.Fatalf("parse range: %v", err)
	}

	got, err := Filter(sessions, FilterOptions{
		ProjectPattern: "foo",
		ExcludePattern: "bar",
		CurrentDir:     "/not-used",
		Start:          start,
		End:            end,
	})
	if err != nil {
		t.Fatalf("filter: %v", err)
	}
	if len(got) != 1 || got[0].Dir != "/repo/foo" {
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

func TestFilterLogsMatchedAndFilteredSessions(t *testing.T) {
	var logs []string
	sessions := []Session{
		{SessionID: "match", Dir: "/repo/a", CreateTime: mustTime(t, "2026-05-18T01:00:00Z"), LastTime: mustTime(t, "2026-05-18T01:30:00Z")},
		{SessionID: "wrong-project", Dir: "/repo/b", CreateTime: mustTime(t, "2026-05-18T02:00:00Z")},
		{SessionID: "wrong-date", Dir: "/repo/a", CreateTime: mustTime(t, "2026-05-19T01:00:00Z")},
	}
	start, end, err := ParseDayRange("20260518", "20260518", time.UTC)
	if err != nil {
		t.Fatalf("parse range: %v", err)
	}

	got, err := Filter(sessions, FilterOptions{
		CurrentDir: "/repo/a",
		Start:      start,
		End:        end,
		Log: func(level, message string) {
			logs = append(logs, level+":"+message)
		},
	})
	if err != nil {
		t.Fatalf("filter: %v", err)
	}
	if len(got) != 1 || got[0].SessionID != "match" {
		t.Fatalf("unexpected result: %#v", got)
	}
	joined := strings.Join(logs, "\n")
	for _, want := range []string{
		"info:matched sessionId=match",
		"info:filtered sessionId=wrong-project reason=project",
		"info:filtered sessionId=wrong-date reason=date",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("logs missing %q in:\n%s", want, joined)
		}
	}
}

func TestParseLastDaysOneDayIsToday(t *testing.T) {
	now := time.Now().UTC()
	start, end, err := ParseLastDays(1, time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	todayEnd := todayStart.AddDate(0, 0, 1).Add(-time.Nanosecond)
	if !start.Equal(todayStart) {
		t.Fatalf("start = %s, want %s", start, todayStart)
	}
	if !end.Equal(todayEnd) {
		t.Fatalf("end = %s, want %s", end, todayEnd)
	}
}

func TestParseLastDaysThreeDays(t *testing.T) {
	now := time.Now().UTC()
	start, end, err := ParseLastDays(3, time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	wantStart := todayStart.AddDate(0, 0, -2)
	wantEnd := todayStart.AddDate(0, 0, 1).Add(-time.Nanosecond)
	if !start.Equal(wantStart) {
		t.Fatalf("start = %s, want %s", start, wantStart)
	}
	if !end.Equal(wantEnd) {
		t.Fatalf("end = %s, want %s", end, wantEnd)
	}
}

func TestParseLastDaysRejectsZero(t *testing.T) {
	_, _, err := ParseLastDays(0, time.UTC)
	if err == nil {
		t.Fatal("expected error for n=0")
	}
}

func TestTopNByCreateTimeReturnsTopTwo(t *testing.T) {
	sessions := []Session{
		{SessionID: "old", CreateTime: mustTime(t, "2026-05-18T01:00:00Z")},
		{SessionID: "newest", CreateTime: mustTime(t, "2026-05-18T03:00:00Z")},
		{SessionID: "mid", CreateTime: mustTime(t, "2026-05-18T02:00:00Z")},
	}
	orig := make([]Session, len(sessions))
	copy(orig, sessions)

	got := TopNByCreateTime(sessions, 2)

	if len(got) != 2 {
		t.Fatalf("got %d sessions, want 2", len(got))
	}
	if got[0].SessionID != "newest" || got[1].SessionID != "mid" {
		t.Fatalf("unexpected order: %v %v", got[0].SessionID, got[1].SessionID)
	}
	// 确认原切片未被修改
	for i, s := range sessions {
		if s.SessionID != orig[i].SessionID {
			t.Fatalf("input slice mutated at index %d", i)
		}
	}
}

func TestTopNByCreateTimeZeroReturnsAll(t *testing.T) {
	sessions := []Session{
		{SessionID: "a", CreateTime: mustTime(t, "2026-05-18T01:00:00Z")},
		{SessionID: "b", CreateTime: mustTime(t, "2026-05-18T02:00:00Z")},
	}
	got := TopNByCreateTime(sessions, 0)
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
}

func TestTopNByCreateTimeNLargerThanLen(t *testing.T) {
	sessions := []Session{
		{SessionID: "a", CreateTime: mustTime(t, "2026-05-18T01:00:00Z")},
	}
	got := TopNByCreateTime(sessions, 99)
	if len(got) != 1 {
		t.Fatalf("got %d, want 1", len(got))
	}
}
