# Cursor Provider Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现 Cursor session provider，使 `query-session -t cursor` 能从 `$HOME/.cursor/chats/*/*/store.db` 扫描并输出与 Claude 相同格式的会话行。

**Architecture:** 新增独立包 `internal/cursor/`（不依赖将被删除的 `cursor/` 目录）。`Scan(chatsRoot, log)` 用 `filepath.Glob` 发现 `store.db`，SQLite 读 meta/blobs，自实现 Protobuf field 9 解析 workspace。CLI 增加 `ProviderCursor` 分支。复用 `internal/session` 过滤与输出。

**Tech Stack:** Go 1.26+，`modernc.org/sqlite`，`encoding/json` / `encoding/hex`，`testing`。

**Spec:** `docs/superpowers/specs/2026-05-20-cursor-provider-design.md`

---

## 文件结构

| 文件 | 职责 |
|------|------|
| `internal/session/session.go` | 增加 `ProviderCursor` |
| `internal/cursor/cursor.go` | Scan、parseStoreDB、meta/blobs/user-query/protobuf |
| `internal/cursor/cursor_test.go` | 合成 `store.db` fixture + 单元测试 |
| `cmd/query-session/main.go` | `-t cursor` 分支、help 文案 |
| `cmd/query-session/main_test.go` | help 含 `cursor` |
| `go.mod` / `go.sum` | `modernc.org/sqlite` |
| `docs/design.md` | Cursor Provider 章节 |
| `docs/get-started.md` | 使用示例 |
| `CLAUDE.md` | 架构一行 |

---

### Task 1: 增加 Provider 常量

**Files:**
- Modify: `internal/session/session.go`

- [ ] **Step 1: 添加常量**

在 `ProviderCodex` 后增加：

```go
const (
	ProviderClaude Provider = "claude"
	ProviderCodex  Provider = "codex"
	ProviderCursor Provider = "cursor"
)
```

- [ ] **Step 2: 验证编译**

Run: `go build ./...`  
Expected: success

---

### Task 2: 添加 SQLite 依赖

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: 添加依赖**

Run:

```bash
go get modernc.org/sqlite
```

- [ ] **Step 2: 验证**

Run: `go mod tidy && go build ./...`  
Expected: success

---

### Task 3: 用户消息解析单元测试（先写失败测试）

**Files:**
- Create: `internal/cursor/cursor_test.go`（仅 helper 测试，Scan 测试在 Task 4）

- [ ] **Step 1: 创建测试文件 — 提取 `<user_query>`**

```go
package cursor

import "testing"

func TestExtractUserQueryText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"array part", `[{"type":"text","text":"<user_query>\nhello\n</user_query>"}]`, "hello"},
		{"plain string", `<user_query>fix bug</user_query>`, "fix bug"},
		{"no tag", `plain text`, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractUserQueryText([]byte(tc.in))
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestIsValidUserQueryMessage(t *testing.T) {
	injection := []byte(`{"role":"user","providerOptions":{"cursor":{"requestContextCompleteness":{"rules":true}}},"content":"<user_info>...</user_info>"}`)
	real := []byte(`{"role":"user","providerOptions":{"cursor":{"requestId":"x"}},"content":[{"type":"text","text":"<user_query>\nhi\n</user_query>"}]}`)
	if isValidUserQueryMessage(injection) {
		t.Fatal("injection should not count")
	}
	if !isValidUserQueryMessage(real) {
		t.Fatal("real user query should count")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/cursor/... -run 'TestExtractUserQuery|TestIsValidUserQuery' -v`  
Expected: FAIL（函数未定义）

---

### Task 4: Scan 集成测试（合成 store.db）

**Files:**
- Modify: `internal/cursor/cursor_test.go`

- [ ] **Step 1: 添加 `createTestStoreDB` helper 与 Scan 测试**

在 `cursor_test.go` 追加（需 `modernc.org/sqlite` 在 Task 6 实现前本测试会 compile 失败 — 先写测试，Task 6 实现后通过）：

```go
import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func createTestStoreDB(t *testing.T, dir string, meta storeMeta, blobs []struct {
	id   string
	data []byte
}) string {
	t.Helper()
	path := filepath.Join(dir, "store.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE meta (key TEXT, value BLOB);
CREATE TABLE blobs (id TEXT, data BLOB);`)
	if err != nil {
		t.Fatalf("schema: %v", err)
	}

	metaJSON, _ := json.Marshal(meta)
	hexMeta := hex.EncodeToString(metaJSON)
	if _, err := db.Exec(`INSERT INTO meta(key,value) VALUES('0',?)`, hexMeta); err != nil {
		t.Fatalf("meta: %v", err)
	}
	for _, b := range blobs {
		if _, err := db.Exec(`INSERT INTO blobs(id,data) VALUES(?,?)`, b.id, b.data); err != nil {
			t.Fatalf("blob: %v", err)
		}
	}
	return path
}

func TestScanExtractsCursorSession(t *testing.T) {
	root := t.TempDir()
	chatID := "chat-abc"
	sessionID := "sess-1111-2222-3333-4444"
	sessionDir := filepath.Join(root, chatID, sessionID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	createdMs := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC).UnixMilli()
	injectionBlob := []byte(`{"role":"user","providerOptions":{"cursor":{"requestContextCompleteness":{"rules":true}}},"content":"<user_info>\nWorkspace Path: /repo/test-project\n</user_info>"}`)
	firstUser := []byte(`{"role":"user","providerOptions":{"cursor":{"requestId":"a"}},"content":[{"type":"text","text":"<user_query>\nfirst question\n</user_query>"}]}`)
	lastUser := []byte(`{"role":"user","providerOptions":{"cursor":{"requestId":"b"}},"content":[{"type":"text","text":"<user_query>\nlast question\n</user_query>"}]}`)

	dbPath := createTestStoreDB(t, sessionDir, storeMeta{
		AgentID:   sessionID,
		CreatedAt: createdMs,
	}, []struct {
		id   string
		data []byte
	}{
		{"blob-inject", injectionBlob},
		{"blob-first", firstUser},
		{"blob-last", lastUser},
	})

	// 确保 mtime 晚于 createTime
	future := time.Date(2026, 5, 20, 18, 0, 0, 0, time.UTC)
	if err := os.Chtimes(dbPath, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	got, err := Scan(root, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d sessions", len(got))
	}
	s := got[0]
	if s.SessionID != sessionID {
		t.Fatalf("sessionId=%q", s.SessionID)
	}
	if s.Dir != "/repo/test-project" {
		t.Fatalf("dir=%q", s.Dir)
	}
	if s.File != dbPath {
		t.Fatalf("file=%q", s.File)
	}
	if s.FirstMsg != "first question" || s.LastMsg != "last question" {
		t.Fatalf("msgs=%q / %q", s.FirstMsg, s.LastMsg)
	}
	if s.UserMsgAmount != 2 {
		t.Fatalf("userMsgAmount=%d", s.UserMsgAmount)
	}
	wantCreate := time.UnixMilli(createdMs).Local()
	if !s.CreateTime.Equal(wantCreate) {
		t.Fatalf("createTime=%v want %v", s.CreateTime, wantCreate)
	}
	if !s.LastTime.Equal(future.Local()) {
		t.Fatalf("lastTime=%v want %v", s.LastTime, future.Local())
	}
}

func TestScanSkipsStoreWithoutUserQuery(t *testing.T) {
	root := t.TempDir()
	sessionDir := filepath.Join(root, "c", "s")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	onlyInjection := []byte(`{"role":"user","providerOptions":{"cursor":{"requestContextCompleteness":{}}},"content":"<user_info>only</user_info>"}`)
	createTestStoreDB(t, sessionDir, storeMeta{AgentID: "s", CreatedAt: 1}, []struct {
		id   string
		data []byte
	}{{"b1", onlyInjection}})

	got, err := Scan(root, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want 0 got %d", len(got))
	}
}

func TestScanIgnoresChatRootStoreDB(t *testing.T) {
	root := t.TempDir()
	// 仅 chats/{chatId}/store.db — 不应匹配 */*/store.db 若 glob 为 */*/store.db
	// 规范要求 */*/store.db，即 chatId/sessionId/store.db；根级不应扫描
	chatDir := filepath.Join(root, "only-chat")
	if err := os.MkdirAll(chatDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	userBlob := []byte(`{"role":"user","content":"<user_query>hi</user_query>"}`)
	createTestStoreDB(t, chatDir, storeMeta{AgentID: "x", CreatedAt: 1}, []struct {
		id   string
		data []byte
	}{{"b", userBlob}})

	got, err := Scan(root, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("chat-root store.db should not match, got %d", len(got))
	}
}
```

- [ ] **Step 2: 运行 Scan 测试确认失败**

Run: `go test ./internal/cursor/... -run TestScan -v`  
Expected: FAIL（`Scan` / `storeMeta` 未定义）

---

### Task 5: 实现 `internal/cursor/cursor.go`

**Files:**
- Create: `internal/cursor/cursor.go`

- [ ] **Step 1: 实现完整 provider**

创建 `internal/cursor/cursor.go`（与 spec 一致，独立实现，不 copy `cursor/`）：

```go
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

	dir, err := resolveWorkspace(db, meta, log)
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

func resolveWorkspace(db *sql.DB, meta storeMeta, log Logger) (string, error) {
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

// protobufFieldString / protobufAllStrings / protobufFields / readVarint
// — 与 spec 一致的最小 varint 解析，实现同设计文档（field 9, wire type 2）

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
```

实现者须补全 `protobufFieldString`、`protobufAllStrings`、`protobufFields`、`readVarint`（可从 spec 描述手写，约 80 行，与已删除的 `cursor/` 参考逻辑等价但独立书写）。

- [ ] **Step 2: 运行 cursor 包全部测试**

Run: `go test ./internal/cursor/... -v`  
Expected: PASS

---

### Task 6: 接入 CLI

**Files:**
- Modify: `cmd/query-session/main.go`
- Modify: `cmd/query-session/main_test.go`

- [ ] **Step 1: main.go 增加 import 与分支**

```go
import (
	// ...
	"query-session/internal/cursor"
)
```

在 `switch session.Provider(provider)` 中 `ProviderCodex` 之后增加：

```go
	case session.ProviderCursor:
		chatsRoot := filepath.Join(home, ".cursor", "chats")
		log("info", "scanning cursor sessions under %s", chatsRoot)
		sessions, err = cursor.Scan(chatsRoot, func(level string, message string) {
			log(level, "%s", message)
		})
		if err != nil {
			return 1, err
		}
```

更新 `printUsage` 中 `-t` 说明：

```text
provider: claude, codex, or cursor (default "claude")
```

并追加 `cursor example:` 块：

```text
cursor example:
	# 当前工作区今天创建的 cursor agent 会话
	query-session -t cursor

	# 指定项目正则
	query-session -t cursor -p "query-session" -s 20260520 -e 20260520
```

- [ ] **Step 2: 更新 main_test.go help 断言**

将：

```go
`provider: claude or codex (default "claude")`,
```

改为：

```go
`provider: claude, codex, or cursor (default "claude")`,
```

- [ ] **Step 3: 运行测试**

Run: `go test ./... -v`  
Expected: PASS

---

### Task 7: 文档

**Files:**
- Modify: `docs/design.md`
- Modify: `docs/get-started.md`
- Modify: `CLAUDE.md`

- [ ] **Step 1: `docs/design.md` 增加 Cursor Provider 节**

内容包括：

- 根目录 `$HOME/.cursor/chats/*/*/store.db`
- meta hex JSON、`createdAt` 毫秒、`CreateTime`/`LastTime` 语义
- 用户消息：`<user_query>` + 排除 `requestContextCompleteness`
- workspace：protobuf field 9 + user_info 回退
- 与 Claude 输出格式相同，`file` 为 `store.db` 路径

- [ ] **Step 2: `docs/get-started.md`**

- 首段提及 cursor
- 增加 `query-session -t cursor` 示例

- [ ] **Step 3: `CLAUDE.md`**

架构列表增加 `- internal/cursor/cursor.go - Cursor Agent store.db 扫描`

---

### Task 8: 全量验证

- [ ] **Step 1: 测试与构建**

```bash
go test ./...
go build -o query-session ./cmd/query-session
```

Expected: 全部 PASS

- [ ] **Step 2: 手工冒烟（可选，本机有 Cursor 数据时）**

```bash
./query-session -t cursor -d -p ".*" -s 20260501 -e 20260531
```

Expected: 输出行含 `file=.../store.db`

---

## Spec 覆盖自检

| Spec 要求 | Task |
|-----------|------|
| `-t cursor` | Task 6 |
| `*/store.db` 两层扫描 | Task 5 `Glob` |
| meta hex / agentId / createdAt | Task 5 `loadStoreMeta` |
| LastTime = mtime | Task 5 `os.Stat` |
| 日期过滤 CreateTime | 复用 session.Filter，Task 6 |
| 用户消息规则 | Task 3–5 |
| workspace protobuf + 回退 | Task 5 |
| 无 cursor/ 依赖 | 仅 `internal/cursor` |
| 单元测试 fixture | Task 4 |
| 文档 | Task 7 |

## 实现后清理（可选，非本 plan 必须）

用户删除 `cursor/` 目录时，确认 `.gitignore` 无多余条目；`req.md` 可保留。
