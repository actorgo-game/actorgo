package cactor

import (
	"fmt"
	"reflect"

	cfacade "github.com/actorgo-game/actorgo/facade"
	clog "github.com/actorgo-game/actorgo/logger"
	cmethod "github.com/actorgo-game/actorgo/net/method"
	cproto "github.com/actorgo-game/actorgo/net/proto"
)

// methodEntry caches the adapted call and request metadata needed on the hot path.
type methodEntry struct {
	invoke           cfacade.TypedInvoke
	requestGoType    reflect.Type
	supportsProtobuf bool // request/response 是否实现 proto.Message
}

// mailbox combines the Actor's serialized queue with handlers registered by MethodID.
type mailbox struct {
	queue     // queue
	name      string
	methodMap map[uint32]*methodEntry
	owner     *Actor
}

func newMailbox(name string, owner *Actor) mailbox {
	return mailbox{queue: newQueue(), name: name, methodMap: make(map[uint32]*methodEntry), owner: owner}
}

// Register installs a mailbox handler. Top-level Actor methods are also
// published to the application method table; child methods remain local.
// Registration failures panic so OnInit can keep a simple call style.
func (m *mailbox) Register(methodID uint32, handler any) {
	if methodID == 0 || handler == nil {
		panic(fmt.Errorf("actorgo: method id and handler are required"))
	}
	if _, exists := m.methodMap[methodID]; exists {
		panic(fmt.Errorf("actorgo: method id %d is already registered in %s mailbox", methodID, m.name))
	}

	adapted, err := cmethod.AdaptHandler(handler)
	if err != nil {
		panic(err)
	}
	if m.owner == nil || m.owner.path == nil || m.owner.system == nil || m.owner.system.app == nil {
		panic(fmt.Errorf("actorgo: method table is not initialized"))
	}
	if m.owner.State() != InitState {
		panic(fmt.Errorf("actorgo: methods may only be registered during OnInit"))
	}
	if m.owner.path.IsParent() {
		if m.owner.system.app.Methods() == nil {
			panic(fmt.Errorf("actorgo: method table is not initialized"))
		}
		if err = m.owner.system.app.Methods().Register(methodID, m.owner.PathString(), adapted.MsgType); err != nil {
			panic(err)
		}
	}
	m.methodMap[methodID] = &methodEntry{
		invoke:           adapted.Invoke,
		requestGoType:    adapted.RequestGoType,
		supportsProtobuf: adapted.Protobuf,
	}
}

func (m *mailbox) getMethod(methodID uint32) (*methodEntry, bool) {
	entry, found := m.methodMap[methodID]
	return entry, found
}

// decodePayload decodes transport bytes with the request codec. In-process calls
// may pass an already typed value and therefore skip serialization entirely.
func (m *mailbox) decodePayload(ctx *cfacade.RequestContext, entry *methodEntry, payload any) (any, *cfacade.InvokeResult) {
	body, ok := payload.([]byte)
	if !ok {
		return payload, nil
	}
	if entry == nil || entry.requestGoType == nil {
		return nil, cfacade.ErrorResult(cproto.StatusCode_STATUS_INTERNAL, "actor method request type missing")
	}
	if ctx == nil {
		ctx = cfacade.NewRequestContext(nil)
	}
	codec := ctx.Codec
	if codec == 0 && m.owner != nil && m.owner.system != nil && m.owner.system.app != nil {
		codec = m.owner.system.app.BodyCodecs().Default()
	}
	if codec != cfacade.CodecJSON && (codec != cfacade.CodecProtobuf || !entry.supportsProtobuf) {
		return nil, cfacade.ErrorResult(cproto.StatusCode_STATUS_UNSUPPORTED_MEDIA, fmt.Sprintf("method does not support body codec %d", codec))
	}
	if m.owner == nil || m.owner.system == nil || m.owner.system.app == nil || m.owner.system.app.BodyCodecs() == nil {
		return nil, cfacade.ErrorResult(cproto.StatusCode_STATUS_INTERNAL, "body codecs are not initialized")
	}
	request := reflect.New(entry.requestGoType.Elem()).Interface()
	if err := m.owner.system.app.BodyCodecs().Unmarshal(codec, body, request); err != nil {
		return nil, cfacade.ErrorResult(cproto.StatusCode_STATUS_INVALID_ARGUMENT, "request body decode failed: "+err.Error())
	}
	return request, nil
}

func (m *mailbox) Pop() *cfacade.Message {
	value := m.queue.Pop()
	if value == nil {
		return nil
	}
	message, ok := value.(*cfacade.Message)
	if !ok {
		clog.Warn("Convert to *Message fail. v = %+v", value)
		return nil
	}
	return message
}
func (m *mailbox) Push(message *cfacade.Message) {
	if message != nil {
		m.queue.Push(message)
	}
}
func (m *mailbox) onStop() { clear(m.methodMap); m.queue.Destroy() }
