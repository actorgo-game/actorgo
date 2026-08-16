package parser

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	cfacade "github.com/actorgo-game/actorgo/facade"
	clog "github.com/actorgo-game/actorgo/logger"
	cproto "github.com/actorgo-game/actorgo/net/proto"
)

// Component hosts AGP over one or more configured connectors and owns all
// accepted client connections.
type Component struct {
	cfacade.Component
	name        string
	connectors  []cfacade.IConnector
	connections *ConnectionManager
	options     Options
}

// New creates an AGP server component. Connectors are started after all
// application components and Actor methods have completed initialization.
func New(name string, connectors []cfacade.IConnector, opts ...Option) *Component {
	if name == "" {
		name = "default"
	}
	options := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	options.Limits = cproto.NormalizeLimits(options.Limits)
	return &Component{name: name, connectors: append([]cfacade.IConnector(nil), connectors...), connections: NewConnectionManager(), options: options}
}

// Name returns the application-unique component name.
func (c *Component) Name() string { return "agp_server_" + c.name }

// Connections exposes the connection index for authentication and server push.
func (c *Component) Connections() *ConnectionManager { return c.connections }

// Init verifies that the shared method table is available.
func (c *Component) Init() {
	if c.App() == nil || c.App().Methods() == nil {
		panic("actorgo agp: method table is nil")
	}
}

// OnAfterInit configures and starts every transport connector.
func (c *Component) OnAfterInit() {
	for _, connector := range c.connectors {
		if connector == nil {
			continue
		}
		connector.Set(c.App())
		if websocketConnector, ok := connector.(interface{ SetRequiredSubprotocol(string) }); ok {
			websocketConnector.SetRequiredSubprotocol("agp.v1")
		}
		connector.OnConnect(c.HandleConn)
		go connector.Start()
	}
}

// HandleConn adapts a connector connection to AGP packet boundaries.
func (c *Component) HandleConn(conn net.Conn) {
	if conn == nil {
		return
	}
	connection := newConnection(c.App(), newPacketTransport(conn, c.options), c.connections, c.options)
	c.connections.Add(connection)
	go connection.Run()
}

// OnBeforeStop first stops accepting sockets, then drains a stable connection
// snapshot so no new client can appear after CloseAll.
func (c *Component) OnBeforeStop() {
	for _, connector := range c.connectors {
		if connector != nil {
			connector.Stop()
		}
	}
	connections := c.connections.Snapshot()
	// Give clients a best-effort GoAway before the connectors and sockets close.
	var wait sync.WaitGroup
	for _, connection := range connections {
		wait.Add(1)
		go func() { defer wait.Done(); _ = connection.GoAway(0, 0) }()
	}
	finished := make(chan struct{})
	go func() { wait.Wait(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(c.options.WriteTimeout):
	}
	for _, connection := range connections {
		connection.Close()
	}
	c.waitDisconnects(connections, c.options.WriteTimeout)
}

// OnStop closes all configured connectors.
func (c *Component) OnStop() {
	for _, connector := range c.connectors {
		if connector != nil {
			connector.Stop()
		}
	}
	clog.Info("[component = %s] has been shut down", c.Name())
}

func (c *Component) waitDisconnects(connections []*Connection, timeout time.Duration) {
	var wait sync.WaitGroup
	for _, connection := range connections {
		wait.Add(1)
		go func() { defer wait.Done(); connection.waitDisconnect(timeout) }()
	}
	finished := make(chan struct{})
	go func() { wait.Wait(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(timeout):
	}
}

// Notify pushes a notification to one connection ID.
func (c *Component) Notify(connectionID string, methodID uint32, payload any) error {
	connection, ok := c.connections.Get(connectionID)
	if !ok {
		return fmt.Errorf("actorgo agp: connection %q not found", connectionID)
	}
	return connection.Notify(methodID, payload)
}

// Bind associates an authenticated UID and server-owned session data with a
// connection for later requests and server push.
func (c *Component) Bind(connectionID string, uid int64, data map[string]string) error {
	return c.connections.Bind(connectionID, uid, data)
}

// NotifyUID pushes a notification to the connection bound to uid.
func (c *Component) NotifyUID(uid int64, methodID uint32, payload any) error {
	connection, ok := c.connections.GetUID(uid)
	if !ok {
		return fmt.Errorf("actorgo agp: uid %d is not connected", uid)
	}
	return connection.Notify(methodID, payload)
}

// Broadcast pushes a notification to every active connection and joins errors.
func (c *Component) Broadcast(methodID uint32, payload any) error {
	var failures []error
	c.connections.Range(func(connection *Connection) bool {
		if err := connection.Notify(methodID, payload); err != nil {
			failures = append(failures, fmt.Errorf("connection %s: %w", connection.ID(), err))
		}
		return true
	})
	return errors.Join(failures...)
}

// Kick sends a terminal notification and closes the selected connection.
func (c *Component) Kick(connectionID string, reasonCode int32, reason string, reconnectable bool) error {
	connection, ok := c.connections.Get(connectionID)
	if !ok {
		return fmt.Errorf("actorgo agp: connection %q not found", connectionID)
	}
	return connection.Kick(reasonCode, reason, reconnectable)
}
