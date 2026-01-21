package ctime

import (
	"testing"
)

func TestTime_IsNow(t *testing.T) {
	t.Logf("result = %v", Now().IsNow())
}

func TestTime_IsFuture(t *testing.T) {
	t.Logf("result = %v", Now().IsFuture())
}

func TestTime_IsPast(t *testing.T) {
	t.Logf("result = %v", Now().IsPast())
}

func TestTime_IsLeapYear(t *testing.T) {
	t.Logf("result = %v", Now().IsLeapYear())
}

func TestTime_IsLongYear(t *testing.T) {
	t.Logf("result = %v", Now().IsLongYear())
}

func TestTime_IsJanuary(t *testing.T) {
	t.Logf("result = %v", Now().IsJanuary())
}

func TestTime_IsFebruary(t *testing.T) {
	t.Logf("result = %v", Now().IsFebruary())
}

func TestTime_IsMarch(t *testing.T) {
	t.Logf("result = %v", Now().IsMarch())
}

func TestTime_IsApril(t *testing.T) {
	t.Logf("result = %v", Now().IsApril())
}

func TestTime_IsMay(t *testing.T) {
	t.Logf("result = %v", Now().IsMay())
}

func TestTime_IsJune(t *testing.T) {
	t.Logf("result = %v", Now().IsJune())
}

func TestTime_IsJuly(t *testing.T) {
	t.Logf("result = %v", Now().IsJuly())
}

func TestTime_IsAugust(t *testing.T) {
	t.Logf("result = %v", Now().IsAugust())
}

func TestTime_IsSeptember(t *testing.T) {
	t.Logf("result = %v", Now().IsSeptember())
}

func TestTime_IsOctober(t *testing.T) {
	t.Logf("result = %v", Now().IsOctober())
}

func TestTime_IsDecember(t *testing.T) {
	t.Logf("result = %v", Now().IsDecember())
}

func TestTime_IsMonday(t *testing.T) {
	t.Logf("result = %v", Now().IsMonday())
}

func TestTime_IsTuesday(t *testing.T) {
	t.Logf("result = %v", Now().IsTuesday())
}

func TestTime_IsWednesday(t *testing.T) {
	t.Logf("result = %v", Now().IsWednesday())
}

func TestTime_IsThursday(t *testing.T) {
	t.Logf("result = %v", Now().IsThursday())
}

func TestTime_IsFriday(t *testing.T) {
	t.Logf("result = %v", Now().IsFriday())
}

func TestTime_IsSaturday(t *testing.T) {
	t.Logf("result = %v", Now().IsSaturday())
}

func TestTime_IsSunday(t *testing.T) {
	t.Logf("result = %v", Now().IsSunday())
}

func TestTime_IsWeekday(t *testing.T) {
	t.Logf("result = %v", Now().IsWeekday())
}

func TestTime_IsWeekend(t *testing.T) {
	t.Logf("result = %v", Now().IsWeekend())
}

func TestTime_IsYesterday(t *testing.T) {
	t.Logf("result = %v", Now().IsYesterday())
}

func TestTime_IsYesterday1(t *testing.T) {
	now := Now()
	now.SubDay()
	t.Logf("result = %v", now.IsYesterday())
}

func TestTime_IsToday(t *testing.T) {
	t.Logf("result = %v", Now().IsToday())
}

func TestTime_IsTomorrow(t *testing.T) {
	t.Logf("result = %v", Now().IsTomorrow())
}
