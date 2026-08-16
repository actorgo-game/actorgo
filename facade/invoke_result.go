package cfacade

import (
	"fmt"

	cproto "github.com/actorgo-game/actorgo/net/proto"
)

// InvokeResult is the transport-neutral result returned by an Actor method.
// Local calls return a concrete response; remote calls return encoded bytes.
type InvokeResult struct {
	Payload any
	Code    int32
	Message string
}

// OKResult builds a successful Actor invocation result.
func OKResult(payload any) *InvokeResult {
	return &InvokeResult{Payload: payload}
}

// ErrorResult builds a failed Actor invocation result with an AGP status code.
func ErrorResult(code cproto.StatusCode, message string) *InvokeResult {
	return &InvokeResult{Code: int32(code), Message: message}
}

// OK reports whether the result represents success.
func (r *InvokeResult) OK() bool {
	return r != nil && r.Code == 0
}

// Decode unmarshals a successful result into target. It also accepts concrete
// local results, giving callers one decode path for local and remote invokes.
func (r *InvokeResult) Decode(codecs IBodyCodecRegistry, codec int32, target any) error {
	if r == nil {
		return fmt.Errorf("actorgo: invoke result is nil")
	}
	if !r.OK() {
		return fmt.Errorf("actorgo: invoke failed: code=%d message=%s", r.Code, r.Message)
	}
	if codecs == nil || target == nil {
		return fmt.Errorf("actorgo: body codecs and decode target are required")
	}
	if body, ok := r.Payload.([]byte); ok {
		return codecs.Unmarshal(codec, body, target)
	}
	body, err := codecs.Marshal(codec, r.Payload)
	if err != nil {
		return err
	}
	return codecs.Unmarshal(codec, body, target)
}

// TypedInvoke is the normalized form stored in an Actor mailbox.
type TypedInvoke func(ctx *RequestContext, payload any) *InvokeResult
