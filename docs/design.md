# 设计说明

本文档描述当前已实现的 Claude provider。Codex provider 是第二阶段工作，当前代码中没有实现。

## 总体结构

`query-session` 是一个 Go CLI。

主要模块：

- `cmd/query-session`：CLI 入口，负责参数解析、错误输出、调用 provider。
- `internal/session`：统一会话模型、日期解析、过滤、排序、输出格式。
- `internal/claude`：Claude 会话扫描和 JSONL 解析。

统一会话模型：

```text
SessionID
Dir
File
CreateTime
LastTime
FirstMsg
LastMsg
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
- `message.content` 必须能提取出非空的人类可读文本。
- 第一条满足以上条件的记录提供 `CreateTime` 和 `FirstMsg`。
- 最后一条满足以上条件的记录提供 `LastTime` 和 `LastMsg`。
- 没有满足条件的人类用户消息时，文件跳过。
- 非法 JSON 行跳过。
- 非法 timestamp 行跳过。
- 单行最大扫描 buffer 当前设置为 10 MiB。

消息内容支持：

- 字符串形式的 `message.content`。
- 数组形式的 `message.content`，只提取数组元素的顶层 `text` 字段并拼接。
- 数组元素没有顶层 `text` 时不产生用户消息文本。

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

这里的 `text` 在 `tool_result.content[]` 内部，不在 `message.content[]` 元素顶层。当前实现不会把它当作人类输入，因此不会覆盖 `LastMsg`。

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
dir=yyy sessionId=xxxx createTime=xxxx lastTime=xxxx file=xxxx.jsonl firstMsg="..." lastMsg="..."
```

`file` 是完整 JSONL 会话文件路径。

消息摘要会做清洗：

- 控制字符、空白、双引号、反斜杠替换为空格。
- 连续空白合并。
- 去掉首尾空白。
- 截断到最多 10 个 Unicode 字符。

单引号保留，因为它不是当前输出格式的分隔符。

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

## Codex 状态

当前实现没有 Codex provider。

`-t codex` 会返回明确错误：

```text
codex provider is not implemented in this phase
```

Codex 的数据源、解析规则和测试应在第二阶段单独实现。
