# Codex Provider Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现 Codex session provider，使 `query-session -t codex` 能从 `$HOME/.codex/sessions` 扫描并展示会话信息。

**Architecture:** 新增 `internal/codex/` 包，与 `internal/claude/` 并行。Scan 签名对齐 Claude（带 Logger、返回 `[]session.Session`）。CLI 去掉 "not implemented" 错误，接入 codex.Scan。

**Tech Stack:** Go 标准库，`encoding/json` 解析 JSONL，`testing` 写单元测试。

---

## 文件结构

- Create: `internal/codex/codex.go` — Codex provider 扫描、JSONL 解析
- Create: `internal/codex/codex_test.go` — Codex provider 单元测试
- Modify: `cmd/query-session/main.go:63-68` — 接入 Codex provider

---

### Task 1: 编写 Codex provider 测试

**Files:**
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

	"query-session/internal/session"
)

func TestScanExtractsCodexSessionFromPayloadID(t *testing.T) {
	root := t.TempDir()
	dayDir := filepath.Join(root, "2026", "05", "18")
	if err := os.MkdirAll(dayDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	jsonl := `{"timestamp":"2026-05-18T00:00:00Z","payload":{"id":"sid-from-payload","cwd":"/repo/a"}}
{"timestamp":"2026-05-18T01:00:00Z","payload":{"role":"user","content":[{"type":"input_text","text":"first question"}]}}
{"timestamp":"2026-05-18T02:00:00Z","payload":{"role":"assistant","content":"ignored"}}
{"timestamp":"2026-05-18T03:00:00Z","payload":{"role":"user","content":[{"type":"input_text","text":"last question"}]}}
`
	filePath := filepath.Join(dayDir, "x.jsonl")
	if err := os.WriteFile(filePath, []byte(jsonl), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	start := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 18, 23, 59, 59, 0, time.UTC)
	got, err := Scan(root, start, end, nil)
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
	if got[0].File != filePath {
		t.Fatalf("file = %q, want %q", got[0].File, filePath)
	}
}

func TestScanFallbackToFilenameForSessionID(t *testing.T) {
	root := t.TempDir()
	dayDir := filepath.Join(root, "2026", "05", "18")
	if err := os.MkdirAll(dayDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	jsonl := `{"timestamp":"2026-05-18T01:00:00Z","payload":{"role":"user","content":[{"type":"input_text","text":"hello"}]}}
`
	if err := os.WriteFile(filepath.Join(dayDir, "my-session-id.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	start := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 18, 23, 59, 59, 0, time.UTC)
	got, err := Scan(root, start, end, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d sessions", len(got))
	}
	if got[0].SessionID != "my-session-id" {
		t.Fatalf("session id = %q, want my-session-id", got[0].SessionID)
	}
}

func TestScanSkipsCodexFileWithoutUserMessages(t *testing.T) {
	root := t.TempDir()
	dayDir := filepath.Join(root, "2026", "05", "18")
	if err := os.MkdirAll(dayDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	jsonl := `{"timestamp":"2026-05-18T01:00:00Z","payload":{"role":"assistant","content":"only assistant"}}
`
	if err := os.WriteFile(filepath.Join(dayDir, "x.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	start := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 18, 23, 59, 59, 0, time.UTC)
	got, err := Scan(root, start, end, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no sessions, got %d", len(got))
	}
}

func TestScanExtractsFirstInputTextFromContentArray(t *testing.T) {
	root := t.TempDir()
	dayDir := filepath.Join(root, "2026", "05", "18")
	if err := os.MkdirAll(dayDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	jsonl := `{"timestamp":"2026-05-18T01:00:00Z","payload":{"id":"sid","role":"user","content":[{"type":"tool_result","text":"tool output"},{"type":"input_text","text":"real user input"},{"type":"input_text","text":"second input"}]}}
`
	if err := os.WriteFile(filepath.Join(dayDir, "x.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	start := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 18, 23, 59, 59, 0, time.UTC)
	got, err := Scan(root, start, end, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d sessions", len(got))
	}
	if got[0].FirstMsg != "real user input" {
		t.Fatalf("firstMsg = %q, want %q", got[0].FirstMsg, "real user input")
	}
}

func TestScanSkipsUserMessageWithEmptyInputText(t *testing.T) {
	root := t.TempDir()
	dayDir := filepath.Join(root, "2026", "05", "18")
	if err := os.MkdirAll(dayDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	jsonl := `{"timestamp":"2026-05-18T01:00:00Z","payload":{"id":"sid","role":"user","content":[{"type":"input_text","text":"  "}]}}
`
	if err := os.WriteFile(filepath.Join(dayDir, "x.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	start := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 18, 23, 59, 59, 0, time.UTC)
	got, err := Scan(root, start, end, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no sessions, got %d", len(got))
	}
}

func TestScanOnlyScansMatchingDateDirs(t *testing.T) {
	root := t.TempDir()
	dayDir := filepath.Join(root, "2026", "05", "18")
	if err := os.MkdirAll(dayDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	otherDir := filepath.Join(root, "2026", "05", "19")
	if err := os.MkdirAll(otherDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	jsonl := `{"timestamp":"2026-05-18T01:00:00Z","payload":{"id":"sid","role":"user","content":[{"type":"input_text","text":"hello"}]}}
`
	if err := os.WriteFile(filepath.Join(dayDir, "a.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(otherDir, "b.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	start := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 18, 23, 59, 59, 0, time.UTC)
	got, err := Scan(root, start, end, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1 (only 5/18)", len(got))
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```bash
go test ./internal/codex
```

Expected: FAIL — `internal/codex` 包不存在。

- [ ] **Step 3: 提交**

```bash
git add internal/codex/codex_test.go
git commit -s -S -m "Add Codex provider failing tests"
```

---

### Task 2: 实现 Codex provider

**Files:**
- Create: `internal/codex/codex.go`

- [ ] **Step 1: 写最小实现**

Create `internal/codex/codex.go`:

```go
package codex

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

type lineRecord struct {
	Timestamp string `json:"timestamp"`
	Payload   struct {
		ID      string          `json:"id"`
		CWD     string          `json:"cwd"`
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
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
		msg := extractUserContent(record.Payload.Content)
		if msg == "" {
			continue
		}
		if out.FirstMsg == "" {
			out.CreateTime = ts
			out.FirstMsg = msg
		}
		out.LastTime = ts
		out.LastMsg = msg
	}

	if out.SessionID == "" {
		out.SessionID = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	}
	if out.SessionID == "" || out.FirstMsg == "" {
		return session.Session{}, false
	}
	return out, true
}

func extractUserContent(raw json.RawMessage) string {
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return ""
	}
	for _, p := range parts {
		if p.Type == "input_text" && strings.TrimSpace(p.Text) != "" {
			return p.Text
		}
	}
	return ""
}
```

- [ ] **Step 2: 运行测试确认通过**

Run:

```bash
go test ./internal/codex ./internal/session
```

Expected: PASS。

- [ ] **Step 3: 提交**

```bash
git add internal/codex/codex.go
git commit -s -S -m "Add Codex session provider"
```

---

### Task 3: CLI 接入 Codex provider

**Files:**
- Modify: `cmd/query-session/main.go`

- [ ] **Step 1: 去掉 Codex not implemented 错误，接入 codex.Scan**

Modify `cmd/query-session/main.go`:

Replace the import block — add `"query-session/internal/codex"`:

```go
import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"query-session/internal/claude"
	"query-session/internal/codex"
	"query-session/internal/session"
)
```

Replace the provider validation block (lines 63-68):

```go
	switch session.Provider(provider) {
	case session.ProviderClaude:
		projectsRoot := filepath.Join(home, ".claude", "projects")
		log("info", "scanning claude sessions under %s", projectsRoot)
		sessions, err = claude.Scan(projectsRoot, "/", func(level string, message string) {
			log(level, "%s", message)
		})
		if err != nil {
			return 1, err
		}
	case session.ProviderCodex:
		root := filepath.Join(home, ".codex", "sessions")
		log("info", "scanning codex sessions under %s", root)
		sessions, err = codex.Scan(root, start, end, func(level string, message string) {
			log(level, "%s", message)
		})
		if err != nil {
			return 1, err
		}
	default:
		return 1, fmt.Errorf("unknown provider: %s", provider)
	}
```

And remove the old Claude-only block that currently handles scanning (lines 84-91 in current code, which does `claude.Scan` directly).

- [ ] **Step 2: 运行全部测试**

Run:

```bash
go test ./...
```

Expected: PASS。

- [ ] **Step 3: 构建验证**

Run:

```bash
go build ./cmd/query-session
```

Expected: build 成功。

- [ ] **Step 4: 清理并提交**

```bash
rm -f query-session
git add cmd/query-session/main.go
git commit -s -S -m "Wire Codex provider into CLI"
```

---

### Task 4: 最终验证

**Files:**
- 无新增文件

- [ ] **Step 1: 运行完整测试**

Run:

```bash
go test ./... -count=1
```

Expected: PASS。

- [ ] **Step 2: 格式化检查**

Run:

```bash
gofmt -w cmd/query-session internal/codex
git diff --check -- cmd/query-session internal/codex
```

Expected: `git diff --check` 无输出，退出码为 0。

- [ ] **Step 3: 构建验证**

Run:

```bash
go build ./cmd/query-session
rm -f query-session
```

Expected: build 成功，产物已删除。

- [ ] **Step 4: 真实数据验证（如果本机有 Codex 会话）**

Run:

```bash
go run ./cmd/query-session -t codex -p '.*' -l=false
go run ./cmd/query-session -t codex -p '.*' -l=false -d=true
```

Expected: 命令成功退出，有会话时输出格式为：

```text
dir=yyy sessionId=xxxx createTime=xxxx lastTime=xxxx file=xxxx.jsonl firstMsg="..." lastMsg="..."
```

- [ ] **Step 5: 确认没有未提交的实现文件**

Run:

```bash
git status --short
```

Expected: 无未提交的 `internal/codex/` 或 `cmd/` 修改。

---

## 自检

- Spec 覆盖：Codex provider 所有细化设计点（Session ID 优先+回退、第一个 input_text 提取、File 字段、Logger）均有对应测试和实现。
- 占位符检查：无 TBD、TODO、延后实现或未定义引用。
- 类型一致性：统一使用 `session.Session`、`session.Logger`（各包内独立定义），Scan 签名与 Claude provider 对齐。
- 范围控制：只涉及 Codex provider + CLI 接入，未触碰过滤/排序/输出/Claude provider。
