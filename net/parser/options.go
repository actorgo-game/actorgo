package parser

import (
	"time"

	cproto "github.com/actorgo-game/actorgo/net/proto"
)

// Options controls AGP limits, timeouts and connection backpressure.
type Options struct {
	Limits            cproto.Limits
	WriteQueueSize    int
	MaxInflight       int
	HandshakeTimeout  time.Duration
	HeartbeatInterval time.Duration
	IdleTimeout       time.Duration
	WriteTimeout      time.Duration
	MaxRequestTimeout time.Duration
	// OnDisconnect is invoked asynchronously once after a connection is removed
	// from the manager. Shutdown waits for it only up to WriteTimeout.
	OnDisconnect func(*Connection)
}

// Option mutates AGP component options during construction.
type Option func(*Options)

func defaultOptions() Options {
	return Options{
		Limits:            cproto.DefaultLimits(),
		WriteQueueSize:    256,
		MaxInflight:       128,
		HandshakeTimeout:  10 * time.Second,
		HeartbeatInterval: 30 * time.Second,
		IdleTimeout:       90 * time.Second,
		WriteTimeout:      10 * time.Second,
		MaxRequestTimeout: 30 * time.Second,
	}
}

// WithLimits replaces packet, body and metadata limits.
func WithLimits(limits cproto.Limits) Option { return func(o *Options) { o.Limits = limits } }

// WithWriteQueue sets the maximum buffered outbound packet count.
func WithWriteQueue(size int) Option {
	return func(o *Options) {
		if size > 0 {
			o.WriteQueueSize = size
		}
	}
}

// WithMaxInflight limits concurrent request handlers on each connection.
func WithMaxInflight(max int) Option {
	return func(o *Options) {
		if max > 0 {
			o.MaxInflight = max
		}
	}
}

// WithHandshakeTimeout limits how long a new connection may remain uninitialized.
func WithHandshakeTimeout(timeout time.Duration) Option {
	return func(o *Options) {
		if timeout > 0 {
			o.HandshakeTimeout = timeout
		}
	}
}

// WithHeartbeat configures the advertised heartbeat interval and idle deadline.
func WithHeartbeat(interval, idle time.Duration) Option {
	return func(o *Options) {
		if interval > 0 {
			o.HeartbeatInterval = interval
		}
		if idle > 0 {
			o.IdleTimeout = idle
		}
	}
}

// WithWriteTimeout limits one packet write.
func WithWriteTimeout(timeout time.Duration) Option {
	return func(o *Options) {
		if timeout > 0 {
			o.WriteTimeout = timeout
		}
	}
}

// WithMaxRequestTimeout caps the timeout requested by an AGP client.
func WithMaxRequestTimeout(timeout time.Duration) Option {
	return func(o *Options) {
		if timeout > 0 {
			o.MaxRequestTimeout = timeout
		}
	}
}

// WithOnDisconnect registers a callback invoked once per connection close.
func WithOnDisconnect(fn func(*Connection)) Option {
	return func(o *Options) {
		o.OnDisconnect = fn
	}
}
