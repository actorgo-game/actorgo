package ccron

import (
	cfacade "github.com/actorgo-game/actorgo/facade"
	clog "github.com/actorgo-game/actorgo/logger"
	"github.com/robfig/cron/v3"
)

const (
	Name = "cron_component"
)

type Component struct {
	cfacade.Component
}

// Name unique components name
func (*Component) Name() string {
	return Name
}

func (p *Component) Init() {
	Start()
	clog.Info("cron component init.")
}

func (p *Component) OnStop() {
	Stop()
	clog.Info("cron component is stopped.")
}

func New(opts ...cron.Option) cfacade.IComponent {
	if len(opts) > 0 {
		Init(opts...)
	}
	return &Component{}
}
