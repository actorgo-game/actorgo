package cactor

import (
	"time"

	ctimeWheel "github.com/actorgo-game/actorgo/extend/time_wheel"
)

var (
	globalTimer = ctimeWheel.NewTimeWheel(10*time.Millisecond, 3600)
)

func init() {
	globalTimer.Start()
}
