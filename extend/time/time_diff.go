package ctime

// DiffInYears 相差多少年
func (c ActorGoTime) DiffInYears(end *ActorGoTime) int64 {
	return c.DiffInMonths(end) / 12
}

// DiffInYearsWithAbs 相差多少年(绝对值)
func (c ActorGoTime) DiffInYearsWithAbs(end *ActorGoTime) int64 {
	return GetAbsValue(c.DiffInYears(end))
}

// DiffInMonths 相差多少月
func (c ActorGoTime) DiffInMonths(end *ActorGoTime) int64 {
	dy, dm, dd := end.Year()-c.Year(), end.Month()-c.Month(), end.Day()-c.Day()

	if dd < 0 {
		dm = dm - 1
	}
	if dy == 0 && dm == 0 {
		return 0
	}
	if dy == 0 && dm != 0 && dd != 0 {
		if int(end.DiffInHoursWithAbs(&c)) < c.DaysInMonth()*HoursPerDay {
			return 0
		}
		return int64(dm)
	}

	return int64(dy*MonthsPerYear + dm)
}

// DiffInMonthsWithAbs 相差多少月(绝对值)
func (c ActorGoTime) DiffInMonthsWithAbs(end *ActorGoTime) int64 {
	return GetAbsValue(c.DiffInMonths(end))
}

// DiffInWeeks 相差多少周
func (c ActorGoTime) DiffInWeeks(end *ActorGoTime) int64 {
	return c.DiffInDays(end) / DaysPerWeek
}

// DiffInWeeksWithAbs 相差多少周(绝对值)
func (c ActorGoTime) DiffInWeeksWithAbs(end *ActorGoTime) int64 {
	return GetAbsValue(c.DiffInWeeks(end))
}

// DiffInDays 相差多少天
func (c ActorGoTime) DiffInDays(end *ActorGoTime) int64 {
	return c.DiffInSeconds(end) / SecondsPerDay
}

// DiffInDaysWithAbs 相差多少天(绝对值)
func (c ActorGoTime) DiffInDaysWithAbs(end *ActorGoTime) int64 {
	return GetAbsValue(c.DiffInDays(end))
}

// DiffInHours 相差多少小时
func (c ActorGoTime) DiffInHours(end *ActorGoTime) int64 {
	return c.DiffInSeconds(end) / SecondsPerHour
}

// DiffInHoursWithAbs 相差多少小时(绝对值)
func (c ActorGoTime) DiffInHoursWithAbs(end *ActorGoTime) int64 {
	return GetAbsValue(c.DiffInHours(end))
}

// DiffInMinutes 相差多少分钟
func (c ActorGoTime) DiffInMinutes(end *ActorGoTime) int64 {
	return c.DiffInSeconds(end) / SecondsPerMinute
}

// DiffInMinutesWithAbs 相差多少分钟(绝对值)
func (c ActorGoTime) DiffInMinutesWithAbs(end *ActorGoTime) int64 {
	return GetAbsValue(c.DiffInMinutes(end))
}

// DiffInSeconds 相差多少秒
func (c ActorGoTime) DiffInSeconds(end *ActorGoTime) int64 {
	return end.ToSecond() - c.ToSecond()
}

// DiffInSecondsWithAbs 相差多少秒(绝对值)
func (c ActorGoTime) DiffInSecondsWithAbs(end *ActorGoTime) int64 {
	return GetAbsValue(c.DiffInSeconds(end))
}

// DiffInMillisecond 相差多少毫秒
func (c ActorGoTime) DiffInMillisecond(end *ActorGoTime) int64 {
	return end.ToMillisecond() - c.ToMillisecond()
}

// DiffInMicrosecond 相差多少微秒
func (c ActorGoTime) DiffInMicrosecond(end *ActorGoTime) int64 {
	return end.ToMicrosecond() - c.ToMicrosecond()
}

// DiffINanosecond 相差多少纳秒
func (c ActorGoTime) DiffInNanosecond(end *ActorGoTime) int64 {
	return end.ToNanosecond() - c.ToNanosecond()
}

// DiffInNowMillisecond 与当前时间相差多少毫秒
func (c ActorGoTime) NowDiffMillisecond() int64 {
	return Now().ToMillisecond() - c.ToMillisecond()
}

// DiffInNowMillisecond 与当前时间相差多少秒
func (c ActorGoTime) NowDiffSecond() int64 {
	return Now().ToSecond() - c.ToSecond()
}
