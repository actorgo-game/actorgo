package cactor

import (
	"time"

	cfacade "github.com/actorgo-game/actorgo/facade"
)

type (
	// IActorLoader is implemented by embeddable handler bases that need the
	// runtime Actor reference before OnInit.
	IActorLoader interface {
		load(actor *Actor)
	}
)

type (
	// IEvent manages subscriptions owned by one Actor.
	IEvent interface {
		Register(name string, fn IEventFunc, uniqueID ...int64)     // 注册事件
		Registers(names []string, fn IEventFunc, uniqueID ...int64) // 注册多个事件
		Unregister(name string)                                     // 注销事件
	}

	// IEventFunc handles one event delivery.
	IEventFunc func(cfacade.IEventData) // 接收事件数据时的处理函数
)

type (
	// IMailBox exposes method registration during an Actor's OnInit hook.
	IMailBox interface {
		// Register installs a handler on this Actor mailbox. Top-level Actor
		// methods are published for external routing; child methods stay local.
		// Registration failures panic (typically during OnInit).
		Register(methodID uint32, handler any)
	}
)

type (
	// ITimer schedules callbacks owned by one Actor.
	ITimer interface {
		Add(d time.Duration, fn func(), async ...bool) uint64                   // 添加定时器,循环执行
		AddOnce(d time.Duration, fn func(), async ...bool) uint64               // 添加定时器,执行一次
		AddFixedHour(hour, minute, second int, fn func(), async ...bool) uint64 // 固定x小时x分x秒,循环执行
		AddFixedMinute(minute, second int, fn func(), async ...bool) uint64     // 固定x分x秒,循环执行
		AddSchedule(s ITimerSchedule, f func(), async ...bool) uint64           // 添加自定义调度
		Remove(id uint64)                                                       // 移除定时器
		RemoveAll()                                                             // 移除所有定时器
	}

	// ITimerSchedule calculates the next time for a custom recurring timer.
	ITimerSchedule interface {
		Next(time.Time) time.Time
	}
)
