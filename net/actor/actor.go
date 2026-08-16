package cactor

import (
	"fmt"
	"strings"
	"sync/atomic"

	ctime "github.com/actorgo-game/actorgo/extend/time"
	cutils "github.com/actorgo-game/actorgo/extend/utils"
	cfacade "github.com/actorgo-game/actorgo/facade"
	clog "github.com/actorgo-game/actorgo/logger"
	cproto "github.com/actorgo-game/actorgo/net/proto"
	"go.uber.org/zap/zapcore"
)

/**
- 每个Actor独立运行在一个goroutine中，所有的逻辑都是串行处理
- Actor 接收两类入口消息：方法调用(mailbox)与事件(Event)
	- 方法调用统一进入单一 mailbox（含客户端 AGP/HTTP、同进程 Invoke、跨节点集群）
	- 事件消息通过订阅/发布进入 Event 队列
- Actor可以创建多个子Actor(ChildActor)，子Actor的消息由父Actor进行路由转发
- Actor可以创建多个定时器(Timer)进行定时业务的处理
- 通过cluster集群组件、discovery发现服务组件，进行跨节点的actor通信
*/

var (
	_nilActor = &Actor{}
)

var (
	InitState   State = 0
	WorkerState State = 1
	StopState   State = 2
)

type (
	State int

	// Actor owns one handler and serializes its mailbox, event, and timer work on
	// the run goroutine. Child messages are routed through their parent first.
	Actor struct {
		system   *System               // actor system
		path     *cfacade.ActorPath    // actor path
		state    atomic.Int32          // actor state
		close    chan struct{}         // close flag
		handler  cfacade.IActorHandler // actor handler
		mail     *mailbox              // unified method mailbox
		event    *actorEvent           // event handle
		timer    *actorTimer           // timer handle
		child    *actorChild           // child actor
		lastAt   int64                 // last process time (count of seconds)
		initDone chan error            // reports OnInit and method registration completion
	}
)

func (p *Actor) run() {
	// CreateActor waits on initDone so all registered methods are visible before
	// network components start accepting requests.
	if err := p.initialize(); err != nil {
		p.onStop()
		p.initDone <- err
		return
	}
	p.initDone <- nil
	defer p.onStop()

	for {
		if p.loop() {
			break
		}
	}
}

func (p *Actor) initialize() (err error) {
	// OnInit commonly registers methods and may panic on a duplicate MethodID.
	// Convert that panic into a creation error so a half-initialized Actor is
	// never returned to the caller.
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("actor %s initialization failed: %v", p.PathString(), recovered)
		}
	}()
	p.onInit()
	return nil
}

func (p *Actor) loop() bool {
	if p.State() == StopState {
		if p.mail.Count() < 1 && p.event.Count() < 1 {
			return true
		}
	}

	select {
	case <-p.mail.C:
		p.processMail()
	case <-p.event.C:
		p.processEvent()
	case <-p.timer.C:
		p.processTimer()
	case <-p.close:
		p.state.Store(int32(StopState))
	}

	return false
}

func (p *Actor) processMail() {
	message := p.mail.Pop()
	if message == nil {
		return
	}
	p.lastAt = ctime.Now().ToSecond()
	if message.MethodID == 0 {
		p.finishTyped(message, cfacade.ErrorResult(cproto.StatusCode_STATUS_INVALID_ARGUMENT, "method id is required"))
		return
	}
	p.processTyped(message)
}

func (p *Actor) processTyped(message *cfacade.Message) {
	// A top-level Actor is the routing boundary for all of its dynamic children.
	if message.TargetPath().IsChild() && !p.path.IsChild() {
		childActor, found := p.findChildActor(message)
		if !found {
			p.finishTyped(message, cfacade.ErrorResult(cproto.StatusCode_STATUS_NOT_FOUND, "actor child not found"))
			return
		}
		childActor.post(message)
		return
	}
	p.invokeTyped(message)
}

func (p *Actor) invokeTyped(message *cfacade.Message) {
	ctx := message.Context
	if ctx == nil {
		ctx = cfacade.NewRequestContext(nil)
	}
	if err := ctx.Err(); err != nil {
		p.finishTyped(message, contextFailure(err, "actor invoke"))
		return
	}

	entry, found := p.mail.getMethod(message.MethodID)
	if !found {
		p.finishTyped(message, cfacade.ErrorResult(cproto.StatusCode_STATUS_NOT_FOUND, "actor method not found"))
		return
	}

	payload, decodeFailure := p.mail.decodePayload(ctx, entry, message.Payload)
	if decodeFailure != nil {
		p.finishTyped(message, decodeFailure)
		return
	}

	started := ctime.Now().ToMillisecond()
	var result *cfacade.InvokeResult
	// Business panics are isolated to the current message and converted into a
	// protocol error; the Actor loop remains alive for subsequent messages.
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				clog.Error("[mail] typed invoke panic. [target = %s, methodID = %d, error = %v]", message.Target, message.MethodID, recovered)
				result = cfacade.ErrorResult(cproto.StatusCode_STATUS_INTERNAL, "actor method panic")
			}
		}()
		result = entry.invoke(ctx, payload)
	}()
	if result == nil {
		result = cfacade.ErrorResult(cproto.StatusCode_STATUS_INTERNAL, "actor method returned nil result")
	}
	executionElapsed := ctime.Now().ToMillisecond() - started
	if executionElapsed > p.system.executionTimeout {
		clog.Warn("[mail] typed invoke slow. [target = %s, methodID = %d, execution = %dms]", message.Target, message.MethodID, executionElapsed)
	}
	p.finishTyped(message, result)
}

func (p *Actor) finishTyped(message *cfacade.Message, result *cfacade.InvokeResult) {
	// The result channel is buffered, so a caller timeout cannot block the Actor
	// while it completes and releases the pooled message.
	if message.ChanInvokeResult != nil {
		message.ChanInvokeResult <- result
	}
	message.Recycle()
}

func (p *Actor) processEvent() {
	eventData := p.event.Pop()
	if eventData == nil {
		return
	}

	p.lastAt = ctime.Now().ToSecond()
	p.event.invokeFunc(eventData)
}

func (p *Actor) processTimer() {
	timerID := p.timer.Pop()
	if timerID < 1 {
		return
	}

	p.timer.invokeFunc(timerID)
}

func (p *Actor) findChildActor(m *cfacade.Message) (*Actor, bool) {
	// 如果当前actor为子actor,则终止本次消息处理
	if p.path.IsChild() {
		clog.Warn("[findChildActor] Child actor cannot be created again。[target = %s->%s]",
			m.Target,
			m.MethodID,
		)
		return nil, false
	}

	// 寻找childActor
	childActor, found := p.child.Get(m.TargetPath().ChildID)
	if !found {
		childActor, found = p.handler.OnFindChild(m)
	}

	if found {
		if cActor, ok := childActor.(*Actor); ok {
			return cActor, true
		}
	}

	return nil, false
}

// InvokeChild delivers a typed request to a child Actor mailbox and waits for
// the result. Unlike posting a child-path message onto the parent mailbox, this
// never re-queues on the parent, so it is safe to call from a parent handler.
func (p *Actor) InvokeChild(ctx *cfacade.RequestContext, childID string, methodID uint32, payload any) *cfacade.InvokeResult {
	if methodID == 0 {
		return cfacade.ErrorResult(cproto.StatusCode_STATUS_INVALID_ARGUMENT, "method id is required")
	}
	if childID == "" {
		return cfacade.ErrorResult(cproto.StatusCode_STATUS_INVALID_ARGUMENT, "child id is required")
	}
	ctx = ensureRequestContext(p.system.app, ctx)
	target := cfacade.NewChildPath(p.path.NodeID, p.path.ActorID, childID)
	invokeCtx, cancel, failure := p.system.newInvokeContext(ctx)
	if failure != nil {
		return failure
	}
	defer cancel()
	message := typedMessage(invokeCtx, target, methodID, payload)
	childActor, found := p.findChildActor(message)
	if !found {
		message.Recycle()
		return cfacade.ErrorResult(cproto.StatusCode_STATUS_NOT_FOUND, "child actor not found")
	}
	resultCh := make(chan *cfacade.InvokeResult, 1)
	message.ChanInvokeResult = resultCh
	childActor.post(message)

	select {
	case result := <-resultCh:
		return result
	case <-invokeCtx.Done():
		return contextFailure(invokeCtx.Err(), "actor invoke")
	}
}

// NotifyChild delivers a one-way typed message to a child Actor mailbox.
func (p *Actor) NotifyChild(ctx *cfacade.RequestContext, childID string, methodID uint32, payload any) *cfacade.InvokeResult {
	if methodID == 0 {
		return cfacade.ErrorResult(cproto.StatusCode_STATUS_INVALID_ARGUMENT, "method id is required")
	}
	if childID == "" {
		return cfacade.ErrorResult(cproto.StatusCode_STATUS_INVALID_ARGUMENT, "child id is required")
	}
	ctx = ensureRequestContext(p.system.app, ctx)
	target := cfacade.NewChildPath(p.path.NodeID, p.path.ActorID, childID)
	notifyCtx, cancel, failure := p.system.newNotifyContext(ctx)
	if failure != nil {
		return failure
	}
	message := typedMessage(notifyCtx, target, methodID, payload)
	message.Cancel = cancel
	childActor, found := p.findChildActor(message)
	if !found {
		message.Recycle()
		return cfacade.ErrorResult(cproto.StatusCode_STATUS_NOT_FOUND, "child actor not found")
	}
	childActor.post(message)
	return cfacade.OKResult(nil)
}

func (p *Actor) onInit() {
	p.handler.OnInit()
	p.state.Store(int32(WorkerState))
}

func (p *Actor) onStop() {
	cutils.Try(func() {
		// Remove routes before removing the Actor, preventing new transports from
		// resolving a target that is already shutting down.
		if p.path.IsParent() && p.system.app != nil && p.system.app.Methods() != nil {
			p.system.app.Methods().UnregisterTarget(p.PathString())
		}
		if p.path.IsParent() {
			p.system.removeActor(p.ActorID())
			p.child.onStop()
		} else {
			if parent, found := p.system.GetActor(p.path.ActorID); found {
				parent.child.Remove(p.path.ChildID)
			}
		}

		p.handler.OnStop()
		p.timer.onStop()
		p.event.onStop()
		p.mail.onStop()
	}, func(errString string) {
		clog.Error(errString)
	})

	p.system.wg.Done()
}

// State returns the current lifecycle state.
func (p *Actor) State() State {
	return State(p.state.Load())
}

func (p *Actor) App() cfacade.IApplication {
	return p.system.app
}

func (p *Actor) ActorID() string {
	if p.path.IsChild() {
		return p.path.ChildID
	}

	return p.path.ActorID
}

func (p *Actor) Path() *cfacade.ActorPath {
	return p.path
}

func (p *Actor) PathString() string {
	return p.path.String()
}

// LastAt second
func (p *Actor) LastAt() int64 {
	return p.lastAt
}

// Invoke calls a top-level Actor method on the current node by MethodID.
func (p *Actor) Invoke(ctx *cfacade.RequestContext, methodID uint32, payload any) *cfacade.InvokeResult {
	target, failure := p.system.methodTarget(methodID)
	if failure != nil {
		return failure
	}
	if target == p.PathString() {
		return cfacade.ErrorResult(cproto.StatusCode_STATUS_FAILED_PRECONDITION, "actor cannot synchronously invoke itself")
	}
	return p.system.InvokeTarget(ctx, target, methodID, payload)
}

// Notify sends a one-way message to a top-level Actor on the current node.
func (p *Actor) Notify(ctx *cfacade.RequestContext, methodID uint32, payload any) *cfacade.InvokeResult {
	return p.system.Notify(ctx, methodID, payload)
}

// Exit requests an orderly stop after already queued mailbox and event work drains.
func (p *Actor) Exit() {
	select {
	case p.close <- struct{}{}:
	default:
	}
	if clog.PrintLevel(zapcore.DebugLevel) {
		clog.Debug("[Exit] actor exit! path = %s", p.path)
	}
}

// System returns the Actor system that owns this Actor.
func (p *Actor) System() *System {
	return p.system
}

// Methods returns the Actor method mailbox. Register publishes MethodIDs used by
// AGP/HTTP/cluster and same-node Actor Invoke/Notify.
func (p *Actor) Methods() IMailBox {
	return p.mail
}

// Event returns this Actor's event subscription API.
func (p *Actor) Event() IEvent {
	return p.event
}

// Child returns the dynamic child manager owned by this Actor.
func (p *Actor) Child() cfacade.IActorChild {
	return p.child
}

// Timer returns this Actor's timer scheduler.
func (p *Actor) Timer() ITimer {
	return p.timer
}

func (p *Actor) post(m *cfacade.Message) {
	p.mail.Push(m)
}

// PostEvent publishes event data through the owning Actor system.
func (p *Actor) PostEvent(data cfacade.IEventData) {
	p.system.PostEvent(data)
}

func newActor(actorID, childID string, handler cfacade.IActorHandler, c *System) (*Actor, error) {
	if strings.TrimSpace(actorID) == "" {
		clog.Error("[newActor] actor id is nil.")
		return _nilActor, ErrActorIDIsNil
	}

	thisActor := Actor{
		path: &cfacade.ActorPath{
			NodeID:  c.NodeID(),
			ActorID: actorID,
			ChildID: childID,
		},
		system:   c,
		close:    make(chan struct{}, 1),
		handler:  handler,
		lastAt:   ctime.Now().ToSecond(),
		initDone: make(chan error, 1),
	}

	// Wire all Actor-owned subsystems before injecting the Actor into Base-style
	// handlers, so OnInit observes a fully usable runtime object.
	child := newChild(&thisActor)
	thisActor.child = &child
	event := newEvent(&thisActor)
	thisActor.event = &event
	timer := newTimer(&thisActor)
	thisActor.timer = &timer
	mail := newMailbox(MailName, &thisActor)
	thisActor.mail = &mail

	if loader, ok := handler.(IActorLoader); ok {
		loader.load(&thisActor)
	}

	return &thisActor, nil
}
