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
		`dir=%s sessionId=%s createTime=%s lastTime=%s file=%s userMsgAmount=%d title="%s"`,
		s.Dir,
		s.SessionID,
		formatOutputTime(s.CreateTime),
		formatOutputTime(s.LastTime),
		s.File,
		s.UserMsgAmount,
		formatTitle(s),
	)
}

func formatTitle(s Session) string {
	for _, candidate := range []string{s.Title, s.FirstMsg} {
		if title := cleanMessage(candidate, 80); title != "" {
			return title
		}
	}
	return "未命名"
}

func cleanMessage(msg string, maxLength int) string {
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
	if len(runes) > maxLength {
		return fmt.Sprintf("%s...[%d]", string(runes[:maxLength]), len(runes))
	}
	return cleaned
}

func formatOutputTime(t time.Time) string {
	return t.Local().Format(outputTimeFormat)
}

func shouldReplaceWithSpace(r rune) bool {
	return unicode.IsSpace(r) || unicode.IsControl(r) || r == '"' || r == '\\'
}
