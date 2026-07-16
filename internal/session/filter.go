package session

import (
	"fmt"
	"regexp"
	"sort"
)

func Filter(sessions []Session, opts FilterOptions) ([]Session, error) {
	var projectRE *regexp.Regexp
	var err error
	if opts.ProjectPattern != "" {
		projectRE, err = regexp.Compile("(?i)" + opts.ProjectPattern)
		if err != nil {
			return nil, err
		}
	}

	var excludeRE *regexp.Regexp
	if opts.ExcludePattern != "" {
		excludeRE, err = regexp.Compile("(?i)" + opts.ExcludePattern)
		if err != nil {
			return nil, err
		}
	}

	filtered := make([]Session, 0, len(sessions))
	for _, s := range sessions {
		if !opts.SkipDateFilter && (s.CreateTime.Before(opts.Start) || s.CreateTime.After(opts.End)) {
			logFilter(opts.Log, "filtered sessionId=%s reason=date dir=%s createTime=%s start=%s end=%s", s.SessionID, s.Dir, s.CreateTime.Format(outputTimeFormat), opts.Start.Format(outputTimeFormat), opts.End.Format(outputTimeFormat))
			continue
		}
		if excludeRE != nil && excludeRE.MatchString(s.Dir) {
			logFilter(opts.Log, "filtered sessionId=%s reason=exclude dir=%s pattern=%s", s.SessionID, s.Dir, opts.ExcludePattern)
			continue
		}
		if projectRE == nil {
			if s.Dir != opts.CurrentDir {
				logFilter(opts.Log, "filtered sessionId=%s reason=project dir=%s currentDir=%s", s.SessionID, s.Dir, opts.CurrentDir)
				continue
			}
		} else if !projectRE.MatchString(s.Dir) {
			logFilter(opts.Log, "filtered sessionId=%s reason=project dir=%s pattern=%s", s.SessionID, s.Dir, opts.ProjectPattern)
			continue
		}
		logFilter(opts.Log, "matched sessionId=%s dir=%s createTime=%s lastTime=%s", s.SessionID, s.Dir, s.CreateTime.Format(outputTimeFormat), s.LastTime.Format(outputTimeFormat))
		filtered = append(filtered, s)
	}
	return filtered, nil
}

func LatestByCreateTime(sessions []Session) *Session {
	if len(sessions) == 0 {
		return nil
	}

	latest := &sessions[0]
	for i := 1; i < len(sessions); i++ {
		if sessions[i].CreateTime.After(latest.CreateTime) {
			latest = &sessions[i]
		}
	}
	return latest
}

func SortSessions(sessions []Session) {
	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].Dir == sessions[j].Dir {
			return sessions[i].CreateTime.Before(sessions[j].CreateTime)
		}
		return sessions[i].Dir < sessions[j].Dir
	})
}

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

func logFilter(log Logger, format string, args ...any) {
	if log != nil {
		log("info", fmt.Sprintf(format, args...))
	}
}
