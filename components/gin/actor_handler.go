package cgin

import (
	"fmt"

	httpactor "github.com/actorgo-game/actorgo/net/httpactor"
	"github.com/gin-gonic/gin"
)

// RegisterActorRoutes mounts the fixed ActorGo method endpoint.
// Existing Gin controllers and middleware continue to work.
func (p *HttpServer) RegisterActorRoutes(handler *httpactor.Handler) error {
	if handler == nil {
		return fmt.Errorf("actorgo gin: actor handler is nil")
	}
	p.Engine.POST(httpactor.Route, gin.WrapH(handler))
	return nil
}
