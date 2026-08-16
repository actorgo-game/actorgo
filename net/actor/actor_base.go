package cactor

import (
	cfacade "github.com/actorgo-game/actorgo/facade"
)

// Base provides the runtime Actor handle and default lifecycle hooks for
// embedding in business Actor handlers.
type Base struct {
	*Actor
}

func (p *Base) load(a *Actor) {
	p.Actor = a
}

func (p *Base) AliasID() string {
	return ""
}

// OnInit Actor初始化前触发该函数
func (*Base) OnInit() {
}

// OnStop Actor停止前触发该函数
func (*Base) OnStop() {
}

// OnFindChild 寻找子Actor时触发该函数.开发者可以自定义创建子Actor
func (*Base) OnFindChild(_ *cfacade.Message) (cfacade.IActor, bool) {
	return nil, false
}

// NewPath builds a top-level Actor path for an explicit node.
func (p *Base) NewPath(nodeID, actorID any) string {
	return cfacade.NewPath(nodeID, actorID)
}

// NewNodePath builds a top-level Actor path on the current node.
func (p *Base) NewNodePath(actorID any) string {
	return cfacade.NewPath(p.path.NodeID, actorID)
}

// NewChildPath builds a child path under another parent on the current node.
func (p *Base) NewChildPath(actorID, childID any) string {
	return cfacade.NewChildPath(p.path.NodeID, actorID, childID)
}

// NewMyChildPath builds a path for one of the current Actor's children.
func (p *Base) NewMyChildPath(childID any) string {
	return cfacade.NewChildPath(p.path.NodeID, p.path.ActorID, childID)
}
