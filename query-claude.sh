#!/bin/bash
#==============================================================================
# 脚本名称: query-agent-session.sh
# 功能描述: 扫描 Claude 最近 N 天的活跃会话,并按项目输出会话信息
#
#----------------------------------------
# Claude Session 文件原理说明:
#----------------------------------------
# Claude Desktop 客户端的会话数据存储在 ~/.claude/projects 目录下
#
# 目录结构:
#   ~/.claude/projects/-Users-weizhoulan-Documents-forkgit-istio/
#                                           ├── xxx.jsonl   (会话文件)
#                                           ├── yyy.jsonl
#                                           └── ...
#
# 命名规则:
#   - 项目目录名: 使用 "-" 拼接完整路径,开头带用户名前缀
#     例如: -Users-weizhoulan-Documents-forkgit-istio
#   - 会话文件名: UUID 格式 + .jsonl 后缀
#     例如: 0bfdc4b7-0e5f-498e-b89a-034377304bf9.jsonl
#
# 会话识别:
#   - 每个 .jsonl 文件代表一个会话,其文件名中的 UUID 即为 session_id
#   - 文件的最后修改时间 (mtime) 即为会话的最后活跃时间
#   - 通过文件的修改时间筛选最近 N 天内的活跃会话
#
# 使用方式:
#   ./query-agent-session.sh [PROJECT_PATTERN]
#   DURATION_DAY=2 PRINT_LAST=false ./query-agent-session.sh forkgit
#
# 参数说明:
#   位置参数:
#     $1 PROJECT_PATTERN - 项目名正则匹配,不区分大小写 (可选)
#
#   环境变量:
#     DURATION_DAY - 扫描最近多少天内的会话 (默认: 1, 即今天)
#     PRINT_LAST   - 是否只输出每个项目的最新会话 true/false (默认: true)
#
# 输出格式:
#   session_id=xxx, time=yyyymmdd_hh:mm:ss, dir=xxx
#==============================================================================

PROJECT_PATTERN=${1:-""}

DURATION_DAY=${DURATION_DAY:-1}
PRINT_LAST=${PRINT_LAST:-false}

PROJECTS_DIR=""
CUTOFF=0


usage() {
    echo "用法: $0 [PROJECT_PATTERN]"
    echo ""
    echo "位置参数:"
    echo "  PROJECT_PATTERN 项目名正则过滤 (可选)"
    echo ""
    echo "环境变量:"
    echo "  DURATION_DAY  扫描最近多少天内的会话 (默认: 1, 即今天)"
    echo "  PRINT_LAST    只显示最新会话 (true/false, 默认: true)"
    echo ""
    echo "示例:"
    echo "  $0"
    echo "  $0 forkgit"
    echo "  DURATION_DAY=2 $0"
    echo "  PRINT_LAST=false $0 forkgit"
}


validate_args() {
    if [[ -n "$DURATION_DAY" && ! "$DURATION_DAY" =~ ^[1-9][0-9]*$ ]]; then
        echo "错误: DURATION_DAY 必须是正整数 (>=1), 实际值: $DURATION_DAY"
        exit 1
    fi

    if [[ -n "$PROJECT_PATTERN" ]]; then
        if ! grep -qE "$PROJECT_PATTERN" <<< "x" 2>/dev/null; then
            if [[ $? -eq 2 ]]; then
                echo "错误: PROJECT_PATTERN 不是有效的正则表达式: $PROJECT_PATTERN" >&2
                exit 1
            fi
        fi
    fi

    if [[ "$PRINT_LAST" != "true" && "$PRINT_LAST" != "false" ]]; then
        echo "错误: PRINT_LAST 必须是 true 或 false, 实际值: $PRINT_LAST"
        exit 1
    fi
}


init_env() {
    PROJECTS_DIR="$HOME/.claude/projects"

    if [[ ! -d "$PROJECTS_DIR" ]]; then
        echo "错误: 项目目录不存在: $PROJECTS_DIR" >&2
        echo "请确认 Claude Desktop 已运行并创建过会话" >&2
        exit 1
    fi

    local today_0
    if [[ "$(uname)" == "Darwin" ]]; then
        today_0=$(date -j -f "%Y-%m-%d %H:%M:%S" "$(date +%Y-%m-%d) 00:00:00" +%s)
    else
        today_0=$(date -d "$(date +%Y-%m-%d)" +%s)
    fi

    local days_ago=$((DURATION_DAY - 1))
    CUTOFF=$((today_0 - days_ago * 86400))
}


get_file_mtime() {
    local file=$1
    if [[ "$(uname)" == "Darwin" ]]; then
        stat -f "%m" "$file"
    else
        stat -c "%Y" "$file"
    fi
}

#------------------------------------------------------------------------------
# 函数: get_file_mtime_formatted
# 功能: 获取文件的修改时间(格式化为 YYYYMMDDHHMMSS),兼容 Linux 和 macOS
# 参数: $1 file - 文件路径
# 返回: 格式化后的时间字符串
#------------------------------------------------------------------------------
get_file_mtime_formatted() {
    local file=$1
    if [[ "$(uname)" == "Darwin" ]]; then
        stat -f "%Sm" -t "%Y%m%d_%H:%M:%S" "$file"
    else
        stat -c "%Y" "$file" | xargs -I {} date -d "@{}" +"%Y%m%d_%H:%M:%S"
    fi
}

#------------------------------------------------------------------------------
# 函数: convert_encoded_path
# 功能: 将 Claude 编码的目录名转换为真实路径
# 说明:
#   Claude Desktop 的项目目录命名规则:
#     - 以 "-" 开头,拼接完整路径
#     - 例如: -Users-weizhoulan-Documents-forkgit-dynamo
#     - 例如: -Users-weizhoulan--hermes-skills-blue-analyze-scripts
#
#   转换原理 (从左到右逐个级别尝试):
#     1. 遍历每个 "-" 位置,将该位置的 "-" 替换为 "/"
#     2. 将剩余部分的 "-" 也替换为 "/"
#     3. 检查转换后的路径是否存在
#     4. 选择存在且路径最长(最具体)的作为最终结果
#
#   特殊情况处理:
#     - 连续 "--" 表示隐藏目录: 前缀部分移除末尾的 "-",后缀部分加 "." 前缀
#       例如: -Users-weizhoulan--hermes-skills -> /Users/weizhoulan/.hermes/skills
#     - 如果找不到对应目录,保持原始编码字符串输出
#
# 参数: $1 encoded_name - 编码的目录名
# 返回: 转换后的真实路径,或原始编码字符串
#------------------------------------------------------------------------------
convert_encoded_path() {
    local encoded_name=$1
    local valid_path=""
    local max_len=0

    for ((i=0; i<${#encoded_name}; i++)); do
        if [[ "${encoded_name:$i:1}" == "-" ]]; then
            local prefix="${encoded_name:0:i}"
            local suffix="${encoded_name:i+1}"

            # 检查是否出现 -- (即 prefix 以 - 结尾,表示隐藏目录)
            if [[ "$prefix" =~ .*-$ ]]; then
                prefix="${prefix%*-}"
                suffix=".${suffix//-//}"
            fi

            local real_path="/${prefix//-//}/$suffix"

            while [[ "$real_path" == //* ]]; do
                real_path="${real_path#/}"
            done

            if [[ -d "$real_path" && ${#real_path} -gt $max_len ]]; then
                valid_path="$real_path"
                max_len=${#real_path}
            fi
        fi
    done

    if [[ -n "$valid_path" ]]; then
        echo "$valid_path"
    else
        echo "$encoded_name"
    fi
}

#------------------------------------------------------------------------------
# 函数: collect_sessions
# 功能: 收集所有符合条件的会话信息到全局数组
# 说明: 扫描所有项目目录,收集满足时间和模式过滤的会话
# 参数: 无
# 返回: 无 (填充 SESSIONS 数组)
#------------------------------------------------------------------------------
SESSIONS=()

collect_sessions() {
    for project_dir in "$PROJECTS_DIR"/*/; do
        local project_name=$(basename "$project_dir")
        [[ "$project_name" == "*" ]] && continue

        local real_path=$(convert_encoded_path "$project_name")

        if [[ -n "$PROJECT_PATTERN" ]]; then
            local lower_real=$(echo "$real_path" | tr '[:upper:]' '[:lower:]')
            local lower_pattern=$(echo "$PROJECT_PATTERN" | tr '[:upper:]' '[:lower:]')
            if [[ ! "$lower_real" =~ $lower_pattern ]]; then
                continue
            fi
        fi

        for file in "$project_dir"*.jsonl; do
            [[ -f "$file" ]] || continue
            local file_mtime=$(get_file_mtime "$file")

            if [[ $file_mtime -ge $CUTOFF ]]; then
                local session_id=$(basename "$file" .jsonl)
                local mod_time=$(get_file_mtime_formatted "$file")
                SESSIONS+=("$file_mtime|$session_id|$mod_time|$real_path")
            fi
        done
    done
}

#------------------------------------------------------------------------------
# 函数: print_sessions
# 功能: 根据 PRINT_LAST 输出会话
# 说明: PRINT_LAST=true 时只输出最新那一个,否则输出全部
# 参数: 无
# 返回: 无
#------------------------------------------------------------------------------
print_sessions() {
    if [[ ${#SESSIONS[@]} -eq 0 ]]; then
        return
    fi

    if [[ "$PRINT_LAST" == "true" ]]; then
        local latest=""
        local latest_mtime=0
        for session in "${SESSIONS[@]}"; do
            local mtime=$(echo "$session" | cut -d'|' -f1)
            if [[ $mtime -gt $latest_mtime ]]; then
                latest_mtime=$mtime
                latest=$session
            fi
        done

        if [[ -n "$latest" ]]; then
            local session_id=$(echo "$latest" | cut -d'|' -f2)
            local mod_time=$(echo "$latest" | cut -d'|' -f3)
            local real_path=$(echo "$latest" | cut -d'|' -f4)
            echo "session_id=$session_id, time=$mod_time, dir=$real_path"
        fi
    else
        printf '%s\n' "${SESSIONS[@]}" | sort -t'|' -k1 -rn | while IFS='|' read -r _mtime session_id mod_time real_path; do
            echo "session_id=$session_id, time=$mod_time, dir=$real_path"
        done
    fi
}


main() {
    if [[ "$PROJECT_PATTERN" == "-h" || "$PROJECT_PATTERN" == "--help" ]]; then
        usage
        exit 0
    fi

    validate_args
    init_env
    collect_sessions
    print_sessions
}

main