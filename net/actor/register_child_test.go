package cactor_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	actorgo "github.com/actorgo-game/actorgo"
	cfacade "github.com/actorgo-game/actorgo/facade"
	cactor "github.com/actorgo-game/actorgo/net/actor"
	cproto "github.com/actorgo-game/actorgo/net/proto"
	cprofile "github.com/actorgo-game/actorgo/profile"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const sharedChildMethod uint32 = 4201

type childShareTestNode struct{}

func (childShareTestNode) NodeID() string                { return "node-child-share" }
func (childShareTestNode) NodeType() string              { return "test" }
func (childShareTestNode) Address() string               { return "127.0.0.1:0" }
func (childShareTestNode) RpcAddress() string            { return "127.0.0.1:0" }
func (childShareTestNode) Settings() cfacade.ProfileJSON { return cprofile.Wrap(map[string]any{}) }
func (childShareTestNode) Enabled() bool                 { return true }

type parentActor struct {
	cactor.Base
	ready chan struct{}
}

func (p *parentActor) AliasID() string { return "parent" }

func (p *parentActor) OnInit() {
	close(p.ready)
}

func (p *parentActor) OnFindChild(msg *cfacade.Message) (cfacade.IActor, bool) {
	child, err := p.Child().Create(msg.TargetPath().ChildID, &childActor{})
	if err != nil {
		return nil, false
	}
	return child, true
}

type childActor struct {
	cactor.Base
}

func (c *childActor) OnInit() {
	c.Methods().Register(sharedChildMethod, c.echo)
}

func (c *childActor) echo(_ *cfacade.RequestContext, req *wrapperspb.StringValue) (*wrapperspb.StringValue, error) {
	return wrapperspb.String("child:" + req.GetValue()), nil
}

func TestRegisterSharedByChildActors(t *testing.T) {
	app := actorgo.NewAppNode(childShareTestNode{}, actorgo.Standalone)
	component := app.ActorSystem().(*cactor.Component)
	component.Set(app)
	component.Init()

	parent := &parentActor{ready: make(chan struct{})}
	if _, err := component.CreateActor(parent.AliasID(), parent); err != nil {
		t.Fatal(err)
	}
	select {
	case <-parent.ready:
	case <-time.After(time.Second):
		t.Fatal("parent did not initialize")
	}
	t.Cleanup(component.OnStop)

	for _, uid := range []int64{11, 22, 33} {
		ctx := cfacade.NewRequestContext(context.Background())
		ctx.Codec = cfacade.CodecProtobuf
		ctx.Session = &cproto.Session{Uid: uid, Data: map[string]string{}}
		target := cfacade.NewChildPath(app.NodeID(), "parent", strconv.FormatInt(uid, 10))
		result := app.ActorSystem().InvokeTarget(ctx, target, sharedChildMethod, wrapperspb.String("ok"))
		if !result.OK() {
			t.Fatalf("uid=%d invoke failed: code=%d message=%s", uid, result.Code, result.Message)
		}
		rsp, ok := result.Payload.(*wrapperspb.StringValue)
		if !ok || rsp.GetValue() != "child:ok" {
			t.Fatalf("uid=%d unexpected payload %#v", uid, result.Payload)
		}
	}
}

func TestChildMethodsAreNotPublishedToExternalTable(t *testing.T) {
	app := actorgo.NewAppNode(childShareTestNode{}, actorgo.Standalone)
	component := app.ActorSystem().(*cactor.Component)
	component.Set(app)
	component.Init()

	parent := &parentActor{ready: make(chan struct{})}
	if _, err := component.CreateActor(parent.AliasID(), parent); err != nil {
		t.Fatal(err)
	}
	select {
	case <-parent.ready:
	case <-time.After(time.Second):
		t.Fatal("parent did not initialize")
	}
	t.Cleanup(component.OnStop)

	ctx := cfacade.NewRequestContext(context.Background())
	ctx.Codec = cfacade.CodecProtobuf
	ctx.Session = &cproto.Session{Uid: 7, Data: map[string]string{}}
	target := cfacade.NewChildPath(app.NodeID(), "parent", "7")
	result := app.ActorSystem().InvokeTarget(ctx, target, sharedChildMethod, wrapperspb.String("internal"))
	if !result.OK() {
		t.Fatalf("internal child invoke failed: code=%d message=%s", result.Code, result.Message)
	}
	rsp, ok := result.Payload.(*wrapperspb.StringValue)
	if !ok || rsp.GetValue() != "child:internal" {
		t.Fatalf("unexpected payload %#v", result.Payload)
	}
	if _, found := app.Methods().MsgType(sharedChildMethod); found {
		t.Fatal("child method must not be published to the external method table")
	}
	result = app.Methods().Dispatch(ctx, sharedChildMethod, nil, cproto.MsgType_REQUEST)
	if result.Code != int32(cproto.StatusCode_STATUS_NOT_FOUND) {
		t.Fatalf("unexpected code %d", result.Code)
	}
}
