package session

import (
	"fmt"
	"time"
)

type Provider string

const (
	ProviderClaude Provider = "claude"
	ProviderCodex  Provider = "codex"
)

type Session struct {
	SessionID  string
	Dir        string
	File       string
	CreateTime time.Time
	LastTime   time.Time
	FirstMsg   string
	LastMsg    string
}

type Logger func(level, message string)

type FilterOptions struct {
	ProjectPattern string
	ExcludePattern string
	CurrentDir     string
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
