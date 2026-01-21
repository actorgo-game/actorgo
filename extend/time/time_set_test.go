package ctime

import (
	"testing"
)

func TestTime_SetYear(t *testing.T) {
	now := Now()
	now.SetYear(2012)
	t.Logf("result = %v", now.ToDateTimeFormat())
}

func TestTime_SetMonth(t *testing.T) {
	now := Now()
	now.SetMonth(12)
	t.Logf("result = %v", now.ToDateTimeFormat())
}

func TestTime_SetDay(t *testing.T) {
	now := Now()
	now.SetDay(12)
	t.Logf("result = %v", now.ToDateTimeFormat())
}

func TestTime_SetHour(t *testing.T) {
	now := Now()
	now.SetHour(0)
	t.Logf("result = %v", now.ToDateTimeFormat())
}

func TestTime_SetMinute(t *testing.T) {
	now := Now()
	now.SetMinute(0)
	t.Logf("result = %v", now.ToDateTimeFormat())
}

func TestTime_SetSecond(t *testing.T) {
	now := Now()
	now.SetSecond(59)
	t.Logf("result = %v", now.ToDateTimeFormat())

	now.SetSecond(60)
	t.Logf("result = %v", now.ToDateTimeFormat())
}
