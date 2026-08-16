package cfacade

import cproto "github.com/actorgo-game/actorgo/net/proto"

// IMethodTable is populated by Actor.Methods().Register.
// A MethodID maps to one top-level Actor method and must be globally unique.
// Child Actor methods stay in their own mailbox and are never exposed through
// an external transport method table.
type IMethodTable interface {
	Register(methodID uint32, target string, msgType cproto.MsgType) error
	UnregisterTarget(target string)
	MsgType(methodID uint32) (cproto.MsgType, bool)
	// Target returns the top-level Actor registered for a MethodID.
	Target(methodID uint32) (string, bool)
	// Dispatch resolves the top-level Actor exclusively by MethodID.
	Dispatch(ctx *RequestContext, methodID uint32, body []byte, msgType cproto.MsgType) *InvokeResult
}
