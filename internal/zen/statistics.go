package zen

import "time"

func CalculateStatistics(sessions []Session, now time.Time) Statistics {
	localNow := now.Local()
	startToday := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, localNow.Location())
	weekday := (int(startToday.Weekday()) + 6) % 7
	startWeek := startToday.AddDate(0, 0, -weekday)
	result := Statistics{Week: make([]DayStat, 7)}
	for i := range result.Week {
		day := startWeek.AddDate(0, 0, i)
		result.Week[i].Date = day.Format("2006-01-02")
	}
	result.Today.Date = startToday.Format("2006-01-02")
	for _, session := range sessions {
		if session.Type != SessionFocus {
			continue
		}
		local := session.StartedAt.Local()
		dayStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
		if dayStart.Equal(startToday) {
			addStat(&result.Today, session)
		}
		index := int(dayStart.Sub(startWeek).Hours() / 24)
		if index >= 0 && index < 7 {
			addStat(&result.Week[index], session)
		}
	}
	finalizeStat(&result.Today)
	for i := range result.Week {
		finalizeStat(&result.Week[i])
	}
	return result
}

func addStat(stat *DayStat, session Session) {
	stat.FocusSeconds += session.ActiveSeconds
	switch session.Status {
	case StatusCompleted:
		stat.Completed++
	case StatusInterrupted:
		stat.Interrupted++
	}
}

func finalizeStat(stat *DayStat) {
	total := stat.Completed + stat.Interrupted
	if total > 0 {
		stat.CompletionRate = stat.Completed * 100 / total
	}
	if stat.Completed > 0 {
		stat.AverageCompleted = stat.FocusSeconds / int64(stat.Completed)
	}
}
