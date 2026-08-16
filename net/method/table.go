package method

import (
	"context"
	"fmt"
	"sync"

	cfacade "github.com/actorgo-game/actorgo/facade"
	cproto "github.com/actorgo-game/actorgo/net/proto"
)

type registeredMethod struct {
	msgType cproto.MsgType
	target  string
}

// Table is the application-wide MethodID catalog populated by
// Actor.Methods().Register. A top-level MethodID also records its ActorPath, so
// ordinary callers only need the MethodID. Body decoding happens in the Actor
// mailbox.
type Table struct {
	mu   sync.RWMutex
	app  cfacade.IApplication
	byID map[uint32]*registeredMethod
}

// NewTable creates an empty application-scoped method table.
func NewTable(app cfacade.IApplication) *Table {
	return &Table{app: app, byID: make(map[uint32]*registeredMethod)}
}

// Register records one externally routable top-level Actor method. Handler
// reflection and body decoding remain private to the Actor mailbox.
func (r *Table) Register(methodID uint32, target string, msgType cproto.MsgType) error {
	if r == nil || r.app == nil {
		return fmt.Errorf("actorgo method: method table is not initialized")
	}
	if methodID <= cproto.SystemMethodKick {
		return fmt.Errorf("actorgo method: method id %d is reserved by the framework", methodID)
	}
	path, err := cfacade.ToActorPath(target)
	if err != nil || path == nil || path.IsChild() {
		return fmt.Errorf("actorgo method: target %q must be a top-level actor", target)
	}
	if msgType != cproto.MsgType_REQUEST && msgType != cproto.MsgType_NOTIFY {
		return fmt.Errorf("actorgo method: invalid message type %d", msgType)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if current := r.byID[methodID]; current != nil {
		return fmt.Errorf("actorgo method: method id %d is already registered by %q", methodID, current.target)
	}
	r.byID[methodID] = &registeredMethod{msgType: msgType, target: target}
	return nil
}

// UnregisterTarget removes catalog entries when a top-level Actor stops.
func (r *Table) UnregisterTarget(target string) {
	if r == nil || target == "" {
		return
	}
	r.mu.Lock()
	for methodID, entry := range r.byID {
		if entry.target == target {
			delete(r.byID, methodID)
		}
	}
	r.mu.Unlock()
}

// MsgType returns the protocol message type inferred from the handler signature.
func (r *Table) MsgType(methodID uint32) (cproto.MsgType, bool) {
	entry, found := r.lookup(methodID)
	if !found {
		return 0, false
	}
	return entry.msgType, true
}

// Target returns the default top-level ActorPath registered for methodID.
func (r *Table) Target(methodID uint32) (string, bool) {
	entry, found := r.lookup(methodID)
	if !found {
		return "", false
	}
	return entry.target, true
}

// Dispatch validates the registered message type and invokes the top-level Actor
// selected by MethodID with the raw transport body.
func (r *Table) Dispatch(ctx *cfacade.RequestContext, methodID uint32, body []byte, msgType cproto.MsgType) *cfacade.InvokeResult {
	entry, found := r.lookup(methodID)
	if !found {
		return cfacade.ErrorResult(cproto.StatusCode_STATUS_NOT_FOUND, "method not found")
	}
	if entry.msgType != msgType {
		return cfacade.ErrorResult(cproto.StatusCode_STATUS_FAILED_PRECONDITION, "method message type does not match request message type")
	}

	if ctx == nil {
		ctx = cfacade.NewRequestContext(context.Background())
	}
	if ctx.Codec == 0 {
		ctx.Codec = r.app.BodyCodecs().Default()
	}
	if err := ctx.Err(); err != nil {
		if err == context.DeadlineExceeded {
			return cfacade.ErrorResult(cproto.StatusCode_STATUS_DEADLINE_EXCEEDED, "request deadline exceeded")
		}
		return cfacade.ErrorResult(cproto.StatusCode_STATUS_CANCELLED, "request cancelled")
	}
	if msgType == cproto.MsgType_NOTIFY {
		return r.app.ActorSystem().NotifyTarget(ctx, entry.target, methodID, body)
	}
	return r.app.ActorSystem().InvokeTarget(ctx, entry.target, methodID, body)
}

func (r *Table) lookup(methodID uint32) (*registeredMethod, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	entry, found := r.byID[methodID]
	r.mu.RUnlock()
	return entry, found
}
