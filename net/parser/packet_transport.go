package parser

import (
	"fmt"
	"net"
	"time"

	cproto "github.com/actorgo-game/actorgo/net/proto"
	"github.com/gorilla/websocket"
)

// packetTransport normalizes TCP length frames and WebSocket message boundaries into
// one complete protobuf Packet byte slice per read/write.
type packetTransport interface {
	ReadPacketBytes() ([]byte, error)
	WritePacketBytes([]byte) error
	RemoteAddr() net.Addr
	SetReadDeadline(time.Time) error
	SetWriteDeadline(time.Time) error
	Close() error
}

type tcpPacketTransport struct {
	net.Conn
	framer *TCPPacketFramer
}

func (c *tcpPacketTransport) ReadPacketBytes() ([]byte, error) {
	return c.framer.ReadPacketBytes(c.Conn)
}
func (c *tcpPacketTransport) WritePacketBytes(data []byte) error {
	return c.framer.WritePacketBytes(c.Conn, data)
}

type websocketMessageConn interface {
	ReadMessage() (messageType int, data []byte, err error)
	WriteMessage(messageType int, data []byte) error
	SetReadLimit(limit int64)
	SetReadDeadline(time.Time) error
	SetWriteDeadline(time.Time) error
	RemoteAddr() net.Addr
	Close() error
}

type websocketPacketTransport struct {
	conn          websocketMessageConn
	maxPacketSize int
}

func (c *websocketPacketTransport) ReadPacketBytes() ([]byte, error) {
	typ, data, err := c.conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	if typ != websocket.BinaryMessage {
		return nil, fmt.Errorf("agp: websocket message must be binary")
	}
	if len(data) == 0 || len(data) > c.maxPacketSize {
		return nil, fmt.Errorf("agp: invalid websocket packet length %d", len(data))
	}
	return data, nil
}
func (c *websocketPacketTransport) WritePacketBytes(data []byte) error {
	if len(data) == 0 || len(data) > c.maxPacketSize {
		return fmt.Errorf("agp: invalid websocket packet length %d", len(data))
	}
	return c.conn.WriteMessage(websocket.BinaryMessage, data)
}
func (c *websocketPacketTransport) RemoteAddr() net.Addr { return c.conn.RemoteAddr() }
func (c *websocketPacketTransport) SetReadDeadline(deadline time.Time) error {
	return c.conn.SetReadDeadline(deadline)
}
func (c *websocketPacketTransport) SetWriteDeadline(deadline time.Time) error {
	return c.conn.SetWriteDeadline(deadline)
}
func (c *websocketPacketTransport) Close() error { return c.conn.Close() }

// newPacketTransport normalizes packet limits once and selects framing from the
// concrete connection type: WebSocket message boundaries or TCP length frames.
func newPacketTransport(conn net.Conn, options Options) packetTransport {
	options.Limits = cproto.NormalizeLimits(options.Limits)
	if ws, ok := conn.(websocketMessageConn); ok {
		ws.SetReadLimit(int64(options.Limits.MaxPacketSize))
		return &websocketPacketTransport{conn: ws, maxPacketSize: options.Limits.MaxPacketSize}
	}
	return &tcpPacketTransport{Conn: conn, framer: NewTCPPacketFramer(uint32(options.Limits.MaxPacketSize))}
}
