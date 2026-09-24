# 开发调试

本文档记录 query-session 的开发、测试和本地调试命令。

## 环境要求

- Go 1.26+（见 `go.mod`）。
- 本机存在对应 provider 数据目录时，才能做真实数据验证：

```text
$HOME/.claude/projects              # Claude
$HOME/.codex/sessions               # Codex
$HOME/.cursor/chats                 # Cursor
${COPILOT_HOME:-$HOME/.copilot}/session-state  # GitHub Copilot CLI
```

Cursor provider 额外依赖：

```text
modernc.org/sqlite   # 纯 Go SQLite，无 CGO
```

Copilot provider 使用 `gopkg.in/yaml.v3` 解析 `workspace.yaml.name`。

## 常用命令

全部测试：

```bash
go test ./... -count=1
```

按包测试：

```bash
go test ./internal/session -count=1
go test ./internal/claude -count=1
go test ./internal/codex -count=1
go test ./internal/cursor -count=1
go test ./internal/copilot -count=1
go test ./cmd/query-session -count=1
```

CLI 端到端用例会在 `t.TempDir()` 构建真实二进制，并用隔离的 `HOME` / `COPILOT_HOME` 构造四种 provider 的测试会话；不读取本机真实会话：

```bash
go test ./cmd/query-session -run '^TestCLIBinaryEndToEnd$' -count=1
```

构建：

```bash
go build -o query-session ./cmd/query-session
```

清理产物：

```bash
rm -f query-session
```

格式化：

```bash
gofmt -w cmd/query-session internal/session internal/claude internal/codex internal/cursor internal/copilot
```

仅检查自己改动的路径：

```bash
git diff --check -- cmd/query-session internal/session internal/claude internal/codex internal/cursor internal/copilot docs
```

## 包职责

| 包 | 职责 |
|----|------|
| `internal/session` | 日期范围、`-p`/`-x` 过滤、`-n` 条数限制、排序、`FormatLine`、消息清洗 |
| `internal/claude` | 扫描 `~/.claude/projects`、目录解码、JSONL 用户消息 |
| `internal/codex` | 按日期扫描 `~/.codex/sessions`、JSONL、`parent_thread_id` 过滤 |
| `internal/cursor` | 扫描 `chats/*/*/store.db`、meta/blobs、Protobuf workspace、`<user_query>` 提取 |
| `internal/copilot` | 扫描 `session-state/*/events.jsonl`、首行 cwd 预筛、`workspace.yaml.name` |
| `cmd/query-session` | CLI、provider 分支、usage |

## 本地真实数据调试

### Claude

```bash
go run ./cmd/query-session -t claude
go run ./cmd/query-session -t claude -n 1
go run ./cmd/query-session -t claude -p '.*'
go run ./cmd/query-session -t claude -s 20260518 -e 20260518 -p '.*' -d=true
```

排查无输出时看 debug：`skip file reason=no-reliable-directory-or-time`、`filtered reason=project|date`。

Claude 有效用户消息：`message.role=user` 且 `message.content` 为**字符串**。`tool_result` 的 content 为数组，会跳过。

```bash
rg -n '"role":"user"' /path/to/session.jsonl | tail -n 10
```

### Codex

```bash
go run ./cmd/query-session -t codex -p '.*'
go run ./cmd/query-session -t codex -p '.*' -n 1
go run ./cmd/query-session -t codex -d=true -p '.*'
```

有效消息：`payload.role=user` 且 `content` 为单元素 `input_text`。多成员 content 与子 agent（`parent_thread_id`）会跳过。

### Cursor

```bash
go run ./cmd/query-session -t cursor
go run ./cmd/query-session -t cursor -p 'query-session' -s 20260501 -e 20261231
go run ./cmd/query-session -t cursor -d=true -p '.*'
```

debug 关注：

- `scan cursor store path=...` — 是否扫到 `store.db`
- `parsed sessionId=...` — 解析成功
- `skip ... reason=no-reliable-directory-or-time` — 零消息会话缺少可确认的目录或创建时间
- `skip ... reason=invalid-meta` — meta 损坏

手动查看 meta（示例）：

```bash
sqlite3 ~/.cursor/chats/{chatId}/{sessionId}/store.db \
  "SELECT value FROM meta WHERE key='0';" | xxd -r -p | jq .
```

单元测试使用临时目录合成 `store.db`，不依赖本机 `~/.cursor`（见 `internal/cursor/cursor_test.go`）。

### GitHub Copilot CLI

```bash
go run ./cmd/query-session  # 默认 copilot
go run ./cmd/query-session -t copilot -p 'query-session' -n 0
go run ./cmd/query-session -t copilot -d=true -p '.*' -n 1
```

首行的 `session.start.data.context.cwd` 用于预筛项目；需要确认当前目录是否与会话**初始** cwd 完全一致。`workspace.yaml.name` 是原生标题，缺失时由首条有效 `user.message.data.content` 回退。无用户消息但有可靠目录/创建时间的会话在表格中显示 `Title` 为 `未命名`、`MsgAmount` 为 0。

宽泛 `-p` 会读取全部匹配日志；`-n` 和日期筛选不会缩小正文扫描范围。损坏首行和超过 64 MiB 的行明确报错。不要把真实用户消息复制进测试文件；单元测试在临时目录合成 JSONL/YAML。

## 推荐开发步骤

1. 先写或更新 `internal/<provider>` 单元测试（Cursor 用 `createTestStoreDB` / `encodeProtobufStringField`）。
2. `go test ./internal/<provider> -v -count=1`
3. 实现最小代码。
4. `go test ./... -count=1`
5. `go build ./cmd/query-session`
6. 更新 `docs/design.md`、`docs/get-started.md`（若行为或 CLI 有变）。
7. 按需提交（用户明确要求时再 `git commit`）。

## 错误输出验证

```bash
go run ./cmd/query-session -t nope
# 期望 stderr: [error] unknown provider: nope
```

## 相关文档

- [design.md](./design.md) — 四 provider 设计细节
- [get-started.md](./get-started.md) — 使用说明
- [test.md](./test.md) — 命令示例
