package cconnector

import (
	"crypto/tls"
	"errors"
	"net"
	"sync"
	"sync/atomic"

	cfacade "github.com/actorgo-game/actorgo/facade"
	clog "github.com/actorgo-game/actorgo/logger"
)

// Connector serializes accepted sockets onto one connection callback and
// coordinates idempotent listener shutdown.
type Connector struct {
	listener      net.Listener
	onConnectFunc cfacade.OnConnectFunc
	connChan      chan net.Conn
	done          chan struct{}
	running       int32
	stopped       uint32
	startOnce     sync.Once
	handoffMu     sync.Mutex
	handoffWG     sync.WaitGroup
	dispatchWG    sync.WaitGroup
}

// NewConnector creates a running connector with a bounded accept queue.
func NewConnector(size int) *Connector {
	connector := &Connector{connChan: make(chan net.Conn, size), done: make(chan struct{})}
	atomic.StoreInt32(&connector.running, 1)
	return connector
}

// OnConnect installs the callback that receives accepted sockets.
func (c *Connector) OnConnect(fn cfacade.OnConnectFunc) {
	if fn != nil {
		c.onConnectFunc = fn
	}
}

// InChan hands an accepted socket to the dispatcher or closes it during shutdown.
func (c *Connector) InChan(conn net.Conn) {
	if conn == nil {
		return
	}
	c.handoffMu.Lock()
	if atomic.LoadUint32(&c.stopped) != 0 {
		c.handoffMu.Unlock()
		_ = conn.Close()
		return
	}
	c.handoffWG.Add(1)
	c.handoffMu.Unlock()
	defer c.handoffWG.Done()
	select {
	case c.connChan <- conn:
	case <-c.done:
		// Stop may race with Accept; close sockets that can no longer be handed off.
		_ = conn.Close()
	}
}

// Start launches the connection callback dispatcher.
func (c *Connector) Start() {
	if c.onConnectFunc == nil {
		panic("onConnectFunc is nil")
	}
	c.startOnce.Do(func() {
		c.handoffMu.Lock()
		if atomic.LoadUint32(&c.stopped) != 0 {
			c.handoffMu.Unlock()
			return
		}
		c.dispatchWG.Add(1)
		c.handoffMu.Unlock()
		go c.dispatch()
	})
}

func (c *Connector) dispatch() {
	defer c.dispatchWG.Done()
	for {
		// Prefer shutdown over a queued socket when both cases are ready.
		select {
		case <-c.done:
			return
		default:
		}
		select {
		case <-c.done:
			return
		case conn := <-c.connChan:
			if conn == nil {
				continue
			}
			if atomic.LoadUint32(&c.stopped) != 0 {
				_ = conn.Close()
				continue
			}
			c.onConnectFunc(conn)
		}
	}
}

func (c *Connector) closeQueued() {
	for {
		select {
		case conn := <-c.connChan:
			if conn != nil {
				_ = conn.Close()
			}
		default:
			return
		}
	}
}

// Stop is idempotent and unblocks both the accept loop and connection dispatcher.
func (c *Connector) Stop() {
	if !atomic.CompareAndSwapUint32(&c.stopped, 0, 1) {
		return
	}
	atomic.StoreInt32(&c.running, 0)
	c.handoffMu.Lock()
	close(c.done)
	c.handoffMu.Unlock()
	if c.listener != nil {
		if err := c.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			clog.Error("Failed to stop: %s", err)
		}
	}
	c.handoffWG.Wait()
	c.dispatchWG.Wait()
	c.closeQueued()
}

// Running reports whether the connector still accepts sockets.
func (c *Connector) Running() bool { return atomic.LoadInt32(&c.running) == 1 }

// GetListener creates a TCP or TLS listener and keeps it for coordinated shutdown.
func (c *Connector) GetListener(certFile, keyFile, address string) (net.Listener, error) {
	var err error
	if certFile == "" || keyFile == "" {
		c.listener, err = net.Listen("tcp", address)
		return c.listener, err
	}
	crt, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	c.listener, err = tls.Listen("tcp", address, &tls.Config{Certificates: []tls.Certificate{crt}, MinVersion: tls.VersionTLS12})
	return c.listener, err
}
