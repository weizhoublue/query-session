# CLAUDE.md

本文件为在本仓库中工作的 AI 助手提供项目指引。

## 项目概述

`query-session` 是查询本机会话信息的 Go CLI，支持 **Claude Desktop**、**Codex**、**Cursor Agent** 三种 provider，统一过滤与一行式输出。

## 常用命令

```bash
# 构建
go build -o query-session ./cmd/query-session

# 测试
go test ./... -count=1

# Claude（默认）— 当前目录今天全部会话
go run ./cmd/query-session

# Codex
go run ./cmd/query-session -t codex -p ".*"

# Cursor
go run ./cmd/query-session -t cursor -p ".*"

# 调试
go run ./cmd/query-session -d -p "query-session" -t cursor
```

## 架构

- `cmd/query-session/main.go` — CLI 入口
- `internal/session/` — 会话模型、过滤、排序、输出
- `internal/claude/` — `~/.claude/projects/*.jsonl`
- `internal/codex/` — `~/.codex/sessions/YYYY/MM/DD/*.jsonl`
- `internal/cursor/` — `~/.cursor/chats/*/*/store.db`（`modernc.org/sqlite`）

流程：`Scan` → `Filter` → `Sort` → `FormatLine`

## 文档

- [docs/get-started.md](docs/get-started.md) — 使用说明
- [docs/design.md](docs/design.md) — 设计说明（三 provider）
- [docs/development.md](docs/development.md) — 开发调试
- [docs/test.md](docs/test.md) — 命令示例
