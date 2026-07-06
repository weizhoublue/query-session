# `--number/-n` 与 `--last/-l` Flag 重构 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 移除原 `-l` boolean flag，新增 `--number/-n N`（取最新 N 条）和 `--last/-l N`（过去 N 天时间窗口）两个 int flag。

**Architecture:** 在 `internal/session` 包新增两个工具函数（`ParseLastDays`、`TopNByCreateTime`）并配套测试；`cmd/query-session/main.go` 重新注册 flag、加入互斥检查、更新输出逻辑；`main_test.go` 同步更新 help 断言。

**Tech Stack:** Go 标准库（`flag`、`sort`、`time`）；无新增外部依赖。

## Global Constraints

- Go 模块路径：`query-session`
- 测试命令：`go test ./... -count=1`
- 构建命令：`go build -o query-session ./cmd/query-session`
- 所有改动必须通过现有测试（仅更新因行为变化而失效的断言）
- 不引入任何新依赖

---

### Task 1：在 `session.go` 新增 `ParseLastDays`

**Files:**
- Modify: `internal/session/session.go`（在 `ParseDayRange` 之后追加）
- Test: `internal/session/filter_test.go`（追加三个测试函数）

**Interfaces:**
- Produces: `func ParseLastDays(n int, loc *time.Location) (time.Time, time.Time, error)`
  - `n=1` → 今天 00:00:00 ~ 23:59:59.999…
  - `n=3` → 前天 00:00:00 ~ 今天 23:59:59.999…
  - `n < 1` → 返回错误

- [ ] **Step 1: 在 `filter_test.go` 末尾追加三个失败测试**

```go
func TestParseLastDaysOneDayIsToday(t *testing.T) {
	now := time.Now().UTC()
	start, end, err := ParseLastDays(1, time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	todayEnd := todayStart.AddDate(0, 0, 1).Add(-time.Nanosecond)
	if !start.Equal(todayStart) {
		t.Fatalf("start = %s, want %s", start, todayStart)
	}
	if !end.Equal(todayEnd) {
		t.Fatalf("end = %s, want %s", end, todayEnd)
	}
}

func TestParseLastDaysThreeDays(t *testing.T) {
	now := time.Now().UTC()
	start, end, err := ParseLastDays(3, time.UTC)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	wantStart := todayStart.AddDate(0, 0, -2)
	wantEnd := todayStart.AddDate(0, 0, 1).Add(-time.Nanosecond)
	if !start.Equal(wantStart) {
		t.Fatalf("start = %s, want %s", start, wantStart)
	}
	if !end.Equal(wantEnd) {
		t.Fatalf("end = %s, want %s", end, wantEnd)
	}
}

func TestParseLastDaysRejectsZero(t *testing.T) {
	_, _, err := ParseLastDays(0, time.UTC)
	if err == nil {
		t.Fatal("expected error for n=0")
	}
}
```

- [ ] **Step 2: 运行测试，确认失败**

```bash
go test ./internal/session/... -run TestParseLastDays -count=1 -v
```

预期：`FAIL`，错误为 `undefined: ParseLastDays`。

- [ ] **Step 3: 在 `internal/session/session.go` 末尾追加实现**

```go
// ParseLastDays 计算"过去 n 天含今天"的时间窗口，与 ParseDayRange 对称。
// n=1 返回今天零点到今天末尾；n=3 返回前天零点到今天末尾。
func ParseLastDays(n int, loc *time.Location) (time.Time, time.Time, error) {
	if n < 1 {
		return time.Time{}, time.Time{}, fmt.Errorf("--last must be >= 1, got %d", n)
	}
	now := time.Now().In(loc)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	start := todayStart.AddDate(0, 0, -(n - 1))
	end := todayStart.AddDate(0, 0, 1).Add(-time.Nanosecond)
	return start, end, nil
}
```

- [ ] **Step 4: 运行测试，确认通过**

```bash
go test ./internal/session/... -run TestParseLastDays -count=1 -v
```

预期：三个测试全部 `PASS`。

- [ ] **Step 5: 运行全套测试，确认无回归**

```bash
go test ./... -count=1
```

预期：全部 `PASS`。

- [ ] **Step 6: Commit**

```bash
git add internal/session/session.go internal/session/filter_test.go
git commit -m "feat(session): add ParseLastDays for N-day window"
```

---

### Task 2：在 `filter.go` 新增 `TopNByCreateTime`

**Files:**
- Modify: `internal/session/filter.go`（在 `SortSessions` 之后追加）
- Test: `internal/session/filter_test.go`（追加三个测试函数）

**Interfaces:**
- Consumes: `Session.CreateTime time.Time`（Task 1 之前已存在）
- Produces: `func TopNByCreateTime(sessions []Session, n int) []Session`
  - `n <= 0` 或 `len(sessions) == 0` → 返回原切片，不复制
  - `n >= len(sessions)` → 返回全部（按 createTime 降序的副本）
  - 否则 → 返回前 n 条（createTime 降序的副本）
  - **不修改**调用方传入的切片

- [ ] **Step 1: 在 `filter_test.go` 末尾追加三个失败测试**

```go
func TestTopNByCreateTimeReturnsTopTwo(t *testing.T) {
	sessions := []Session{
		{SessionID: "old",    CreateTime: mustTime(t, "2026-05-18T01:00:00Z")},
		{SessionID: "newest", CreateTime: mustTime(t, "2026-05-18T03:00:00Z")},
		{SessionID: "mid",    CreateTime: mustTime(t, "2026-05-18T02:00:00Z")},
	}
	orig := make([]Session, len(sessions))
	copy(orig, sessions)

	got := TopNByCreateTime(sessions, 2)

	if len(got) != 2 {
		t.Fatalf("got %d sessions, want 2", len(got))
	}
	if got[0].SessionID != "newest" || got[1].SessionID != "mid" {
		t.Fatalf("unexpected order: %v %v", got[0].SessionID, got[1].SessionID)
	}
	// 确认原切片未被修改
	for i, s := range sessions {
		if s.SessionID != orig[i].SessionID {
			t.Fatalf("input slice mutated at index %d", i)
		}
	}
}

func TestTopNByCreateTimeZeroReturnsAll(t *testing.T) {
	sessions := []Session{
		{SessionID: "a", CreateTime: mustTime(t, "2026-05-18T01:00:00Z")},
		{SessionID: "b", CreateTime: mustTime(t, "2026-05-18T02:00:00Z")},
	}
	got := TopNByCreateTime(sessions, 0)
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
}

func TestTopNByCreateTimeNLargerThanLen(t *testing.T) {
	sessions := []Session{
		{SessionID: "a", CreateTime: mustTime(t, "2026-05-18T01:00:00Z")},
	}
	got := TopNByCreateTime(sessions, 99)
	if len(got) != 1 {
		t.Fatalf("got %d, want 1", len(got))
	}
}
```

- [ ] **Step 2: 运行测试，确认失败**

```bash
go test ./internal/session/... -run TestTopNByCreateTime -count=1 -v
```

预期：`FAIL`，错误为 `undefined: TopNByCreateTime`。

- [ ] **Step 3: 在 `internal/session/filter.go` 末尾（`logFilter` 之前）追加实现**

```go
// TopNByCreateTime 按 createTime 降序排序后返回前 n 条。
// n <= 0 时返回全部（不做截取）；不修改原切片。
func TopNByCreateTime(sessions []Session, n int) []Session {
	if n <= 0 || len(sessions) == 0 {
		return sessions
	}
	sorted := make([]Session, len(sessions))
	copy(sorted, sessions)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].CreateTime.After(sorted[j].CreateTime)
	})
	if n >= len(sorted) {
		return sorted
	}
	return sorted[:n]
}
```

- [ ] **Step 4: 运行测试，确认通过**

```bash
go test ./internal/session/... -run TestTopNByCreateTime -count=1 -v
```

预期：三个测试全部 `PASS`。

- [ ] **Step 5: 运行全套测试，确认无回归**

```bash
go test ./... -count=1
```

预期：全部 `PASS`。

- [ ] **Step 6: Commit**

```bash
git add internal/session/filter.go internal/session/filter_test.go
git commit -m "feat(session): add TopNByCreateTime"
```

---

### Task 3：更新 `cmd/query-session/main.go` 及其测试

**Files:**
- Modify: `cmd/query-session/main.go`（全函数 `run` + `printUsage`）
- Modify: `cmd/query-session/main_test.go`（更新 help 断言）

**Interfaces:**
- Consumes:
  - `session.ParseLastDays(n int, loc *time.Location) (time.Time, time.Time, error)` — Task 1
  - `session.TopNByCreateTime(sessions []Session, n int) []Session` — Task 2
- Produces: 无（顶层 CLI 入口）

- [ ] **Step 1: 在 `main_test.go` 中更新 `TestRunHelpCombinesShortAndLongFlags`，删除旧断言、加入新断言**

将该测试中的 `want` slice 替换为：

```go
for _, want := range []string{
    "-d / --debug",
    "-e / --end-day string",
    "-l / --last int",
    "cover past N days including today",
    "-n / --number int",
    "print top N sessions by createTime",
    "-p / --project string",
    "-s / --start-day string",
    "-t / --type string",
    `provider: claude, codex, or cursor (default "claude")`,
} {
```

- [ ] **Step 2: 新增两个 flag 行为测试**

在 `main_test.go` 末尾追加：

```go
func TestRunLastConflictsWithStartDay(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code, err := run([]string{"-l", "3", "-s", "20260101"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if err == nil || !strings.Contains(err.Error(), "--last conflicts with") {
		t.Fatalf("err = %v, want conflict error", err)
	}
}

func TestRunNegativeNumberReturnsError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code, err := run([]string{"-n", "-1"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if err == nil || !strings.Contains(err.Error(), "--number must be") {
		t.Fatalf("err = %v, want number error", err)
	}
}
```

- [ ] **Step 3: 运行测试，确认新增测试失败（旧测试可能也失败）**

```bash
go test ./cmd/query-session/... -count=1 -v
```

预期：多个失败，因为 `-l`/`-n` flag 尚未重写。

- [ ] **Step 4: 重写 `run()` 中 flag 声明部分**

将以下旧代码：

```go
var last bool
// ...
fs.BoolVar(&last, "l", false, "print latest session only")
fs.BoolVar(&last, "last", false, "print latest session only")
```

替换为：

```go
var number int
var lastDays int
// ...
fs.IntVar(&number, "n", 0, "print top N sessions by createTime")
fs.IntVar(&number, "number", 0, "print top N sessions by createTime")
fs.IntVar(&lastDays, "l", 0, "cover past N days including today")
fs.IntVar(&lastDays, "last", 0, "cover past N days including today")
```

- [ ] **Step 5: 在 `fs.Parse` 之后加入验证和互斥检查**

在 `if err := fs.Parse(args); ...` 块之后，`log := ...` 之前插入：

```go
if number < 0 {
    return 1, fmt.Errorf("--number must be >= 1, got %d", number)
}
if lastDays < 0 {
    return 1, fmt.Errorf("--last must be >= 1, got %d", lastDays)
}
if lastDays > 0 {
    var conflicting string
    fs.Visit(func(f *flag.Flag) {
        if f.Name == "s" || f.Name == "start-day" || f.Name == "e" || f.Name == "end-day" {
            conflicting = f.Name
        }
    })
    if conflicting != "" {
        return 1, fmt.Errorf("--last conflicts with --%s; use one or the other", conflicting)
    }
}
```

- [ ] **Step 6: 替换日期范围计算**

将：

```go
start, end, err := session.ParseDayRange(startDay, endDay, time.Local)
if err != nil {
    return 1, err
}
```

替换为：

```go
var start, end time.Time
if lastDays > 0 {
    start, end, err = session.ParseLastDays(lastDays, time.Local)
} else {
    start, end, err = session.ParseDayRange(startDay, endDay, time.Local)
}
if err != nil {
    return 1, err
}
```

注意：此处 `err` 需在外层先声明（`var err error`），或将上方已有的 `sessions, err = ...` 改为先 `var err error` 再赋值。实际上，只需把 `:=` 改成将 `err` 提升到 `var` 声明即可：

在 `var sessions []session.Session` 声明附近添加 `var err error`，然后将 `start, end, err :=` 改为 `start, end, err =`。

- [ ] **Step 7: 替换输出逻辑（移除旧 `if last {...}` 块，改用 `--number` 路径）**

删除：

```go
if last {
    latest := session.LatestByCreateTime(filtered)
    if latest != nil {
        log("info", "selected latest sessionId=%s dir=%s createTime=%s", latest.SessionID, latest.Dir, latest.CreateTime.Local().Format("20060102_15:04:05"))
        fmt.Fprintln(stdout, session.FormatLine(*latest))
    }
    return 0, nil
}
```

在 `session.SortSessions(filtered)` 之前插入：

```go
if number > 0 {
    result := session.TopNByCreateTime(filtered, number)
    log("info", "printing top %d of %d matched sessions", len(result), len(filtered))
    for _, s := range result {
        fmt.Fprintln(stdout, session.FormatLine(s))
    }
    return 0, nil
}
```

- [ ] **Step 8: 更新 `printUsage`**

将 `printUsage` 中与 `-l`/`--last` 相关的内容替换：

旧：
```
  -l / --last
        print latest session only (default false)
```

新：
```
  -l / --last int
        cover past N days including today (mutually exclusive with -s/-e)
  -n / --number int
        print top N sessions by createTime (most recent first)
```

同时更新示例部分，把：
```
	# 当前目录今天的最后一个创建的 session
	query-session -l
```
替换为：
```
	# 当前目录今天 createTime 最新的 1 条
	query-session -n 1

	# 过去 7 天中 createTime 最新的 3 条（所有项目）
	query-session -n 3 -l 7 -p ".*"
```

- [ ] **Step 9: 运行全套测试，确认全部通过**

```bash
go test ./... -count=1
```

预期：全部 `PASS`。

- [ ] **Step 10: 构建验证**

```bash
go build -o /tmp/query-session-test ./cmd/query-session && echo "build ok"
```

预期：`build ok`，无编译错误。

- [ ] **Step 11: Commit**

```bash
git add cmd/query-session/main.go cmd/query-session/main_test.go
git commit -m "feat: replace -l bool with --number/-n and --last/-l int flags"
```
