package ccluster

import (
	cfacade "github.com/actorgo-game/actorgo/facade"
	cnatsCluster "github.com/actorgo-game/actorgo/net/cluster/nats_cluster"
)

const (
	Name = "cluster_component"
)

type Component struct {
	cfacade.Component
	cfacade.ICluster
}

func New() *Component {
	return &Component{}
}

func (c *Component) Name() string {
	return Name
}

func (c *Component) Init() {
	c.ICluster = c.loadCluster()
	c.ICluster.Init()
}

func (c *Component) OnStop() {
	c.ICluster.Stop()
}

func (c *Component) loadCluster() cfacade.ICluster {
	return cnatsCluster.New(c.App())
}
