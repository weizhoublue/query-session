package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"query-session/internal/claude"
	"query-session/internal/codex"
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

	fs := flag.NewFlagSet("query-session", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&provider, "t", string(session.ProviderClaude), "provider")
	fs.StringVar(&provider, "type", string(session.ProviderClaude), "provider")
	fs.BoolVar(&debug, "d", false, "debug logging")
	fs.BoolVar(&debug, "debug", false, "debug logging")
	fs.IntVar(&number, "n", 0, "print top N sessions by createTime")
	fs.IntVar(&number, "number", 0, "print top N sessions by createTime")
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
	if lastDays > 0 {
		start, end, err = session.ParseLastDays(lastDays, time.Local)
	} else {
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
		sessions, err = codex.Scan(root, start, end, func(level string, message string) {
			log(level, "%s", message)
		})
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
	default:
		return 1, fmt.Errorf("unknown provider: %s", provider)
	}

	filtered, err := session.Filter(sessions, session.FilterOptions{
		ProjectPattern: project,
		ExcludePattern: exclude,
		CurrentDir:     currentDir,
		Start:          start,
		End:            end,
		Log: func(level string, message string) {
			log(level, "%s", message)
		},
	})
	if err != nil {
		return 1, err
	}
	if len(filtered) == 0 {
		log("info", "no sessions matched filters")
		return 0, nil
	}

	if number > 0 {
		result := session.TopNByCreateTime(filtered, number)
		log("info", "printing top %d of %d matched sessions", len(result), len(filtered))
		for _, s := range result {
			fmt.Fprintln(stdout, session.FormatLine(s))
		}
		return 0, nil
	}

	session.SortSessions(filtered)
	log("info", "printing %d matched sessions", len(filtered))
	for _, s := range filtered {
		fmt.Fprintln(stdout, session.FormatLine(s))
	}
	return 0, nil
}

func printUsage(w io.Writer, today string) {
	fmt.Fprintf(w, `Usage:
  query-session [options]

Options:
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
        provider: claude, codex, or cursor (default "claude")

claude example:
	# 当前目录今天的所有 session
	query-session

	# 当前目录今天 createTime 最新的 1 条
	query-session -n 1

	# 过去 7 天中 createTime 最新的 3 条（所有项目）
	query-session -n 3 -l 7 -p ".*"

	# 今天的 所有项目的 session ，  -p 是大小写忽略的正则匹配
	query-session -p ".*"

	# 输出指定 时间内 指定 正则项目的  
	query-session -p "aiAgent"  -s 20260513 -e 20260514

	# -p 匹配过滤， 而 -x 是排除过滤 -x 的优先级比 -p 高 ， -x 是大小写忽略的正则匹配
	query-session -p "git" -x 'aiagent' -s 20260513 -e 20260514

codex example:
	# 输出当前目录今天的所有 session
	query-session -t codex

cursor example:
	# 当前工作区今天创建的 cursor agent 会话
	query-session -t cursor
`, today, today)
}
