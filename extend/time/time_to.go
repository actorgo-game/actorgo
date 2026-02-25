package ctime

import (
	cstring "github.com/actorgo-game/actorgo/extend/string"
)

// ToSecond 输出秒级时间戳
func (c ActorGoTime) ToSecond() int64 {
	return c.Time.Unix()

}

// ToMillisecond 输出毫秒级时间戳
func (c ActorGoTime) ToMillisecond() int64 {
	return c.Time.UnixMilli()
}

func (c ActorGoTime) ToMillisecondString() string {
	t := c.ToMillisecond()
	return cstring.ToString(t)
}

// ToMicrosecond 输出微秒级时间戳
func (c ActorGoTime) ToMicrosecond() int64 {
	return c.Time.UnixMicro()
}

// ToNanosecond 输出纳秒级时间戳
func (c ActorGoTime) ToNanosecond() int64 {
	return c.Time.UnixNano()
}

// ToDateMillisecondFormat 2023-04-10 12:26:57.420
func (c ActorGoTime) ToDateMillisecondFormat() string {
	return c.Format(DateTimeMillisecondFormat)
}

// ToDateTimeFormat 2006-01-02 15:04:05
func (c ActorGoTime) ToDateTimeFormat() string {
	return c.Format(DateTimeFormat)
}

// ToDateFormat 2006-01-02
func (c ActorGoTime) ToDateFormat() string {
	return c.Format(DateFormat)
}

// ToTimeFormat 15:04:05
func (c ActorGoTime) ToTimeFormat() string {
	return c.Format(TimeFormat)
}

// ToShortDateTimeFormat 20060102150405
func (c ActorGoTime) ToShortDateTimeFormat() string {
	return c.Format(ShortDateTimeFormat)
}

// ToShortDateFormat 20060102
func (c ActorGoTime) ToShortDateFormat() string {
	return c.Format(ShortDateFormat)
}

// ToShortIntDateFormat 20060102
func (c ActorGoTime) ToShortIntDateFormat() int32 {
	strDate := c.ToShortDateFormat()
	return cstring.ToInt32D(strDate, 0)
}

// ToShortTimeFormat 150405
func (c ActorGoTime) ToShortTimeFormat() string {
	return c.Format(ShortTimeFormat)
}
