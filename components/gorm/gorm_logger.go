package cgorm

import (
	"strings"

	clog "github.com/actorgo-game/actorgo/logger"
)

type gormLogger struct {
	log *clog.ActorLogger
}

func (l gormLogger) Printf(s string, i ...any) {
	l.log.Debugf(strings.ReplaceAll(s, "\n", ""), i...)
}
