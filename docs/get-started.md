# 快速开始

`query-session` 用于查询本机 **Claude Desktop**、**Codex**、**Cursor Agent** 的会话 ID 与摘要信息，输出统一的一行格式，便于人工检索。

## Provider 一览

| `-t` 值 | 数据位置 | 会话文件 |
|---------|----------|----------|
| `claude`（默认） | `$HOME/.claude/projects` | `*.jsonl` |
| `codex` | `$HOME/.codex/sessions/YYYY/MM/DD` | `*.jsonl` |
| `cursor` | `$HOME/.cursor/chats/{chatId}/{sessionId}` | `store.db` |

详细设计见 [design.md](./design.md)。

## 构建

```bash
go build -o query-session ./cmd/query-session
```

## CLI 参数

```text
-t / --type      provider: claude | codex | cursor（默认 claude）
-d / --debug     debug 日志 → stderr
-n / --number N  按 createTime 降序输出前 N 条（默认 0 = 全部）
-l / --last N    日期窗口：过去 N 天含今天（与 -s/-e 互斥）
-p / --project   项目目录正则（空 = 仅当前工作目录）
-x / --exclude   排除目录正则（优先于 -p）
-s / --start-day 开始日期 YYYYMMDD（默认今天）
-e / --end-day   结束日期 YYYYMMDD（默认今天）
```

## 输出格式

```text
dir=yyy sessionId=xxxx createTime=xxxx lastTime=xxxx file=xxxx userMsgAmount=N firstMsg="..." lastMsg="..."
```

| 字段 | 含义 |
|------|------|
| `dir` | 工作区 / 项目目录 |
| `sessionId` | 会话 ID |
| `createTime` | 会话创建时间（见下表） |
| `lastTime` | 最后活动时间（见下表） |
| `file` | 会话存储文件完整路径 |
| `userMsgAmount` | 有效用户消息条数 |
| `firstMsg` / `lastMsg` | 首/末条用户消息摘要（最多 20 字，超出 `...[N]`）；仅一条时 `lastMsg` 为空 |

时间格式：`YYYYMMDD_HH:mm:ss`（本地时区）。

### 各 provider 的时间与 sessionId

| | Claude / Codex | Cursor |
|--|----------------|--------|
| `createTime` | 首条有效用户消息时间 | `meta.createdAt`（会话创建） |
| `lastTime` | 末条有效用户消息时间 | `store.db` 文件 mtime |
| `sessionId` | 文件名 / `payload.id` | `meta.agentId` 或目录名 |
| 日期过滤 | 按 `createTime` | 按 `createTime` |

---

## Claude（默认）

当前目录、今天全部会话：

```bash
./query-session
# 等价
./query-session -t claude
```

常用：

```bash
./query-session -n 1                                 # 今天最新一条（当前目录）
./query-session -p '.*'                              # 今天所有项目
./query-session -n 3 -l 7 -p '.*'                    # 过去 7 天 createTime 最新的 3 条
./query-session -p 'query-session' -s 20260520 -e 20260520
./query-session -p 'git' -x 'aiagent' -s 20260513 -e 20260514   # -x 排除优先
./query-session -d=true -p '.*'                        # debug
```

只统计 `message.role=user` 且 `message.content` 为字符串的人类输入；`tool_result` 等数组 content 会跳过。

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

---

## 过滤与排序（通用）

- **项目**：`-p` 为空 → 仅 `dir == 当前目录`；`-p '.*'` → 全部项目。
- **排除**：`-x` 匹配到的 `dir` 一律排除。
- **日期**：默认今天；`-l N` 覆盖过去 N 天（含今天，与 `-s`/`-e` 互斥）；`-s`/`-e` 按 `createTime` 含边界过滤；`-s` 晚于 `-e` 报错。
- **排序**：未指定 `-n` 时，先 `dir` 升序，再 `createTime` 升序。
- **条数**：`-n N` 在过滤后按 `createTime` 降序取前 N 条；可与 `-l` 组合（如 `-n 3 -l 7`）。

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
[info] skip cursor store path=... reason=no-user-query
```

## 更多

- 命令速查：[test.md](./test.md)
- 开发与测试：[development.md](./development.md)
