package ctime

import (
	"testing"
)

func TestTime_DaysInYear(t *testing.T) {
	now := Now()
	t.Logf("result = %v", now.DaysInYear())
}

func TestTime_DaysInMonth(t *testing.T) {
	now := Now()
	t.Logf("result = %v", now.DaysInMonth())
}

func TestTime_MonthOfYear(t *testing.T) {
	now := Now()
	t.Logf("result = %v", now.MonthOfYear())
}

func TestTime_DayOfYear(t *testing.T) {
	now := Now()
	t.Logf("result = %v", now.DayOfYear())
}

func TestTime_DayOfMonth(t *testing.T) {
	now := Now()
	t.Logf("result = %v", now.DayOfMonth())
}

func TestTime_DayOfWeek(t *testing.T) {
	now := Now()
	t.Logf("result = %v", now.DayOfWeek())
}

func TestTime_WeekOfYear(t *testing.T) {
	now := Now()
	t.Logf("result = %v", now.WeekOfYear())
}

func TestTime_WeekOfMonth(t *testing.T) {
	now := Now()
	t.Logf("result = %v", now.WeekOfMonth())
}

func TestTime_Year(t *testing.T) {
	now := Now()
	t.Logf("result = %v", now.Year())
}

func TestTime_Quarter(t *testing.T) {
	now := Now()
	t.Logf("result = %v", now.Quarter())
}

func TestTime_Month(t *testing.T) {
	now := Now()
	t.Logf("result = %v", now.Month())
}

func TestTime_Week(t *testing.T) {
	now := Now()
	t.Logf("result = %v", now.Week())
}

func TestTime_Day(t *testing.T) {
	now := Now()
	t.Logf("result = %v", now.Day())
}

func TestTime_Hour(t *testing.T) {
	now := Now()
	t.Logf("result = %v", now.Hour())
}

func TestTime_Minute(t *testing.T) {
	now := Now()
	t.Logf("result = %v", now.Minute())
}

func TestTime_Second(t *testing.T) {
	now := Now()
	t.Logf("result = %v", now.Second())
}

func TestTime_Millisecond(t *testing.T) {
	now := Now()
	t.Logf("result = %v", now.Millisecond())
}

func TestTime_Microsecond(t *testing.T) {
	now := Now()
	t.Logf("result = %v", now.Microsecond())
}

func TestTime_Nanosecond(t *testing.T) {
	now := Now()
	t.Logf("result = %v", now.Nanosecond())
}

func TestTime_Timezone(t *testing.T) {
	now := Now()
	t.Logf("result = %v", now.Timezone())
}
