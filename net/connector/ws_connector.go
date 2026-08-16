package cconnector

import (
	"io"
	"net/http"
	"slices"
	"time"

	cfacade "github.com/actorgo-game/actorgo/facade"
	clog "github.com/actorgo-game/actorgo/logger"
	"github.com/gorilla/websocket"
)

type (
	// WSConnector upgrades HTTP connections and dispatches binary WebSocket streams.
	WSConnector struct {
		cfacade.Component
		*Connector
		Options
		upgrade             *websocket.Upgrader
		requiredSubprotocol string
	}

	// WSConn adapts gorilla/websocket messages to net.Conn for the connector.
	WSConn struct {
		*websocket.Conn
		reader io.Reader
	}
)

func (*WSConnector) Name() string {
	return "websocket_connector"
}

// OnStop closes the listener and stops connection dispatch.
func (w *WSConnector) OnStop() {
	w.Connector.Stop()
}

// NewWS creates a WebSocket connector for the supplied listen address.
func NewWS(address string, opts ...Option) *WSConnector {
	if address == "" {
		clog.Warn("create websocket fail. address is null.")
		return nil
	}

	ws := &WSConnector{
		Options: Options{
			address:  address,
			certFile: "",
			keyFile:  "",
			chanSize: 256,
		},
		upgrade: &websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(_ *http.Request) bool {
				return true
			},
		},
	}

	for _, opt := range opts {
		opt(&ws.Options)
	}

	ws.Connector = NewConnector(ws.chanSize)

	return ws
}

// Start serves WebSocket upgrade requests until the listener is closed.
func (w *WSConnector) Start() {
	listener, err := w.GetListener(w.certFile, w.keyFile, w.address)
	if err != nil {
		clog.Fatal("failed to listen: %s", err)
	}

	clog.Info("Websocket connector listening at Address %s", w.address)
	if w.certFile != "" || w.keyFile != "" {
		clog.Info("certFile = %s, keyFile = %s", w.certFile, w.keyFile)
	}

	w.Connector.Start()

	http.Serve(listener, w)
}

// SetUpgrade replaces the WebSocket upgrader before Start.
func (w *WSConnector) SetUpgrade(upgrade *websocket.Upgrader) {
	if upgrade != nil {
		w.upgrade = upgrade
	}
}

// SetRequiredSubprotocol requires explicit AGP version negotiation at upgrade.
func (w *WSConnector) SetRequiredSubprotocol(protocol string) {
	// AGP WebSocket clients must explicitly negotiate agp.v1; this prevents a
	// generic WebSocket client from sending bytes interpreted as AGP packets.
	w.requiredSubprotocol = protocol
	if protocol != "" {
		w.upgrade.Subprotocols = []string{protocol}
	}
}

// ServeHTTP validates AGP negotiation, upgrades the request, and queues the connection.
func (w *WSConnector) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if w.requiredSubprotocol != "" && !slices.Contains(websocket.Subprotocols(r), w.requiredSubprotocol) {
		rw.Header().Set("Sec-WebSocket-Protocol", w.requiredSubprotocol)
		http.Error(rw, "websocket subprotocol required", http.StatusUpgradeRequired)
		return
	}
	wsConn, err := w.upgrade.Upgrade(rw, r, nil)
	if err != nil {
		clog.Info("Upgrade failure, URI=%s, Error=%s", r.RequestURI, err.Error())
		return
	}

	conn := NewWSConn(wsConn)
	w.InChan(&conn)
}

// NewWSConn return an initialized *WSConn
func NewWSConn(conn *websocket.Conn) WSConn {
	c := WSConn{
		Conn: conn,
	}
	return c
}

func (c *WSConn) Read(b []byte) (int, error) {
	if c.reader == nil {
		_, r, err := c.NextReader()
		if err != nil {
			return 0, err
		}
		c.reader = r
	}
	n, err := c.reader.Read(b)
	if err != nil && err != io.EOF {
		return n, err
	} else if err == io.EOF {
		_, r, err := c.NextReader()
		if err != nil {
			return 0, err
		}
		c.reader = r
	}

	return n, nil
}

func (c *WSConn) Write(b []byte) (int, error) {
	err := c.WriteMessage(websocket.BinaryMessage, b)
	if err != nil {
		return 0, err
	}

	return len(b), nil
}

func (c *WSConn) SetDeadline(t time.Time) error {
	if err := c.SetReadDeadline(t); err != nil {
		return err
	}

	return c.SetWriteDeadline(t)
}
