# test

## claude

```shell
go build ./cmd/query-session

# 输出 当前目录 claude 所有日期最新 10 条 session
./query-session -t claude

# 输出 当前目录 claude 所有日期最后一个创建的 session
./query-session -t claude -n 1

# 输出 claude 所有日期的所有项目最新 10 条 session，-p 是大小写忽略的正则匹配
./query-session -t claude -p ".*"

# 输出 claude 所有日期的所有项目的全局最后一个创建
./query-session -t claude -p ".*" -n 1

# 输出 claude 指定 时间内 指定 正则项目的  
./query-session -t claude -p "aiAgent"  -s 20260513 -e 20260514

# -p 匹配过滤， 而 -x 是排除过滤 -x 的优先级比 -p 高  ， -x 是大小写忽略的正则匹配
./query-session -t claude -p "git" -x 'aiagent' -s 20260513 -e 20260514

```


## codex

```shell

go build ./cmd/query-session


# 输出 当前目录 codex 所有日期最新 10 条 session
./query-session -t codex

# 输出 当前目录 codex 所有日期最后一个创建的 session
./query-session -t codex -n 1

# 输出 codex 所有日期的所有项目最新 10 条 session，-p 是大小写忽略的正则匹配
./query-session -t codex -p ".*"

# 输出 codex 所有日期的所有项目的全局最后一个创建
./query-session -t codex -p ".*" -n 1

# 输出   指定 时间内 指定 正则项目的  
./query-session -t codex -p ".*"  -s 20260513 -e 20260514

# -p 匹配过滤， 而 -x 是排除过滤 -x 的优先级比 -p 高  ， -x 是大小写忽略的正则匹配
./query-session -t codex -p ".*" -x 'aiagent' -s 20260513 -e 20260514

```




## cursor

```shell

go build ./cmd/query-session


# 输出 当前目录 cursor 所有日期最新 10 条 session
./query-session -t cursor

# 输出 当前目录 cursor 所有日期最后一个创建的 session
./query-session -t cursor -n 1

# 输出 cursor 所有日期的所有项目最新 10 条 session，-p 是大小写忽略的正则匹配
./query-session -t cursor -p ".*"

# 输出 cursor 所有日期的所有项目的全局最后一个创建
./query-session -t cursor -p ".*" -n 1

# 输出   指定 时间内 指定 正则项目的  
./query-session -t cursor -p ".*"  -s 20260519 -e 20260520

# -p 匹配过滤， 而 -x 是排除过滤 -x 的优先级比 -p 高  ， -x 是大小写忽略的正则匹配
./query-session -t cursor -p ".*" -x 'query-session' -s 20260519 -e 20260520

```

## copilot

```shell
# 默认查询初始 cwd 为当前目录的 Copilot CLI 会话
./query-session

# 匹配所有项目（扫描匹配项目的完整事件日志）
./query-session -t copilot -p ".*" -n 1

# 指定项目、时间和排除规则
./query-session -t copilot -p 'query-session' -x 'archived' -l 7 -n 0
```

四种 provider 均输出 `title="..."`，不再输出 `firstMsg`、`lastMsg`；缺少原生标题时回退首条用户输入，零消息会话可显示 `未命名`。

回归与端到端测试（使用临时目录合成会话，不依赖本机 Agent 数据）：

```shell
go test ./... -count=1
go test ./cmd/query-session -run '^TestCLIBinaryEndToEnd$' -count=1
```
