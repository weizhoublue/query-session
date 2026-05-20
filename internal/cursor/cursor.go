package cursor

import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"query-session/internal/session"

	_ "modernc.org/sqlite"
)

type Logger func(level, message string)

type storeMeta struct {
	AgentID          string `json:"agentId"`
	CreatedAt        int64  `json:"createdAt"`
	LatestRootBlobID string `json:"latestRootBlobId"`
}

type blobMessage struct {
	Role            string          `json:"role"`
	Content         json.RawMessage `json:"content"`
	ProviderOptions struct {
		Cursor struct {
			RequestContextCompleteness json.RawMessage `json:"requestContextCompleteness"`
		} `json:"cursor"`
	} `json:"providerOptions"`
}

var (
	userQueryRE     = regexp.MustCompile(`(?s)<user_query>\s*(.*?)\s*</user_query>`)
	workspacePathRE = regexp.MustCompile(`Workspace Path:\s*(.+)`)
)

func Scan(chatsRoot string, log Logger) ([]session.Session, error) {
	pattern := filepath.Join(chatsRoot, "*", "*", "store.db")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	var out []session.Session
	for _, path := range paths {
		logInfo(log, "scan cursor store path=%s", path)
		s, ok, err := parseStoreDB(path, log)
		if err != nil {
			return nil, err
		}
		if ok {
			logInfo(log, "parsed sessionId=%s dir=%s createTime=%s", s.SessionID, s.Dir, s.CreateTime.Format("20060102_15:04:05"))
			out = append(out, s)
		} else {
			logInfo(log, "skip cursor store path=%s reason=no-user-query", path)
		}
	}
	return out, nil
}

func parseStoreDB(path string, log Logger) (session.Session, bool, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return session.Session{}, false, err
	}
	defer db.Close()

	meta, err := loadStoreMeta(db)
	if err != nil {
		logInfo(log, "skip cursor store path=%s reason=invalid-meta err=%v", path, err)
		return session.Session{}, false, nil
	}

	sessionID := meta.AgentID
	if sessionID == "" {
		sessionID = filepath.Base(filepath.Dir(path))
	}

	dir, err := resolveWorkspace(db, meta)
	if err != nil {
		return session.Session{}, false, err
	}

	first, last, count, err := collectUserQueries(db)
	if err != nil {
		return session.Session{}, false, err
	}
	if count == 0 {
		return session.Session{}, false, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return session.Session{}, false, err
	}

	var s session.Session
	s.SessionID = sessionID
	s.Dir = dir
	s.File = path
	s.FirstMsg = first
	s.LastMsg = last
	s.UserMsgAmount = count
	if meta.CreatedAt > 0 {
		s.CreateTime = time.UnixMilli(meta.CreatedAt).Local()
	}
	s.LastTime = info.ModTime().Local()
	return s, true, nil
}

func loadStoreMeta(db *sql.DB) (storeMeta, error) {
	var raw []byte
	err := db.QueryRow(`SELECT value FROM meta WHERE key = '0'`).Scan(&raw)
	if err != nil {
		return storeMeta{}, err
	}
	decoded := raw
	if looksLikeHexASCII(raw) {
		decoded, err = hex.DecodeString(string(raw))
		if err != nil {
			return storeMeta{}, err
		}
	}
	var meta storeMeta
	if err := json.Unmarshal(decoded, &meta); err != nil {
		return storeMeta{}, err
	}
	return meta, nil
}

func collectUserQueries(db *sql.DB) (first, last string, count int, err error) {
	rows, err := db.Query(`SELECT data FROM blobs ORDER BY rowid`)
	if err != nil {
		return "", "", 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return "", "", 0, err
		}
		if !isValidUserQueryMessage(data) {
			continue
		}
		text := extractUserQueryText(data)
		if text == "" {
			continue
		}
		count++
		if first == "" {
			first = text
		}
		last = text
	}
	return first, last, count, rows.Err()
}

func isValidUserQueryMessage(data []byte) bool {
	if !json.Valid(data) {
		return false
	}
	var msg blobMessage
	if err := json.Unmarshal(data, &msg); err != nil || msg.Role != "user" {
		return false
	}
	rcc := msg.ProviderOptions.Cursor.RequestContextCompleteness
	if len(rcc) > 0 && string(rcc) != "null" {
		return false
	}
	return messageContainsUserQuery(msg.Content)
}

func messageContainsUserQuery(content json.RawMessage) bool {
	return strings.Contains(string(content), "<user_query>")
}

func extractUserQueryText(data []byte) string {
	var msg blobMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return ""
	}
	text := contentText(msg.Content)
	if m := userQueryRE.FindStringSubmatch(text); len(m) == 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func contentText(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var parts []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		var b strings.Builder
		for _, p := range parts {
			b.WriteString(p.Text)
		}
		return b.String()
	}
	return string(raw)
}

func resolveWorkspace(db *sql.DB, meta storeMeta) (string, error) {
	if meta.LatestRootBlobID != "" {
		var root []byte
		err := db.QueryRow(`SELECT data FROM blobs WHERE id = ?`, meta.LatestRootBlobID).Scan(&root)
		if err == nil {
			if ws := extractWorkspaceFromProtobuf(root); ws != "" {
				return ws, nil
			}
		}
	}
	return findWorkspaceInUserBlobs(db)
}

func findWorkspaceInUserBlobs(db *sql.DB) (string, error) {
	rows, err := db.Query(`SELECT data FROM blobs WHERE length(data) > 50 ORDER BY length(data) DESC`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return "", err
		}
		if !json.Valid(data) {
			continue
		}
		var msg struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(data, &msg); err != nil || msg.Role != "user" {
			continue
		}
		if m := workspacePathRE.FindStringSubmatch(msg.Content); len(m) == 2 {
			return strings.TrimSpace(m[1]), nil
		}
	}
	return "", rows.Err()
}

func extractWorkspaceFromProtobuf(root []byte) string {
	if uri := protobufFieldString(root, 9); uri != "" {
		return fileURIToPath(uri)
	}
	for _, s := range protobufAllStrings(root) {
		if strings.HasPrefix(s, "file://") {
			if p := fileURIToPath(s); p != "" {
				return p
			}
		}
	}
	return ""
}

func fileURIToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return ""
	}
	return filepath.Clean(u.Path)
}

type protoField struct {
	number   int
	wireType int
	value    []byte
}

func protobufFieldString(data []byte, wantField int) string {
	for _, field := range protobufFields(data) {
		if field.number == wantField && field.wireType == 2 && utf8.Valid(field.value) && isMostlyPrintable(field.value) {
			return string(field.value)
		}
	}
	return ""
}

func protobufAllStrings(data []byte) []string {
	var out []string
	for _, field := range protobufFields(data) {
		if field.wireType == 2 && utf8.Valid(field.value) && isMostlyPrintable(field.value) {
			out = append(out, string(field.value))
		}
	}
	return out
}

func protobufFields(data []byte) []protoField {
	var fields []protoField
	i := 0
	for i < len(data) {
		key, n, ok := readVarint(data, i)
		if !ok {
			break
		}
		i += n
		number := int(key >> 3)
		wireType := int(key & 7)

		switch wireType {
		case 0:
			_, n, ok = readVarint(data, i)
			if !ok {
				return fields
			}
			i += n
		case 2:
			length, n, ok := readVarint(data, i)
			if !ok {
				return fields
			}
			i += n
			end := i + int(length)
			if end > len(data) {
				return fields
			}
			fields = append(fields, protoField{
				number:   number,
				wireType: wireType,
				value:    data[i:end],
			})
			i = end
		default:
			return fields
		}
	}
	return fields
}

func readVarint(data []byte, offset int) (uint64, int, bool) {
	var x uint64
	var s uint
	for i := offset; i < len(data); i++ {
		b := data[i]
		if b < 0x80 {
			return x | uint64(b)<<s, i - offset + 1, true
		}
		x |= uint64(b&0x7f) << s
		s += 7
	}
	return 0, 0, false
}

func isMostlyPrintable(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	printable := 0
	for _, c := range b {
		if c == '\n' || c == '\r' || c == '\t' || (c >= 32 && c < 127) {
			printable++
		}
	}
	return float64(printable)/float64(len(b)) > 0.85
}

func looksLikeHexASCII(b []byte) bool {
	if len(b) == 0 || len(b)%2 != 0 {
		return false
	}
	for _, c := range b {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

func logInfo(log Logger, format string, args ...any) {
	if log != nil {
		log("info", fmt.Sprintf(format, args...))
	}
}
