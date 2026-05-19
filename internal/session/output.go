package session

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

const outputTimeFormat = "20060102_15:04:05"

func FormatLine(s Session) string {
	lastMsg := s.LastMsg
	if s.FirstMsg == s.LastMsg {
		lastMsg = ""
	}
	return fmt.Sprintf(
		`dir=%s sessionId=%s createTime=%s lastTime=%s file=%s userMsgAmount=%d firstMsg="%s" lastMsg="%s"`,
		s.Dir,
		s.SessionID,
		formatOutputTime(s.CreateTime),
		formatOutputTime(s.LastTime),
		s.File,
		s.UserMsgAmount,
		CleanMessageSummary(s.FirstMsg),
		CleanMessageSummary(lastMsg),
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
	if len(runes) > 20 {
		return fmt.Sprintf("%s...[%d]", string(runes[:20]), len(runes))
	}
	return cleaned
}

func formatOutputTime(t time.Time) string {
	return t.Local().Format(outputTimeFormat)
}

func shouldReplaceWithSpace(r rune) bool {
	return unicode.IsSpace(r) || unicode.IsControl(r) || r == '"' || r == '\\'
}
