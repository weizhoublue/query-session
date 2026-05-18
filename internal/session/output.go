package session

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

const outputTimeFormat = "20060102_15:04:05"

func FormatLine(s Session) string {
	return fmt.Sprintf(
		`dir=%s sessionId=%s createTime=%s lastTime=%s file=%s firstMsg="%s" lastMsg="%s"`,
		s.Dir,
		s.SessionID,
		formatOutputTime(s.CreateTime),
		formatOutputTime(s.LastTime),
		s.File,
		CleanMessageSummary(s.FirstMsg),
		CleanMessageSummary(s.LastMsg),
	)
}

func CleanMessageSummary(msg string) string {
	var b strings.Builder
	previousSpace := true

	for _, r := range msg {
		if shouldReplaceWithSpace(r) {
			if !previousSpace {
				b.WriteRune(' ')
				previousSpace = true
			}
			continue
		}
		b.WriteRune(r)
		previousSpace = false
	}

	cleaned := strings.TrimSpace(b.String())
	runes := []rune(cleaned)
	if len(runes) > 10 {
		return string(runes[:10])
	}
	return cleaned
}

func formatOutputTime(t time.Time) string {
	return t.Local().Format(outputTimeFormat)
}

func shouldReplaceWithSpace(r rune) bool {
	return unicode.IsSpace(r) || unicode.IsControl(r) || r == '"' || r == '\\'
}
