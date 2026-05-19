package codex

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"query-session/internal/session"
)

type Logger func(level, message string)

type lineRecord struct {
	Timestamp string `json:"timestamp"`
	Payload   struct {
		ID      string          `json:"id"`
		CWD     string          `json:"cwd"`
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
		Source  json.RawMessage `json:"source"`
	} `json:"payload"`
}

func Scan(root string, start, end time.Time, log Logger) ([]session.Session, error) {
	var sessions []session.Session
	for day := dayStart(start); !day.After(end); day = day.AddDate(0, 0, 1) {
		dayDir := filepath.Join(root, day.Format("2006"), day.Format("01"), day.Format("02"))
		if log != nil {
			log("info", "scan codex day "+dayDir)
		}
		files, err := filepath.Glob(filepath.Join(dayDir, "*.jsonl"))
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			parsed, ok := parseFile(file, log)
			if !ok {
				if log != nil {
					log("info", "skip codex file "+file)
				}
				continue
			}
			if log != nil {
				log("info", "parsed codex session "+parsed.SessionID)
			}
			sessions = append(sessions, parsed)
		}
	}
	return sessions, nil
}

func dayStart(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func parseFile(path string, log Logger) (session.Session, bool) {
	file, err := os.Open(path)
	if err != nil {
		if log != nil {
			log("error", "open codex file failed "+path+": "+err.Error())
		}
		return session.Session{}, false
	}
	defer file.Close()

	var out session.Session
	out.File = path

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		var record lineRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			if log != nil {
				log("error", "invalid codex jsonl line "+path+": "+err.Error())
			}
			continue
		}
		if isSubAgentSession(record.Payload.Source) {
			if log != nil {
				log("info", "skip codex sub-agent session "+path)
			}
			return session.Session{}, false
		}
		if out.SessionID == "" && record.Payload.ID != "" {
			out.SessionID = record.Payload.ID
		}
		if out.Dir == "" && record.Payload.CWD != "" {
			out.Dir = record.Payload.CWD
		}
		if record.Payload.Role != "user" {
			continue
		}
		ts, err := time.Parse(time.RFC3339Nano, record.Timestamp)
		if err != nil {
			if log != nil {
				log("error", "invalid codex timestamp "+path+": "+err.Error())
			}
			continue
		}
		msg := extractUserContent(record.Payload.Content)
		if msg == "" {
			continue
		}
		out.UserMsgAmount++
		if out.FirstMsg == "" {
			out.CreateTime = ts
			out.FirstMsg = msg
		}
		out.LastTime = ts
		out.LastMsg = msg
	}
	if err := scanner.Err(); err != nil {
		if log != nil {
			log("error", "scanner error "+path+": "+err.Error())
		}
		return session.Session{}, false
	}

	if out.SessionID == "" {
		out.SessionID = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	}
	if out.SessionID == "" || out.FirstMsg == "" {
		return session.Session{}, false
	}
	return out, true
}

func isSubAgentSession(source json.RawMessage) bool {
	if len(source) == 0 {
		return false
	}
	var s struct {
		Subagent struct {
			ThreadSpawn struct {
				ParentThreadID string `json:"parent_thread_id"`
			} `json:"thread_spawn"`
		} `json:"subagent"`
	}
	if err := json.Unmarshal(source, &s); err != nil {
		return false
	}
	return s.Subagent.ThreadSpawn.ParentThreadID != ""
}

func extractUserContent(raw json.RawMessage) string {
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return ""
	}
	if len(parts) != 1 {
		return ""
	}
	p := parts[0]
	if p.Type == "input_text" && strings.TrimSpace(p.Text) != "" {
		return strings.TrimSpace(p.Text)
	}
	return ""
}
