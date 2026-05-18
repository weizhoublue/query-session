# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

query-session 是一个查询 Claude Desktop 会话 ID 和信息的 CLI 工具，用于从本地会话文件中提取会话信息并进行过滤展示。

## 常用命令

```bash
# 构建
go build -o query-session ./cmd/query-session

# 运行测试
go test ./...

# 运行单个测试文件
go test -v ./internal/session/... -run TestFilter

# 调试模式运行
go run ./cmd/query-session -d -p "istio"
```

## 架构

- `cmd/query-session/main.go` - CLI 入口，参数解析与过滤器
- `internal/claude/claude.go` - Claude Desktop 会话扫描实现，解析 `~/.claude/projects/` 目录
- `internal/session/session.go` - 会话数据结构与日期范围过滤逻辑

核心流程：Scan → Filter → Sort/Format

Claude 会话存储在 `~/.claude/projects/<encoded-dir>/<session-id>.jsonl`，目录名需解码（如 `-Users-weizhoulan-Documents-git-xxx` → `/Users/weizhoulan/Documents/git/xxx`）。