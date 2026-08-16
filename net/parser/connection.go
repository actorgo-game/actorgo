package parser

import (
	"context"
	"fmt"
	"maps"
	"net"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	cfacade "github.com/actorgo-game/actorgo/facade"
	clog "github.com/actorgo-game/actorgo/logger"
	cproto "github.com/actorgo-game/actorgo/net/proto"
	"github.com/nats-io/nuid"
	"google.golang.org/protobuf/proto"
)

// ConnectionState is the lifecycle state of one AGP client connection.
type ConnectionState int32

// outboundPacket optionally carries an acknowledgement channel for control
// packets that must reach the socket before a lifecycle transition completes.
type outboundPacket struct {
	packet *cproto.Packet
	result chan error
}

const (
	// A connection must complete the protobuf handshake before application calls.
	ConnectionHandshaking ConnectionState = iota + 1
	ConnectionReady
	ConnectionDraining
	ConnectionClosed
)

// Connection owns AGP framing, request cancellation and the session associated
// with one TCP or WebSocket client.
type Connection struct {
	app            cfacade.IApplication
	conn           packetTransport
	methods        cfacade.IMethodTable
	manager        *ConnectionManager
	options        Options
	codec          *cproto.PacketCodec
	id             string
	session        *cproto.Session
	sessionMu      sync.RWMutex
	state          atomic.Int32
	writeCh        chan outboundPacket
	done           chan struct{}
	closeOnce      sync.Once
	disconnectOnce sync.Once
	disconnectMu   sync.Mutex
	disconnectDone chan struct{}
	inflightMu     sync.Mutex
	inflight       map[uint32]context.CancelFunc
}

func newConnection(app cfacade.IApplication, conn packetTransport, manager *ConnectionManager, options Options) *Connection {
	id := nuid.Next()
	remoteIP := ""
	if conn != nil && conn.RemoteAddr() != nil {
		remoteIP = conn.RemoteAddr().String()
		if host, _, err := net.SplitHostPort(remoteIP); err == nil {
			remoteIP = host
		}
	}
	c := &Connection{
		app: app, conn: conn, methods: app.Methods(), manager: manager, options: options,
		codec: cproto.NewPacketCodec(options.Limits), id: id,
		session: &cproto.Session{Sid: id, Ip: remoteIP, Data: map[string]string{}},
		writeCh: make(chan outboundPacket, options.WriteQueueSize), done: make(chan struct{}),
		disconnectDone: make(chan struct{}),
		inflight:       make(map[uint32]context.CancelFunc),
	}
	c.state.Store(int32(ConnectionHandshaking))
	return c
}

// ID returns the server-assigned connection identifier.
func (c *Connection) ID() string { return c.id }

// Session returns a snapshot so handlers cannot mutate connection state without
// going through the manager.
func (c *Connection) Session() *cproto.Session {
	c.sessionMu.RLock()
	defer c.sessionMu.RUnlock()
	return &cproto.Session{Sid: c.session.Sid, Uid: c.session.Uid, Ip: c.session.Ip, Data: maps.Clone(c.session.Data)}
}

// UID returns the currently bound user ID, or zero when unbound.
func (c *Connection) UID() int64 {
	c.sessionMu.RLock()
	defer c.sessionMu.RUnlock()
	return c.session.Uid
}

func (c *Connection) bind(uid int64, data map[string]string) {
	c.sessionMu.Lock()
	c.session.Uid = uid
	c.session.Data = maps.Clone(data)
	if c.session.Data == nil {
		c.session.Data = make(map[string]string)
	}
	c.sessionMu.Unlock()
}

// State returns the current connection lifecycle state.
func (c *Connection) State() ConnectionState {
	return ConnectionState(c.state.Load())
}

// Run performs the handshake/state validation loop. Packet handlers may run in
// separate goroutines, but all writes are serialized by writeLoop.
func (c *Connection) Run() {
	if c.conn == nil {
		c.Close()
		return
	}
	go c.writeLoop()
	if c.options.HandshakeTimeout > 0 {
		_ = c.conn.SetReadDeadline(time.Now().Add(c.options.HandshakeTimeout))
	}
	for {
		data, err := c.conn.ReadPacketBytes()
		if err != nil {
			c.Close()
			return
		}
		packet, err := c.codec.Decode(data)
		if err != nil {
			clog.Warn("agp packet rejected. [connectionID = %s, err = %v]", c.id, err)
			c.Close()
			return
		}
		if c.State() != ConnectionHandshaking && c.options.IdleTimeout > 0 {
			_ = c.conn.SetReadDeadline(time.Now().Add(c.options.IdleTimeout))
		}
		if !c.process(packet) {
			c.Close()
			return
		}
	}
}

func (c *Connection) process(packet *cproto.Packet) bool {
	// System methods are accepted only in their valid lifecycle state and always
	// use protobuf bodies, independent of the application's default body codec.
	switch kind := packet.Kind.(type) {
	case *cproto.Packet_Request:
		state := c.State()
		if state == ConnectionHandshaking {
			return packet.Codec == cfacade.CodecProtobuf && kind.Request.MethodId == cproto.SystemMethodHandshake && c.handleHandshake(kind.Request)
		}
		if state != ConnectionReady {
			return false
		}
		if kind.Request.MethodId == cproto.SystemMethodHeartbeat {
			if packet.Codec != cfacade.CodecProtobuf {
				return false
			}
			c.handleHeartbeat(kind.Request)
			return true
		}
		if kind.Request.MethodId <= cproto.SystemMethodKick {
			return false
		}
		ctx, cancel := c.requestContext(packet, kind.Request.RequestId, kind.Request.TimeoutMs)
		if !c.reserve(kind.Request.RequestId, cancel) {
			cancel()
			return false
		}
		go c.handleRequest(packet, kind.Request, ctx, cancel)
		return true
	case *cproto.Packet_Notify:
		if c.State() != ConnectionReady {
			return false
		}
		if kind.Notify.MethodId == cproto.SystemMethodCancel {
			if packet.Codec != cfacade.CodecProtobuf {
				return false
			}
			return c.handleCancel(kind.Notify.Body)
		}
		if kind.Notify.MethodId <= cproto.SystemMethodKick {
			return false
		}
		c.handleNotify(packet, kind.Notify)
		return true
	default:
		return false
	}
}

func (c *Connection) handleHandshake(request *cproto.Request) bool {
	if request == nil || request.RequestId == 0 {
		return false
	}
	handshake := new(cproto.HandshakeRequest)
	if err := proto.Unmarshal(request.Body, handshake); err != nil {
		return false
	}
	if !slices.Contains(handshake.SupportedVersions, uint32(1)) {
		return false
	}
	response := &cproto.HandshakeResponse{
		ProtocolVersion:     1,
		HeartbeatIntervalMs: uint32(c.options.HeartbeatInterval / time.Millisecond),
		MaxFrameSize:        uint32(c.options.Limits.MaxPacketSize), ConnectionId: c.id,
	}
	body, err := proto.Marshal(response)
	if err != nil {
		return false
	}
	c.state.Store(int32(ConnectionReady))
	if c.options.IdleTimeout > 0 {
		_ = c.conn.SetReadDeadline(time.Now().Add(c.options.IdleTimeout))
	}
	return c.send(responsePacket(request.RequestId, cfacade.CodecProtobuf, cfacade.OKResult(nil), body))
}

func (c *Connection) handleHeartbeat(request *cproto.Request) {
	heartbeat := new(cproto.HeartbeatRequest)
	if err := proto.Unmarshal(request.Body, heartbeat); err != nil {
		c.Close()
		return
	}
	body, err := proto.Marshal(&cproto.HeartbeatResponse{ClientTimeMs: heartbeat.ClientTimeMs, ServerTimeMs: time.Now().UnixMilli()})
	if err != nil {
		c.Close()
		return
	}
	if !c.send(responsePacket(request.RequestId, cfacade.CodecProtobuf, cfacade.OKResult(nil), body)) {
		c.Close()
	}
}

func (c *Connection) handleRequest(packet *cproto.Packet, request *cproto.Request, ctx *cfacade.RequestContext, cancel context.CancelFunc) {
	defer func() { cancel(); c.release(request.RequestId) }()

	result := c.methods.Dispatch(ctx, request.MethodId, request.Body, cproto.MsgType_REQUEST)
	var body []byte
	if result != nil && result.OK() && result.Payload != nil {
		if encoded, ok := result.Payload.([]byte); ok {
			body = encoded
		} else {
			var err error
			body, err = c.app.BodyCodecs().Marshal(ctx.Codec, result.Payload)
			if err != nil {
				result = cfacade.ErrorResult(cproto.StatusCode_STATUS_INTERNAL, "response body encode failed")
				body = nil
			}
		}
	}
	if result == nil {
		result = cfacade.ErrorResult(cproto.StatusCode_STATUS_INTERNAL, "method table returned nil result")
	}
	if !c.send(responsePacket(request.RequestId, packet.Codec, result, body)) {
		c.Close()
	}
}

func (c *Connection) handleNotify(packet *cproto.Packet, notify *cproto.Notify) {
	ctx, cancel := c.requestContext(packet, 0, 0)
	defer cancel()

	result := c.methods.Dispatch(ctx, notify.MethodId, notify.Body, cproto.MsgType_NOTIFY)
	if result != nil && !result.OK() {
		clog.Warn("agp notify rejected. [connectionID = %s, methodID = %d, code = %d]", c.id, notify.MethodId, result.Code)
	}
}

func (c *Connection) handleCancel(body []byte) bool {
	cancelNotify := new(cproto.CancelNotify)
	if err := proto.Unmarshal(body, cancelNotify); err != nil || cancelNotify.RequestId == 0 {
		return false
	}
	c.inflightMu.Lock()
	cancel := c.inflight[cancelNotify.RequestId]
	c.inflightMu.Unlock()
	if cancel != nil {
		cancel()
	}
	return true
}

// requestContext creates the server-owned deadline and snapshots packet/session
// metadata so Actor handlers never observe later connection mutations.
func (c *Connection) requestContext(packet *cproto.Packet, requestID, timeoutMS uint32) (*cfacade.RequestContext, context.CancelFunc) {
	timeout := time.Duration(timeoutMS) * time.Millisecond
	if timeout <= 0 || timeout > c.options.MaxRequestTimeout {
		timeout = c.options.MaxRequestTimeout
	}
	base, cancel := context.WithTimeout(context.Background(), timeout)
	ctx := cfacade.NewRequestContext(base)
	ctx.RequestID, ctx.Transport = requestID, cfacade.TransportAGP
	ctx.Codec = packet.Codec
	ctx.Session, ctx.Metadata = c.Session(), cloneMetadata(packet.Metadata)
	return ctx, cancel
}

// reserve atomically enforces request ID uniqueness and the per-connection
// in-flight limit before a request handler goroutine starts.
func (c *Connection) reserve(requestID uint32, cancel context.CancelFunc) bool {
	// Reserve before starting the handler so a concurrent CancelNotify cannot
	// arrive between goroutine creation and inflight registration.
	c.inflightMu.Lock()
	defer c.inflightMu.Unlock()
	if _, exists := c.inflight[requestID]; exists || len(c.inflight) >= c.options.MaxInflight {
		return false
	}
	c.inflight[requestID] = cancel
	return true
}
func (c *Connection) release(requestID uint32) {
	c.inflightMu.Lock()
	delete(c.inflight, requestID)
	c.inflightMu.Unlock()
}

// Notify pushes an application notification using the configured default codec.
func (c *Connection) Notify(methodID uint32, payload any) error {
	if c.State() != ConnectionReady {
		return fmt.Errorf("actorgo agp: connection is not ready")
	}
	if methodID <= cproto.SystemMethodKick {
		return fmt.Errorf("actorgo agp: notify method %d is reserved", methodID)
	}
	codec := c.app.BodyCodecs().Default()
	body, err := c.app.BodyCodecs().Marshal(codec, payload)
	if err != nil {
		return err
	}
	packet := &cproto.Packet{Kind: &cproto.Packet_Notify{Notify: &cproto.Notify{MethodId: methodID, Body: body}}, Codec: codec}
	if !c.send(packet) {
		return fmt.Errorf("actorgo agp: connection write queue is full")
	}
	return nil
}

func (c *Connection) send(packet *cproto.Packet) bool {
	return c.enqueue(outboundPacket{packet: packet})
}

func (c *Connection) sendSync(packet *cproto.Packet) error {
	result := make(chan error, 1)
	if !c.enqueue(outboundPacket{packet: packet, result: result}) {
		return fmt.Errorf("actorgo agp: connection write queue is full")
	}
	timer := time.NewTimer(c.options.WriteTimeout)
	defer timer.Stop()
	select {
	case err := <-result:
		return err
	case <-c.done:
		return fmt.Errorf("actorgo agp: connection closed")
	case <-timer.C:
		return fmt.Errorf("actorgo agp: write timeout")
	}
}

// enqueue applies bounded write-side backpressure and never blocks after close.
func (c *Connection) enqueue(outbound outboundPacket) bool {
	select {
	case <-c.done:
		return false
	default:
	}
	select {
	case c.writeCh <- outbound:
		return true
	default:
		return false
	}
}

func (c *Connection) writeLoop() {
	// Gorilla WebSocket and net.Conn writes must not run concurrently; one queue
	// also provides a single backpressure point for every outbound packet.
	for {
		select {
		case <-c.done:
			return
		case outbound := <-c.writeCh:
			data, err := c.codec.Encode(outbound.packet)
			if err == nil {
				if c.options.WriteTimeout > 0 {
					_ = c.conn.SetWriteDeadline(time.Now().Add(c.options.WriteTimeout))
				}
				err = c.conn.WritePacketBytes(data)
			}
			if outbound.result != nil {
				outbound.result <- err
			}
			if err != nil {
				clog.Warn("agp packet write failed. [connectionID = %s, err = %v]", c.id, err)
				c.Close()
				return
			}
		}
	}
}

// Kick sends the terminal kick notification synchronously before closing.
func (c *Connection) Kick(reasonCode int32, reason string, reconnectable bool) error {
	if c.State() == ConnectionClosed {
		return fmt.Errorf("actorgo agp: connection is closed")
	}
	c.state.Store(int32(ConnectionDraining))
	body, err := proto.Marshal(&cproto.KickNotify{ReasonCode: reasonCode, Reason: reason, Reconnectable: reconnectable})
	if err == nil {
		err = c.sendSync(&cproto.Packet{Kind: &cproto.Packet_Notify{Notify: &cproto.Notify{MethodId: cproto.SystemMethodKick, Body: body}}, Codec: cfacade.CodecProtobuf})
	}
	c.Close()
	c.waitDisconnect(c.options.WriteTimeout)
	return err
}

// GoAway asks the client to reconnect later and then closes the connection.
func (c *Connection) GoAway(reasonCode int32, retryAfter time.Duration) error {
	if c.State() == ConnectionClosed {
		return nil
	}
	c.state.Store(int32(ConnectionDraining))
	body, err := proto.Marshal(&cproto.GoAwayNotify{ReasonCode: reasonCode, RetryAfterMs: uint32(retryAfter / time.Millisecond)})
	if err == nil {
		err = c.sendSync(&cproto.Packet{Kind: &cproto.Packet_Notify{Notify: &cproto.Notify{MethodId: cproto.SystemMethodGoAway, Body: body}}, Codec: cfacade.CodecProtobuf})
	}
	c.Close()
	c.waitDisconnect(c.options.WriteTimeout)
	return err
}

// Close is idempotent and cancels every in-flight request before unregistering
// the connection from its manager.
func (c *Connection) Close() {
	closed := false
	c.closeOnce.Do(func() {
		closed = true
		c.state.Store(int32(ConnectionClosed))
		c.inflightMu.Lock()
		for _, cancel := range c.inflight {
			if cancel != nil {
				cancel()
			}
		}
		clear(c.inflight)
		c.inflightMu.Unlock()
		close(c.done)
		if c.conn != nil {
			_ = c.conn.Close()
		}
		if c.manager != nil {
			c.manager.Remove(c.id)
		}
	})
	if closed {
		done := c.disconnectChannel()
		c.disconnectOnce.Do(func() { go c.runDisconnect(done) })
	}
}

// runDisconnect isolates user cleanup from the socket close critical section.
func (c *Connection) runDisconnect(done chan struct{}) {
	defer close(done)
	defer func() {
		if recovered := recover(); recovered != nil {
			clog.Error("agp OnDisconnect panic. [connectionID = %s, error = %v]", c.id, recovered)
		}
	}()
	if c.options.OnDisconnect != nil {
		c.options.OnDisconnect(c)
	}
}

func (c *Connection) waitDisconnect(timeout time.Duration) {
	if timeout <= 0 {
		return
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-c.disconnectChannel():
	case <-timer.C:
	}
}

func (c *Connection) disconnectChannel() chan struct{} {
	c.disconnectMu.Lock()
	if c.disconnectDone == nil {
		c.disconnectDone = make(chan struct{})
	}
	done := c.disconnectDone
	c.disconnectMu.Unlock()
	return done
}

func cloneMetadata(source map[string][]byte) map[string][]byte {
	if len(source) == 0 {
		return nil
	}
	target := make(map[string][]byte, len(source))
	for key, value := range source {
		target[key] = append([]byte(nil), value...)
	}
	return target
}

func responsePacket(requestID uint32, codec int32, result *cfacade.InvokeResult, body []byte) *cproto.Packet {
	if result == nil {
		result = cfacade.ErrorResult(cproto.StatusCode_STATUS_INTERNAL, "missing invoke result")
	}
	return &cproto.Packet{Kind: &cproto.Packet_Response{Response: &cproto.Response{RequestId: requestID, Code: result.Code, Message: result.Message, Body: body}}, Codec: codec}
}
