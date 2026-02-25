package cgops

import (
	cfacade "github.com/actorgo-game/actorgo/facade"
	clogger "github.com/actorgo-game/actorgo/logger"
	"github.com/google/gops/agent"
)

// Component gops 监听进程数据
type Component struct {
	cfacade.Component
	options agent.Options
}

func New(options ...agent.Options) *Component {
	component := &Component{}
	if len(options) > 0 {
		component.options = options[0]
	}
	return component
}

func (c *Component) Name() string {
	return "gops_component"
}

func (c *Component) Init() {
	if err := agent.Listen(c.options); err != nil {
		clogger.Error(err.Error())
	}
}

func (c *Component) OnAfterInit() {
}

func (c *Component) OnStop() {
	//agent.Close()
}
