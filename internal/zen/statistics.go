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
		for i := range result.Week {
			if dayStart.Format("2006-01-02") == result.Week[i].Date {
				addStat(&result.Week[i], session)
				break
			}
		}
	}
	finalizeStat(&result.Today)
	for i := range result.Week {
		finalizeStat(&result.Week[i])
		result.WeekFocusSeconds += result.Week[i].FocusSeconds
		result.WeekCompleted += result.Week[i].Completed
		result.AverageCompleted += result.Week[i].completedSeconds
	}
	if result.WeekCompleted > 0 {
		result.AverageCompleted /= int64(result.WeekCompleted)
	}
	return result
}

func addStat(stat *DayStat, session Session) {
	stat.FocusSeconds += session.ActiveSeconds
	switch session.Status {
	case StatusCompleted:
		stat.Completed++
		stat.completedSeconds += session.ActiveSeconds
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
		stat.AverageCompleted = stat.completedSeconds / int64(stat.Completed)
	}
}
