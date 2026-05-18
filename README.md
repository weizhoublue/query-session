# query-session

## 意图

我想做一个 能够查询 claude 、 codex 的会话 id 和信息的 二进制

- 使用 golang 编写
- 配备完备的 单元测试



## claude 会话 id 和内容的查询 思路

参考 query-claude.sh 中的实现 

 Claude Desktop 客户端的会话数据存储在 ~/.claude/projects 目录下

目录提取：
    目录结构:
    ~/.claude/projects/ 目录下是每一个 按照工作目录 分类的 会话

    ~/.claude/projects/<dirName> 是 目录 dirName 下的所有历史会话

    其中， <dirName>  有点特别
    <dirName>=-Users-weizhoulan-Documents-forkgit-istio 
    其 真实 工作目录是 /Users/weizhoulan/Documents/forkgit/istio
    把他 / 转为了 - 

    特殊情况处理:
        <dirName>  连续 "--" 表示隐藏目录: 前缀部分移除末尾的 "-",后缀部分加 "." 前缀
        例如: -Users-weizhoulan--hermes-skills -> /Users/weizhoulan/.hermes/skills


会话提取
    一个目录下大的 所有 jsonl 文件，就是 一个 会话 ， xxx.jsonl 其中的 xxx 就是会话 id
   ~/.claude/projects/-Users-weizhoulan-Documents-forkgit-istio/
                                           ├── xxx.jsonl   (会话文件)
                                           ├── yyy.jsonl
                                           └── ...
 
   会遇到 如下 同名 id 的目录，我发现 目录中 是 该 会话的 subagent 的子会话，我们可以不用关心，只关心主会话
   ~/.claude/projects/-Users-weizhoulan-Documents-forkgit-istio/<id> 
   ~/.claude/projects/-Users-weizhoulan-Documents-forkgit-istio/<id>.jsonl

会话内容
   ~/.claude/projects/-Users-weizhoulan-Documents-forkgit-istio/<id>.jsonl
   文件中每一行都一个 会话中的 消息记录， 是 json 格式

   我们可以 只关心  message.role=user 的 行，这是用户输入的消息记录，  
   把 文件中 从第一行开始 往后 寻找，  第一个出现   message.role=user  的 记录，其中 消息中的 timestamp 作为会话的创建时间 
   把 文件中 从最后一行 开始 往前 寻找，  第一个出现   message.role=user  的 记录，其中 消息中的 timestamp 作为会话最后一次的用户问题

        {
        "parentUuid": "6da920be-24fe-4c7c-80ac-5bbb6663e5d4",
        "isSidechain": false,
        "promptId": "71c1a37a-269a-4bbf-aa5f-97b4174c38e9",
        "type": "user",
        "message": {
            "role": "user",
            "content": "<command-message>superpowers:brainstorming</command-message>\n<command-name>/superpowers:brainstorming</command-name>\n<command-args> 理解 @README.md  中的需求</command-args>"
        },
        "uuid": "1692067e-9084-4b3b-9dcc-70a4f005e6bf",
        "timestamp": "2026-05-17T01:36:08.400Z",
        "userType": "external",
        "entrypoint": "cli",
        "cwd": "/Users/weizhoulan/Documents/git/commu",
        "sessionId": "b5452b80-f512-45f6-b281-7615cf137ce4",
        "version": "2.1.142",
        "gitBranch": "main"
        }


## codex 会话 id 和内容的查询 思路

所有目录的所有会话， 是基于 时间 来分目录的
    ~/.codex/sessions/年份/月份/日    ， 目录下包含了当天创建的 所有 项目的   jsonl 文件

     ~/.codex/sessions/年份/月份/日/rollout-2026-05-18T13-58-29-019e39aa-17f0-7a90-8436-f968107475ab.jsonl

    从文件名中，我们就可以看出 创建时间是  2026-05-18T13-58-29  ，会话 id 是 019e39aa-17f0-7a90-8436-f968107475ab

    jsonl 文件中就是 会话的 内容 

    我们可以只关心  payload.role=user 这种 用户输入消息
    {"timestamp":"2026-05-18T05:58:35.786Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"tomorow is monday"}]}}

    我们可以从文件第一行 开始往下寻找，找到第一个用户输入的信息 和输入时间 
    我们可以从文件第一行 开始往上寻找，找到用户最后一个输入的信息 和 输入时间


    会话目录， 从第一个 能 有 payload.cwd 内容的 行中，可以提取出目录 /Users/weizhoulan/Documents/git/commu 
    {
  "timestamp": "2026-05-17T16:35:23.273Z",
  "type": "session_meta",
  "payload": {
    "id": "019e36ca-9b12-71c3-821a-cdaccf78db35",
    "timestamp": "2026-05-17T16:35:08.178Z",
    "cwd": "/Users/weizhoulan/Documents/git/commu",
    "originator": "codex_exec",
    "cli_version": "0.130.0",
    "source": "exec",
    "model_provider": "openai",


## 二进制 使用


query-session 
    -t claude    , 可选，默认 claude ，可支持 claude 和 codex 。  当时 claude 时，基于 claude 的会话信息寻找原理来找 。 当 codex 时，基于 codex 的会话信息寻找原理来找 。  

    -d true / false ，  开启日志， 日志输出到 标准错误输出 , 其中可展示 遍历到的每一个 会话 目录、id、创建时间
            格式格式：[info/error] message


    有如下 几 个 过滤展示的 选项， 他们的效果是相互叠加的 ：

        -l --last true/false ， 默认 true,  如果为true ，表示在其他所有过滤条件的基础上，只展示 最新创建的那一个 会话 

        -p --project '正则式'   默认无。 表示对 会话目录名 进行 过滤 
            当 -p 为空 时，基于 当前运行命令所在目录，过滤出 当前目录的会话，  
            如果 -p '正则式' ， 那么 过滤出 会话目录名 匹配 正则式 的会话， 而不受限于当前目录 ， 通配是忽略大小写的
            如果 -p '.*' ， 那么展示所有目录的会话

        -s --start-day 20260517       基于所有项目的会话的创建时间进行 会话展示过滤 , 会话的创建时间是从这一天之后的（包括这天），那么可展示。 默认值 为空，表示今天
        -e --end-day 20260518      基于所有项目的会话的创建时间进行 会话展示过滤 , 会话的创建时间是到这一天之前的（包括这天），那么可展示。 默认值 为空，表示今天



命令查询出来 每一行的格式如下，其中，按照 dir 进行 排序， 相同 dir 的，按照 createTime 排序

dir=yyy sessionId=xxxx createTime=xxxx lastTime=xxxx firstMsg="用户提问的第一个问题， 最多 10 个字符" lastMsg="用户最后一次的问题，最多 10 个字符"



## 开发步骤建议

1 先对 claude 、 codex 会话 id 收集和 会话内容 的原理进行验证 ， 并 满足 功能的 最优 变量 会话记录 并满足过滤的 算法， 以确保性能
2 先实现 claude
3 实现 codex


