package cfacade

import (
	"context"
	"maps"

	cproto "github.com/actorgo-game/actorgo/net/proto"
)

// TransportType identifies the request entry protocol.
type TransportType uint8

const (
	TransportUnknown TransportType = iota
	TransportAGP
	TransportHTTP
	TransportCluster
)

// RequestContext carries cancellation, transport and caller state through all
// transport, cluster and Actor boundaries.
type RequestContext struct {
	context.Context

	RequestID uint32
	Transport TransportType
	Codec     int32

	Session  *cproto.Session
	Metadata map[string][]byte
}

// NewRequestContext wraps parent and substitutes context.Background for nil.
func NewRequestContext(parent context.Context) *RequestContext {
	if parent == nil {
		parent = context.Background()
	}
	return &RequestContext{Context: parent}
}

// Clone copies mutable request state and replaces the cancellation parent.
// Actor mailboxes use it so queued work never shares mutable transport state.
func (c *RequestContext) Clone(parent context.Context) *RequestContext {
	if parent == nil {
		parent = context.Background()
	}
	if c == nil {
		return NewRequestContext(parent)
	}
	cloned := &RequestContext{
		Context:   parent,
		RequestID: c.RequestID,
		Transport: c.Transport,
		Codec:     c.Codec,
		Metadata:  cloneMetadata(c.Metadata),
	}
	if c.Session != nil {
		cloned.Session = &cproto.Session{
			Sid:  c.Session.Sid,
			Uid:  c.Session.Uid,
			Ip:   c.Session.Ip,
			Data: maps.Clone(c.Session.Data),
		}
	}
	return cloned
}

func cloneMetadata(source map[string][]byte) map[string][]byte {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string][]byte, len(source))
	for key, value := range source {
		cloned[key] = append([]byte(nil), value...)
	}
	return cloned
}
