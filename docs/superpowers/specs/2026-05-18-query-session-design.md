# query-session 设计

## 目标

构建一个名为 `query-session` 的 Go CLI 二进制，用于从本地 JSONL 会话文件中查询 Claude 和 Codex 的会话元信息。

这个工具用于快速人工查看。输出应包含会话目录、会话 ID、第一条和最后一条用户消息摘要，以及对应时间。

## 范围

包含：

- 从 `$HOME/.claude/projects` 查询 Claude 会话。
- 从 `$HOME/.codex/sessions` 查询 Codex 会话。
- 支持按 provider、项目目录、创建日期范围过滤。
- 支持只返回最新创建的一个会话。
- 每个会话输出一行，面向人工阅读。
- 为 provider 解析、过滤、排序、消息清洗补充单元测试。

不包含：

- JSON 输出。
- 交互式 UI。
- 解析 assistant 或 tool 消息。
- 支持 Claude 和 Codex 之外的 provider。
- 为已删除项目路径恢复维护缓存。

## CLI

二进制：

```text
query-session
```

参数：

- `-t`：provider 类型。默认 `claude`。有效值：`claude`、`codex`。
- `-d`：开启 debug 日志，输出到 stderr。默认 `false`。
- `-l`、`--last`：在所有过滤后只返回最新创建的会话。默认 `true`。
- `-p`、`--project`：项目目录大小写不敏感正则。默认空。
- `-s`、`--start-day`：开始日期，格式 `YYYYMMDD`。默认本地今天。
- `-e`、`--end-day`：结束日期，格式 `YYYYMMDD`。默认本地今天。

校验：

- 拒绝未知 provider。
- 拒绝非法日期格式。
- 拒绝非法项目正则。
- 拒绝 `start-day > end-day`。

## 数据模型

所有 provider 解析器返回统一的内部会话结构：

```text
SessionID
Dir
CreateTime
LastTime
FirstMsg
LastMsg
```

`CreateTime` 和 `LastTime` 从用户消息时间戳解析，不使用文件修改时间。

## Claude Provider

根目录：

```text
$HOME/.claude/projects
```

项目发现：

- 每个一级子目录是一个 Claude 编码后的项目目录。
- 只读取该项目目录内一级 `*.jsonl` 文件作为会话。
- 忽略 `<session-id>/` 这类子目录。

会话 ID：

- 使用文件名去掉 `.jsonl` 后的 basename。

目录解码：

- 从文件系统根目录开始，按最长存在路径前缀解码 Claude 编码目录。
- `-` 可能表示路径分隔符。
- `--` 可能表示隐藏目录片段，例如 `.hermes`。
- 一旦某个路径层级无法在磁盘上确认存在，就停止解码剩余后缀。
- 剩余后缀原样追加，不再继续翻译后续的 `-` 或 `--`。

示例：

```text
encoded: -Users-weizhoulan-Documents-git-query-session
known existing prefix: /Users/weizhoulan/Documents/git
decoded: /Users/weizhoulan/Documents/git/query-session
```

消息解析：

- 按行读取 JSONL 文件。
- 只关心 `message.role == "user"` 的记录。
- 第一条用户记录提供 `CreateTime` 和 `FirstMsg`。
- 最后一条用户记录提供 `LastTime` 和 `LastMsg`。
- 没有用户消息的文件跳过。

## Codex Provider

根目录：

```text
$HOME/.codex/sessions
```

目录发现：

- Codex 会话存储在 `YYYY/MM/DD/*.jsonl` 下。
- 日期范围参数只用于访问匹配日期目录，减少扫描量。
- 不从文件名提取会话 ID 或时间戳。

会话 ID：

- 使用第一条包含 `payload.id` 的 JSONL 记录。
- 通常自然来自 `session_meta` 记录。
- 如果没有 `payload.id`，跳过该文件。

目录：

- 使用第一条包含 `payload.cwd` 的 JSONL 记录。
- 如果没有 `payload.cwd`，`Dir` 保持为空。

消息解析：

- 只关心 `payload.role == "user"` 的记录。
- 不要求特定的顶层 `type` 或 `payload.type`。
- 第一条用户记录提供 `CreateTime` 和 `FirstMsg`。
- 最后一条用户记录提供 `LastTime` 和 `LastMsg`。
- 没有用户消息的文件跳过。

Codex 消息内容：

- 如果 `payload.content` 是字符串，直接使用。
- 如果 `payload.content` 是数组，拼接其中可读文本值。

## 过滤

日期过滤：

- 日期按本地时区解释。
- `start-day` 和 `end-day` 都包含边界当天。
- 基于 `CreateTime` 过滤。

项目过滤：

- 如果 `-p/--project` 为空，只包含 `Dir` 精确等于当前工作目录的会话。
- 如果 `-p/--project` 非空，作为大小写不敏感正则匹配 `Dir`。
- 因此 `.*` 会匹配所有目录。

最新会话过滤：

- 如果 `--last=true`，在其他所有过滤后选择 `CreateTime` 最新的会话。
- 如果 `--last=false`，输出所有过滤后的会话。

## 排序

输出多个会话时：

- 先按 `Dir` 升序排序。
- 相同 `Dir` 再按 `CreateTime` 升序排序。

当 `--last=true` 时，只输出一个会话，排序没有可见影响。

## 输出

每个会话输出一行：

```text
dir=yyy sessionId=xxxx createTime=xxxx lastTime=xxxx firstMsg="..." lastMsg="..."
```

时间格式：

```text
YYYYMMDD_HH:mm:ss
```

JSONL 中的时间戳输出前转换为本地时间。

消息摘要：

- 将换行、制表符、引号、反斜杠，以及其他控制字符或特殊字符替换为空格。
- 合并连续空白。
- 去掉首尾空白。
- 最多保留 10 个 Unicode 字符。
- 保留足够可读内容，用于识别会话主题。
- 不追求原始消息的精确序列化。

## 日志

当 `-d=true` 时，日志输出到 stderr。

格式：

```text
[info] message
[error] message
```

Debug 日志应包含扫描目录、跳过文件、解析错误、已解析会话基础信息。

## 错误处理

- provider 根目录不存在时报错。
- CLI 参数非法时报错。
- 非法 JSONL 行跳过；开启 debug 时记录日志。
- 没有可用用户消息的会话文件跳过。
- 没有 `payload.id` 的 Codex 会话文件跳过。

## 测试

Claude 测试：

- 解码普通项目路径。
- 解码隐藏目录路径。
- 在最长存在前缀后停止解码，并原样追加剩余后缀。
- 忽略会话子目录。
- 提取第一条和最后一条 `message.role == "user"` 记录。
- 跳过没有用户消息的文件。

Codex 测试：

- 从 `payload.id` 提取会话 ID，而不是从文件名提取。
- 从第一条 `payload.cwd` 提取目录。
- 提取第一条和最后一条 `payload.role == "user"` 记录。
- 支持字符串形式的 `payload.content`。
- 支持数组形式的 `payload.content`，并提取可读文本部分。
- 跳过没有用户消息的文件。

通用测试：

- 校验日期解析，并拒绝 `start-day > end-day`。
- 当项目正则为空时，按当前目录精确过滤。
- 当项目正则存在时，按大小写不敏感正则过滤。
- 按 `CreateTime` 选择最新会话。
- 多会话按 `Dir`、`CreateTime` 排序。
- 清洗消息摘要，并截断到 10 个 Unicode 字符。
