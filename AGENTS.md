# Project Overview

`query-session` is a Go-based CLI application designed to parse and list local chat session data from AI clients (currently supporting Claude, with Codex planned for a future phase). It scans local JSONL files (e.g., in `~/.claude/projects`), extracts the session metadata (start/end times, first/last user messages), and supports filtering by project directory, date, and latest session.

## Architecture

- **`cmd/query-session`**: The CLI entry point. Responsible for parsing arguments, managing error outputs, and invoking the appropriate provider.
- **`internal/session`**: The core domain package containing the unified session models, filtering logic, and output formatting.
- **`internal/claude`**: The provider implementation for Claude. It handles path decoding of project directories and the parsing of `*.jsonl` session files to extract human-initiated messages.

## Tech Stack
- **Language**: Go 1.22
- **Testing**: standard `testing` package

## Building and Running

### Build the CLI
```bash
go build -o query-session ./cmd/query-session
```

### Run the CLI
```bash
./query-session -t claude -d true
```
> Note: Currently only the `claude` provider is implemented.

### Run Tests
```bash
go test ./...
```

## Development Conventions

- **Code Structure**: The project follows the standard Go layout with a clear separation of concerns between the CLI entry point (`cmd`) and internal logic (`internal`).
- **Testing**: New functionality and parsers (such as the upcoming Codex provider) must be accompanied by comprehensive unit tests. Existing tests can be found alongside the source code (e.g., `claude_test.go`, `filter_test.go`).
- **Parsing Strategy**: When parsing session logs, focus only on actual human user interactions (e.g., `message.role = "user"` and `message.content` is a string) while discarding tool outputs or internal mechanisms.
- **Error Handling**: Follow the established logging convention: normal errors output as `[error] message`, and debug logs (enabled via `-d true`) output as `[info] message` to standard error.