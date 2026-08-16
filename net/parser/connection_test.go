package parser

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	actorgo "github.com/actorgo-game/actorgo"
	cfacade "github.com/actorgo-game/actorgo/facade"
	cactor "github.com/actorgo-game/actorgo/net/actor"
	cproto "github.com/actorgo-game/actorgo/net/proto"
	cprofile "github.com/actorgo-game/actorgo/profile"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const connectionEchoMethod uint32 = 3101

type connectionTestNode struct{}

func (connectionTestNode) NodeID() string                { return "agp-node" }
func (connectionTestNode) NodeType() string              { return "test" }
func (connectionTestNode) Address() string               { return "127.0.0.1:0" }
func (connectionTestNode) RpcAddress() string            { return "127.0.0.1:0" }
func (connectionTestNode) Settings() cfacade.ProfileJSON { return cprofile.Wrap(map[string]any{}) }
func (connectionTestNode) Enabled() bool                 { return true }

type connectionEchoActor struct {
	cactor.Base
	ready chan struct{}
}

func (a *connectionEchoActor) AliasID() string { return "echo" }
func (a *connectionEchoActor) OnInit() {
	a.Methods().Register(connectionEchoMethod, func(_ *cfacade.RequestContext, request *wrapperspb.StringValue) (*wrapperspb.StringValue, error) {
		return wrapperspb.String(strings.ToUpper(request.Value)), nil
	})
	close(a.ready)
}

func TestConnectionHandshakeAndJSONRequest(t *testing.T) {
	app := actorgo.NewAppNode(connectionTestNode{}, actorgo.Standalone)
	actorComponent := app.ActorSystem().(*cactor.Component)
	actorComponent.Set(app)
	actorComponent.Init()
	actor := &connectionEchoActor{ready: make(chan struct{})}
	if _, err := actorComponent.CreateActor(actor.AliasID(), actor); err != nil {
		t.Fatal(err)
	}
	select {
	case <-actor.ready:
	case <-time.After(time.Second):
		t.Fatal("actor did not initialize")
	}
	defer actorComponent.OnStop()

	serverComponent := New("test", nil, WithHandshakeTimeout(time.Second), WithMaxRequestTimeout(time.Second), WithWriteTimeout(100*time.Millisecond))
	serverComponent.Set(app)
	serverComponent.Init()
	server, client := net.Pipe()
	defer client.Close()
	serverComponent.HandleConn(server)
	defer serverComponent.OnBeforeStop()

	packetCodec := cproto.NewPacketCodec(cproto.DefaultLimits())
	framer := NewTCPPacketFramer(cproto.DefaultMaxPacketSize)
	handshakeBody, _ := proto.Marshal(&cproto.HandshakeRequest{SupportedVersions: []uint32{1}})
	writeTestPacket(t, client, framer, packetCodec, &cproto.Packet{Kind: &cproto.Packet_Request{Request: &cproto.Request{RequestId: 1, MethodId: cproto.SystemMethodHandshake, Body: handshakeBody}}, Codec: cfacade.CodecProtobuf})
	handshakeResponse := readTestPacket(t, client, framer, packetCodec).GetResponse()
	if handshakeResponse == nil || handshakeResponse.Code != int32(cproto.StatusCode_STATUS_OK) {
		t.Fatalf("bad handshake response %#v", handshakeResponse)
	}
	handshake := new(cproto.HandshakeResponse)
	if err := proto.Unmarshal(handshakeResponse.Body, handshake); err != nil {
		t.Fatal(err)
	}
	if handshake.ProtocolVersion != 1 {
		t.Fatalf("unexpected protocol version %d", handshake.ProtocolVersion)
	}

	writeTestPacket(t, client, framer, packetCodec, &cproto.Packet{Kind: &cproto.Packet_Request{Request: &cproto.Request{RequestId: 2, MethodId: connectionEchoMethod, TimeoutMs: 500, Body: []byte(`"hello"`)}}, Codec: cfacade.CodecJSON})
	responsePacket := readTestPacket(t, client, framer, packetCodec)
	response := responsePacket.GetResponse()
	if response == nil || response.Code != int32(cproto.StatusCode_STATUS_OK) {
		t.Fatalf("bad response %#v", response)
	}
	if string(response.Body) != `"HELLO"` {
		t.Fatalf("unexpected JSON response %s", response.Body)
	}

	writeTestPacket(t, client, framer, packetCodec, &cproto.Packet{Kind: &cproto.Packet_Request{Request: &cproto.Request{RequestId: 3, MethodId: 9999, TimeoutMs: 500, Body: []byte(`"x"`)}}, Codec: cfacade.CodecJSON})
	unknownMethod := readTestPacket(t, client, framer, packetCodec).GetResponse()
	if unknownMethod == nil || unknownMethod.Code != int32(cproto.StatusCode_STATUS_NOT_FOUND) {
		t.Fatalf("expected unknown method rejection %#v", unknownMethod)
	}
}

func TestConnectionOnDisconnect(t *testing.T) {
	called := make(chan string, 1)
	manager := NewConnectionManager()
	connection := &Connection{
		id:       "connection-disconnect",
		session:  &cproto.Session{Sid: "connection-disconnect", Uid: 9, Data: map[string]string{}},
		manager:  manager,
		done:     make(chan struct{}),
		inflight: map[uint32]context.CancelFunc{},
		options: Options{OnDisconnect: func(c *Connection) {
			called <- c.ID()
		}},
	}
	manager.Add(connection)
	connection.Close()
	select {
	case id := <-called:
		if id != "connection-disconnect" {
			t.Fatalf("unexpected connection id %q", id)
		}
	case <-time.After(time.Second):
		t.Fatal("OnDisconnect was not called")
	}
	if manager.Count() != 0 {
		t.Fatalf("expected connection removed, count=%d", manager.Count())
	}
}

func TestConnectionCloseDoesNotBlockOnDisconnect(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	manager := NewConnectionManager()
	connection := &Connection{
		id:       "connection-async-disconnect",
		session:  &cproto.Session{Sid: "connection-async-disconnect", Uid: 10, Data: map[string]string{}},
		manager:  manager,
		done:     make(chan struct{}),
		inflight: map[uint32]context.CancelFunc{},
		options: Options{OnDisconnect: func(*Connection) {
			close(started)
			<-release
		}},
	}
	manager.Add(connection)
	connection.Close()
	if manager.Count() != 0 {
		t.Fatal("connection must be removed before OnDisconnect completes")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("OnDisconnect did not start")
	}
	close(release)
	connection.waitDisconnect(time.Second)
}

func TestDrainingConnectionRejectsBusinessRequests(t *testing.T) {
	connection := &Connection{}
	connection.state.Store(int32(ConnectionDraining))
	packet := &cproto.Packet{
		Kind:  &cproto.Packet_Request{Request: &cproto.Request{RequestId: 1, MethodId: connectionEchoMethod}},
		Codec: cfacade.CodecProtobuf,
	}
	if connection.process(packet) {
		t.Fatal("draining connection accepted a business request")
	}
}

type addressOnlyTransport struct{ address net.Addr }

func (*addressOnlyTransport) ReadPacketBytes() ([]byte, error) { return nil, net.ErrClosed }
func (*addressOnlyTransport) WritePacketBytes([]byte) error    { return nil }
func (c *addressOnlyTransport) RemoteAddr() net.Addr           { return c.address }
func (*addressOnlyTransport) SetReadDeadline(time.Time) error  { return nil }
func (*addressOnlyTransport) SetWriteDeadline(time.Time) error { return nil }
func (*addressOnlyTransport) Close() error                     { return nil }

func TestConnectionSessionStoresRemoteIPWithoutPort(t *testing.T) {
	app := actorgo.NewAppNode(connectionTestNode{}, actorgo.Standalone)
	transport := &addressOnlyTransport{address: &net.TCPAddr{IP: net.ParseIP("192.0.2.10"), Port: 32100}}
	connection := newConnection(app, transport, nil, defaultOptions())
	if got := connection.Session().Ip; got != "192.0.2.10" {
		t.Fatalf("unexpected session IP %q", got)
	}
}

func TestConnectionBindReplacesSessionData(t *testing.T) {
	connection := &Connection{session: &cproto.Session{Sid: "connection-1", Data: map[string]string{"old": "value"}}}
	connection.bind(42, map[string]string{"role": "player"})

	session := connection.Session()
	if session.Uid != 42 || session.GetString("role") != "player" {
		t.Fatalf("unexpected session %#v", session)
	}
	if session.Contains("old") {
		t.Fatalf("stale session data was retained: %#v", session.Data)
	}

	// Session returns a snapshot rather than exposing connection-owned state.
	session.Set("role", "admin")
	if got := connection.Session().GetString("role"); got != "player" {
		t.Fatalf("connection session was mutated through snapshot: %q", got)
	}
}

func writeTestPacket(t *testing.T, conn net.Conn, framer *TCPPacketFramer, codec *cproto.PacketCodec, packet *cproto.Packet) {
	t.Helper()
	data, err := codec.Encode(packet)
	if err != nil {
		t.Fatal(err)
	}
	if err := framer.WritePacketBytes(conn, data); err != nil {
		t.Fatal(err)
	}
}
func readTestPacket(t *testing.T, conn net.Conn, framer *TCPPacketFramer, codec *cproto.PacketCodec) *cproto.Packet {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	data, err := framer.ReadPacketBytes(conn)
	if err != nil {
		t.Fatal(err)
	}
	packet, err := codec.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	return packet
}

func TestConnectionCancelIsInstalledBeforeDispatch(t *testing.T) {
	connection := &Connection{inflight: make(map[uint32]context.CancelFunc), options: Options{MaxInflight: 1}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if !connection.reserve(42, cancel) {
		t.Fatal("reserve failed")
	}
	body, err := proto.Marshal(&cproto.CancelNotify{RequestId: 42})
	if err != nil {
		t.Fatal(err)
	}
	if !connection.handleCancel(body) {
		t.Fatal("cancel notification was rejected")
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("reserved request was not cancelled")
	}
}
