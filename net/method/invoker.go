package method

import (
	"errors"
	"fmt"
	"reflect"

	cfacade "github.com/actorgo-game/actorgo/facade"
	clog "github.com/actorgo-game/actorgo/logger"
	cproto "github.com/actorgo-game/actorgo/net/proto"
	"google.golang.org/protobuf/proto"
)

var (
	requestContextType = reflect.TypeOf((*cfacade.RequestContext)(nil))
	errorType          = reflect.TypeOf((*error)(nil)).Elem()
	protoMessageType   = reflect.TypeOf((*proto.Message)(nil)).Elem()
)

// adaptedHandler is the private reflection result retained until mailbox installation.
type adaptedHandler struct {
	msgType       cproto.MsgType
	requestGoType reflect.Type
	protobuf      bool
	invoke        cfacade.TypedInvoke
}

// InvokeError lets an Actor handler return a protocol status instead of the
// generic STATUS_INTERNAL used for ordinary errors.
type InvokeError struct {
	Code    cproto.StatusCode
	Message string
}

// Error implements error.
func (e *InvokeError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func resultFromError(err error) *cfacade.InvokeResult {
	if err == nil {
		return cfacade.OKResult(nil)
	}
	var invokeError *InvokeError
	if errors.As(err, &invokeError) {
		return cfacade.ErrorResult(invokeError.Code, invokeError.Message)
	}
	clog.Error("[method] actor handler failed. [error = %v]", err)
	return cfacade.ErrorResult(cproto.StatusCode_STATUS_INTERNAL, "internal error")
}

// Handler describes a validated Actor method for mailbox installation.
type Handler struct {
	MsgType       cproto.MsgType
	RequestGoType reflect.Type
	Protobuf      bool
	Invoke        cfacade.TypedInvoke
}

// AdaptHandler validates handler signatures and returns invoke metadata used by
// Actor mailboxes to decode raw transport bodies.
func AdaptHandler(handler any) (*Handler, error) {
	adapted, err := adapt(handler)
	if err != nil {
		return nil, err
	}
	return &Handler{
		MsgType:       adapted.msgType,
		RequestGoType: adapted.requestGoType,
		Protobuf:      adapted.protobuf,
		Invoke:        adapted.invoke,
	}, nil
}

// adapt validates the deliberately small public handler signature surface and
// derives request/notify semantics from its return values.
func adapt(handler any) (*adaptedHandler, error) {
	if handler == nil {
		return nil, fmt.Errorf("actorgo method: handler is nil")
	}
	handlerValue := reflect.ValueOf(handler)
	handlerType := handlerValue.Type()
	if handlerType.Kind() != reflect.Func || handlerValue.IsNil() {
		return nil, fmt.Errorf("actorgo method: handler must be a function")
	}
	if handlerType.NumIn() != 2 || handlerType.In(0) != requestContextType || handlerType.In(1).Kind() != reflect.Pointer {
		return nil, fmt.Errorf("actorgo method: handler must accept (*facade.RequestContext, *Request)")
	}

	requestGoType := handlerType.In(1)
	adapted := &adaptedHandler{
		requestGoType: requestGoType,
		protobuf:      requestGoType.Implements(protoMessageType),
	}

	// Return arity is the only source of message type: two returns are req/res,
	// while one error return is a one-way notify.
	switch {
	case handlerType.NumOut() == 2 &&
		handlerType.Out(0).Kind() == reflect.Pointer &&
		handlerType.Out(1) == errorType:
		responseType := handlerType.Out(0)
		if responseType.Implements(protoMessageType) != adapted.protobuf {
			return nil, fmt.Errorf("actorgo method: request and response must both be protobuf messages or ordinary structs")
		}
		adapted.msgType = cproto.MsgType_REQUEST
		adapted.invoke = buildInvoke(handlerValue, requestGoType, true)
	case handlerType.NumOut() == 1 && handlerType.Out(0) == errorType:
		adapted.msgType = cproto.MsgType_NOTIFY
		adapted.invoke = buildInvoke(handlerValue, requestGoType, false)
	default:
		return nil, fmt.Errorf("actorgo method: handler must return (*Response, error) or error")
	}
	return adapted, nil
}

// buildInvoke pays reflection only at the invocation boundary and normalizes
// both supported signatures into TypedInvoke.
func buildInvoke(handler reflect.Value, requestGoType reflect.Type, unary bool) cfacade.TypedInvoke {
	return func(ctx *cfacade.RequestContext, payload any) *cfacade.InvokeResult {
		if ctx == nil {
			ctx = cfacade.NewRequestContext(nil)
		}
		request, failure := requestValue(payload, requestGoType)
		if failure != nil {
			return failure
		}
		results := handler.Call([]reflect.Value{reflect.ValueOf(ctx), request})
		if unary {
			if !results[1].IsNil() {
				return resultFromError(results[1].Interface().(error))
			}
			if results[0].IsNil() {
				return cfacade.ErrorResult(cproto.StatusCode_STATUS_INTERNAL, "actor method returned a nil response")
			}
			return cfacade.OKResult(results[0].Interface())
		}
		if !results[0].IsNil() {
			return resultFromError(results[0].Interface().(error))
		}
		return cfacade.OKResult(nil)
	}
}

// requestValue rejects type mismatches before reflect.Call can panic.
func requestValue(payload any, requestGoType reflect.Type) (reflect.Value, *cfacade.InvokeResult) {
	if payload == nil {
		return reflect.Value{}, cfacade.ErrorResult(cproto.StatusCode_STATUS_INTERNAL, "actor method received a nil request")
	}
	value := reflect.ValueOf(payload)
	if value.Type() != requestGoType {
		return reflect.Value{}, cfacade.ErrorResult(
			cproto.StatusCode_STATUS_INTERNAL,
			fmt.Sprintf("actor request type mismatch: got %T, want %s", payload, requestGoType),
		)
	}
	return value, nil
}
