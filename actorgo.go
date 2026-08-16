package actorgo

import (
	cfacade "github.com/actorgo-game/actorgo/facade"
	ccluster "github.com/actorgo-game/actorgo/net/cluster"
	cdiscovery "github.com/actorgo-game/actorgo/net/discovery"
)

type (
	// AppBuilder collects components and top-level Actor handlers before the
	// application lifecycle starts.
	AppBuilder struct {
		*Application
		components []cfacade.IComponent
	}
)

// Configure loads the requested node from a profile and returns its builder.
func Configure(profileFilePath, nodeIDStr string, mode NodeMode) *AppBuilder {
	appBuilder := &AppBuilder{
		Application: NewApp(profileFilePath, nodeIDStr, mode),
		components:  make([]cfacade.IComponent, 0),
	}

	return appBuilder
}

// ConfigureNode builds an application from a programmatically supplied node.
func ConfigureNode(node cfacade.INode, mode NodeMode) *AppBuilder {
	appBuilder := &AppBuilder{
		Application: NewAppNode(node, mode),
		components:  make([]cfacade.IComponent, 0),
	}

	return appBuilder
}

// Startup installs the built-in cluster components when required, registers
// user components, and then enters the blocking application lifecycle.
func (p *AppBuilder) Startup() {
	app := p.Application

	if app.NodeMode() == Cluster {
		cluster := ccluster.New()
		app.SetCluster(cluster)
		app.Register(cluster)

		discovery := cdiscovery.New()
		app.SetDiscovery(discovery)
		app.Register(discovery)
	}

	// Register custom components
	app.Register(p.components...)

	// startup
	app.Startup()
}

// Register queues components for registration during Startup.
func (p *AppBuilder) Register(component ...cfacade.IComponent) {
	p.components = append(p.components, component...)
}

// AddActors queues top-level Actor handlers for creation before listeners start.
func (p *AppBuilder) AddActors(actors ...cfacade.IActorHandler) {
	p.actorSystem.Add(actors...)
}
