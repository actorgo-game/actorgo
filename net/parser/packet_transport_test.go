package parser

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type fakeWebSocketConn struct {
	readType  int
	readData  []byte
	readErr   error
	writeType int
	writeData []byte
}

func (c *fakeWebSocketConn) ReadMessage() (int, []byte, error) {
	return c.readType, c.readData, c.readErr
}
func (c *fakeWebSocketConn) WriteMessage(messageType int, data []byte) error {
	c.writeType = messageType
	c.writeData = append([]byte(nil), data...)
	return nil
}
func (*fakeWebSocketConn) SetReadLimit(int64)               {}
func (*fakeWebSocketConn) SetReadDeadline(time.Time) error  { return nil }
func (*fakeWebSocketConn) SetWriteDeadline(time.Time) error { return nil }
func (*fakeWebSocketConn) RemoteAddr() net.Addr             { return fakeAddr("client") }
func (*fakeWebSocketConn) Close() error                     { return nil }

type fakeAddr string

func (a fakeAddr) Network() string { return "test" }
func (a fakeAddr) String() string  { return string(a) }

func TestWebSocketPacketBoundary(t *testing.T) {
	wire := &fakeWebSocketConn{readType: websocket.BinaryMessage, readData: []byte{1, 2, 3}}
	conn := &websocketPacketTransport{conn: wire, maxPacketSize: 16}

	got, err := conn.ReadPacketBytes()
	if err != nil {
		t.Fatalf("read packet: %v", err)
	}
	if string(got) != string([]byte{1, 2, 3}) {
		t.Fatalf("unexpected packet %v", got)
	}
	if err := conn.WritePacketBytes([]byte{4, 5}); err != nil {
		t.Fatalf("write packet: %v", err)
	}
	if wire.writeType != websocket.BinaryMessage || string(wire.writeData) != string([]byte{4, 5}) {
		t.Fatalf("unexpected websocket write type=%d data=%v", wire.writeType, wire.writeData)
	}
}

func TestWebSocketRejectsTextMessage(t *testing.T) {
	wire := &fakeWebSocketConn{readType: websocket.TextMessage, readData: []byte("not-agp")}
	conn := &websocketPacketTransport{conn: wire, maxPacketSize: 16}

	if _, err := conn.ReadPacketBytes(); err == nil {
		t.Fatal("expected text message to be rejected")
	}
}

func TestWebSocketReadError(t *testing.T) {
	want := errors.New("read failed")
	wire := &fakeWebSocketConn{readErr: want}
	conn := &websocketPacketTransport{conn: wire, maxPacketSize: 16}

	if _, err := conn.ReadPacketBytes(); !errors.Is(err, want) {
		t.Fatalf("expected %v, got %v", want, err)
	}
}
