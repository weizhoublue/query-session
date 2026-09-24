# 快速开始

`query-session` 用于查询本机 **Claude Desktop**、**Codex**、**Cursor Agent**、**GitHub Copilot CLI** 的会话 ID 与标题，输出统一的一行格式，便于人工检索。

## Provider 一览

| `-t` 值 | 数据位置 | 会话文件 |
|---------|----------|----------|
| `claude` | `$HOME/.claude/projects` | `*.jsonl` |
| `codex` | `$HOME/.codex/sessions/YYYY/MM/DD` | `*.jsonl` |
| `cursor` | `$HOME/.cursor/chats/{chatId}/{sessionId}` | `store.db` |
| `copilot`（默认） | `${COPILOT_HOME:-$HOME/.copilot}/session-state/{sessionId}` | `events.jsonl`、`workspace.yaml` |

详细设计见 [design.md](./design.md)。

## 构建

```bash
go build -o query-session ./cmd/query-session
```

## CLI 参数

```text
-t / --type      provider: claude | codex | cursor | copilot（默认 copilot）
-d / --debug     debug 日志 → stderr
-n / --number N  按 createTime 降序输出前 N 条（默认 10；0 = 全部）
-l / --last N    日期窗口：过去 N 天含今天（与 -s/-e 互斥）
-p / --project   项目目录正则（空 = 仅当前工作目录）
-x / --exclude   排除目录正则（优先于 -p）
-s / --start-day 开始日期 YYYYMMDD（指定时启用日期过滤）
-e / --end-day   结束日期 YYYYMMDD（指定时启用日期过滤）
```

## 输出格式

查询条件摘要输出到 stderr：

```text
provider: copilot
project: /path/to/project
date: all
number: 10
matched: 8
output: 8
```

`exclude` 仅在传入 `-x` 时显示。`matched` 是过滤后的数量，`output` 是条数限制后的实际输出量。

session 输出到 stdout：

```text
dir=yyy sessionId=xxxx createTime=xxxx lastTime=xxxx file=xxxx userMsgAmount=N title="..."
```

| 字段 | 含义 |
|------|------|
| `dir` | 工作区 / 项目目录 |
| `sessionId` | 会话 ID |
| `createTime` | 会话创建时间（见下表） |
| `lastTime` | 最后活动时间（见下表） |
| `file` | 会话存储文件完整路径 |
| `userMsgAmount` | 有效用户消息条数 |
| `title` | 原生会话标题；缺失时用首条有效用户输入回退，再缺失则为 `未命名`。单行清洗、最多 80 个 Unicode 字符，超出追加 `...[N]` |

旧版 stdout 的 `firstMsg` / `lastMsg` 已移除；依赖固定字段的脚本需改读 `title`。没有有效用户输入但具有可靠目录和创建时间的会话也会显示，此时 `userMsgAmount=0`。

时间格式：`YYYYMMDD_HH:mm:ss`（本地时区）。

### 各 provider 的时间与 sessionId

| | Claude | Codex | Cursor | Copilot |
|--|--------|-------|--------|---------|
| `createTime` | 首条有效用户消息时间 | 同左 | `meta.createdAt` | 首条有效用户消息时间 |
| `lastTime` | 末条有效用户消息时间 | 同左 | `store.db` 文件 mtime | 末条有效用户消息时间 |
| 零消息时的时间 | 首条带时间的有效事件 | `session_meta.timestamp` | `meta.createdAt` / `store.db` mtime | `session.start.timestamp` 同时用于首末时间 |
| `sessionId` | 文件名 | `payload.id` 或文件名 | `meta.agentId` 或目录名 | `session.start.data.sessionId` |
| 原生标题 | 最后一个 `ai-title` | 无独立短标题，回退首条输入 | `meta.name` | `workspace.yaml.name` |

---

## Claude

当前目录、所有日期最新 10 条会话：

```bash
./query-session -t claude
```

常用：

```bash
./query-session -t claude -n 1
./query-session -t claude -p '.*'
./query-session -t claude -n 3 -l 7 -p '.*'
./query-session -t claude -p 'query-session' -s 20260520 -e 20260520
./query-session -t claude -p 'git' -x 'aiagent' -s 20260513 -e 20260514
./query-session -t claude -d=true -p '.*'
```

只统计 `message.role=user` 且 `message.content` 为字符串的人类输入；`tool_result` 等数组 content 会跳过。
零消息会话要求项目路径实际存在且日志有有效事件时间。

---

## Codex

```bash
./query-session -t codex
./query-session -t codex -p '.*'
./query-session -t codex -p '.*' -n 1
./query-session -t codex -s 20260518 -e 20260518 -p '.*'
```

- `sessionId`：优先 `payload.id`，否则文件名。
- `dir`：`payload.cwd`。
- 用户消息：`payload.content` 单成员且 `type=input_text`。
- 含 `parent_thread_id` 的子 agent 会话自动跳过。
- 零消息会话须有 `session_meta` 里的 `payload.cwd` 和有效时间戳。

---

## Cursor

```bash
./query-session -t cursor
./query-session -t cursor -n 1
./query-session -t cursor -p '.*'
./query-session -t cursor -p 'query-session' -s 20260520 -e 20260520
./query-session -t cursor -d=true -p '.*' -s 20260101 -e 20261231
```

说明：

- 扫描 `~/.cursor/chats/*/*/store.db`（每个 Agent 会话一个库）。
- 只计含 `<user_query>` 的真实用户输入；启动时的 `<user_info>` / rules 注入（`requestContextCompleteness`）不计入。
- `dir` 优先从 Protobuf workspace（`file://`）解析，否则从注入消息里的 `Workspace Path:` 读取。
- 无效或损坏的 `store.db` 在 debug 下会 `skip ... invalid-meta`，不影响其他会话。
- 零消息会话仅在 `meta.createdAt` 与 workspace 都可靠时显示。

---

## GitHub Copilot CLI

```bash
./query-session                    # 默认 copilot
./query-session -t copilot
./query-session -t copilot -p 'query-session' -n 0
./query-session -t copilot -l 7 -p '.*' -n 3
```

- 读取 `~/.copilot/session-state/*/events.jsonl`（设置 `COPILOT_HOME` 时改用指定目录）；只统计 `user.message` 中非空的原始 `data.content`，不使用 `transformedContent`。
- `dir` 是 `session.start.data.context.cwd`（会话最初目录），`title` 来自 `workspace.yaml.name`；缺名称、缺用户消息时显示 `未命名`。
- 先读取各日志首行的 cwd，再读取匹配项目的完整日志；默认当前目录查询不会读取所有项目正文。`-p '.*'` 会扫描全部匹配日志，`-n` 或日期筛选只限制结果，不减少这一步的读取量。Copilot 的 SQLite 索引可能缺少消息，不作为查询来源。
- 首条 `session.start` 损坏或单行超过 64 MiB 时明确报错，不静默漏报会话。

---

## 过滤与排序（通用）

- **项目**：`-p` 为空 → 仅 `dir == 当前目录`；`-p '.*'` → 全部项目。
- **排除**：`-x` 匹配到的 `dir` 一律排除。
- **日期**：默认不限日期；`-l N` 覆盖过去 N 天（含今天，与 `-s`/`-e` 互斥）；`-s`/`-e` 按 `createTime` 含边界过滤；`-s` 晚于 `-e` 报错。
- **排序**：`-n > 0` 时按 `createTime` 降序；`-n 0` 时先 `dir` 升序，再 `createTime` 升序。
- **条数**：默认取最新 10 条；`-n 0` 输出全部匹配结果；`-n N` 在过滤后按 `createTime` 降序取前 N 条，可与 `-l` 组合（如 `-n 3 -l 7`）。

## Debug 日志

```bash
./query-session -d=true -p '.*'
```

stderr 示例：

```text
[info] scanning claude sessions under /Users/.../.claude/projects
[info] scan project encoded=... dir=...
[info] matched sessionId=... dir=... createTime=... lastTime=...

[info] scanning cursor sessions under /Users/.../.cursor/chats
[info] scan cursor store path=.../store.db
[info] parsed sessionId=... dir=... createTime=...
[info] skip cursor store path=... reason=no-reliable-directory-or-time
```

## 更多

- 命令速查：[test.md](./test.md)
- 开发与测试：[development.md](./development.md)
