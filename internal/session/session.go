package session

import (
	"fmt"
	"time"
)

type Provider string

const (
	ProviderClaude Provider = "claude"
	ProviderCodex  Provider = "codex"
	ProviderCursor Provider = "cursor"
)

type Session struct {
	SessionID     string
	Dir           string
	File          string
	CreateTime    time.Time
	LastTime      time.Time
	FirstMsg      string
	LastMsg       string
	UserMsgAmount int
}

type Logger func(level, message string)

type FilterOptions struct {
	ProjectPattern string
	ExcludePattern string
	CurrentDir     string
	SkipDateFilter bool
	Start          time.Time
	End            time.Time
	Log            Logger
}

func ParseDayRange(startDay, endDay string, loc *time.Location) (time.Time, time.Time, error) {
	start, err := time.ParseInLocation("20060102", startDay, loc)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid start-day %q: %w", startDay, err)
	}

	endDayStart, err := time.ParseInLocation("20060102", endDay, loc)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid end-day %q: %w", endDay, err)
	}
	if start.After(endDayStart) {
		return time.Time{}, time.Time{}, fmt.Errorf("start-day must not be later than end-day")
	}

	end := endDayStart.AddDate(0, 0, 1).Add(-time.Nanosecond)
	return start, end, nil
}

// ParseLastDays 计算"过去 n 天含今天"的时间窗口，与 ParseDayRange 对称。
// n=1 返回今天零点到今天末尾；n=3 返回前天零点到今天末尾。
func ParseLastDays(n int, loc *time.Location) (time.Time, time.Time, error) {
	if n < 1 {
		return time.Time{}, time.Time{}, fmt.Errorf("--last must be >= 1, got %d", n)
	}
	now := time.Now().In(loc)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	start := todayStart.AddDate(0, 0, -(n - 1))
	end := todayStart.AddDate(0, 0, 1).Add(-time.Nanosecond)
	return start, end, nil
}
