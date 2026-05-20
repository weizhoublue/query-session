# 设计说明

本文档描述 query-session 的 Claude、Codex 和 Cursor 三个 provider 的设计与实现约定。

## 总体结构

`query-session` 是一个 Go CLI，依赖 `modernc.org/sqlite`（仅 Cursor provider 使用，无 CGO）。

主要模块：

| 模块 | 职责 |
|------|------|
| `cmd/query-session` | CLI 入口：参数解析、provider 分发、错误输出 |
| `internal/session` | 统一会话模型、日期解析、过滤、排序、输出格式 |
| `internal/claude` | Claude Desktop 会话：扫描 `~/.claude/projects`，JSONL 解析 |
| `internal/codex` | Codex 会话：扫描 `~/.codex/sessions`，JSONL 解析 |
| `internal/cursor` | Cursor Agent 会话：扫描 `~/.cursor/chats`，SQLite `store.db` 解析 |

核心流程：

```text
Scan (provider) → Filter → Sort → FormatLine
```

## CLI 参数

| 参数 | 默认 | 说明 |
|------|------|------|
| `-t` / `--type` | `claude` | provider：`claude`、`codex`、`cursor` |
| `-d` / `--debug` | `false` | debug 日志输出到 stderr |
| `-l` / `--last` | `false` | 过滤后只输出 `createTime` 最新的一条 |
| `-p` / `--project` | 空 | 项目目录正则（大小写不敏感）；空则精确匹配当前工作目录 |
| `-x` / `--exclude` | 空 | 排除目录正则，优先级高于 `-p` |
| `-s` / `--start-day` | 今天 | 开始日期 `YYYYMMDD`（含当天） |
| `-e` / `--end-day` | 今天 | 结束日期 `YYYYMMDD`（含当天） |

校验：未知 provider、非法日期、非法正则、`start-day > end-day` 时报错退出。

## 统一会话模型

所有 provider 解析为同一结构：

```text
SessionID
Dir
File
CreateTime
LastTime
FirstMsg
LastMsg
UserMsgAmount
```

### 时间与 `file` 字段（按 provider 不同）

| 字段 | Claude | Codex | Cursor |
|------|--------|-------|--------|
| `CreateTime` | 第一条有效用户消息的 `timestamp` | 同上 | `meta.createdAt`（毫秒 → 本地时区） |
| `LastTime` | 最后一条有效用户消息的 `timestamp` | 同上 | `store.db` 的 `mtime` |
| `File` | `*.jsonl` 完整路径 | 同上 | `store.db` 完整路径 |
| 日期过滤 `-s`/`-e` | 基于 `CreateTime` | 同上 | 同上 |
| `-l` 取最新 | 基于 `CreateTime` | 同上 | 同上 |

Claude / Codex **不使用**文件修改时间作为会话时间。

## 过滤和排序

日期过滤：

- 基于 `CreateTime`。
- `start-day` 和 `end-day` 都包含边界当天。
- 日期按本地时区解释。
- `start-day > end-day` 时直接报错。

项目过滤：

- `-p` 为空时，要求 `Dir` 精确等于当前工作目录。
- `-p` 非空时，作为大小写不敏感正则匹配 `Dir`。
- `-x` 非空时，匹配 `Dir` 的会话排除（优先于 `-p`）。

最新会话：

- `-l` / `--last` 默认 `false`。
- `-l=true` 时，在所有过滤之后按 `CreateTime` 取最新一个。

多行排序：

- 先按 `Dir` 升序。
- 相同 `Dir` 按 `CreateTime` 升序。

## 输出设计

每个会话一行：

```text
dir=yyy sessionId=xxxx createTime=xxxx lastTime=xxxx file=xxxx userMsgAmount=N firstMsg="..." lastMsg="..."
```

消息摘要清洗（`internal/session`）：

- 控制字符、空白、双引号、反斜杠替换为空格。
- 连续空白合并；截断至 20 个 Unicode 字符，超出追加 `...[N]`。
- 单引号保留。

`UserMsgAmount`：该会话中**有效用户消息**条数（各 provider 判定规则不同）。

当 `FirstMsg == LastMsg`（仅一条有效用户消息）时，`lastMsg` 输出为空字符串。

时间输出格式：`YYYYMMDD_HH:mm:ss`（本地时区）。

---

## Claude Provider

### 数据源

```text
$HOME/.claude/projects/<encoded-project-dir>/<session-id>.jsonl
```

规则：

- 一级子目录为编码后的项目路径。
- 仅读取项目目录下**一级** `*.jsonl`；忽略 `<session-id>/` 子目录（subagent）。
- `SessionID` = 文件名去掉 `.jsonl`。

### 项目目录解码

编码示例：`-Users-weizhoulan-Documents-git-query-session` → `/Users/weizhoulan/Documents/git/query-session`。

策略（最长存在路径前缀）：

- 从 `/` 起解析；`-` 为路径分隔符；`--` 表示隐藏段（如 `.hermes`）。
- 每段检查磁盘上是否存在且为目录；无法确认则停止解码，剩余后缀原样保留。

### JSONL 解析

有效用户消息：

- `message.role == "user"`。
- `timestamp` 可解析（RFC3339/RFC3339Nano）。
- `message.content` 为**非空字符串**（数组形式的 `tool_result` 等跳过）。

第一条 / 最后一条有效用户消息 → `CreateTime`/`FirstMsg`、`LastTime`/`LastMsg`。无有效用户消息则跳过文件。

单行 buffer 上限 10 MiB。

---

## Codex Provider

### 数据源

```text
$HOME/.codex/sessions/YYYY/MM/DD/*.jsonl
```

规则：

- 按 `-s`/`-e` 日期范围只遍历匹配的 `YYYY/MM/DD` 目录。
- `SessionID`：优先 `payload.id`，否则文件名（去 `.jsonl`）。
- `Dir`：第一条含 `payload.cwd` 的记录。

### JSONL 解析

有效用户消息须同时满足：

1. `payload.role == "user"`
2. `payload.content` 为**仅 1 个成员**的数组
3. 该成员 `type == "input_text"` 且 `text` trim 后非空

多成员 content（系统提示 + 环境上下文）跳过。

### 子会话过滤

若任意行含非空 `payload.source.subagent.thread_spawn.parent_thread_id`，整文件作为子 agent 会话跳过。

---

## Cursor Provider

实现位于 `internal/cursor/`，**不依赖**仓库内 `cursor/` 参考目录（该目录可删除）。调研笔记见根目录 `req.md`。

### 数据源

```text
$HOME/.cursor/chats/{chatId}/{sessionId}/store.db
```

规则：

- `filepath.Glob(chats/*/*/store.db)`，仅两层子目录。
- Chat 根目录下的 `store.db` **不会**被匹配。
- `chatId` 仅存在于路径，不写入输出。

### SQLite：`meta` 表

- `key = '0'` 的 `value`：通常为 **hex 编码的 JSON**；也支持 **plain JSON**（非 hex 时直接 `json.Unmarshal`）。
- 字段：
  - `agentId` → `SessionID`（空则回退为目录名 `{sessionId}`）
  - `createdAt` → `CreateTime`（毫秒）
  - `latestRootBlobId` → 对话树根 blob，用于 workspace

无效 meta（缺失、坏 hex、坏 JSON）→ 跳过该 `store.db`，`Scan` 不整体失败。

### SQLite：`blobs` 表

- `id`：内容哈希。
- `data`：JSON 消息或 Protobuf 二进制。

按 `rowid` 顺序遍历 JSON 行。

### Workspace（`Dir`）

1. `SELECT data FROM blobs WHERE id = latestRootBlobId`，解析 Protobuf **field 9** 的 `file://` URI → 本地路径。
2. 回退：在 `role=user` 的 JSON 中匹配 `Workspace Path:\s*(.+)`（常见于 `<user_info>` 注入 blob）。
3. 仍无法解析时 `Dir=""`（有 user query 仍会输出，默认 `-p` 空时会被项目过滤掉）。

Protobuf 为最小 varint 实现，无需 protoc。

### 用户消息

有效用户消息须同时满足：

1. `role == "user"`
2. **非**上下文注入：`providerOptions.cursor.requestContextCompleteness` 不存在、为空或 JSON `null`
3. 内容含 `<user_query>`

从 `<user_query>...</user_query>` 提取正文；`content` 可为 string 或 `[{type,text}]` 数组。

无有效用户消息 → 跳过该 `store.db`。

Subagent 无独立 `store.db`，对话在父会话 blobs 内；不解析 `agent-transcripts/*.jsonl`。

### 依赖

```text
modernc.org/sqlite v1.34.5
```

---

## 错误和日志

普通错误：

```text
[error] message
```

debug（`-d=true`）输出到 stderr：

```text
[info] message
[error] message
```

| Provider | 典型 debug 行 |
|----------|----------------|
| Claude | `scan project`、`scan file`、`parsed`、`skip file reason=no-user-message`、`matched`、`filtered`、`selected latest` |
| Codex | `scan codex day`、`skip codex sub-agent session`、`parsed codex session` |
| Cursor | `scan cursor store path=...`、`parsed sessionId=...`、`skip cursor store path=... reason=no-user-query` 或 `invalid-meta` |

---

## 相关文档

- 使用说明：[get-started.md](./get-started.md)
- 命令示例：[test.md](./test.md)
- 开发调试：[development.md](./development.md)
- Cursor 设计 spec：[superpowers/specs/2026-05-20-cursor-provider-design.md](./superpowers/specs/2026-05-20-cursor-provider-design.md)
