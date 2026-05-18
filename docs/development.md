# 开发调试

本文档记录当前 Claude 阶段的开发、测试和调试命令。

## 环境要求

- Go 1.22 或更高兼容版本。
- 本机存在 Claude 会话目录时，才能进行真实数据验证：

```text
$HOME/.claude/projects
```

## 常用命令

运行全部测试：

```bash
go test ./... -count=1
```

运行 session 包测试：

```bash
go test ./internal/session -count=1
```

运行 Claude provider 测试：

```bash
go test ./internal/claude ./internal/session -count=1
```

运行 CLI 测试：

```bash
go test ./cmd/query-session -count=1
```

构建：

```bash
go build ./cmd/query-session
```

清理本地构建产物：

```bash
rm -f query-session
```

格式化：

```bash
gofmt -w cmd/query-session internal/session internal/claude
```

检查当前实现文件是否有空白错误：

```bash
git diff --check -- cmd/query-session internal/session internal/claude docs
```

不要直接对全仓库运行 `git diff --check` 后清理所有问题；当前 `README.md` 有用户既有改动，不能顺手修改。

## 本地真实数据调试

查询当前目录今天全部 Claude 会话：

```bash
go run ./cmd/query-session
```

查询今天最新创建的 Claude 会话：

```bash
go run ./cmd/query-session -l=true
```

查询所有项目今天的全部 Claude 会话：

```bash
go run ./cmd/query-session -p '.*' -l=false
```

查询指定日期：

```bash
go run ./cmd/query-session -s 20260518 -e 20260518 -p '.*' -l=false
```

开启 debug：

```bash
go run ./cmd/query-session -d=true -p '.*' -l=false
```

debug 输出应该能看到扫描和过滤链路：

```text
[info] scan project encoded=... dir=... path=...
[info] scan file sessionId=... file=... dir=...
[info] parsed sessionId=... dir=... createTime=... lastTime=...
[info] matched sessionId=... dir=... createTime=... lastTime=...
```

如果某个会话没有输出，优先看：

- 是否出现 `scan file`，确认 JSONL 文件被扫描。
- 是否出现 `skip file reason=no-user-message`，确认没有可用的人类用户消息。
- 是否出现 `filtered reason=project`，确认项目目录过滤不匹配。
- 是否出现 `filtered reason=date`，确认创建时间不在日期范围内。

验证错误输出：

```bash
go run ./cmd/query-session -t nope
```

期望 stderr：

```text
[error] unknown provider: nope
```

验证 Codex 未实现：

```bash
go run ./cmd/query-session -t codex
```

期望 stderr：

```text
[error] codex provider is not implemented in this phase
```

## 推荐开发步骤

新增或修改行为时按以下顺序：

1. 先写或更新单元测试。
2. 运行目标包测试，确认测试能覆盖目标行为。
3. 实现最小代码。
4. 运行目标包测试。
5. 运行 `go test ./... -count=1`。
6. 运行 `go build ./cmd/query-session`。
7. 删除构建产物 `rm -f query-session`。
8. 用 `git diff --check -- <你改过的路径>` 检查自己的改动。
9. 使用 `git commit -s -S -m "One-line English summary"` 提交。

## 当前包职责

`internal/session`：

- 日期范围解析。
- 项目过滤。
- latest 选择。
- 排序。
- 输出格式。
- 消息摘要清洗。

`internal/claude`：

- 扫描 `$HOME/.claude/projects`。
- 解码 Claude 项目目录。
- 读取一级 JSONL 会话文件。
- 提取第一条和最后一条用户消息。
- 跳过 Claude 记录中的 `tool_result` 用户角色记录，避免工具返回内容覆盖 `LastMsg`。

## Claude 用户消息调试

Claude JSONL 中并不是所有 `message.role=user` 都是人类输入。

有效的人类用户消息需要同时满足：

- JSON 行可解析。
- `message.role == "user"`。
- `timestamp` 可解析。
- `message.content` 能提取出非空文本。

当前提取文本的规则：

- `message.content` 是字符串时直接使用。
- `message.content` 是数组时，只读取数组元素顶层 `text` 字段。
- `tool_result.content[].text` 不读取。

排查某个文件最后一条用户消息：

```bash
rg -n '"role":"user"|"role": "user"' /path/to/session.jsonl | tail -n 10
```

如果最后几条 `role=user` 是工具结果，应以最后一条真正的人类输入作为 `lastMsg`。

`cmd/query-session`：

- 解析 CLI 参数。
- 选择 provider。
- 调用过滤和输出逻辑。
- 统一错误输出。

## 第二阶段提示

Codex provider 不应混在 Claude 修复中实现。

第二阶段应单独增加：

- `internal/codex/codex.go`
- `internal/codex/codex_test.go`
- CLI 中 `-t codex` 的真实分支
- 对应文档更新
