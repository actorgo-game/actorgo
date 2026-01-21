package cdiscovery

import (
	cfacade "github.com/actorgo-game/actorgo/facade"
	clog "github.com/actorgo-game/actorgo/logger"
)

var (
	discoveryMap = make(map[string]cfacade.IDiscovery)
)

func init() {
	Register(&DiscoveryDefault{})
	Register(&DiscoveryMaster{})
	//RegisterDiscovery(&DiscoveryETCD{})
}

func Register(discovery cfacade.IDiscovery) {
	if discovery == nil {
		clog.Fatal("Discovery instance is nil")
		return
	}

	if discovery.Name() == "" {
		clog.Fatal("Discovery name is empty. %T", discovery)
		return
	}
	discoveryMap[discovery.Name()] = discovery
}
