package httpactor

import (
	"context"
	"net"
	"net/http"
	"time"

	cfacade "github.com/actorgo-game/actorgo/facade"
	clog "github.com/actorgo-game/actorgo/logger"
)

// Component runs the Actor HTTP handler as a standalone net/http server.
type Component struct {
	cfacade.Component
	name    string
	address string
	handler *Handler
	server  *http.Server
	options []Option
}

// NewComponent creates a standalone Actor HTTP endpoint.
func NewComponent(name, address string, opts ...Option) *Component {
	if name == "" {
		name = "default"
	}
	return &Component{name: name, address: address, options: opts}
}

// Name returns the application-unique component name.
func (c *Component) Name() string { return "http_actor_" + c.name }

// Handler returns the initialized HTTP transport adapter.
func (c *Component) Handler() *Handler { return c.handler }

// Init builds the handler and HTTP server after the application is attached.
func (c *Component) Init() {
	if c.address == "" {
		c.address = "127.0.0.1:9080"
	}
	c.handler = NewHandler(c.App(), c.options...)
	c.server = &http.Server{Addr: c.address, Handler: c.handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
}

// OnAfterInit starts listening after Actor routes are available.
func (c *Component) OnAfterInit() {
	// Listen only after Actor initialization has populated the method table.
	go func() {
		listener, err := net.Listen("tcp", c.address)
		if err != nil {
			clog.Error("http actor listen failed. [address = %s, err = %v]", c.address, err)
			return
		}
		if err := c.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			clog.Error("http actor serve failed. [err = %v]", err)
		}
	}()
}

// OnStop performs a bounded graceful HTTP shutdown.
func (c *Component) OnStop() {
	if c.server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = c.server.Shutdown(ctx)
}
