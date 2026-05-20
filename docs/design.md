# 设计说明

本文档描述 query-session 的 Claude、Codex 和 Cursor provider 设计。

## 总体结构

`query-session` 是一个 Go CLI。

主要模块：

- `cmd/query-session`：CLI 入口，负责参数解析、错误输出、调用 provider。
- `internal/session`：统一会话模型、日期解析、过滤、排序、输出格式。
- `internal/claude`：Claude 会话扫描和 JSONL 解析。
- `internal/codex`：Codex 会话扫描和 JSONL 解析。
- `internal/cursor`：Cursor Agent 会话扫描和 `store.db` 解析。

统一会话模型：

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

`CreateTime` 和 `LastTime` 都来自用户消息时间戳，不使用文件修改时间。

## Claude 数据源

Claude 会话来自：

```text
$HOME/.claude/projects
```

目录结构：

```text
$HOME/.claude/projects/<encoded-project-dir>/<session-id>.jsonl
```

规则：

- `$HOME/.claude/projects` 下的一级子目录代表项目目录。
- 项目目录内一级 `*.jsonl` 文件代表主会话。
- `<session-id>/` 这类子目录会被忽略。
- 会话 ID 来自 JSONL 文件名，去掉 `.jsonl` 后缀。

## Claude 项目目录解码

Claude 会把真实项目路径编码成目录名。

示例：

```text
-Users-weizhoulan-Documents-git-query-session
```

可能对应：

```text
/Users/weizhoulan/Documents/git/query-session
```

难点是目录名本身也可能包含 `-`，所以不能简单把所有 `-` 替换成 `/`。

当前实现使用“最长存在路径前缀”策略：

- 从文件系统根目录 `/` 开始解析。
- `-` 可能表示路径分隔符。
- `--` 可能表示隐藏目录，例如 `.hermes`。
- 每解析出一个候选路径段，都检查该路径是否存在且是目录。
- 如果候选路径存在且是目录，就继续向下解析。
- 一旦某个层级无法确认存在，就停止解码。
- 剩余后缀原样追加，不再继续翻译后续的 `-` 或 `--`。

示例：

```text
encoded: -Users-weizhoulan-Documents-git-query-session
known prefix: /Users/weizhoulan/Documents/git
decoded: /Users/weizhoulan/Documents/git/query-session
```

隐藏目录示例：

```text
encoded: -Users-weizhoulan--hermes-skills
decoded: /Users/weizhoulan/.hermes/skills
```

如果 `.hermes` 已不存在，则在已确认前缀后保留未解析尾部。

## Claude JSONL 解析

每个 JSONL 文件一行是一条会话记录。

当前候选记录先要求：

```text
message.role == "user"
```

但仅满足 `message.role == "user"` 不够。Claude 会把工具返回结果也记录成 `message.role=user`，例如 `tool_result`。这类记录不是人类主动输入，不能用于 `FirstMsg` / `LastMsg`。

解析规则：

- JSON 行必须能正常解析。
- `message.role` 必须等于 `user`。
- `timestamp` 必须能按 RFC3339/RFC3339Nano 解析。
- `message.content` 必须是非空字符串。
- 第一条满足以上条件的记录提供 `CreateTime` 和 `FirstMsg`。
- 最后一条满足以上条件的记录提供 `LastTime` 和 `LastMsg`。
- 没有满足条件的人类用户消息时，文件跳过。
- 非法 JSON 行跳过。
- 非法 timestamp 行跳过。
- 单行最大扫描 buffer 当前设置为 10 MiB。

消息内容规则：

- 只接受字符串形式的 `message.content`。
- 字符串 trim 后为空时跳过。
- 数组、对象、`null` 等其他形式一律跳过。

`tool_result` 被跳过的原因：

```json
{
  "message": {
    "role": "user",
    "content": [
      {
        "type": "tool_result",
        "content": [
          { "type": "text", "text": "tool output" }
        ]
      }
    ]
  }
}
```

这里的 `message.content` 是数组，不是字符串。当前实现不会把它当作人类输入，因此不会覆盖 `LastMsg`。

## 过滤和排序

日期过滤：

- 基于 `CreateTime`。
- `start-day` 和 `end-day` 都包含边界当天。
- 日期按本地时区解释。
- `start-day > end-day` 时直接报错。

项目过滤：

- `-p` 为空时，要求 `Dir` 精确等于当前工作目录。
- `-p` 非空时，作为大小写不敏感正则匹配 `Dir`。

最新会话：

- `-l` / `--last` 默认是 `false`。
- `-l=true` 时，在所有过滤之后按 `CreateTime` 取最新一个。

多行排序：

- 先按 `Dir` 升序。
- 相同 `Dir` 按 `CreateTime` 升序。

## 输出设计

输出一行一个会话：

```text
dir=yyy sessionId=xxxx createTime=xxxx lastTime=xxxx file=xxxx.jsonl userMsgAmount=N firstMsg="..." lastMsg="..."
```

`file` 是完整 JSONL 会话文件路径。

消息摘要会做清洗：

- 控制字符、空白、双引号、反斜杠替换为空格。
- 连续空白合并。
- 去掉首尾空白。
- 截断到最多 20 个 Unicode 字符，超出时追加 `...[N]`（N 为清洗后完整长度）。

单引号保留，因为它不是当前输出格式的分隔符。

`UserMsgAmount` 是 session 中有效用户消息的条数，用 `messageString()` / `extractUserContent()` 提取出非空字符串的 user-role 行数。

当 `FirstMsg == LastMsg`（session 中只有一条有效用户消息）时，`lastMsg` 输出为空字符串，避免冗余。

## 错误和日志

普通错误输出：

```text
[error] message
```

debug 日志只有 `-d=true` 时输出到 stderr：

```text
[info] message
[error] message
```

debug 日志覆盖：

- `scan project`：扫描到的 Claude 项目目录和解码后的真实目录。
- `scan file`：扫描到的 JSONL 会话文件。
- `parsed`：成功解析出的会话。
- `skip file`：因为没有人类用户消息而跳过的文件。
- `matched`：通过过滤条件的会话。
- `filtered`：被日期或项目条件过滤掉的会话。
- `selected latest`：`-l=true` 时最终选中的会话。

Codex 额外日志：

- `scan codex day`：扫描到的 Codex 日期目录。
- `skip codex sub-agent session`：被过滤的子 agent 会话。

## Codex Provider

Codex 会话来自：

```text
$HOME/.codex/sessions
```

目录结构：

```text
$HOME/.codex/sessions/YYYY/MM/DD/*.jsonl
```

规则：

- 按日期范围遍历 `YYYY/MM/DD/` 子目录，只扫描匹配日期目录减少无效扫描。
- 每个日期目录下的 `*.jsonl` 文件即会话。

会话 ID（优先级从高到低）：

1. 使用第一条包含 `payload.id` 的 JSONL 记录。
2. 如果遍历完仍未找到 `payload.id`，从文件名提取 ID（去掉 `.jsonl` 后缀）。

目录：

- 使用第一条包含 `payload.cwd` 的 JSONL 记录。
- 如果没有 `payload.cwd`，`Dir` 保持为空。

文件路径：

- `File` 字段填入 JSONL 文件完整路径。

## Codex JSONL 解析

有效用户消息需同时满足：

1. `payload.role == "user"`
2. `payload.content` 是数组且只有 1 个成员
3. 该成员的 `type == "input_text"` 且 `text` trim 后非空

解析规则：

- JSON 行必须能正常解析。
- `payload.role` 必须等于 `user`。
- `timestamp` 必须能按 RFC3339/RFC3339Nano 解析。
- `payload.content` 必须是单成员数组，取该成员的 `text` 值（trim 后非空）。
- 第一条有效用户消息提供 `CreateTime` 和 `FirstMsg`。
- 最后一条有效用户消息提供 `LastTime` 和 `LastMsg`。
- 没有有效用户消息时，文件跳过。
- 非法 JSON 行跳过。
- 非法 timestamp 行跳过。
- 单行最大扫描 buffer 为 10 MiB。

多成员 content 数组（如系统提示词 AGENTS.md + environment_context）会被跳过，不会被误认为用户输入。

## Codex 子会话过滤

Codex 的子 agent 会话在 `session_meta` 记录中包含 `payload.source.subagent.thread_spawn.parent_thread_id`，指向父会话 ID。

扫描时如果文件中任意一行包含非空的 `parent_thread_id`，该文件作为子会话跳过，只保留主会话。

## Cursor Provider

Cursor Agent 会话来自：

```text
$HOME/.cursor/chats
```

目录结构：

```text
$HOME/.cursor/chats/{chatId}/{sessionId}/store.db
```

规则：

- 只扫描 `chats/*/*/store.db`（两层子目录），忽略 Chat 根目录下空的 `store.db`。
- 会话 ID 来自 `meta` 表 `key='0'` 的 hex JSON 字段 `agentId`，与目录名 `{sessionId}` 一致。
- `File` 字段为 `store.db` 完整路径。

### meta 表

- `key = '0'` 的 `value` 为 hex 编码 JSON。
- `createdAt` 为毫秒时间戳，映射到 `CreateTime`（本地时区）。
- `latestRootBlobId` 为对话树根 blob id，用于解析 workspace。

### 时间字段

- `CreateTime`：`meta.createdAt`。
- `LastTime`：`store.db` 文件修改时间（`mtime`）。
- 日期过滤 `-s` / `-e` 基于 `CreateTime`（与 Claude 相同）。

### Workspace（Dir）

1. 读取 `latestRootBlobId` 对应 blob，解析 Protobuf **field 9** 的 `file://` URI。
2. 回退：在 `role=user` 的 JSON blob 中匹配 `Workspace Path: ...`（常见于 `<user_info>` 注入）。

### 用户消息

有效用户消息须同时满足：

1. `role == "user"`。
2. 非上下文注入：`providerOptions.cursor.requestContextCompleteness` 为空或 `null`。
3. 内容包含 `<user_query>`。

- 从 `<user_query>...</user_query>` 提取正文作为 `FirstMsg` / `LastMsg`。
- `UserMsgAmount` 为有效用户消息条数。
- 无有效用户消息时跳过该 `store.db`。

### Cursor debug 日志

- `scan cursor store path=...`
- `parsed sessionId=...`
- `skip cursor store path=... reason=no-user-query` 或 `invalid-meta`
