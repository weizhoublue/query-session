package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"query-session/internal/claude"
	"query-session/internal/codex"
	"query-session/internal/copilot"
	"query-session/internal/cursor"
	"query-session/internal/session"
)

func main() {
	code, err := run(os.Args[1:], os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[error] %v\n", err)
	}
	os.Exit(code)
}

const version = "0.7.4"

func run(args []string, stdout, stderr io.Writer) (int, error) {
	today := time.Now().Local().Format("20060102")

	var provider string
	var debug bool
	var number int
	var lastDays int
	var project string
	var exclude string
	var startDay string
	var endDay string
	var showVersion bool

	fs := flag.NewFlagSet("query-session", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&showVersion, "v", false, "print version")
	fs.BoolVar(&showVersion, "version", false, "print version")
	fs.StringVar(&provider, "t", string(session.ProviderCopilot), "provider")
	fs.StringVar(&provider, "type", string(session.ProviderCopilot), "provider")
	fs.BoolVar(&debug, "d", false, "debug logging")
	fs.BoolVar(&debug, "debug", false, "debug logging")
	fs.IntVar(&number, "n", 10, "print top N sessions by createTime")
	fs.IntVar(&number, "number", 10, "print top N sessions by createTime")
	fs.IntVar(&lastDays, "l", 0, "cover past N days including today")
	fs.IntVar(&lastDays, "last", 0, "cover past N days including today")
	fs.StringVar(&project, "p", "", "project pattern (case-insensitive)")
	fs.StringVar(&project, "project", "", "project pattern (case-insensitive)")
	fs.StringVar(&exclude, "x", "", "exclude project pattern (case-insensitive)")
	fs.StringVar(&exclude, "exclude", "", "exclude project pattern (case-insensitive)")
	fs.StringVar(&startDay, "s", today, "start day in YYYYMMDD")
	fs.StringVar(&startDay, "start-day", today, "start day in YYYYMMDD")
	fs.StringVar(&endDay, "e", today, "end day in YYYYMMDD")
	fs.StringVar(&endDay, "end-day", today, "end day in YYYYMMDD")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			printUsage(stdout, today)
			return 0, nil
		}
		return 2, err
	}
	if fs.NArg() > 0 {
		quoted := make([]string, fs.NArg())
		for i, arg := range fs.Args() {
			quoted[i] = fmt.Sprintf("%q", arg)
		}
		return 2, fmt.Errorf("unexpected arguments: %s", strings.Join(quoted, " "))
	}

	if showVersion {
		fmt.Fprintln(stdout, version)
		return 0, nil
	}

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

	var err error
	var currentDir, home string
	log := func(level, format string, args ...any) {
		if debug {
			fmt.Fprintf(stderr, "[%s] %s\n", level, fmt.Sprintf(format, args...))
		}
	}
	var start, end time.Time
	dateFilter := lastDays > 0
	var showDirectory bool
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "s" || f.Name == "start-day" || f.Name == "e" || f.Name == "end-day" {
			dateFilter = true
		}
		if f.Name == "p" || f.Name == "project" {
			showDirectory = true
		}
	})
	if lastDays > 0 {
		start, end, err = session.ParseLastDays(lastDays, time.Local)
	} else if dateFilter {
		start, end, err = session.ParseDayRange(startDay, endDay, time.Local)
	}
	if err != nil {
		return 1, err
	}

	currentDir, err = os.Getwd()
	if err != nil {
		return 1, err
	}
	home, err = os.UserHomeDir()
	if err != nil {
		return 1, err
	}
	filterOpts := session.FilterOptions{
		ProjectPattern: project,
		ExcludePattern: exclude,
		CurrentDir:     currentDir,
		SkipDateFilter: !dateFilter,
		Start:          start,
		End:            end,
		Log: func(level string, message string) {
			log(level, "%s", message)
		},
	}
	filterOpts.Matcher, err = session.NewDirMatcher(filterOpts)
	if err != nil {
		return 1, err
	}

	var sessions []session.Session
	switch session.Provider(provider) {
	case session.ProviderClaude:
		projectsRoot := filepath.Join(home, ".claude", "projects")
		log("info", "scanning claude sessions under %s", projectsRoot)
		sessions, err = claude.Scan(projectsRoot, "/", func(level string, message string) {
			log(level, "%s", message)
		})
		if err != nil {
			return 1, err
		}
	case session.ProviderCodex:
		root := filepath.Join(home, ".codex", "sessions")
		log("info", "scanning codex sessions under %s", root)
		logger := func(level string, message string) {
			log(level, "%s", message)
		}
		if dateFilter {
			sessions, err = codex.Scan(root, start, end, logger)
		} else {
			sessions, err = codex.ScanAll(root, logger)
		}
		if err != nil {
			return 1, err
		}
	case session.ProviderCursor:
		chatsRoot := filepath.Join(home, ".cursor", "chats")
		log("info", "scanning cursor sessions under %s", chatsRoot)
		sessions, err = cursor.Scan(chatsRoot, func(level string, message string) {
			log(level, "%s", message)
		})
		if err != nil {
			return 1, err
		}
	case session.ProviderCopilot:
		copilotHome := os.Getenv("COPILOT_HOME")
		if copilotHome == "" {
			copilotHome = filepath.Join(home, ".copilot")
		}
		root := filepath.Join(copilotHome, "session-state")
		log("info", "scanning copilot sessions under %s", root)
		sessions, err = copilot.Scan(root, filterOpts.Matcher, func(level string, message string) {
			log(level, "%s", message)
		})
		if err != nil {
			return 1, err
		}
	default:
		return 1, fmt.Errorf("unknown provider: %s", provider)
	}

	filtered, err := session.Filter(sessions, filterOpts)
	if err != nil {
		return 1, err
	}
	var result []session.Session
	if number > 0 {
		result = session.TopNByCreateTime(filtered, number)
	} else {
		session.SortSessions(filtered)
		result = filtered
	}
	if err := printQuerySummary(stdout, provider, project, exclude, dateFilter, lastDays, start, end, number, len(filtered), len(result), currentDir); err != nil {
		return 1, err
	}
	if len(filtered) == 0 {
		log("info", "no sessions matched filters")
	} else if number > 0 {
		log("info", "printing top %d of %d matched sessions", len(result), len(filtered))
	} else {
		log("info", "printing %d matched sessions", len(filtered))
	}
	if err := session.FormatTable(stdout, result, showDirectory); err != nil {
		return 1, err
	}
	return 0, nil
}

func printQuerySummary(w io.Writer, provider, project, exclude string, dateFilter bool, lastDays int, start, end time.Time, number, matched, output int, currentDir string) error {
	if project == "" {
		project = currentDir
	}
	if _, err := fmt.Fprintf(w, "provider: %s\ndirectory: %s\n", provider, project); err != nil {
		return err
	}
	if exclude != "" {
		if _, err := fmt.Fprintf(w, "exclude: %s\n", exclude); err != nil {
			return err
		}
	}
	if !dateFilter {
		if _, err := fmt.Fprintln(w, "time range: all"); err != nil {
			return err
		}
	} else if lastDays > 0 {
		if _, err := fmt.Fprintf(w, "time range: last %d days\n", lastDays); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintf(w, "time range: %s..%s\n", start.Format("20060102"), end.Format("20060102")); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w, "session limit/matched/output: %d/%d/%d\n\n", number, matched, output)
	return err
}

func printUsage(w io.Writer, today string) {
	fmt.Fprintf(w, `Usage:
  query-session [options]

Options:
  -v / --version
        print version
  -d / --debug
        debug logging
  -e / --end-day string
        end day in YYYYMMDD (default %q)
  -l / --last int
        cover past N days including today (mutually exclusive with -s/-e)
  -n / --number int
        print top N sessions by createTime (most recent first)
  -x / --exclude string
        exclude project pattern (case-insensitive, higher priority than -p)
  -p / --project string
        project pattern (case-insensitive)
  -s / --start-day string
        start day in YYYYMMDD (default %q)
  -t / --type string
        provider: claude, codex, cursor, or copilot (default "copilot")

当前目录:
	# 当前目录所有日期的 copilot 最新 10 个 session
	query-session

	# 当前目录所有日期的 copilot 最近 1 个 session
	query-session -n 1

	# 当前目录 过去 3 天内 copilot 的最近的 2 个 session
	query-session -l 3 -n 2

	# 指定时间范围
	query-session  -s 20260513 -e 20260514

所有目录（非当前目录）
	# 所有目录（非当前目录） 过去 7 天中 copilot 最新的 3 条。-p 是大小写忽略的正则匹配
	query-session -n 3 -l 7 -p ".*"

	# 通过正则式指定 目录
	query-session -p "aiAgent"  -s 20260513 -e 20260514

	# -p 匹配目录， 而 -x 是排除目录 。 -x 的优先级比 -p 高 ， -x 是大小写忽略的正则匹配
	query-session -p "git" -x 'aiagent' -s 20260513 -e 20260514

其他 agent：
	# 输出当前目录所有日期的最新 10 个 claude 会话
	query-session -t claude

	# 输出当前目录所有日期的最新 10 个 codex 会话
	query-session -t codex

	# 输出当前工作区所有日期的最新 10 个 cursor 会话
	query-session -t cursor
`, today, today)
}
