package session

import (
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
			continue
		}
		if projectRE == nil {
			if s.Dir != opts.CurrentDir {
				continue
			}
		} else if !projectRE.MatchString(s.Dir) {
			continue
		}
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
