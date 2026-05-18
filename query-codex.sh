#!/bin/bash
#
# 查询 Codex 会话记录
#
# 算法说明：
#   1. 遍历 ~/.codex/sessions/年份/月份/日 目录下的 jsonl 文件
#   2. 从文件名提取 session_id 和创建时间
#   3. 从 jsonl 文件中 grep 提取 cwd 字段作为 dir
#   4. 如果指定 DIR_PATTERN（参数1），则对 dir 进行正则匹配过滤（忽略大小写）
#   5. 输出格式：session_id=xxx, time=xxx, dir=xxx
#   6. 如果 PRINT_LAST=true，则只输出最新一条记录
#
# 使用示例：
#   ./query-codex-session.sh                    # 查询今天（默认1天）
#   DURATION_DAY=2 ./query-codex-session.sh     # 查询昨天和今天（2天）
#   ./query-codex-session.sh "dynamo"           # 查询今天，过滤 dir 包含 "dynamo"
#   DURATION_DAY=2 ./query-codex-session.sh "hermes"  # 查询2天，过滤 dir 包含 "hermes"
#   PRINT_LAST=true ./query-codex-session.sh    # 只显示最新一条
#   PRINT_LAST=true ./query-codex-session.sh "hermes"  # 过滤并只显示最新
#
# 参数说明：
#   $1: DIR_PATTERN - 目录正则过滤（可选），支持正则表达式，忽略大小写
#   环境变量 DURATION_DAY: 查询天数（默认1）
#   环境变量 PRINT_LAST: 是否只显示最新一条（默认true）
#

SESSIONS_DIR="$HOME/.codex/sessions"
DIR_PATTERN="${1:-}"

DURATION_DAY=${DURATION_DAY:-1}
PRINT_LAST=${PRINT_LAST:-false}

if ! [[ "$DURATION_DAY" =~ ^[1-9][0-9]*$ ]]; then
    echo "Error: DURATION_DAY must be a positive integer >= 1" >&2
    exit 1
fi

if [[ "$PRINT_LAST" != "true" && "$PRINT_LAST" != "false" ]]; then
    echo "Error: PRINT_LAST must be 'true' or 'false'" >&2
    exit 1
fi

if [[ "$DIR_PATTERN" == "-h" || "$DIR_PATTERN" == "--help" ]]; then
    sed -n '2,24p' "$0"
    exit 0
fi

if [ ! -d "$SESSIONS_DIR" ]; then
    echo "Error: Sessions directory not found: $SESSIONS_DIR" >&2
    exit 1
fi

if [[ "$(uname)" == "Darwin" ]]; then
    date_func() { date -v-${1}d +%Y/%m/%d; }
else
    date_func() { date -d "${1} days ago" +%Y/%m/%d; }
fi

for ((i=0; i<DURATION_DAY; i++)); do
    day_date=$(date_func $i)
    day_path="$SESSIONS_DIR/$day_date"

    if [ -d "$day_path" ]; then
        for file in "$day_path"/*.jsonl; do
            [ -e "$file" ] || continue
            filename=$(basename "$file" .jsonl)
            info=$(echo "$filename" | sed 's/^rollout-\([0-9-]*T[0-9-]*\)-\([a-f0-9-]*\)$/\1 \2/')
            raw_timestamp=$(echo "$info" | awk '{print $1}')
            timestamp=$(echo "$raw_timestamp" | awk -F'T' '{gsub(/-/, ":", $2); print $1 "_" $2}')
            session_id=$(echo "$info" | awk '{print $2}')

            dir=$(grep '"cwd":' "$file" 2>/dev/null | head -1 | sed 's/.*"cwd": *"\([^"]*\)".*/\1/')

            if [ -n "$DIR_PATTERN" ]; then
                if ! echo "$dir" | grep -E -i "$DIR_PATTERN" > /dev/null 2>&1; then
                    continue
                fi
            fi

            echo "session_id=$session_id, time=$timestamp, dir=$dir"
        done
    fi
done | sort | {
    if [ "$PRINT_LAST" = "true" ]; then
        tail -n 1
    else
        cat
    fi
}