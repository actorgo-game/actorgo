package cactor

import cfacade "github.com/actorgo-game/actorgo/facade"

var (
	Name = "actor_component"
)

// Component integrates the Actor system into the application lifecycle.
type Component struct {
	cfacade.Component
	*System
	actorHandlers []cfacade.IActorHandler
}

// New creates an empty Actor component; handlers may be queued with Add.
func New() *Component {
	return &Component{
		System: NewSystem(),
	}
}

func (c *Component) Name() string {
	return Name
}

// Init attaches the application services required for routing and codecs.
func (c *Component) Init() {
	c.System.SetApp(c.App())
}

// OnAfterInit creates all configured top-level Actors before later components start.
func (c *Component) OnAfterInit() {
	// Register actor
	for _, actor := range c.actorHandlers {
		if _, err := c.CreateActor(actor.AliasID(), actor); err != nil {
			panic(err)
		}
	}
}

// OnStop drains and stops every Actor owned by the system.
func (c *Component) OnStop() {
	c.System.Stop()
}

// Add queues top-level Actor handlers for creation during OnAfterInit.
func (c *Component) Add(actors ...cfacade.IActorHandler) {
	c.actorHandlers = append(c.actorHandlers, actors...)
}
