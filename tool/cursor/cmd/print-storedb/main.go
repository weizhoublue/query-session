// print-storedb 解读 Cursor Agent 的 store.db（SQLite），并输出提炼后的会话摘要。
//
// 用法:
//
//	go run ./cmd/print-storedb /path/to/store.db
//
// # 文件与目录
//
//   - 有效 DB 在 Session 目录: ~/.cursor/chats/{chatId}/{sessionId}/store.db
//   - Chat 根目录下的 store.db 常为空；chatId 只能从路径推断，不在 meta 里
//   - latestRootBlobId 是对话树根 blob 的 id（内容哈希），不是磁盘目录
//
// # 表结构
//
//   - meta: key='0' 的 value 为 hex 编码 JSON，含 agentId/name/createdAt/latestRootBlobId 等
//   - blobs: id 为内容哈希，data 为 JSON 消息或 Protobuf 索引/快照
//
// # 最终解读日志（RESOLVED）提炼规则
//
//  1. sessionId
//     meta.agentId，与目录名 {sessionId} 一致。
//
//  2. createdAt
//     meta.createdAt 为毫秒时间戳，输出时转为本地时间（time.Local）。
//
//  3. workspace（用户项目目录）
//     优先: latestRootBlobId 对应 blob 的 Protobuf field 9（file:// URI）→ 本地路径。
//     回退: blobs 中带 <user_info> 的 user 消息里的 "Workspace Path: ..."。
//     注意: 这是打开 Agent 时的工作区，不是 ~/.cursor/chats/... 沙箱路径。
//
//  4. user 消息统计（blobs 中 JSON 且 role=user）
//     - userMessageCount: 所有 role=user 条数。
//     - contextInjectionCount: 启动时注入的上下文（providerOptions.cursor.requestContextCompleteness 非空），
//       典型内容为 <user_info>、rules、skills 等，不算用户真实输入。
//     - userQueryCount: 真实用户输入 = 非注入 且 content 含 <user_query>；
//       content 可为 string 或 [{type,text}] 数组。
//
//  5. chatId
//     从 store.db 路径 .../chats/{chatId}/{sessionId}/store.db 解析；DB 内无此字段。
//
// # Subagent 说明
//
//   Subagent 通常无独立 store.db；Task 结果写在父 Session 的 blobs 里。
//   对话文本另存于 ~/.cursor/projects/.../agent-transcripts/{sessionId}/subagents/*.jsonl
package main

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

	_ "modernc.org/sqlite"
)

type storeMeta struct {
	AgentID          string `json:"agentId"`
	CreatedAt        int64  `json:"createdAt"`
	LatestRootBlobID string `json:"latestRootBlobId"`
	LastUsedModel    string `json:"lastUsedModel"`
	Name             string `json:"name"`
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <store.db>\n", os.Args[0])
		os.Exit(1)
	}

	db, err := sql.Open("sqlite", os.Args[1])
	if err != nil {
		fatal("open db: %v", err)
	}
	defer db.Close()

	tables, err := listTables(db)
	if err != nil {
		fatal("list tables: %v", err)
	}
	if len(tables) == 0 {
		fmt.Println("(no user tables)")
		return
	}

	for _, table := range tables {
		if err := dumpTable(db, table); err != nil {
			fatal("dump table %q: %v", table, err)
		}
	}

	if err := printWorkspaceFromRootBlob(db, os.Args[1]); err != nil {
		fatal("resolve workspace: %v", err)
	}
}

func printWorkspaceFromRootBlob(db *sql.DB, storeDBPath string) error {
	meta, err := loadStoreMeta(db)
	if err != nil {
		return err
	}

	fmt.Printf("\n========== RESOLVED FROM latestRootBlobId ==========\n")
	fmt.Printf("store.db: %s\n", storeDBPath)
	fmt.Printf("sessionId (agentId): %s\n", meta.AgentID)
	if meta.Name != "" {
		fmt.Printf("name: %s\n", meta.Name)
	}
	if meta.CreatedAt > 0 {
		created := time.UnixMilli(meta.CreatedAt).In(time.Local)
		fmt.Printf("createdAt: %d (%s)\n", meta.CreatedAt, created.Format("2006-01-02 15:04:05 MST"))
	}
	if meta.LastUsedModel != "" {
		fmt.Printf("lastUsedModel: %s\n", meta.LastUsedModel)
	}
	userStats, err := countUserMessages(db)
	if err != nil {
		return fmt.Errorf("count user messages: %w", err)
	}
	fmt.Printf("userMessageCount (role=user, all): %d\n", userStats.Total)
	fmt.Printf("contextInjectionCount (requestContextCompleteness): %d\n", userStats.ContextInjection)
	fmt.Printf("userQueryCount (real user input, excl injection): %d\n", userStats.UserQuery)
	fmt.Printf("latestRootBlobId: %s\n", meta.LatestRootBlobID)

	if meta.LatestRootBlobID == "" {
		fmt.Println("workspace: <not found: latestRootBlobId is empty>")
		return nil
	}

	var rootData []byte
	err = db.QueryRow(`SELECT data FROM blobs WHERE id = ?`, meta.LatestRootBlobID).Scan(scanBytes(&rootData))
	if err == sql.ErrNoRows {
		fmt.Println("workspace: <not found: root blob missing>")
		return nil
	}
	if err != nil {
		return fmt.Errorf("query root blob: %w", err)
	}

	workspace := extractWorkspaceDir(rootData)
	source := "root blob (protobuf field 9 file:// URI)"
	if workspace == "" {
		workspace, err = findWorkspaceInUserBlobs(db)
		if err != nil {
			return err
		}
		source = "user blob (Workspace Path in <user_info>)"
	}

	if workspace == "" {
		fmt.Println("workspace: <not found>")
		return nil
	}

	fmt.Printf("workspace source: %s\n", source)
	fmt.Printf("workspace: %s\n", workspace)

	// Chat ID is not stored in store.db; derive from filesystem layout when possible.
	if chatID := chatIDFromStorePath(storeDBPath); chatID != "" {
		fmt.Printf("chatId (from path): %s\n", chatID)
	}
	return nil
}

type userMessageStats struct {
	Total            int
	ContextInjection int
	UserQuery        int
}

type blobMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
	ProviderOptions     struct {
		Cursor struct {
			RequestContextCompleteness json.RawMessage `json:"requestContextCompleteness"`
			RequestID                  string          `json:"requestId"`
		} `json:"cursor"`
	} `json:"providerOptions"`
}

func countUserMessages(db *sql.DB) (userMessageStats, error) {
	rows, err := db.Query(`SELECT data FROM blobs`)
	if err != nil {
		return userMessageStats{}, err
	}
	defer rows.Close()

	var stats userMessageStats
	for rows.Next() {
		var data []byte
		if err := rows.Scan(scanBytes(&data)); err != nil {
			return userMessageStats{}, err
		}
		if !json.Valid(data) {
			continue
		}
		var msg blobMessage
		if err := json.Unmarshal(data, &msg); err != nil || msg.Role != "user" {
			continue
		}

		stats.Total++
		if hasRequestContextCompleteness(msg) {
			stats.ContextInjection++
			continue
		}
		if messageHasUserQuery(msg.Content) {
			stats.UserQuery++
		}
	}
	return stats, rows.Err()
}

func hasRequestContextCompleteness(msg blobMessage) bool {
	rcc := msg.ProviderOptions.Cursor.RequestContextCompleteness
	return len(rcc) > 0 && string(rcc) != "null"
}

func messageHasUserQuery(content json.RawMessage) bool {
	if len(content) == 0 {
		return false
	}

	// content may be a string or an array of {type,text} parts.
	var text string
	if err := json.Unmarshal(content, &text); err == nil {
		return strings.Contains(text, "<user_query>")
	}

	var parts []struct {
		Text string `json:"text"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(content, &parts); err == nil {
		for _, p := range parts {
			if strings.Contains(p.Text, "<user_query>") {
				return true
			}
		}
	}
	return strings.Contains(string(content), "<user_query>")
}

func loadStoreMeta(db *sql.DB) (storeMeta, error) {
	var raw []byte
	err := db.QueryRow(`SELECT value FROM meta WHERE key = '0'`).Scan(scanBytes(&raw))
	if err != nil {
		return storeMeta{}, fmt.Errorf("read meta: %w", err)
	}

	decoded := raw
	if looksLikeHexASCII(raw) {
		var err error
		decoded, err = hex.DecodeString(string(raw))
		if err != nil {
			return storeMeta{}, fmt.Errorf("decode meta hex: %w", err)
		}
	}

	var meta storeMeta
	if err := json.Unmarshal(decoded, &meta); err != nil {
		return storeMeta{}, fmt.Errorf("parse meta json: %w", err)
	}
	return meta, nil
}

func scanBytes(dst *[]byte) *[]byte {
	return dst
}

// extractWorkspaceDir reads Cursor root-blob protobuf and returns field 9 file:// URI as a local path.
func extractWorkspaceDir(root []byte) string {
	if uri := protobufFieldString(root, 9); uri != "" {
		return fileURIToPath(uri)
	}
	// Fallback: any file:// string embedded in protobuf.
	for _, uri := range protobufAllStrings(root) {
		if strings.HasPrefix(uri, "file://") {
			if path := fileURIToPath(uri); path != "" {
				return path
			}
		}
	}
	return ""
}

func fileURIToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return ""
	}
	if u.Scheme != "file" {
		return ""
	}
	return filepath.Clean(u.Path)
}

func protobufFieldString(data []byte, wantField int) string {
	for _, field := range protobufFields(data) {
		if field.number == wantField && field.wireType == 2 {
			if utf8.Valid(field.value) && isMostlyPrintable(field.value) {
				return string(field.value)
			}
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

type protoField struct {
	number   int
	wireType int
	value    []byte
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

var workspacePathRE = regexp.MustCompile(`Workspace Path:\s*(.+)`)

func findWorkspaceInUserBlobs(db *sql.DB) (string, error) {
	rows, err := db.Query(`SELECT data FROM blobs WHERE length(data) > 100 ORDER BY length(data) DESC`)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	for rows.Next() {
		var data []byte
		if err := rows.Scan(scanBytes(&data)); err != nil {
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

func chatIDFromStorePath(storeDBPath string) string {
	// .../chats/{chatId}/{sessionId}/store.db
	sessionDir := filepath.Dir(storeDBPath)
	chatDir := filepath.Dir(sessionDir)
	parent := filepath.Base(filepath.Dir(chatDir))
	if parent != "chats" {
		return ""
	}
	return filepath.Base(chatDir)
}

func listTables(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`
		SELECT name
		FROM sqlite_master
		WHERE type = 'table'
		  AND name NOT LIKE 'sqlite_%'
		ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}

func dumpTable(db *sql.DB, table string) error {
	rows, err := db.Query(fmt.Sprintf(`SELECT * FROM %q`, table))
	if err != nil {
		return err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return err
	}

	fmt.Printf("\n========== TABLE: %s ==========\n", table)

	rowNum := 0
	for rows.Next() {
		rowNum++
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return err
		}

		fmt.Printf("--- row %d ---\n", rowNum)
		for i, col := range cols {
			fmt.Printf("%s: %s\n", col, formatValue(col, values[i]))
		}
	}
	return rows.Err()
}

func formatValue(col string, v any) string {
	v = unwrapValue(v)
	switch x := v.(type) {
	case nil:
		return "<NULL>"
	case []byte:
		return formatBytes(col, x)
	case string:
		return formatBytes(col, []byte(x))
	case int64:
		return fmt.Sprintf("%d", x)
	case float64:
		return fmt.Sprintf("%v", x)
	case bool:
		return fmt.Sprintf("%t", x)
	default:
		return fmt.Sprintf("%v (%T)", v, v)
	}
}

func unwrapValue(v any) any {
	for {
		if v == nil {
			return nil
		}
		if p, ok := v.(*any); ok {
			v = *p
			continue
		}
		return v
	}
}

func formatBytes(col string, b []byte) string {
	if len(b) == 0 {
		return `""`
	}

	// meta.value in Cursor store.db is hex-encoded JSON.
	if col == "value" && looksLikeHexASCII(b) {
		if decoded, err := hex.DecodeString(string(b)); err == nil {
			if pretty := tryPrettyJSON(decoded); pretty != "" {
				return "hex-decoded JSON:\n" + indent(pretty)
			}
			if utf8.Valid(decoded) {
				return "hex-decoded text:\n" + indent(string(decoded))
			}
			return "hex-decoded bytes:\n" + indent(hex.EncodeToString(decoded))
		}
	}

	if pretty := tryPrettyJSON(b); pretty != "" {
		return "json:\n" + indent(pretty)
	}
	if utf8.Valid(b) && isMostlyPrintable(b) {
		return "text:\n" + indent(string(b))
	}

	return fmt.Sprintf("bytes(len=%d) hex:\n%s", len(b), indent(hex.EncodeToString(b)))
}

func looksLikeHexASCII(b []byte) bool {
	if len(b) == 0 || len(b)%2 != 0 {
		return false
	}
	for _, c := range b {
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

func tryPrettyJSON(b []byte) string {
	if !json.Valid(b) {
		return ""
	}
	var tmp any
	if err := json.Unmarshal(b, &tmp); err != nil {
		return ""
	}
	out, err := json.MarshalIndent(tmp, "", "  ")
	if err != nil {
		return ""
	}
	return string(out)
}

func isMostlyPrintable(b []byte) bool {
	printable := 0
	for _, c := range b {
		if c == '\n' || c == '\r' || c == '\t' || (c >= 32 && c < 127) {
			printable++
		}
	}
	return float64(printable)/float64(len(b)) > 0.85
}

func indent(s string) string {
	if s == "" {
		return ""
	}
	return "  " + strings.ReplaceAll(s, "\n", "\n  ")
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
