# 设计规范：`--number / -n` 与 `--last / -l` Flag 重构

**日期：** 2026-07-06  
**状态：** 已批准

---

## 背景

当前 `--last` / `-l` 是一个 boolean flag，含义是"只输出 createTime 最新的 1 条 session"。  
本次重构将其拆分为两个语义更清晰的选项：

- 原 `-l` boolean → **移除**
- 新增 `--number / -n N`：按 createTime 取最新前 N 条
- 新增 `--last / -l N`：将时间窗口扩展为过去 N 天（含今天）

---

## Flag 规范

### 移除

| Flag | 旧含义 |
|------|--------|
| `-l` / `--last` (bool) | 输出 createTime 最新的 1 条，等价于新的 `-n 1` |

### 新增

#### `--number / -n N`（int，默认 0）

- `0` 表示不限制，输出所有匹配的 session（保持原有行为）
- `N > 0` 表示按 createTime 降序排序后，取前 N 条
- 必须显式提供数字，不支持省略值
- 可与 `-l` 组合使用：先由 `-l` 确定时间范围，再由 `-n` 截取

**示例：**
```
query-session -n 1          # 当前目录今天 createTime 最新的 1 条
query-session -n 3 -p ".*"  # 所有项目今天最新的 3 条
query-session -n 3 -l 7     # 过去 7 天中最新的 3 条
```

#### `--last / -l N`（int，默认 0）

- `0` 表示不启用，时间范围由 `-s`/`-e` 决定（默认今天）
- `N > 0` 表示覆盖过去 N 天，含今天（今天算第 1 天）
  - `-l 1` = 仅今天（等同默认）
  - `-l 3` = 今天 + 昨天 + 前天
- **与 `-s`/`--start-day`、`-e`/`--end-day` 互斥**：同时设置时报错退出

**示例：**
```
query-session -l 3           # 当前目录过去 3 天的所有 session
query-session -l 7 -p ".*"  # 所有项目过去 7 天的 session
query-session -l 3 -n 5     # 过去 3 天中最新的 5 条
```

---

## 架构变更

### `internal/session/session.go`（无变化）

`FilterOptions` 结构体无需改动，`Start`/`End` 字段仍是时间范围传入点。

### `internal/session/filter.go`（新增函数）

```go
// ParseLastDays 计算"过去 N 天含今天"的时间窗口，与 ParseDayRange 对称。
// n=1 返回今天零点到今天末尾；n=3 返回前天零点到今天末尾。
func ParseLastDays(n int, loc *time.Location) (start, end time.Time)

// TopNByCreateTime 按 createTime 降序排序后返回前 n 条。
// n <= 0 时返回全部（不截取）。不修改原切片。
func TopNByCreateTime(sessions []Session, n int) []Session
```

### `cmd/query-session/main.go`（变更）

1. **移除** `last bool` 变量及对应的两个 `BoolVar` 注册
2. **新增** `number int` 变量，注册 `-n` / `--number`
3. **新增** `lastDays int` 变量，注册 `-l` / `--last`
4. **互斥检查**：用 `fs.Visit` 检测 `-l` 与 `-s`/`-e` 是否同时显式设置，若是则报错
5. **日期范围计算**：
   - `lastDays > 0` → 调用 `session.ParseLastDays(lastDays, time.Local)` 得到 `start`/`end`
   - 否则 → 沿用原 `session.ParseDayRange(startDay, endDay, time.Local)`
6. **输出截取**：Filter 之后若 `number > 0`，调用 `session.TopNByCreateTime(filtered, number)`，返回结果直接按 createTime 降序输出（**跳过** `SortSessions`）；否则走原有 `SortSessions` 路径

---

## 主流程（伪代码）

```
解析 flag
│
├─ if lastDays > 0 AND (-s 或 -e 被显式设置)
│    → 报错："--last conflicts with --start-day/--end-day"
│
├─ if lastDays > 0
│    start, end = ParseLastDays(lastDays, time.Local)
│  else
│    start, end = ParseDayRange(startDay, endDay, time.Local)
│
Scan(provider) → all sessions
Filter(all, FilterOptions{start, end, ...}) → filtered
│
├─ if number > 0
│    result = TopNByCreateTime(filtered, number)
│    输出 result（createTime 降序，无需 SortSessions）
│  else
│    SortSessions(filtered)
│    输出 filtered
```

---

## 错误处理

| 情况 | 行为 |
|------|------|
| `-l 0` 或 `-n 0` | 等同于未设置该 flag（正常运行） |
| `-l N` 且同时有 `-s`/`-e` | 返回错误，exit code 1 |
| `-n` 不带数字 | Go flag 自动报错（int flag 必须带值） |
| `-l` 或 `-n` 传入负数 | 报错："--last/--number must be >= 1" |

---

## 向后兼容性

| 旧用法 | 迁移建议 |
|--------|----------|
| `query-session -l` | 改为 `query-session -n 1` |
| `query-session --last` | 改为 `query-session --number 1` |

其他所有现有 flag（`-t`、`-p`、`-x`、`-s`、`-e`、`-d`）行为不变。

---

## 测试要点

- `ParseLastDays(1)` 返回今天整天
- `ParseLastDays(3)` 返回前天零点到今天末尾
- `TopNByCreateTime(sessions, 2)` 返回最新 2 条，不修改原切片
- `-l 3 -s 20260101` 组合时报错
- `-n 3` 输出按 createTime 降序
- `-n 3 -l 7` 组合正常工作
