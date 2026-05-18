package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"query-session/internal/session"
)

type Logger func(level, message string)

type jsonlEntry struct {
	Timestamp string `json:"timestamp"`
	Message   struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

func Scan(projectsRoot, fsRoot string, log Logger) ([]session.Session, error) {
	projectEntries, err := os.ReadDir(projectsRoot)
	if err != nil {
		return nil, err
	}

	var sessions []session.Session
	for _, projectEntry := range projectEntries {
		if !projectEntry.IsDir() {
			continue
		}

		projectName := projectEntry.Name()
		projectDir := filepath.Join(projectsRoot, projectName)
		fileEntries, err := os.ReadDir(projectDir)
		if err != nil {
			return nil, err
		}

		decodedDir := DecodeProjectDir(projectName, fsRoot)
		for _, fileEntry := range fileEntries {
			if fileEntry.IsDir() || filepath.Ext(fileEntry.Name()) != ".jsonl" {
				continue
			}

			s, ok, err := scanFile(filepath.Join(projectDir, fileEntry.Name()), decodedDir, log)
			if err != nil {
				return nil, err
			}
			if ok {
				s.SessionID = strings.TrimSuffix(fileEntry.Name(), ".jsonl")
				sessions = append(sessions, s)
			}
		}
	}
	return sessions, nil
}

func DecodeProjectDir(encoded, fsRoot string) string {
	current := filepath.Clean(fsRoot)
	pos := 0
	for pos < len(encoded) {
		segment, nextPos, rawRemainder := nextEncodedSegment(encoded, pos)
		if segment == "" {
			break
		}

		candidate := filepath.Join(current, segment)
		if _, err := os.Stat(candidate); err != nil {
			return filepath.Join(current, rawRemainder)
		}
		current = candidate
		pos = nextPos
	}
	return current
}

func scanFile(path, dir string, log Logger) (session.Session, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return session.Session{}, false, err
	}
	defer f.Close()

	var result session.Session
	result.Dir = dir
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		entry, ok := parseLine(scanner.Bytes(), path, lineNum, log)
		if !ok || entry.Message.Role != "user" {
			continue
		}

		timestamp, err := time.Parse(time.RFC3339Nano, entry.Timestamp)
		if err != nil {
			debug(log, "invalid timestamp in %s:%d: %v", path, lineNum, err)
			continue
		}

		msg := messageText(entry.Message.Content)
		if result.CreateTime.IsZero() {
			result.CreateTime = timestamp
			result.FirstMsg = msg
		}
		result.LastTime = timestamp
		result.LastMsg = msg
	}
	if err := scanner.Err(); err != nil {
		return session.Session{}, false, err
	}

	return result, !result.CreateTime.IsZero(), nil
}

func parseLine(line []byte, path string, lineNum int, log Logger) (jsonlEntry, bool) {
	var entry jsonlEntry
	if err := json.Unmarshal(line, &entry); err != nil {
		debug(log, "invalid JSON in %s:%d: %v", path, lineNum, err)
		return jsonlEntry{}, false
	}
	return entry, true
}

func messageText(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return ""
	}

	var b strings.Builder
	for _, part := range parts {
		if part.Text == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(part.Text)
	}
	return b.String()
}

func nextEncodedSegment(encoded string, pos int) (string, int, string) {
	hidden := false
	if strings.HasPrefix(encoded[pos:], "--") {
		hidden = true
		pos += 2
	} else if encoded[pos] == '-' {
		pos++
	}

	end := strings.IndexByte(encoded[pos:], '-')
	if end == -1 {
		end = len(encoded)
	} else {
		end += pos
	}

	segment := encoded[pos:end]
	if hidden {
		segment = "." + segment
	}
	return segment, end, encoded[pos:]
}

func debug(log Logger, format string, args ...any) {
	if log != nil {
		log("debug", fmt.Sprintf(format, args...))
	}
}
