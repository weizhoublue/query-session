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
		decodedDir := DecodeProjectDir(projectName, fsRoot)
		logInfo(log, "scan project encoded=%s dir=%s path=%s", projectName, decodedDir, projectDir)

		fileEntries, err := os.ReadDir(projectDir)
		if err != nil {
			return nil, err
		}

		for _, fileEntry := range fileEntries {
			if fileEntry.IsDir() || filepath.Ext(fileEntry.Name()) != ".jsonl" {
				continue
			}

			filePath := filepath.Join(projectDir, fileEntry.Name())
			sessionID := strings.TrimSuffix(fileEntry.Name(), ".jsonl")
			logInfo(log, "scan file sessionId=%s file=%s dir=%s", sessionID, filePath, decodedDir)
			s, ok, err := scanFile(filePath, decodedDir, log)
			if err != nil {
				return nil, err
			}
			if ok {
				s.SessionID = sessionID
				s.File = filePath
				logInfo(log, "parsed sessionId=%s dir=%s createTime=%s lastTime=%s", s.SessionID, s.Dir, s.CreateTime.Format("20060102_15:04:05"), s.LastTime.Format("20060102_15:04:05"))
				sessions = append(sessions, s)
			} else {
				logInfo(log, "skip file sessionId=%s reason=no-user-message file=%s", sessionID, filePath)
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
		info, err := os.Stat(candidate)
		if err != nil || !info.IsDir() {
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
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		entry, ok := parseLine(scanner.Bytes(), path, lineNum, log)
		if !ok || entry.Message.Role != "user" {
			continue
		}

		timestamp, err := time.Parse(time.RFC3339Nano, entry.Timestamp)
		if err != nil {
			logParseError(log, "invalid timestamp in %s:%d: %v", path, lineNum, err)
			continue
		}

		msg := messageText(entry.Message.Content)
		if msg == "" {
			continue
		}
		if result.CreateTime.IsZero() {
			result.CreateTime = timestamp
			result.FirstMsg = msg
		}
		result.LastTime = timestamp
		result.LastMsg = msg
	}
	if err := scanner.Err(); err != nil {
		logParseError(log, "failed to scan %s: %v", path, err)
		return session.Session{}, false, nil
	}

	return result, !result.CreateTime.IsZero(), nil
}

func parseLine(line []byte, path string, lineNum int, log Logger) (jsonlEntry, bool) {
	var entry jsonlEntry
	if err := json.Unmarshal(line, &entry); err != nil {
		logParseError(log, "invalid JSON in %s:%d: %v", path, lineNum, err)
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
	rawStart := pos
	hidden := false
	if strings.HasPrefix(encoded[pos:], "--") {
		hidden = true
		pos += 2
	} else if encoded[pos] == '-' {
		pos++
		rawStart = pos
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
	return segment, end, encoded[rawStart:]
}

func logParseError(log Logger, format string, args ...any) {
	if log != nil {
		log("error", fmt.Sprintf(format, args...))
	}
}

func logInfo(log Logger, format string, args ...any) {
	if log != nil {
		log("info", fmt.Sprintf(format, args...))
	}
}
