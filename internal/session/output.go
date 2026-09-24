package session

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
	"unicode"
)

const outputTimeFormat = "20060102_15:04:05"

func FormatTable(w io.Writer, sessions []Session, showDirectory bool) error {
	table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	header := "SessionId\tTitle\tMsgAmount\tCreateTime\tLastTime"
	if showDirectory {
		header += "\tDirectory"
	}
	if _, err := fmt.Fprintln(table, header); err != nil {
		return err
	}
	for _, s := range sessions {
		id := strconv.Quote(s.SessionID)
		line := fmt.Sprintf("%s\t%s\t%d\t%s\t%s",
			id[1:len(id)-1], formatTitle(s), s.UserMsgAmount,
			formatOutputTime(s.CreateTime), formatOutputTime(s.LastTime))
		if showDirectory {
			dir := strconv.Quote(s.Dir)
			line += "\t" + dir[1:len(dir)-1]
		}
		if _, err := fmt.Fprintln(table, line); err != nil {
			return err
		}
	}
	return table.Flush()
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
