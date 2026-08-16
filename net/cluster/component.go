package ccluster

import (
	cfacade "github.com/actorgo-game/actorgo/facade"
	clog "github.com/actorgo-game/actorgo/logger"
	cnatsCluster "github.com/actorgo-game/actorgo/net/cluster/nats_cluster"
	crabbitmqCluster "github.com/actorgo-game/actorgo/net/cluster/rabbitmq_cluster"
	cprofile "github.com/actorgo-game/actorgo/profile"
)

const (
	Name = "cluster_component"

	modeNATS     = "nats"
	modeRabbitMQ = "rabbitmq"
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
	mode := cprofile.GetConfig("cluster").GetString("mode", modeNATS)
	switch mode {
	case modeRabbitMQ:
		clog.Info("cluster transport mode = rabbitmq")
		return crabbitmqCluster.New(c.App())
	case modeNATS, "":
		clog.Info("cluster transport mode = nats")
		return cnatsCluster.New(c.App())
	default:
		clog.Warn("unknown cluster.mode %q, fallback to nats", mode)
		return cnatsCluster.New(c.App())
	}
}
