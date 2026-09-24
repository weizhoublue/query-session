# 快速开始

`query-session` 用于查询本机 **Claude Desktop**、**Codex**、**Cursor Agent**、**GitHub Copilot CLI** 的会话 ID 与标题，输出统一的摘要和对齐表格，便于人工检索。

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

查询条件摘要和会话表格均输出到 stdout（错误和 debug 日志仍输出到 stderr）：

```text
provider: copilot
directory: /path/to/project
time range: all
session limit/matched/output: 10/2/2

SessionId                             Title             MsgAmount  CreateTime         LastTime
3cc5c8d4-6d18-4ba9-b1c5-486f953a80b1  Research Copilot  5          20260923_23:38:57  20260923_23:38:57
82d74f12-d89d-45e5-af28-f6292e570101  Fix table output  1          20260923_22:10:00  20260923_22:15:30
```

`directory` 是当前目录，指定非空 `-p` 时为项目正则；`time range` 为 `all`、`last N days` 或 `YYYYMMDD..YYYYMMDD`。`exclude` 仅在传入 `-x` 时显示（位于 `directory` 后）。`session limit` 是 `-n` 请求的最大输出条数（默认 10，0 = 不限量）；`matched` 是过滤后、截取前的数量，`output` 是实际输出量。无匹配时仍显示摘要和表头。

显式传入 `-p` 或 `--project` 时，表格最后增加 `Directory` 列，显示每条会话的实际目录（即使传入 `-p ""` 或没有匹配结果）；不传时保持上述五列格式。例如 `query-session -n 3 -l 7 -p '.*'` 会显示所有匹配目录的最新三条会话及各自的 `Directory`。

| 字段 | 含义 |
|------|------|
| `SessionId` | 会话 ID；异常控制字符在展示时转义 |
| `Title` | 原生会话标题；缺失时用首条有效用户输入回退，再缺失则为 `未命名`。单行清洗、最多 80 个 Unicode 字符，超出追加 `...[N]` |
| `MsgAmount` | 有效用户消息条数 |
| `CreateTime` | 会话创建时间（见下表） |
| `LastTime` | 最后活动时间（见下表） |
| `Directory` | 显式传入 `-p` / `--project` 时才显示；该会话的实际目录，不是摘要中的查询正则；控制字符在展示时转义 |

旧版 stdout 的 `dir=...` 等逐条字段已改为摘要加表格；依赖旧字段或 stderr 摘要的脚本需调整。没有有效用户消息（`MsgAmount=0`）的会话不显示，也不计入 `matched` 或 `-n` 名额；全部被过滤时仍显示摘要和表头。

时间格式：`YYYYMMDD_HH:mm:ss`（本地时区）。

### 各 provider 的时间与 sessionId

| | Claude | Codex | Cursor | Copilot |
|--|--------|-------|--------|---------|
| `createTime` | 首条有效用户消息时间 | 同左 | `meta.createdAt` | 首条有效用户消息时间 |
| `lastTime` | 末条有效用户消息时间 | 同左 | `store.db` 文件 mtime | 末条有效用户消息时间 |
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
零消息会话不显示；扫描时仍要求项目路径实际存在且日志有有效事件时间。

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
- 零消息会话不显示；扫描时仍要求 `session_meta` 里的 `payload.cwd` 和有效时间戳。

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
- 零消息会话不显示；扫描时仍要求 `meta.createdAt` 与 workspace 都可靠。

---

## GitHub Copilot CLI

```bash
./query-session                    # 默认 copilot
./query-session -t copilot
./query-session -t copilot -p 'query-session' -n 0
./query-session -t copilot -l 7 -p '.*' -n 3
```

- 读取 `~/.copilot/session-state/*/events.jsonl`（设置 `COPILOT_HOME` 时改用指定目录）；只统计 `user.message` 中非空的原始 `data.content`，不使用 `transformedContent`。
- `dir` 是 `session.start.data.context.cwd`（会话最初目录），`title` 来自 `workspace.yaml.name`；缺名称时回退首条有效用户输入。零消息会话不显示。
- 先读取各日志首行的 cwd，再读取匹配项目的完整日志；默认当前目录查询不会读取所有项目正文。`-p '.*'` 会扫描全部匹配日志，`-n` 或日期筛选只限制结果，不减少这一步的读取量。Copilot 的 SQLite 索引可能缺少消息，不作为查询来源。
- 首条 `session.start` 损坏或单行超过 64 MiB 时明确报错，不静默漏报会话。

---

## 过滤与排序（通用）

- **项目**：`-p` 为空 → 仅 `dir == 当前目录`；`-p '.*'` → 全部项目。
- **排除**：`-x` 匹配到的 `dir` 一律排除。
- **消息**：`MsgAmount=0` 的会话不显示，不计入匹配数量或 `-n` 名额。
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
