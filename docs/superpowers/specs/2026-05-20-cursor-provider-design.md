# Cursor Provider 设计

## 目标

为 `query-session` 增加第三个 provider：`cursor`。从本机 Cursor Agent 的 SQLite 会话库 `store.db` 扫描会话，输出格式与 Claude/Codex 一致，便于人工快速查看。

**约束：** `cursor/` 目录（含 `print-storedb`）将被删除。所有实现代码放在主仓库 `internal/cursor/`，不 import、不依赖 `cursor/` 下任何包或文件。调研结论见仓库根目录 `req.md`（可选参考，非运行时依赖）。

## 范围

包含：

- `-t cursor` 从 `$HOME/.cursor/chats` 扫描会话。
- 解析 `store.db` 的 `meta` / `blobs` 表，提取会话 ID、工作区目录、用户消息摘要与时间。
- 复用现有 `internal/session` 的 Filter、Sort、FormatLine、`-l` 逻辑。
- 单元测试覆盖解析与跳过规则。

不包含：

- 依赖或引用 `cursor/` 目录代码。
- 解析 `~/.cursor/projects/.../agent-transcripts/` 下 subagent 的独立 jsonl（subagent 无独立 `store.db`）。
- 输出 `chatId`、`name`、`lastUsedModel` 等额外字段。
- JSON 输出、交互 UI。
- 修改通用 Filter 的日期字段语义（仍基于 `CreateTime`）。

## CLI

在现有参数上扩展：

- `-t` / `--type`：增加有效值 `cursor`。
- 默认仍为 `claude`。

示例：

```bash
query-session -t cursor
query-session -t cursor -p "query-session" -s 20260520 -e 20260520
query-session -t cursor -l
```

扫描根目录：

```text
$HOME/.cursor/chats
```

## 数据源与目录结构

```text
~/.cursor/chats/{chatId}/{sessionId}/store.db
```

说明：

- 同一 `chatId` 下可有多个 `sessionId` 子目录，每个 Agent 会话一个 `store.db`。
- Chat 根目录下的 `store.db` 常为空或无效，扫描时只匹配 `chats/*/*/store.db`（恰好两层子目录）。
- `chatId` 仅存在于路径中，DB 内无此字段；本 provider 不输出 `chatId`。

## SQLite 表结构（实现需自包含理解）

### `meta` 表

- `key = '0'` 的 `value` 为 **hex 编码的 JSON**（先 `hex.Decode` 再 `json.Unmarshal`）。
- 关键字段：

| JSON 字段 | 用途 |
|-----------|------|
| `agentId` | 会话 ID，与目录名 `{sessionId}` 一致 |
| `createdAt` | 毫秒时间戳，会话创建时间 |
| `latestRootBlobId` | 对话树根 blob 的 id（内容哈希），非磁盘路径 |
| `name` | 会话显示名（仅调试参考，不输出） |

### `blobs` 表

- `id`：内容哈希字符串。
- `data`：JSON 消息、或 Protobuf 二进制（索引/快照）。

关注 `data` 为合法 JSON 且含 `"role"` 的行。

## 统一会话模型映射

解析结果填入 `session.Session`：

| 字段 | Cursor 来源 |
|------|-------------|
| `SessionID` | `meta.agentId`；若为空则用路径中 `{sessionId}` |
| `Dir` | workspace（见下文） |
| `File` | `store.db` 完整路径 |
| `CreateTime` | `meta.createdAt` → `time.UnixMilli`，本地时区 |
| `LastTime` | `store.db` 文件 `mtime`（`os.Stat`） |
| `FirstMsg` | 第一条有效用户消息的 `<user_query>` 内文本 |
| `LastMsg` | 最后一条有效用户消息的 `<user_query>` 内文本 |
| `UserMsgAmount` | 有效用户消息条数 |

### 时间语义（已确认）

- **方案 B：** `CreateTime` = `meta.createdAt`；`LastTime` = 文件 `mtime`。
- **日期过滤：** `-s` / `-e` 仅根据 `CreateTime` 判断（与 Claude 一致，Filter 不改）。
- **`-l`：** `LatestByCreateTime` 仍按 `CreateTime` 取最新。

### Workspace（`Dir`）

优先级：

1. 读 `meta.latestRootBlobId`，`SELECT data FROM blobs WHERE id = ?`。
2. 解析该 blob 的 **Protobuf**，取 **field 9** 的 `file://` URI，转为本地路径（`url.Parse` + `filepath.Clean`）。
3. 回退：遍历 `blobs` 中 `role=user` 的 JSON，在 `content` 字符串里用正则匹配 `Workspace Path:\s*(.+)`（典型于 `<user_info>` 注入消息）。

若仍无法解析，`Dir` 为空；该会话仍可在 `-p ".*"` 时出现，但默认 `-p` 空时会被项目过滤掉。

Protobuf 解析只需实现足够读取 field 9 字符串的 varint 解码（wire type 2 length-delimited），无需完整 protoc 生成代码。

## 用户消息规则

与 Claude 一样，只统计**人类真实输入**，排除上下文注入。

有效用户消息须同时满足：

1. `role == "user"`。
2. **非**上下文注入：`providerOptions.cursor.requestContextCompleteness` 不存在、为空或 JSON `null`。
3. 内容包含 `<user_query>`（`content` 可为 string 或 `[{type,text}]` 数组）。

消息文本提取：

- 从 `content` 取出文本后，用正则或字符串处理提取 `<user_query>...</user_query>` 内部正文（trim 空白）。
- 若无闭合标签，可退化为去掉标签名后的可见文本（实现时保持简单、可测）。

`FirstMsg` / `LastMsg`：

- 按 blobs 表 `SELECT` 遍历顺序（或 `rowid` 顺序）中**首次/末次**出现的有效用户消息。
- 当仅一条有效用户消息时，输出层已有规则：`LastMsg` 为空（`FormatLine` 行为不变）。

无有效用户消息 → **跳过**该 `store.db`（等同 Claude 无用户消息 skip）。

`UserMsgAmount` = 有效用户消息计数（非全部 `role=user` 行）。

## 扫描流程

```text
Scan(chatsRoot, log) → []session.Session
```

1. `filepath.Glob(chatsRoot + "/*/*/store.db")` 或等价 Walk（仅两层）。
2. 对每个路径调用 `parseStoreDB(path, log)`。
3. 打开 SQLite（`modernc.org/sqlite`），读 meta；失败或 `createdAt==0` 且无用户消息 → skip。
4. 解析 workspace、遍历 blobs 统计用户消息。
5. `Stat` 取 `mtime` 填 `LastTime`。
6. 聚合返回；错误单文件可 log 后 skip，不中断全量扫描（与 claude 单文件错误策略对齐：严重错误可返回 err，解析 skip 用 debug log）。

Debug 日志建议（`-d`）：

- `scan cursor store path=...`
- `parsed sessionId=... dir=... createTime=...`
- `skip cursor store path=... reason=no-user-query`
- `skip cursor store path=... reason=invalid-meta`

## 架构

```text
cmd/query-session/main.go     # switch ProviderCursor → cursor.Scan
internal/cursor/cursor.go     # Scan, parseStoreDB, meta/blobs/protobuf  helpers
internal/cursor/cursor_test.go
internal/session/session.go   # + ProviderCursor = "cursor"
```

**不**创建对 `cursor/` 的 Go module 引用；**不** shell 调用 `sqlite3` CLI。

## 依赖

主 `go.mod` 增加：

```text
modernc.org/sqlite
```

与 CGO 无关，便于 CI 交叉编译。

## 测试

`internal/cursor/cursor_test.go`：

- 在 `testdata/` 下放置**最小**合成 `store.db`（或测试内用 SQL 创建内存库），包含：
  - hex meta、`role=user` 带 `<user_query>` 的 blob、带 `requestContextCompleteness` 的注入行（应排除）。
  - workspace 可用简化 JSON user_info 回退路径验证。
- 断言 `SessionID`、`Dir`、`FirstMsg`、`LastMsg`、`UserMsgAmount`、`CreateTime`、`LastTime`（mtime 可 mock 或测相对顺序）。

不依赖真实 `~/.cursor/chats` 路径。

## 文档更新

实现后同步：

- `docs/design.md`：新增 Cursor Provider 章节。
- `docs/get-started.md`：`-t cursor` 示例。
- `CLAUDE.md`：架构列表增加 `internal/cursor`。

`req.md` 保留为调研笔记，可不修改。

## 错误处理

- `chats` 目录不存在：返回空列表或包装 `os` 错误（与 claude projects 根不存在行为一致）。
- 单库损坏/SQL 错误：debug 下 log，`continue` 扫描下一个。
- 无法解析 workspace：`Dir=""`，不因此 skip（除非也无用户消息）。

## 与 Claude/Codex 的差异摘要

| 项目 | Claude | Cursor |
|------|--------|--------|
| 存储 | JSONL | SQLite `store.db` |
| `File` | `.jsonl` | `store.db` |
| `CreateTime` | 首条用户消息时间 | `meta.createdAt` |
| `LastTime` | 末条用户消息时间 | `store.db` mtime |
| 用户消息 | `message.role=user` 字符串 content | `<user_query>` + 排除注入 |
| 项目目录 | 编码目录解码 | workspace 从 blob |

输出一行格式不变：

```text
dir=... sessionId=... createTime=... lastTime=... file=... userMsgAmount=N firstMsg="..." lastMsg="..."
```

## 决策记录

| 问题 | 选择 |
|------|------|
| 时间字段 | B：`createTime`=meta.createdAt，`lastTime`=mtime |
| 日期过滤 | A：按 `createTime` |
| 代码组织 | 独立 `internal/cursor`，删除 `cursor/` 后不遗留依赖 |
| 实现方式 | 自写解析逻辑（参考 req.md 语义），非迁移旧 module |
