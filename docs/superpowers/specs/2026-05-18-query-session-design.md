# query-session Design

## Goal

Build a Go CLI binary named `query-session` that lists Claude and Codex session metadata from local JSONL session files.

The tool is for quick human inspection. It should show the session directory, session ID, first and last user message summaries, and their timestamps.

## Scope

In scope:

- Query Claude sessions from `$HOME/.claude/projects`.
- Query Codex sessions from `$HOME/.codex/sessions`.
- Filter by provider, project directory, and creation date range.
- Optionally return only the latest created session.
- Print one human-readable line per session.
- Include unit tests for provider parsing, filtering, sorting, and message cleanup.

Out of scope:

- JSON output.
- Interactive UI.
- Parsing assistant or tool messages.
- Supporting providers other than Claude and Codex.
- Persisting a cache for deleted project path recovery.

## CLI

Binary:

```text
query-session
```

Flags:

- `-t`: Provider type. Default `claude`. Valid values: `claude`, `codex`.
- `-d`: Enable debug logs to stderr. Default `false`.
- `-l`, `--last`: Return only the latest created session after all filters. Default `true`.
- `-p`, `--project`: Case-insensitive project directory regex. Default empty.
- `-s`, `--start-day`: Start date in `YYYYMMDD`. Default local today.
- `-e`, `--end-day`: End date in `YYYYMMDD`. Default local today.

Validation:

- Reject unknown provider values.
- Reject invalid date formats.
- Reject invalid project regex values.
- Reject `start-day > end-day`.

## Data Model

All provider parsers return the same internal session shape:

```text
SessionID
Dir
CreateTime
LastTime
FirstMsg
LastMsg
```

`CreateTime` and `LastTime` are parsed from user message timestamps, not file modification times.

## Claude Provider

Root directory:

```text
$HOME/.claude/projects
```

Project discovery:

- Each first-level child directory is a Claude encoded project directory.
- Only first-level `*.jsonl` files inside that project directory are sessions.
- Subdirectories such as `<session-id>/` are ignored.

Session ID:

- The session ID is the file basename without `.jsonl`.

Directory decoding:

- Decode the Claude encoded directory by matching the longest existing path prefix from the filesystem root.
- `-` may represent a path separator.
- `--` may represent a hidden directory segment such as `.hermes`.
- Once a path segment can no longer be verified on disk, stop decoding the remaining suffix.
- Append the remaining suffix as-is, without continuing to translate later `-` or `--`.

Example:

```text
encoded: -Users-weizhoulan-Documents-git-query-session
known existing prefix: /Users/weizhoulan/Documents/git
decoded: /Users/weizhoulan/Documents/git/query-session
```

Message parsing:

- Read the JSONL file line by line.
- Only records with `message.role == "user"` are relevant.
- The first such record provides `CreateTime` and `FirstMsg`.
- The last such record provides `LastTime` and `LastMsg`.
- Files with no user messages are skipped.

## Codex Provider

Root directory:

```text
$HOME/.codex/sessions
```

Directory discovery:

- Codex sessions are stored under `YYYY/MM/DD/*.jsonl`.
- The date range flags are used to visit only matching date directories.
- The filename is not used to extract session ID or timestamps.

Session ID:

- Use the first JSONL record that contains `payload.id`.
- Prefer the `session_meta` record naturally because it carries the session ID.
- If no `payload.id` exists, skip the file.

Directory:

- Use the first JSONL record that contains `payload.cwd`.
- If no `payload.cwd` exists, keep `Dir` empty.

Message parsing:

- Only records with `payload.role == "user"` are relevant.
- Do not require any specific top-level `type` or `payload.type`.
- The first such record provides `CreateTime` and `FirstMsg`.
- The last such record provides `LastTime` and `LastMsg`.
- Files with no user messages are skipped.

Codex message content:

- If `payload.content` is a string, use it directly.
- If `payload.content` is an array, concatenate readable text values from the array.

## Filtering

Date filtering:

- Dates are interpreted in the local timezone.
- `start-day` and `end-day` are inclusive.
- Filtering is based on `CreateTime`.

Project filtering:

- If `-p/--project` is empty, only sessions whose `Dir` exactly equals the current working directory are included.
- If `-p/--project` is non-empty, treat it as a case-insensitive regex matched against `Dir`.
- `.*` therefore matches all directories.

Latest filtering:

- If `--last=true`, select the session with the latest `CreateTime` after all other filters.
- If `--last=false`, output all filtered sessions.

## Sorting

When outputting multiple sessions:

- Sort by `Dir` ascending.
- For the same `Dir`, sort by `CreateTime` ascending.

When `--last=true`, sorting has no visible effect because only one session is printed.

## Output

Each session prints one line:

```text
dir=yyy sessionId=xxxx createTime=xxxx lastTime=xxxx firstMsg="..." lastMsg="..."
```

Time format:

```text
YYYYMMDD_HH:mm:ss
```

Timestamps from JSONL are converted to local time before printing.

Message summaries:

- Replace newlines, tabs, quotes, backslashes, and other control or special characters with spaces.
- Collapse repeated whitespace.
- Trim leading and trailing whitespace.
- Truncate to at most 10 Unicode characters.
- Preserve enough readable content to identify the session topic.
- Do not aim for exact message serialization.

## Logging

When `-d=true`, log to stderr.

Format:

```text
[info] message
[error] message
```

Debug logs should include scanned directories, skipped files, parse errors, and parsed session basics.

## Error Handling

- Missing provider root directory is an error.
- Invalid CLI arguments are errors.
- Invalid JSONL lines are skipped with debug logging when debug is enabled.
- A session file with no usable user messages is skipped.
- A Codex session file without `payload.id` is skipped.

## Tests

Claude tests:

- Decode normal project paths.
- Decode hidden directory paths.
- Stop decoding after the longest existing prefix and append the remaining suffix unchanged.
- Ignore session subdirectories.
- Extract first and last `message.role == "user"` records.
- Skip files without user messages.

Codex tests:

- Extract session ID from `payload.id`, not from filename.
- Extract directory from first `payload.cwd`.
- Extract first and last `payload.role == "user"` records.
- Support string `payload.content`.
- Support array `payload.content` with readable text parts.
- Skip files without user messages.

Common tests:

- Validate date parsing and reject `start-day > end-day`.
- Filter by exact current directory when project regex is empty.
- Filter by case-insensitive project regex when provided.
- Select latest session by `CreateTime`.
- Sort multiple sessions by `Dir`, then `CreateTime`.
- Clean and truncate message summaries to 10 Unicode characters.
