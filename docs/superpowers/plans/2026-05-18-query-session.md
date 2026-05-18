# query-session Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 构建 Go CLI `query-session`，从本地 Claude 和 Codex JSONL 会话文件中查询会话信息。

**Architecture:** 采用单二进制 + 小包拆分。`cmd/query-session` 只负责 CLI 入口，`internal/session` 负责统一模型、过滤、排序、输出，`internal/claude` 和 `internal/codex` 分别负责 provider 解析。

**Tech Stack:** Go 标准库，`flag` 解析 CLI，`encoding/json` 解析 JSONL，`testing` 写单元测试。

---

## 文件结构

- Create: `go.mod`：声明 Go module。
- Create: `cmd/query-session/main.go`：CLI 入口，解析参数并调用应用层。
- Create: `internal/session/session.go`：统一 `Session`、`Options`、provider 类型、日期解析。
- Create: `internal/session/filter.go`：项目过滤、日期过滤、latest 选择、排序。
- Create: `internal/session/output.go`：时间格式化、消息清洗、行输出。
- Create: `internal/session/filter_test.go`：通用过滤和排序测试。
- Create: `internal/session/output_test.go`：输出和消息清洗测试。
- Create: `internal/claude/claude.go`：Claude provider 扫描、目录解码、JSONL 消息解析。
- Create: `internal/claude/claude_test.go`：Claude provider 单元测试。
- Create: `internal/codex/codex.go`：Codex provider 扫描、session meta 和用户消息解析。
- Create: `internal/codex/codex_test.go`：Codex provider 单元测试。

---

### Task 1: 初始化 Go module 和通用 session 包

**Files:**
- Create: `go.mod`
- Create: `internal/session/session.go`
- Create: `internal/session/filter.go`
- Create: `internal/session/filter_test.go`

- [ ] **Step 1: 写失败测试**

Create `internal/session/filter_test.go`:

```go
package session

import (
	"testing"
	"time"
)

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse time: %v", err)
	}
	return parsed
}

func TestParseDayRangeRejectsStartAfterEnd(t *testing.T) {
	_, _, err := ParseDayRange("20260519", "20260518", time.Local)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFilterExactCurrentDirWhenProjectEmpty(t *testing.T) {
	sessions := []Session{
		{Dir: "/repo/a", CreateTime: mustTime(t, "2026-05-18T01:00:00Z")},
		{Dir: "/repo/b", CreateTime: mustTime(t, "2026-05-18T02:00:00Z")},
	}

	start, end, err := ParseDayRange("20260518", "20260518", time.UTC)
	if err != nil {
		t.Fatalf("parse range: %v", err)
	}

	got, err := Filter(sessions, FilterOptions{
		CurrentDir: "/repo/a",
		Start:      start,
		End:        end,
	})
	if err != nil {
		t.Fatalf("filter: %v", err)
	}
	if len(got) != 1 || got[0].Dir != "/repo/a" {
		t.Fatalf("unexpected result: %#v", got)
	}
}

func TestFilterProjectRegexCaseInsensitive(t *testing.T) {
	sessions := []Session{
		{Dir: "/Users/me/Foo", CreateTime: mustTime(t, "2026-05-18T01:00:00Z")},
		{Dir: "/Users/me/bar", CreateTime: mustTime(t, "2026-05-18T02:00:00Z")},
	}

	start, end, err := ParseDayRange("20260518", "20260518", time.UTC)
	if err != nil {
		t.Fatalf("parse range: %v", err)
	}

	got, err := Filter(sessions, FilterOptions{
		ProjectPattern: "foo",
		CurrentDir:     "/not-used",
		Start:          start,
		End:            end,
	})
	if err != nil {
		t.Fatalf("filter: %v", err)
	}
	if len(got) != 1 || got[0].Dir != "/Users/me/Foo" {
		t.Fatalf("unexpected result: %#v", got)
	}
}

func TestLatestUsesCreateTime(t *testing.T) {
	sessions := []Session{
		{SessionID: "old", CreateTime: mustTime(t, "2026-05-18T01:00:00Z")},
		{SessionID: "new", CreateTime: mustTime(t, "2026-05-18T03:00:00Z")},
		{SessionID: "middle", CreateTime: mustTime(t, "2026-05-18T02:00:00Z")},
	}

	got := LatestByCreateTime(sessions)
	if got == nil || got.SessionID != "new" {
		t.Fatalf("unexpected latest: %#v", got)
	}
}

func TestSortByDirThenCreateTime(t *testing.T) {
	sessions := []Session{
		{SessionID: "b2", Dir: "/b", CreateTime: mustTime(t, "2026-05-18T02:00:00Z")},
		{SessionID: "a2", Dir: "/a", CreateTime: mustTime(t, "2026-05-18T02:00:00Z")},
		{SessionID: "a1", Dir: "/a", CreateTime: mustTime(t, "2026-05-18T01:00:00Z")},
	}

	SortSessions(sessions)
	got := []string{sessions[0].SessionID, sessions[1].SessionID, sessions[2].SessionID}
	want := []string{"a1", "a2", "b2"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```bash
go test ./internal/session
```

Expected: FAIL，因为 module 和 session 包尚不存在。

- [ ] **Step 3: 写最小实现**

Create `go.mod`:

```go
module query-session

go 1.22
```

Create `internal/session/session.go`:

```go
package session

import (
	"fmt"
	"time"
)

type Provider string

const (
	ProviderClaude Provider = "claude"
	ProviderCodex  Provider = "codex"
)

type Session struct {
	SessionID  string
	Dir        string
	CreateTime time.Time
	LastTime   time.Time
	FirstMsg   string
	LastMsg    string
}

type FilterOptions struct {
	ProjectPattern string
	CurrentDir     string
	Start          time.Time
	End            time.Time
}

func ParseDayRange(startDay, endDay string, loc *time.Location) (time.Time, time.Time, error) {
	start, err := time.ParseInLocation("20060102", startDay, loc)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid start-day %q: %w", startDay, err)
	}
	endDayStart, err := time.ParseInLocation("20060102", endDay, loc)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid end-day %q: %w", endDay, err)
	}
	if start.After(endDayStart) {
		return time.Time{}, time.Time{}, fmt.Errorf("start-day must not be later than end-day")
	}
	end := endDayStart.Add(24*time.Hour - time.Nanosecond)
	return start, end, nil
}
```

Create `internal/session/filter.go`:

```go
package session

import (
	"regexp"
	"sort"
)

func Filter(sessions []Session, opts FilterOptions) ([]Session, error) {
	var projectRE *regexp.Regexp
	var err error
	if opts.ProjectPattern != "" {
		projectRE, err = regexp.Compile("(?i)" + opts.ProjectPattern)
		if err != nil {
			return nil, err
		}
	}

	filtered := make([]Session, 0, len(sessions))
	for _, s := range sessions {
		if s.CreateTime.Before(opts.Start) || s.CreateTime.After(opts.End) {
			continue
		}
		if projectRE == nil {
			if s.Dir != opts.CurrentDir {
				continue
			}
		} else if !projectRE.MatchString(s.Dir) {
			continue
		}
		filtered = append(filtered, s)
	}
	return filtered, nil
}

func LatestByCreateTime(sessions []Session) *Session {
	if len(sessions) == 0 {
		return nil
	}
	latest := sessions[0]
	for _, s := range sessions[1:] {
		if s.CreateTime.After(latest.CreateTime) {
			latest = s
		}
	}
	return &latest
}

func SortSessions(sessions []Session) {
	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].Dir == sessions[j].Dir {
			return sessions[i].CreateTime.Before(sessions[j].CreateTime)
		}
		return sessions[i].Dir < sessions[j].Dir
	})
}
```

- [ ] **Step 4: 运行测试确认通过**

Run:

```bash
go test ./internal/session
```

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add go.mod internal/session/session.go internal/session/filter.go internal/session/filter_test.go
git commit -s -S -m "Add session filtering core"
```

---

### Task 2: 实现输出格式和消息摘要清洗

**Files:**
- Create: `internal/session/output.go`
- Create: `internal/session/output_test.go`

- [ ] **Step 1: 写失败测试**

Create `internal/session/output_test.go`:

```go
package session

import (
	"testing"
	"time"
)

func TestCleanMessageSummary(t *testing.T) {
	got := CleanMessageSummary("你好\n\t\"abc\"\\def  ghi")
	want := "你好 abc def"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestCleanMessageSummaryTruncatesUnicode(t *testing.T) {
	got := CleanMessageSummary("一二三四五六七八九十十一")
	want := "一二三四五六七八九十"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestFormatLine(t *testing.T) {
	createTime := time.Date(2026, 5, 18, 9, 1, 2, 0, time.Local)
	lastTime := time.Date(2026, 5, 18, 10, 3, 4, 0, time.Local)
	got := FormatLine(Session{
		Dir:        "/repo/a",
		SessionID:  "sid",
		CreateTime: createTime,
		LastTime:   lastTime,
		FirstMsg:   "first\nmessage",
		LastMsg:    "last message",
	})
	want := `dir=/repo/a sessionId=sid createTime=20260518_09:01:02 lastTime=20260518_10:03:04 firstMsg="first mess" lastMsg="last messa"`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```bash
go test ./internal/session
```

Expected: FAIL，缺少 `CleanMessageSummary` 和 `FormatLine`。

- [ ] **Step 3: 写最小实现**

Create `internal/session/output.go`:

```go
package session

import (
	"fmt"
	"strings"
	"unicode"
)

const outputTimeFormat = "20060102_15:04:05"

func CleanMessageSummary(msg string) string {
	var b strings.Builder
	lastSpace := false
	for _, r := range msg {
		if shouldReplaceWithSpace(r) {
			if !lastSpace {
				b.WriteRune(' ')
				lastSpace = true
			}
			continue
		}
		b.WriteRune(r)
		lastSpace = false
	}

	cleaned := strings.TrimSpace(b.String())
	runes := []rune(cleaned)
	if len(runes) > 10 {
		runes = runes[:10]
	}
	return string(runes)
}

func shouldReplaceWithSpace(r rune) bool {
	return unicode.IsControl(r) || unicode.IsSpace(r) || r == '"' || r == '\\'
}

func FormatLine(s Session) string {
	return fmt.Sprintf(
		`dir=%s sessionId=%s createTime=%s lastTime=%s firstMsg="%s" lastMsg="%s"`,
		s.Dir,
		s.SessionID,
		s.CreateTime.Local().Format(outputTimeFormat),
		s.LastTime.Local().Format(outputTimeFormat),
		CleanMessageSummary(s.FirstMsg),
		CleanMessageSummary(s.LastMsg),
	)
}
```

- [ ] **Step 4: 运行测试确认通过**

Run:

```bash
go test ./internal/session
```

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/session/output.go internal/session/output_test.go
git commit -s -S -m "Add session output formatting"
```

---

### Task 3: 实现 Claude provider

**Files:**
- Create: `internal/claude/claude.go`
- Create: `internal/claude/claude_test.go`

- [ ] **Step 1: 写失败测试**

Create `internal/claude/claude_test.go`:

```go
package claude

import (
	"os"
	"path/filepath"
	"testing"

	"query-session/internal/session"
)

func TestDecodeProjectDirStopsAfterLongestExistingPrefix(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Users", "me", "Documents", "git"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	got := DecodeProjectDir("-Users-me-Documents-git-query-session", root)
	want := filepath.Join(root, "Users", "me", "Documents", "git", "query-session")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestDecodeProjectDirHiddenSegment(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Users", "me", ".hermes"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	got := DecodeProjectDir("-Users-me--hermes-skills", root)
	want := filepath.Join(root, "Users", "me", ".hermes", "skills")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestScanExtractsClaudeUserMessages(t *testing.T) {
	home := t.TempDir()
	project := filepath.Join(home, ".claude", "projects", "-tmp-project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(project, "sid"), 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}
	jsonl := `{"timestamp":"2026-05-18T01:00:00Z","message":{"role":"user","content":"first question"}}
{"timestamp":"2026-05-18T01:10:00Z","message":{"role":"assistant","content":"ignored"}}
{"timestamp":"2026-05-18T02:00:00Z","message":{"role":"user","content":"last question"}}
`
	if err := os.WriteFile(filepath.Join(project, "sid.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := Scan(filepath.Join(home, ".claude", "projects"), home)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d sessions", len(got))
	}
	assertSession(t, got[0], session.Session{
		SessionID: "sid",
		Dir:       filepath.Join(home, "tmp-project"),
		FirstMsg:  "first question",
		LastMsg:   "last question",
	})
}

func assertSession(t *testing.T, got session.Session, want session.Session) {
	t.Helper()
	if got.SessionID != want.SessionID || got.Dir != want.Dir || got.FirstMsg != want.FirstMsg || got.LastMsg != want.LastMsg {
		t.Fatalf("got %#v want %#v", got, want)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```bash
go test ./internal/claude
```

Expected: FAIL，缺少 Claude provider。

- [ ] **Step 3: 写最小实现**

Create `internal/claude/claude.go`:

```go
package claude

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"query-session/internal/session"
)

type lineRecord struct {
	Timestamp string `json:"timestamp"`
	Message   struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
}

func Scan(projectsRoot, fsRoot string) ([]session.Session, error) {
	entries, err := os.ReadDir(projectsRoot)
	if err != nil {
		return nil, err
	}

	var sessions []session.Session
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		projectPath := filepath.Join(projectsRoot, entry.Name())
		dir := DecodeProjectDir(entry.Name(), fsRoot)
		files, err := filepath.Glob(filepath.Join(projectPath, "*.jsonl"))
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			parsed, ok := parseFile(file, dir)
			if !ok {
				continue
			}
			sessions = append(sessions, parsed)
		}
	}
	return sessions, nil
}

func DecodeProjectDir(encoded, fsRoot string) string {
	if !strings.HasPrefix(encoded, "-") {
		return filepath.Join(fsRoot, encoded)
	}

	rest := strings.TrimPrefix(encoded, "-")
	current := fsRoot
	for rest != "" {
		name, remaining, ok := nextSegment(rest)
		if !ok {
			return filepath.Join(current, rest)
		}
		candidate := filepath.Join(current, name)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			current = candidate
			rest = remaining
			continue
		}
		return filepath.Join(current, rest)
	}
	return current
}

func nextSegment(rest string) (string, string, bool) {
	if strings.HasPrefix(rest, "-") {
		hidden := strings.TrimPrefix(rest, "-")
		idx := strings.Index(hidden, "-")
		if idx < 0 {
			return "." + hidden, "", true
		}
		return "." + hidden[:idx], hidden[idx+1:], true
	}
	idx := strings.Index(rest, "-")
	if idx < 0 {
		return rest, "", true
	}
	return rest[:idx], rest[idx+1:], true
}

func parseFile(path, dir string) (session.Session, bool) {
	file, err := os.Open(path)
	if err != nil {
		return session.Session{}, false
	}
	defer file.Close()

	var out session.Session
	out.SessionID = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	out.Dir = dir

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record lineRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			continue
		}
		if record.Message.Role != "user" {
			continue
		}
		ts, err := time.Parse(time.RFC3339Nano, record.Timestamp)
		if err != nil {
			continue
		}
		if out.FirstMsg == "" {
			out.CreateTime = ts
			out.FirstMsg = record.Message.Content
		}
		out.LastTime = ts
		out.LastMsg = record.Message.Content
	}

	if out.FirstMsg == "" {
		return session.Session{}, false
	}
	return out, true
}
```

- [ ] **Step 4: 运行测试确认通过**

Run:

```bash
go test ./internal/claude ./internal/session
```

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/claude/claude.go internal/claude/claude_test.go
git commit -s -S -m "Add Claude session provider"
```

---

### Task 4: 实现 Codex provider

**Files:**
- Create: `internal/codex/codex.go`
- Create: `internal/codex/codex_test.go`

- [ ] **Step 1: 写失败测试**

Create `internal/codex/codex_test.go`:

```go
package codex

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScanExtractsCodexSessionWithoutUsingFilename(t *testing.T) {
	root := t.TempDir()
	dayDir := filepath.Join(root, "2026", "05", "18")
	if err := os.MkdirAll(dayDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	jsonl := `{"timestamp":"2026-05-18T00:00:00Z","type":"session_meta","payload":{"id":"sid-from-payload","cwd":"/repo/a"}}
{"timestamp":"2026-05-18T01:00:00Z","payload":{"role":"user","content":[{"type":"input_text","text":"first question"}]}}
{"timestamp":"2026-05-18T02:00:00Z","payload":{"role":"assistant","content":"ignored"}}
{"timestamp":"2026-05-18T03:00:00Z","payload":{"role":"user","content":"last question"}}
`
	if err := os.WriteFile(filepath.Join(dayDir, "future-format-name.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	start := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 18, 23, 59, 59, 0, time.UTC)
	got, err := Scan(root, start, end)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d sessions", len(got))
	}
	if got[0].SessionID != "sid-from-payload" {
		t.Fatalf("session id = %q", got[0].SessionID)
	}
	if got[0].Dir != "/repo/a" {
		t.Fatalf("dir = %q", got[0].Dir)
	}
	if got[0].FirstMsg != "first question" || got[0].LastMsg != "last question" {
		t.Fatalf("messages = %q / %q", got[0].FirstMsg, got[0].LastMsg)
	}
}

func TestScanSkipsCodexFileWithoutPayloadID(t *testing.T) {
	root := t.TempDir()
	dayDir := filepath.Join(root, "2026", "05", "18")
	if err := os.MkdirAll(dayDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	jsonl := `{"timestamp":"2026-05-18T01:00:00Z","payload":{"role":"user","content":"question"}}`
	if err := os.WriteFile(filepath.Join(dayDir, "x.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	start := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 18, 23, 59, 59, 0, time.UTC)
	got, err := Scan(root, start, end)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no sessions, got %#v", got)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```bash
go test ./internal/codex
```

Expected: FAIL，缺少 Codex provider。

- [ ] **Step 3: 写最小实现**

Create `internal/codex/codex.go`:

```go
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

type lineRecord struct {
	Timestamp string `json:"timestamp"`
	Payload   struct {
		ID      string          `json:"id"`
		CWD     string          `json:"cwd"`
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"payload"`
}

func Scan(root string, start, end time.Time) ([]session.Session, error) {
	var sessions []session.Session
	for day := dayStart(start); !day.After(end); day = day.AddDate(0, 0, 1) {
		dayDir := filepath.Join(root, day.Format("2006"), day.Format("01"), day.Format("02"))
		files, err := filepath.Glob(filepath.Join(dayDir, "*.jsonl"))
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			parsed, ok := parseFile(file)
			if !ok {
				continue
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

func parseFile(path string) (session.Session, bool) {
	file, err := os.Open(path)
	if err != nil {
		return session.Session{}, false
	}
	defer file.Close()

	var out session.Session
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record lineRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			continue
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
			continue
		}
		msg := contentText(record.Payload.Content)
		if out.FirstMsg == "" {
			out.CreateTime = ts
			out.FirstMsg = msg
		}
		out.LastTime = ts
		out.LastMsg = msg
	}

	if out.SessionID == "" || out.FirstMsg == "" {
		return session.Session{}, false
	}
	return out, true
}

func contentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var parts []map[string]any
	if err := json.Unmarshal(raw, &parts); err != nil {
		return ""
	}
	var b strings.Builder
	for _, part := range parts {
		if value, ok := part["text"].(string); ok {
			if b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(value)
		}
	}
	return b.String()
}
```

- [ ] **Step 4: 运行测试确认通过**

Run:

```bash
go test ./internal/codex ./internal/session
```

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/codex/codex.go internal/codex/codex_test.go
git commit -s -S -m "Add Codex session provider"
```

---

### Task 5: 实现 CLI 入口和端到端过滤输出

**Files:**
- Create: `cmd/query-session/main.go`

- [ ] **Step 1: 写入口实现**

Create `cmd/query-session/main.go`:

```go
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"query-session/internal/claude"
	"query-session/internal/codex"
	"query-session/internal/session"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "[error] %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	provider := flag.String("t", string(session.ProviderClaude), "provider type: claude or codex")
	debug := flag.Bool("d", false, "enable debug logging")
	last := flag.Bool("last", true, "print only latest created session")
	flag.BoolVar(last, "l", true, "print only latest created session")
	project := flag.String("project", "", "project directory regex")
	flag.StringVar(project, "p", "", "project directory regex")
	startDay := flag.String("start-day", today(), "start day YYYYMMDD")
	flag.StringVar(startDay, "s", today(), "start day YYYYMMDD")
	endDay := flag.String("end-day", today(), "end day YYYYMMDD")
	flag.StringVar(endDay, "e", today(), "end day YYYYMMDD")
	flag.Parse()

	start, end, err := session.ParseDayRange(*startDay, *endDay, time.Local)
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	var sessions []session.Session
	switch session.Provider(*provider) {
	case session.ProviderClaude:
		root := home + "/.claude/projects"
		logInfo(*debug, "scan claude root "+root)
		sessions, err = claude.Scan(root, "/")
	case session.ProviderCodex:
		root := home + "/.codex/sessions"
		logInfo(*debug, "scan codex root "+root)
		sessions, err = codex.Scan(root, start, end)
	default:
		return fmt.Errorf("unknown provider %q", *provider)
	}
	if err != nil {
		return err
	}

	filtered, err := session.Filter(sessions, session.FilterOptions{
		ProjectPattern: *project,
		CurrentDir:     cwd,
		Start:          start,
		End:            end,
	})
	if err != nil {
		return err
	}

	if *last {
		latest := session.LatestByCreateTime(filtered)
		if latest == nil {
			return nil
		}
		fmt.Println(session.FormatLine(*latest))
		return nil
	}

	session.SortSessions(filtered)
	for _, s := range filtered {
		fmt.Println(session.FormatLine(s))
	}
	return nil
}

func today() string {
	return time.Now().Local().Format("20060102")
}

func logInfo(enabled bool, msg string) {
	if enabled {
		fmt.Fprintf(os.Stderr, "[info] %s\n", msg)
	}
}
```

- [ ] **Step 2: 运行全部测试**

Run:

```bash
go test ./...
```

Expected: PASS。

- [ ] **Step 3: 构建二进制**

Run:

```bash
go build ./cmd/query-session
```

Expected: PASS，并在当前目录生成 `query-session`。

- [ ] **Step 4: 运行基本命令验证**

Run:

```bash
./query-session -t claude -p '.*' -l=false
./query-session -t codex -p '.*' -l=false
```

Expected: 命令成功退出。若本机有对应会话，则输出格式为：

```text
dir=yyy sessionId=xxxx createTime=xxxx lastTime=xxxx firstMsg="..." lastMsg="..."
```

- [ ] **Step 5: 清理构建产物并提交**

```bash
rm -f query-session
git add cmd/query-session/main.go
git commit -s -S -m "Add query-session CLI"
```

---

### Task 6: 补充 debug 日志

**Files:**
- Modify: `internal/claude/claude.go`
- Modify: `internal/codex/codex.go`
- Modify: `cmd/query-session/main.go`

- [ ] **Step 1: 修改 provider 签名**

Modify `internal/claude/claude.go`:

```go
type Logger func(level, message string)

func Scan(projectsRoot, fsRoot string, log Logger) ([]session.Session, error) {
	entries, err := os.ReadDir(projectsRoot)
	if err != nil {
		return nil, err
	}

	var sessions []session.Session
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		projectPath := filepath.Join(projectsRoot, entry.Name())
		if log != nil {
			log("info", "scan claude project "+projectPath)
		}
		dir := DecodeProjectDir(entry.Name(), fsRoot)
		files, err := filepath.Glob(filepath.Join(projectPath, "*.jsonl"))
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			parsed, ok := parseFile(file, dir, log)
			if !ok {
				if log != nil {
					log("info", "skip claude file "+file)
				}
				continue
			}
			if log != nil {
				log("info", "parsed claude session "+parsed.SessionID)
			}
			sessions = append(sessions, parsed)
		}
	}
	return sessions, nil
}

func parseFile(path, dir string, log Logger) (session.Session, bool) {
	file, err := os.Open(path)
	if err != nil {
		if log != nil {
			log("error", "open claude file failed "+path+": "+err.Error())
		}
		return session.Session{}, false
	}
	defer file.Close()

	var out session.Session
	out.SessionID = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	out.Dir = dir

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record lineRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			if log != nil {
				log("error", "invalid claude jsonl line "+path+": "+err.Error())
			}
			continue
		}
		if record.Message.Role != "user" {
			continue
		}
		ts, err := time.Parse(time.RFC3339Nano, record.Timestamp)
		if err != nil {
			if log != nil {
				log("error", "invalid claude timestamp "+path+": "+err.Error())
			}
			continue
		}
		if out.FirstMsg == "" {
			out.CreateTime = ts
			out.FirstMsg = record.Message.Content
		}
		out.LastTime = ts
		out.LastMsg = record.Message.Content
	}

	if out.FirstMsg == "" {
		return session.Session{}, false
	}
	return out, true
}
```

Modify `internal/codex/codex.go`:

```go
type Logger func(level, message string)

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
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record lineRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			if log != nil {
				log("error", "invalid codex jsonl line "+path+": "+err.Error())
			}
			continue
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
		msg := contentText(record.Payload.Content)
		if out.FirstMsg == "" {
			out.CreateTime = ts
			out.FirstMsg = msg
		}
		out.LastTime = ts
		out.LastMsg = msg
	}

	if out.SessionID == "" || out.FirstMsg == "" {
		return session.Session{}, false
	}
	return out, true
}
```

- [ ] **Step 2: 更新测试调用**

Modify Claude tests so calls pass `nil` logger:

```go
got, err := Scan(filepath.Join(home, ".claude", "projects"), home, nil)
```

Modify Codex tests so calls pass `nil` logger:

```go
got, err := Scan(root, start, end, nil)
```

- [ ] **Step 3: 更新 CLI logger**

Modify `cmd/query-session/main.go` provider calls:

```go
logger := makeLogger(*debug)

switch session.Provider(*provider) {
case session.ProviderClaude:
	root := home + "/.claude/projects"
	logger("info", "scan claude root "+root)
	sessions, err = claude.Scan(root, "/", logger)
case session.ProviderCodex:
	root := home + "/.codex/sessions"
	logger("info", "scan codex root "+root)
	sessions, err = codex.Scan(root, start, end, logger)
default:
	return fmt.Errorf("unknown provider %q", *provider)
}
```

Replace `logInfo` with:

```go
func makeLogger(enabled bool) func(level, message string) {
	return func(level, message string) {
		if enabled {
			fmt.Fprintf(os.Stderr, "[%s] %s\n", level, message)
		}
	}
}
```

- [ ] **Step 4: 运行测试确认通过**

Run:

```bash
go test ./...
```

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add internal/claude/claude.go internal/claude/claude_test.go internal/codex/codex.go internal/codex/codex_test.go cmd/query-session/main.go
git commit -s -S -m "Add debug logging"
```

---

### Task 7: 最终验证

**Files:**
- Modify: no source files expected

- [ ] **Step 1: 运行完整测试**

Run:

```bash
go test ./...
```

Expected: PASS。

- [ ] **Step 2: 运行格式化**

Run:

```bash
gofmt -w cmd/query-session internal/session internal/claude internal/codex
git diff --check
```

Expected: `git diff --check` 无输出，退出码为 0。

- [ ] **Step 3: 构建验证**

Run:

```bash
go build ./cmd/query-session
rm -f query-session
```

Expected: build 成功，构建产物已删除。

- [ ] **Step 4: 检查提交签名和工作区**

Run:

```bash
git log --show-signature --oneline -5
git status --short
```

Expected: 最近实现提交均由 `git commit -s -S` 创建。`git status --short` 只允许显示用户原本未提交的 `README.md`、`query-claude.sh`、`query-codex.sh`，不允许出现未提交实现文件。

---

## 自检

- Spec 覆盖：Claude provider、Codex provider、过滤、排序、输出、日志、错误处理、测试均有对应任务。
- 占位符检查：未发现未完成标记、延后实现说明或未定义的后续补充项。
- 类型一致性：统一使用 `session.Session`、`FilterOptions`、`ParseDayRange`、`Filter`、`LatestByCreateTime`、`SortSessions`、`FormatLine`。
- 范围控制：未加入 JSON 输出、交互式 UI、额外 provider 或缓存。
