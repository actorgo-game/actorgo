package ccron

import (
	"testing"
	"time"

	ctime "github.com/actorgo-game/actorgo/extend/time"
	clog "github.com/actorgo-game/actorgo/logger"
)

func TestAddEveryDayFunc(t *testing.T) {
	AddEveryDayFunc(func() {
		now := ctime.Now()
		clog.Info(now.ToDateTimeFormat())
	}, 17, 32, 5)

	AddEveryHourFunc(func() {
		now := ctime.Now()
		clog.Info(now.ToDateTimeFormat())
		panic("print panic~~~")
	}, 5, 5)

	AddDurationFunc(func() {
		now := ctime.Now()
		clog.Info(now.ToDateTimeFormat())
	}, 1*time.Minute)

	Run()
}
