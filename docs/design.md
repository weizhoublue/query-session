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

当前只关心：

```text
message.role == "user"
```

解析规则：

- 第一条用户消息提供 `CreateTime` 和 `FirstMsg`。
- 最后一条用户消息提供 `LastTime` 和 `LastMsg`。
- 没有用户消息的文件跳过。
- 非法 JSON 行跳过。
- 非法 timestamp 行跳过。
- 单行最大扫描 buffer 当前设置为 10 MiB。

消息内容支持：

- 字符串形式的 `message.content`。
- 数组形式的 `message.content`，提取其中的 `text` 字段并拼接。

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
dir=yyy sessionId=xxxx createTime=xxxx lastTime=xxxx firstMsg="..." lastMsg="..."
```

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

## Codex 状态

当前实现没有 Codex provider。

`-t codex` 会返回明确错误：

```text
codex provider is not implemented in this phase
```

Codex 的数据源、解析规则和测试应在第二阶段单独实现。
