package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"query-session/internal/claude"
	"query-session/internal/session"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	today := time.Now().Local().Format("20060102")

	var provider string
	var debug bool
	var last bool
	var project string
	var startDay string
	var endDay string

	fs := flag.NewFlagSet("query-session", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&provider, "t", string(session.ProviderClaude), "provider")
	fs.BoolVar(&debug, "d", false, "debug logging")
	fs.BoolVar(&last, "l", true, "print latest session only")
	fs.BoolVar(&last, "last", true, "print latest session only")
	fs.StringVar(&project, "p", "", "project pattern")
	fs.StringVar(&project, "project", "", "project pattern")
	fs.StringVar(&startDay, "s", today, "start day in YYYYMMDD")
	fs.StringVar(&startDay, "start-day", today, "start day in YYYYMMDD")
	fs.StringVar(&endDay, "e", today, "end day in YYYYMMDD")
	fs.StringVar(&endDay, "end-day", today, "end day in YYYYMMDD")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	logInfo := func(format string, args ...any) {
		if debug {
			fmt.Fprintf(stderr, "[info] "+format+"\n", args...)
		}
	}
	fail := func(err error) int {
		if debug {
			fmt.Fprintf(stderr, "[error] %v\n", err)
		} else {
			fmt.Fprintln(stderr, err)
		}
		return 1
	}

	if provider != string(session.ProviderClaude) {
		if provider == string(session.ProviderCodex) {
			return fail(fmt.Errorf("codex provider is not implemented in this phase"))
		}
		return fail(fmt.Errorf("unknown provider: %s", provider))
	}

	start, end, err := session.ParseDayRange(startDay, endDay, time.Local)
	if err != nil {
		return fail(err)
	}

	currentDir, err := os.Getwd()
	if err != nil {
		return fail(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fail(err)
	}

	projectsRoot := filepath.Join(home, ".claude", "projects")
	logInfo("scanning claude sessions under %s", projectsRoot)
	sessions, err := claude.Scan(projectsRoot, "/", func(_ string, message string) {
		logInfo("%s", message)
	})
	if err != nil {
		return fail(err)
	}

	filtered, err := session.Filter(sessions, session.FilterOptions{
		ProjectPattern: project,
		CurrentDir:     currentDir,
		Start:          start,
		End:            end,
	})
	if err != nil {
		return fail(err)
	}
	if len(filtered) == 0 {
		return 0
	}

	if last {
		latest := session.LatestByCreateTime(filtered)
		if latest != nil {
			fmt.Fprintln(stdout, session.FormatLine(*latest))
		}
		return 0
	}

	session.SortSessions(filtered)
	for _, s := range filtered {
		fmt.Fprintln(stdout, session.FormatLine(s))
	}
	return 0
}
