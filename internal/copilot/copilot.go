package copilot

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"query-session/internal/session"

	"gopkg.in/yaml.v3"
)

const maxLineBytes = 64 << 20

type Logger func(level, message string)

type event struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Data      struct {
		SessionID string          `json:"sessionId"`
		Content   json.RawMessage `json:"content"`
		Context   struct {
			CWD string `json:"cwd"`
		} `json:"context"`
	} `json:"data"`
}

func Scan(root string, matcher *session.DirMatcher, log Logger) ([]session.Session, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []session.Session
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name(), "events.jsonl")
		s, ok, err := scanFile(path, entry.Name(), matcher, log)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, s)
		}
	}
	return out, nil
}

func scanFile(path, id string, matcher *session.DirMatcher, log Logger) (session.Session, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return session.Session{}, false, err
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	var first event
	lineNo := 0
	for {
		line, err := readLine(reader, maxLineBytes)
		if err != nil {
			return session.Session{}, false, fmt.Errorf("%s: %w", path, err)
		}
		lineNo++
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		if err := json.Unmarshal(line, &first); err != nil {
			return session.Session{}, false, fmt.Errorf("%s:%d: invalid session.start: %w", path, lineNo, err)
		}
		break
	}
	if first.Type != "session.start" || first.Data.SessionID != id || !filepath.IsAbs(first.Data.Context.CWD) {
		return session.Session{}, false, fmt.Errorf("%s:%d: invalid session.start id or cwd", path, lineNo)
	}
	start, err := time.Parse(time.RFC3339Nano, first.Timestamp)
	if err != nil {
		return session.Session{}, false, fmt.Errorf("%s:%d: invalid session.start timestamp: %w", path, lineNo, err)
	}
	if matcher != nil && !matcher.Match(first.Data.Context.CWD) {
		return session.Session{}, false, nil
	}

	s := session.Session{
		SessionID:  id,
		Dir:        first.Data.Context.CWD,
		File:       path,
		CreateTime: start,
		LastTime:   start,
	}
	for {
		line, err := readLine(reader, maxLineBytes)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return session.Session{}, false, fmt.Errorf("%s:%d: %w", path, lineNo+1, err)
		}
		lineNo++
		if !bytes.Contains(line, []byte("user.message")) {
			continue
		}
		var e event
		if err := json.Unmarshal(line, &e); err != nil {
			logError(log, "%s:%d: invalid event: %v", path, lineNo, err)
			continue
		}
		if e.Type != "user.message" {
			continue
		}
		var text string
		if err := json.Unmarshal(e.Data.Content, &text); err != nil || strings.TrimSpace(text) == "" {
			continue
		}
		ts, err := time.Parse(time.RFC3339Nano, e.Timestamp)
		if err != nil {
			logError(log, "%s:%d: invalid user timestamp: %v", path, lineNo, err)
			continue
		}
		s.UserMsgAmount++
		if s.UserMsgAmount == 1 {
			s.CreateTime = ts
			s.FirstMsg = strings.TrimSpace(text)
		}
		s.LastTime = ts
	}

	metaPath := filepath.Join(filepath.Dir(path), "workspace.yaml")
	meta, err := os.ReadFile(metaPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return session.Session{}, false, err
	}
	if err == nil {
		var workspace struct {
			Name string `yaml:"name"`
		}
		if err := yaml.Unmarshal(meta, &workspace); err != nil {
			return session.Session{}, false, fmt.Errorf("%s: invalid workspace metadata: %w", metaPath, err)
		}
		s.Title = workspace.Name
	}
	return s, true, nil
}

func readLine(reader *bufio.Reader, limit int) ([]byte, error) {
	var line []byte
	for {
		part, err := reader.ReadSlice('\n')
		if len(line)+len(part) > limit {
			return nil, fmt.Errorf("JSONL line exceeds %d bytes", limit)
		}
		line = append(line, part...)
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if errors.Is(err, io.EOF) && len(line) > 0 {
			return line, nil
		}
		return line, err
	}
}

func logError(log Logger, format string, args ...any) {
	if log != nil {
		log("error", fmt.Sprintf(format, args...))
	}
}
