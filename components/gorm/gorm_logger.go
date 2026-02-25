package cgorm

import (
	"strings"

	clog "github.com/actorgo-game/actorgo/logger"
)

type gormLogger struct {
	log *clog.ActorLogger
}

func (l gormLogger) Printf(s string, i ...interface{}) {
	l.log.Debugf(strings.ReplaceAll(s, "\n", ""), i...)
}
