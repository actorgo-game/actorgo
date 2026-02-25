package ctime

import (
	"testing"
)

func TestTime_Now(t *testing.T) {
	now := Now()

	AddOffsetTime(-60)

	now1 := Now()
	t.Logf("now = %s, now-offset = %s\n", now.ToDateTimeFormat(), now1.ToDateTimeFormat())
}

func TestTime_Yesterday(t *testing.T) {
	yesterday := Yesterday()
	t.Log(yesterday.ToDateTimeFormat())
}

func TestTime_Tomorrow(t *testing.T) {
	yesterday := Tomorrow()
	t.Log(yesterday.ToDateTimeFormat())
}

func TestTime_CreateFromTimestamp(t *testing.T) {
	ct := CreateFromTimestamp(1614150502000)
	t.Log(ct.ToDateTimeFormat())
}

func TestTime_CreateFromDateTime(t *testing.T) {
	ct := CreateFromDateTime(2012, 12, 24, 23, 59, 59)
	t.Log(ct.ToDateTimeFormat())
}

func TestTime_CreateFromDate(t *testing.T) {
	ct := CreateFromDate(2012, 12, 24)
	t.Log(ct.ToDateTimeFormat())
}

func TestTime_CreateFromTime(t *testing.T) {
	ct := CreateFromTime(23, 59, 59)
	t.Log(ct.ToDateTimeFormat())
}
