# test

## claude

```shell
go build ./cmd/query-session

# 输出 当前目录  claude 今天的所有 session
./query-session

# 输出 当前目录  claude 今天的最后一个创建的 session
./query-session -l

# 输出 claude 今天的 所有项目的 session ，  -p 是大小写忽略的正则匹配
./query-session -p ".*"

# 输出 claude 今天的 所有项目的 session 的 全局最后一个创建
./query-session -p ".*" -l

# 输出 claude 指定 时间内 指定 正则项目的  
./query-session -p "aiAgent"  -s 20260513 -e 20260514

# -p 匹配过滤， 而 -x 是排除过滤 -x 的优先级比 -p 高  ， -x 是大小写忽略的正则匹配
./query-session -p "git" -x 'aiagent' -s 20260513 -e 20260514

```


## codex

```shell

go build ./cmd/query-session


# 输出 当前目录    今天的所有 session
./query-session -t codex

# 输出 当前目录    今天的最后一个创建的 session
./query-session -t codex -l

# 输出   今天的 所有项目的 session ，  -p 是大小写忽略的正则匹配
./query-session -t codex -p ".*"

# 输出  今天的 所有项目的 session 的 全局最后一个创建
./query-session -t codex -p ".*" -l

# 输出   指定 时间内 指定 正则项目的  
./query-session -t codex -p ".*"  -s 20260513 -e 20260514

# -p 匹配过滤， 而 -x 是排除过滤 -x 的优先级比 -p 高  ， -x 是大小写忽略的正则匹配
./query-session -t codex -p ".*" -x 'aiagent' -s 20260513 -e 20260514

```



