package cactor

import (
	"sync"
	"time"

	ctimeWheel "github.com/actorgo-game/actorgo/extend/time_wheel"
)

var (
	globalTimer     = ctimeWheel.NewTimeWheel(10*time.Millisecond, 3600)
	globalTimerOnce sync.Once
)

// ensureGlobalTimer starts the shared time wheel exactly once.
func ensureGlobalTimer() {
	globalTimerOnce.Do(func() {
		globalTimer.Start()
	})
}
