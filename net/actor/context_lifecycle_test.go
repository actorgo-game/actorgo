package cactor_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	actorgo "github.com/actorgo-game/actorgo"
	cfacade "github.com/actorgo-game/actorgo/facade"
	cactor "github.com/actorgo-game/actorgo/net/actor"
	cproto "github.com/actorgo-game/actorgo/net/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	lifecycleRequestMethod uint32 = 4301
	lifecycleNotifyMethod  uint32 = 4302
	lifecycleBarrierMethod uint32 = 4303
	selfInvokeMethod       uint32 = 4304
)

type lifecycleActor struct {
	cactor.Base
	entered   chan struct{}
	release   chan struct{}
	notifyErr chan error
	secondRun atomic.Int32
}

func (a *lifecycleActor) AliasID() string { return "lifecycle" }

func (a *lifecycleActor) OnInit() {
	a.Methods().Register(lifecycleRequestMethod, a.request)
	a.Methods().Register(lifecycleNotifyMethod, a.notify)
	a.Methods().Register(lifecycleBarrierMethod, a.barrier)
}

func (a *lifecycleActor) request(_ *cfacade.RequestContext, req *wrapperspb.StringValue) (*wrapperspb.StringValue, error) {
	switch req.GetValue() {
	case "first":
		close(a.entered)
		<-a.release
	case "second":
		a.secondRun.Add(1)
	}
	return req, nil
}

func (a *lifecycleActor) notify(ctx *cfacade.RequestContext, _ *wrapperspb.StringValue) error {
	a.notifyErr <- ctx.Err()
	return nil
}

func (a *lifecycleActor) barrier(_ *cfacade.RequestContext, req *wrapperspb.StringValue) (*wrapperspb.StringValue, error) {
	return req, nil
}

func newLifecycleApp(t *testing.T, actor *lifecycleActor) *actorgo.Application {
	t.Helper()
	app := actorgo.NewAppNode(childShareTestNode{}, actorgo.Standalone)
	component := app.ActorSystem().(*cactor.Component)
	component.Set(app)
	component.Init()
	if _, err := component.CreateActor(actor.AliasID(), actor); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(component.OnStop)
	return app
}

func TestTimedOutQueuedInvokeIsNotExecuted(t *testing.T) {
	actor := &lifecycleActor{entered: make(chan struct{}), release: make(chan struct{}), notifyErr: make(chan error, 1)}
	app := newLifecycleApp(t, actor)
	firstDone := make(chan *cfacade.InvokeResult, 1)
	go func() {
		firstDone <- app.ActorSystem().Invoke(cfacade.NewRequestContext(context.Background()), lifecycleRequestMethod, wrapperspb.String("first"))
	}()
	select {
	case <-actor.entered:
	case <-time.After(time.Second):
		t.Fatal("first request did not enter the Actor")
	}

	parent, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	result := app.ActorSystem().Invoke(cfacade.NewRequestContext(parent), lifecycleRequestMethod, wrapperspb.String("second"))
	if result.Code != int32(cproto.StatusCode_STATUS_DEADLINE_EXCEEDED) {
		t.Fatalf("unexpected timeout result: code=%d message=%s", result.Code, result.Message)
	}

	close(actor.release)
	if result = <-firstDone; !result.OK() {
		t.Fatalf("first request failed: code=%d message=%s", result.Code, result.Message)
	}
	result = app.ActorSystem().Invoke(cfacade.NewRequestContext(context.Background()), lifecycleBarrierMethod, wrapperspb.String("barrier"))
	if !result.OK() {
		t.Fatalf("barrier failed: code=%d message=%s", result.Code, result.Message)
	}
	if actor.secondRun.Load() != 0 {
		t.Fatal("timed-out queued request was executed")
	}
}

func TestNotifyOwnsDetachedContextUntilHandled(t *testing.T) {
	actor := &lifecycleActor{entered: make(chan struct{}), release: make(chan struct{}), notifyErr: make(chan error, 1)}
	app := newLifecycleApp(t, actor)
	firstDone := make(chan *cfacade.InvokeResult, 1)
	go func() {
		firstDone <- app.ActorSystem().Invoke(cfacade.NewRequestContext(context.Background()), lifecycleRequestMethod, wrapperspb.String("first"))
	}()
	<-actor.entered

	parent, cancel := context.WithCancel(context.Background())
	result := app.ActorSystem().Notify(cfacade.NewRequestContext(parent), lifecycleNotifyMethod, wrapperspb.String("notify"))
	if !result.OK() {
		t.Fatalf("notify failed: code=%d message=%s", result.Code, result.Message)
	}
	cancel()
	close(actor.release)
	<-firstDone
	select {
	case err := <-actor.notifyErr:
		if err != nil {
			t.Fatalf("notify inherited ingress cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("notify was not handled")
	}
}

type selfInvokeActor struct{ cactor.Base }

func (a *selfInvokeActor) AliasID() string { return "self-invoke" }
func (a *selfInvokeActor) OnInit()         { a.Methods().Register(selfInvokeMethod, a.call) }
func (a *selfInvokeActor) call(ctx *cfacade.RequestContext, req *wrapperspb.StringValue) (*wrapperspb.Int32Value, error) {
	result := a.Invoke(ctx, selfInvokeMethod, req)
	return wrapperspb.Int32(result.Code), nil
}

func TestActorSynchronousSelfInvokeFailsImmediately(t *testing.T) {
	actor := &selfInvokeActor{}
	app := actorgo.NewAppNode(childShareTestNode{}, actorgo.Standalone)
	component := app.ActorSystem().(*cactor.Component)
	component.Set(app)
	component.Init()
	if _, err := component.CreateActor(actor.AliasID(), actor); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(component.OnStop)

	result := app.ActorSystem().Invoke(cfacade.NewRequestContext(context.Background()), selfInvokeMethod, wrapperspb.String("self"))
	if !result.OK() {
		t.Fatalf("outer invoke failed: code=%d message=%s", result.Code, result.Message)
	}
	response, ok := result.Payload.(*wrapperspb.Int32Value)
	if !ok || response.GetValue() != int32(cproto.StatusCode_STATUS_FAILED_PRECONDITION) {
		t.Fatalf("unexpected self-invoke response %#v", result.Payload)
	}
}
