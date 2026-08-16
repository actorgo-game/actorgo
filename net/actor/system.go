package cactor

import (
	"context"
	"maps"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	cutils "github.com/actorgo-game/actorgo/extend/utils"
	cfacade "github.com/actorgo-game/actorgo/facade"
	clog "github.com/actorgo-game/actorgo/logger"
	cproto "github.com/actorgo-game/actorgo/net/proto"
)

type (
	// System Actor系统
	// It owns top-level Actors and provides the common local/remote routing path.
	System struct {
		app              cfacade.IApplication
		actorMap         *sync.Map       // key:actorID, value:*actor
		actorEventMap    *sync.Map       // map[string]map[string]int64 => key:eventName, value:map[actorPath]uniqueID
		wg               *sync.WaitGroup // wait group
		callTimeout      time.Duration   // call调用超时
		messageSeq       atomic.Uint64   // 消息序列号
		executionTimeout int64           // 消息执行超时(毫秒)
	}
)

// NewSystem creates an unbound Actor system with default call and slow-call limits.
func NewSystem() *System {
	system := &System{
		actorMap:         &sync.Map{},
		actorEventMap:    &sync.Map{},
		wg:               &sync.WaitGroup{},
		callTimeout:      3 * time.Second,
		executionTimeout: 100,
	}

	return system
}

// SetApp binds application routing, codecs, discovery, and cluster services.
func (p *System) SetApp(app cfacade.IApplication) {
	p.app = app
}

// NodeID returns the bound application node ID.
func (p *System) NodeID() string {
	if p.app == nil {
		return ""
	}

	return p.app.NodeID()
}

// Stop asks every top-level Actor to exit and waits for all parent and child loops.
func (p *System) Stop() {
	p.actorMap.Range(func(key, value any) bool {
		actor, ok := value.(*Actor)
		if ok {
			cutils.Try(func() {
				actor.Exit()
			}, func(err string) {
				clog.Warn("[OnStop] - [actorID = %s, err = %s]", actor.path, err)
			})
		}
		return true
	})

	clog.Info("[OnStop] actor system stopping!")
	p.wg.Wait()
	clog.Info("[OnStop]actor system stopped!")
}

// GetIActor 根据ActorID获取IActor
func (p *System) GetIActor(id string) (cfacade.IActor, bool) {
	return p.GetActor(id)
}

// GetActor 根据ActorID获取*actor
func (p *System) GetActor(id string) (*Actor, bool) {
	actorValue, found := p.actorMap.Load(id)
	if !found {
		return nil, false
	}

	actor, found := actorValue.(*Actor)
	return actor, found
}

// GetChildActor resolves a dynamic child below a top-level Actor.
func (p *System) GetChildActor(actorID, childID string) (*Actor, bool) {
	parentActor, found := p.GetActor(actorID)
	if !found {
		return nil, found
	}

	return parentActor.child.GetActor(childID)
}

// GetActorWithPath resolves either a top-level or child Actor path on this node.
func (p *System) GetActorWithPath(path string) (*Actor, bool) {
	actorPath, err := cfacade.ToActorPath(path)
	if err != nil {
		clog.Warn("[GetActorWithPath] Actor path is error. path = %s, err = %v", path, err)
		return nil, false
	}

	if actorPath.IsChild() {
		return p.GetChildActor(actorPath.ActorID, actorPath.ChildID)
	}

	return p.GetActor(actorPath.ActorID)
}

func (p *System) removeActor(actorID string) {
	p.actorMap.Delete(actorID)
}

// CreateActor 创建Actor
func (p *System) CreateActor(id string, handler cfacade.IActorHandler) (cfacade.IActor, error) {
	if strings.TrimSpace(id) == "" {
		return nil, ErrActorIDIsNil
	}

	if actor, found := p.GetIActor(id); found {
		return actor, nil
	}

	thisActor, err := newActor(id, "", handler, p)
	if err != nil {
		return nil, err
	}

	p.actorMap.Store(id, thisActor) // add to map
	p.wg.Add(1)
	go thisActor.run() // new actor is running!
	// Method registration happens in OnInit; do not expose the Actor until that
	// initialization has completed successfully.
	if err := <-thisActor.initDone; err != nil {
		return nil, err
	}

	return thisActor, nil
}

// Invoke calls the local top-level Actor registered for methodID.
func (p *System) Invoke(ctx *cfacade.RequestContext, methodID uint32, payload any) *cfacade.InvokeResult {
	target, failure := p.methodTarget(methodID)
	if failure != nil {
		return failure
	}
	return p.InvokeTarget(ctx, target, methodID, payload)
}

// InvokeNode calls methodID on another node. The destination node resolves the
// top-level Actor from its method table, so callers do not need an ActorPath.
func (p *System) InvokeNode(ctx *cfacade.RequestContext, nodeID string, methodID uint32, payload any) *cfacade.InvokeResult {
	if methodID == 0 {
		return cfacade.ErrorResult(cproto.StatusCode_STATUS_INVALID_ARGUMENT, "method id is required")
	}
	if nodeID == "" || nodeID == p.NodeID() {
		return p.Invoke(ctx, methodID, payload)
	}
	ctx = ensureRequestContext(p.app, ctx)
	return p.invokeRemote(ctx, "", nodeID, methodID, payload)
}

// InvokeTarget calls an explicit ActorPath. It is intended for dynamic child
// Actors; ordinary top-level calls should use Invoke or InvokeNode.
func (p *System) InvokeTarget(ctx *cfacade.RequestContext, target string, methodID uint32, payload any) *cfacade.InvokeResult {
	targetPath, failure := p.prepareTyped(ctx, target, methodID)
	if failure != nil {
		return failure
	}
	ctx = ensureRequestContext(p.app, ctx)
	// forward to remote actor
	if targetPath.NodeID != "" && targetPath.NodeID != p.NodeID() {
		return p.invokeRemote(ctx, target, targetPath.NodeID, methodID, payload)
	}
	// Local child: deliver directly to the child mailbox so a parent handler
	// waiting on InvokeChild cannot deadlock on itself.
	if targetPath.IsChild() {
		parent, found := p.GetActor(targetPath.ActorID)
		if !found {
			return cfacade.ErrorResult(cproto.StatusCode_STATUS_NOT_FOUND, "actor not found")
		}
		return parent.InvokeChild(ctx, targetPath.ChildID, methodID, payload)
	}

	invokeCtx, cancel, failure := p.newInvokeContext(ctx)
	if failure != nil {
		return failure
	}
	defer cancel()
	message := typedMessage(invokeCtx, target, methodID, payload)
	resultCh := make(chan *cfacade.InvokeResult, 1)
	message.ChanInvokeResult = resultCh
	if !p.post(message) {
		message.Recycle()
		return cfacade.ErrorResult(cproto.StatusCode_STATUS_NOT_FOUND, "actor not found")
	}

	select {
	case result := <-resultCh:
		return result
	case <-invokeCtx.Done():
		return contextFailure(invokeCtx.Err(), "actor invoke")
	}
}

// Notify sends a one-way message to the local top-level Actor registered for
// methodID.
func (p *System) Notify(ctx *cfacade.RequestContext, methodID uint32, payload any) *cfacade.InvokeResult {
	target, failure := p.methodTarget(methodID)
	if failure != nil {
		return failure
	}
	return p.NotifyTarget(ctx, target, methodID, payload)
}

// NotifyNode sends methodID to another node without exposing its ActorPath.
func (p *System) NotifyNode(ctx *cfacade.RequestContext, nodeID string, methodID uint32, payload any) *cfacade.InvokeResult {
	if methodID == 0 {
		return cfacade.ErrorResult(cproto.StatusCode_STATUS_INVALID_ARGUMENT, "method id is required")
	}
	if nodeID == "" || nodeID == p.NodeID() {
		return p.Notify(ctx, methodID, payload)
	}
	ctx = ensureRequestContext(p.app, ctx)
	return p.notifyRemote(ctx, "", nodeID, methodID, payload)
}

// NotifyTarget sends to an explicit ActorPath and is primarily used by dynamic
// child Actors.
func (p *System) NotifyTarget(ctx *cfacade.RequestContext, target string, methodID uint32, payload any) *cfacade.InvokeResult {
	targetPath, failure := p.prepareTyped(ctx, target, methodID)
	if failure != nil {
		return failure
	}
	ctx = ensureRequestContext(p.app, ctx)
	if targetPath.NodeID != "" && targetPath.NodeID != p.NodeID() {
		return p.notifyRemote(ctx, target, targetPath.NodeID, methodID, payload)
	}
	if targetPath.IsChild() {
		parent, found := p.GetActor(targetPath.ActorID)
		if !found {
			return cfacade.ErrorResult(cproto.StatusCode_STATUS_NOT_FOUND, "actor not found")
		}
		return parent.NotifyChild(ctx, targetPath.ChildID, methodID, payload)
	}
	notifyCtx, cancel, failure := p.newNotifyContext(ctx)
	if failure != nil {
		return failure
	}
	message := typedMessage(notifyCtx, target, methodID, payload)
	message.Cancel = cancel
	if !p.post(message) {
		message.Recycle()
		return cfacade.ErrorResult(cproto.StatusCode_STATUS_NOT_FOUND, "actor not found")
	}
	return cfacade.OKResult(nil)
}

func (p *System) methodTarget(methodID uint32) (string, *cfacade.InvokeResult) {
	if methodID == 0 {
		return "", cfacade.ErrorResult(cproto.StatusCode_STATUS_INVALID_ARGUMENT, "method id is required")
	}
	if p.app == nil || p.app.Methods() == nil {
		return "", cfacade.ErrorResult(cproto.StatusCode_STATUS_UNAVAILABLE, "method table is not initialized")
	}
	target, found := p.app.Methods().Target(methodID)
	if !found {
		return "", cfacade.ErrorResult(cproto.StatusCode_STATUS_NOT_FOUND, "top-level actor method not found")
	}
	return target, nil
}

func (p *System) prepareTyped(_ *cfacade.RequestContext, target string, methodID uint32) (*cfacade.ActorPath, *cfacade.InvokeResult) {
	if methodID == 0 {
		return nil, cfacade.ErrorResult(cproto.StatusCode_STATUS_INVALID_ARGUMENT, "method id is required")
	}
	targetPath, err := cfacade.ToActorPath(target)
	if err != nil {
		return nil, cfacade.ErrorResult(cproto.StatusCode_STATUS_INVALID_ARGUMENT, "invalid actor target")
	}
	return targetPath, nil
}

func (p *System) invokeRemote(ctx *cfacade.RequestContext, target, nodeID string, methodID uint32, payload any) *cfacade.InvokeResult {
	if p.app.Cluster() == nil {
		return cfacade.ErrorResult(cproto.StatusCode_STATUS_UNAVAILABLE, "cluster is not configured")
	}
	ctx, cancel, failure := p.newInvokeContext(ctx)
	if failure != nil {
		return failure
	}
	defer cancel()
	body, err := encodeClusterPayload(p.app, ctx, payload)
	if err != nil {
		return cfacade.ErrorResult(cproto.StatusCode_STATUS_INTERNAL, "cluster request body encode failed")
	}
	message := p.buildClusterMessage(ctx, target, methodID, cproto.MsgType_REQUEST, body)
	deadline, _ := ctx.Deadline()
	timeout := time.Until(deadline)
	if timeout <= 0 {
		return cfacade.ErrorResult(cproto.StatusCode_STATUS_DEADLINE_EXCEEDED, "cluster request deadline exceeded")
	}
	response, err := p.app.Cluster().Request(nodeID, message, timeout)
	if err != nil {
		if ctx.Err() != nil {
			return contextFailure(ctx.Err(), "cluster request")
		}
		return cfacade.ErrorResult(cproto.StatusCode_STATUS_UNAVAILABLE, err.Error())
	}
	return &cfacade.InvokeResult{Payload: response.Payload, Code: response.Code, Message: response.Message}
}

func (p *System) notifyRemote(ctx *cfacade.RequestContext, target, nodeID string, methodID uint32, payload any) *cfacade.InvokeResult {
	if p.app.Cluster() == nil {
		return cfacade.ErrorResult(cproto.StatusCode_STATUS_UNAVAILABLE, "cluster is not configured")
	}
	ctx, cancel, failure := p.newNotifyContext(ctx)
	if failure != nil {
		return failure
	}
	defer cancel()
	body, err := encodeClusterPayload(p.app, ctx, payload)
	if err != nil {
		return cfacade.ErrorResult(cproto.StatusCode_STATUS_INTERNAL, "cluster notify body encode failed")
	}
	if err := p.app.Cluster().Publish(nodeID, p.buildClusterMessage(ctx, target, methodID, cproto.MsgType_NOTIFY, body)); err != nil {
		return cfacade.ErrorResult(cproto.StatusCode_STATUS_UNAVAILABLE, err.Error())
	}
	return cfacade.OKResult(nil)
}

// buildClusterMessage copies the request boundary needed to preserve correlation,
// codec, deadline, metadata, and session identity across a cluster hop.
func (p *System) buildClusterMessage(ctx *cfacade.RequestContext, target string, methodID uint32, msgType cproto.MsgType, body []byte) *cproto.ClusterMessage {
	// Clone mutable context data because the message may outlive the caller
	// while it is serialized or published by the cluster implementation.
	message := &cproto.ClusterMessage{
		MessageId: p.messageSeq.Add(1), MsgType: msgType, RequestId: ctx.RequestID, MethodId: methodID,
		TargetPath: target, Metadata: cloneBytesMap(ctx.Metadata),
		Codec: ctx.Codec, Payload: body,
	}
	if deadline, ok := ctx.Context.Deadline(); ok {
		message.DeadlineUnixMs = deadline.UnixMilli()
	}
	if ctx.Session != nil {
		message.Session = &cproto.Session{Sid: ctx.Session.Sid, Uid: ctx.Session.Uid, Ip: ctx.Session.Ip, Data: maps.Clone(ctx.Session.Data)}
	}
	return message
}

// encodeClusterPayload preserves raw transport bytes and encodes typed local values.
func encodeClusterPayload(app cfacade.IApplication, ctx *cfacade.RequestContext, payload any) ([]byte, error) {
	if body, ok := payload.([]byte); ok {
		return body, nil
	}
	return app.BodyCodecs().Marshal(ctx.Codec, payload)
}

// ensureRequestContext supplies defaults without replacing caller-owned context state.
func ensureRequestContext(app cfacade.IApplication, ctx *cfacade.RequestContext) *cfacade.RequestContext {
	if ctx == nil {
		ctx = cfacade.NewRequestContext(context.Background())
	}
	if ctx.Context == nil {
		ctx.Context = context.Background()
	}
	if ctx.Codec == 0 {
		ctx.Codec = app.BodyCodecs().Default()
	}
	return ctx
}

// newInvokeContext gives every queued request a real deadline. Cancelling the
// caller therefore also marks a message that is still waiting in the mailbox.
func (p *System) newInvokeContext(source *cfacade.RequestContext) (*cfacade.RequestContext, context.CancelFunc, *cfacade.InvokeResult) {
	source = ensureRequestContext(p.app, source)
	if err := source.Err(); err != nil {
		return nil, nil, contextFailure(err, "actor invoke")
	}
	if _, ok := source.Deadline(); ok {
		ctx, cancel := context.WithCancel(source.Context)
		return source.Clone(ctx), cancel, nil
	}
	ctx, cancel := context.WithTimeout(source.Context, p.callTimeout)
	return source.Clone(ctx), cancel, nil
}

// newNotifyContext detaches one-way work from the ingress cancellation while
// retaining its deadline. The Actor cancels this context after handling it.
func (p *System) newNotifyContext(source *cfacade.RequestContext) (*cfacade.RequestContext, context.CancelFunc, *cfacade.InvokeResult) {
	source = ensureRequestContext(p.app, source)
	if err := source.Err(); err != nil {
		return nil, nil, contextFailure(err, "actor notify")
	}
	parent := context.WithoutCancel(source.Context)
	if deadline, ok := source.Deadline(); ok {
		ctx, cancel := context.WithDeadline(parent, deadline)
		return source.Clone(ctx), cancel, nil
	}
	ctx, cancel := context.WithTimeout(parent, p.callTimeout)
	return source.Clone(ctx), cancel, nil
}

func contextFailure(err error, operation string) *cfacade.InvokeResult {
	if err == context.DeadlineExceeded {
		return cfacade.ErrorResult(cproto.StatusCode_STATUS_DEADLINE_EXCEEDED, operation+" deadline exceeded")
	}
	return cfacade.ErrorResult(cproto.StatusCode_STATUS_CANCELLED, operation+" cancelled")
}

// typedMessage acquires and fills the pooled message used by all Actor entry points.
func typedMessage(ctx *cfacade.RequestContext, target string, methodID uint32, payload any) *cfacade.Message {
	message := cfacade.GetMessage()
	message.MethodID = methodID
	message.Target, message.Context, message.Payload = target, ctx, payload
	return message
}

// cloneBytesMap prevents mutable metadata from being shared across asynchronous calls.
func cloneBytesMap(source map[string][]byte) map[string][]byte {
	if len(source) == 0 {
		return nil
	}
	target := make(map[string][]byte, len(source))
	for key, value := range source {
		target[key] = append([]byte(nil), value...)
	}
	return target
}

// post delivers a typed message to the target Actor mailbox.
func (p *System) post(m *cfacade.Message) bool {
	if m == nil {
		clog.Error("Message is nil.")
		return false
	}

	targetPath := m.TargetPath()
	if targetPath == nil {
		clog.Error("Message target is invalid. [target = %s]", m.Target)
		return false
	}
	if targetActor, found := p.GetActor(targetPath.ActorID); found {
		state := targetActor.State()
		if state == InitState || state == WorkerState {
			targetActor.post(m)
			return true
		}
		return false
	}

	clog.Warn("[Post] actor not found. [target = %s, methodID = %d]", m.Target, m.MethodID)
	return false
}

// PostEvent 提交事件
func (p *System) PostEvent(data cfacade.IEventData) {
	if data == nil {
		clog.Error("[PostEvent] Event is nil.")
		return
	}

	if len(data.Name()) < 1 {
		clog.Warn("[PostEvent] Event name is empty. value = %v", data)
		return
	}

	valueMap, found := p.actorEventMap.Load(data.Name())
	if !found {
		return
	}

	// map[string]int64
	actorIDSMap, ok := valueMap.(*sync.Map)
	if !ok {
		return
	}

	actorIDSMap.Range(func(key, value any) bool {
		path := key.(string)
		targetActor, found := p.GetActorWithPath(path)
		if !found {
			return true
		}

		// no set unique
		if value == nil {
			if targetActor.State() == WorkerState {
				targetActor.event.Push(data)
			}

			return true
		}

		uniqueID, ok := value.(int64)
		if !ok {
			clog.Warn("[PostEvent] UniqueID set error in actorEventMap. value = %v", value)
			return true
		}

		if uniqueID == data.UniqueID() {
			if targetActor.State() == WorkerState {
				targetActor.event.Push(data)
			}

			return true
		}

		return true
	})
}

// SetCallTimeout sets the default end-to-end deadline when the caller supplies none.
func (p *System) SetCallTimeout(d time.Duration) {
	p.callTimeout = d
}

// SetExecutionTimeout sets the slow-handler warning threshold in milliseconds.
func (p *System) SetExecutionTimeout(t int64) {
	if t > 1 {
		p.executionTimeout = t
	}
}

func (p *System) addActorEvent(actorPath string, eventName string, uniqueID ...int64) {
	// map[string]map[string]int64 => key:eventName, value:map[actorPath]uniqueID
	value, _ := p.actorEventMap.LoadOrStore(eventName, &sync.Map{})
	eventMap := value.(*sync.Map)

	if len(uniqueID) > 0 {
		eventMap.Store(actorPath, uniqueID[0])
	} else {
		eventMap.Store(actorPath, nil) // no set unique
	}
}

func (p *System) removeActorEvent(actorPath string, eventNames ...string) {
	for _, eventName := range eventNames {
		value, found := p.actorEventMap.Load(eventName)
		if !found {
			continue
		}

		if actorIDMap, found := value.(*sync.Map); found {
			actorIDMap.Delete(actorPath)
		}
	}
}
