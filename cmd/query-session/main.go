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
	var last bool
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
	fs.BoolVar(&last, "l", false, "print latest session only")
	fs.BoolVar(&last, "last", false, "print latest session only")
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

	log := func(level, format string, args ...any) {
		if debug {
			fmt.Fprintf(stderr, "[%s] %s\n", level, fmt.Sprintf(format, args...))
		}
	}
	start, end, err := session.ParseDayRange(startDay, endDay, time.Local)
	if err != nil {
		return 1, err
	}

	currentDir, err := os.Getwd()
	if err != nil {
		return 1, err
	}
	home, err := os.UserHomeDir()
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

	if last {
		latest := session.LatestByCreateTime(filtered)
		if latest != nil {
			log("info", "selected latest sessionId=%s dir=%s createTime=%s", latest.SessionID, latest.Dir, latest.CreateTime.Local().Format("20060102_15:04:05"))
			fmt.Fprintln(stdout, session.FormatLine(*latest))
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
  -l / --last
        print latest session only (default false)
  -x / --exclude string
        exclude project pattern (case-insensitive, higher priority than -p)
  -p / --project string
        project pattern (case-insensitive)
  -s / --start-day string
        start day in YYYYMMDD (default %q)
  -t / --type string
        provider: claude or codex (default "claude")

claude example:
	# 当前目录今天的所有 session
	query-session

	# 当前目录今天的最后一个创建的 session
	query-session -l

	# 今天的 所有项目的 session ，  -p 是大小写忽略的正则匹配
	query-session -p ".*"

	# 输出今天的 所有项目的 session 的 全局最后一个创建
	query-session -p ".*" -l

	# 输出指定 时间内 指定 正则项目的  
	query-session -p "aiAgent"  -s 20260513 -e 20260514

	# -p 匹配过滤， 而 -x 是排除过滤 -x 的优先级比 -p 高 ， -x 是大小写忽略的正则匹配
	query-session -p "git" -x 'aiagent' -s 20260513 -e 20260514

codex example:
	# 输出当前目录今天的所有 session
	query-session -t codex
`, today, today)
}
