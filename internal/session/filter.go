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

	filtered := make([]Session, 0, len(sessions))
	for _, s := range sessions {
		if s.CreateTime.Before(opts.Start) || s.CreateTime.After(opts.End) {
			logFilter(opts.Log, "filtered sessionId=%s reason=date dir=%s createTime=%s start=%s end=%s", s.SessionID, s.Dir, s.CreateTime.Format(outputTimeFormat), opts.Start.Format(outputTimeFormat), opts.End.Format(outputTimeFormat))
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

func logFilter(log Logger, format string, args ...any) {
	if log != nil {
		log("info", fmt.Sprintf(format, args...))
	}
}
