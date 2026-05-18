# 快速开始

`query-session` 用于查询本机 Claude 和 Codex 会话信息。

## 构建

```bash
go build ./cmd/query-session
```

构建后会在当前目录生成：

```text
query-session
```

## 基本用法

查询当前目录今天的全部 Claude 会话：

```bash
./query-session
```

等价于：

```bash
./query-session -t claude -l=false
```

当前 CLI 参数：

```text
-d / --debug
-e / --end-day
-l / --last
-p / --project
-s / --start-day
-t / --type
```

`-l` / `--last` 默认是 `false`，所以不带 `-l` 时会输出所有匹配会话。

## 输出格式

每个会话输出一行：

```text
dir=yyy sessionId=xxxx createTime=xxxx lastTime=xxxx file=xxxx.jsonl firstMsg="..." lastMsg="..."
```

字段含义：

- `dir`：会话所属项目目录。
- `sessionId`：Claude 会话 ID。
- `createTime`：第一条用户消息时间。
- `lastTime`：最后一条用户消息时间。
- `file`：完整 JSONL 会话文件路径。
- `firstMsg`：第一条用户消息摘要，最多 10 个 Unicode 字符。
- `lastMsg`：最后一条用户消息摘要，最多 10 个 Unicode 字符。

时间格式：

```text
YYYYMMDD_HH:mm:ss
```

## 查询当前目录全部会话

默认 `-p` 为空，会精确匹配当前运行命令所在目录。

```bash
./query-session
```

多行结果按 `dir` 升序排序，相同 `dir` 按 `createTime` 升序排序。

## 查询指定项目

`-p` 或 `--project` 使用大小写不敏感正则匹配 `dir`。

```bash
./query-session -p 'query-session' -l=false
```

查询所有项目：

```bash
./query-session -p '.*' -l=false
```

## 查询指定日期

`-s` / `--start-day` 和 `-e` / `--end-day` 使用 `YYYYMMDD`。

查询某一天：

```bash
./query-session -s 20260518 -e 20260518 -p '.*' -l=false
```

查询日期范围：

```bash
./query-session -s 20260517 -e 20260518 -p '.*' -l=false
```

日期按本地时区解释，起止日期都包含边界当天。

如果 `-s` 晚于 `-e`，命令会报错。

## 只查询最新创建的会话

`-l` / `--last` 默认是 `false`。需要最新创建的一个会话时，显式开启：

```bash
./query-session -p '.*' -l=true
```

这会在所有过滤条件之后，按 `createTime` 选择最新创建的一个会话。

显式关闭或使用默认值：

```bash
./query-session -p '.*' -l=false
```

## 开启 debug 日志

debug 日志输出到 stderr。

```bash
./query-session -d=true -p '.*' -l=false
```

日志格式：

```text
[info] message
[error] message
```

debug 日志会展示：

- 扫描到的 Claude 项目目录。
- 扫描到的 JSONL 会话文件。
- 成功解析出的会话。
- 因没有人类用户消息被跳过的文件。
- 过滤命中或过滤原因。
- `-l=true` 时最终选择的最新会话。

## 查询 Codex 会话

使用 `-t codex` 切换到 Codex provider：

```bash
./query-session -t codex -p '.*' -l=false
```

Codex 会话 ID 优先取自 `payload.id`，回退到文件名。目录取自 `payload.cwd`。消息内容取 `payload.content` 数组中第一个 `input_text`。

其他过滤、排序、输出格式与 Claude 一致。
