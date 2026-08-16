package method_test

import (
	"context"
	"strings"
	"testing"
	"time"

	actorgo "github.com/actorgo-game/actorgo"
	cfacade "github.com/actorgo-game/actorgo/facade"
	cactor "github.com/actorgo-game/actorgo/net/actor"
	cmethod "github.com/actorgo-game/actorgo/net/method"
	cproto "github.com/actorgo-game/actorgo/net/proto"
	cprofile "github.com/actorgo-game/actorgo/profile"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	echoMethod  uint32 = 1001
	plainMethod uint32 = 1002
)

type testNode struct{}

func (testNode) NodeID() string                { return "node-1" }
func (testNode) NodeType() string              { return "test" }
func (testNode) Address() string               { return "127.0.0.1:0" }
func (testNode) RpcAddress() string            { return "127.0.0.1:0" }
func (testNode) Settings() cfacade.ProfileJSON { return cprofile.Wrap(map[string]any{}) }
func (testNode) Enabled() bool                 { return true }

type plainRequest struct {
	Value string `json:"value"`
}
type plainResponse struct {
	Value string `json:"value"`
}

type echoActor struct {
	cactor.Base
	ready chan struct{}
}

func (a *echoActor) AliasID() string { return "echo" }
func (a *echoActor) OnInit() {
	a.Methods().Register(echoMethod, a.upper)
	a.Methods().Register(plainMethod, a.upperPlain)
	close(a.ready)
}

func (a *echoActor) upper(_ *cfacade.RequestContext, request *wrapperspb.StringValue) (*wrapperspb.StringValue, error) {
	return wrapperspb.String(strings.ToUpper(request.Value)), nil
}

func (a *echoActor) upperPlain(_ *cfacade.RequestContext, request *plainRequest) (*plainResponse, error) {
	return &plainResponse{Value: strings.ToUpper(request.Value)}, nil
}

func setup(t *testing.T) (*actorgo.Application, cfacade.IMethodTable) {
	t.Helper()
	app := actorgo.NewAppNode(testNode{}, actorgo.Standalone)
	component := app.ActorSystem().(*cactor.Component)
	component.Set(app)
	component.Init()
	actor := &echoActor{ready: make(chan struct{})}
	if _, err := component.CreateActor(actor.AliasID(), actor); err != nil {
		t.Fatal(err)
	}
	select {
	case <-actor.ready:
	case <-time.After(time.Second):
		t.Fatal("actor did not initialize")
	}
	t.Cleanup(component.OnStop)
	return app, app.Methods()
}

func TestProtobufMethodSupportsJSONAndPB(t *testing.T) {
	_, table := setup(t)
	for _, test := range []struct {
		name  string
		codec int32
		body  []byte
	}{
		{name: "json", codec: cfacade.CodecJSON, body: []byte(`"hello"`)},
		{name: "protobuf", codec: cfacade.CodecProtobuf, body: mustMarshal(t, wrapperspb.String("hello"))},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := cfacade.NewRequestContext(context.Background())
			ctx.Codec = test.codec
			result := table.Dispatch(ctx, echoMethod, test.body, cproto.MsgType_REQUEST)
			if !result.OK() {
				t.Fatalf("unexpected result: %+v", result)
			}
			response, ok := result.Payload.(*wrapperspb.StringValue)
			if !ok || response.Value != "HELLO" {
				t.Fatalf("unexpected response %#v", result.Payload)
			}
		})
	}
}

func TestJSONMethodRejectsProtobufCodec(t *testing.T) {
	_, table := setup(t)
	ctx := cfacade.NewRequestContext(context.Background())
	ctx.Codec = cfacade.CodecProtobuf
	result := table.Dispatch(ctx, plainMethod, []byte{}, cproto.MsgType_REQUEST)
	if result.Code != int32(cproto.StatusCode_STATUS_UNSUPPORTED_MEDIA) {
		t.Fatalf("unexpected code %d", result.Code)
	}
}

func TestActorSystemInvokeResolvesTargetByMethodID(t *testing.T) {
	app, _ := setup(t)
	ctx := cfacade.NewRequestContext(context.Background())
	ctx.Codec = cfacade.CodecProtobuf
	result := app.ActorSystem().Invoke(ctx, echoMethod, wrapperspb.String("hello"))
	if !result.OK() {
		t.Fatalf("unexpected result: %+v", result)
	}
	response, ok := result.Payload.(*wrapperspb.StringValue)
	if !ok || response.Value != "HELLO" {
		t.Fatalf("unexpected response %#v", result.Payload)
	}
}

func TestTableRejectsCollisions(t *testing.T) {
	app := actorgo.NewAppNode(testNode{}, actorgo.Standalone)
	table := cmethod.NewTable(app)
	if err := table.Register(7, "node-1.echo", cproto.MsgType_REQUEST); err != nil {
		t.Fatal(err)
	}
	if err := table.Register(7, "node-1.other", cproto.MsgType_REQUEST); err == nil {
		t.Fatal("expected duplicate method id error")
	}
}

func TestTableRejectsReservedMethodID(t *testing.T) {
	app := actorgo.NewAppNode(testNode{}, actorgo.Standalone)
	table := cmethod.NewTable(app)
	err := table.Register(cproto.SystemMethodKick, "node-1.echo", cproto.MsgType_NOTIFY)
	if err == nil {
		t.Fatal("expected reserved method id error")
	}
}

func TestTableRejectsChildTarget(t *testing.T) {
	app := actorgo.NewAppNode(testNode{}, actorgo.Standalone)
	table := cmethod.NewTable(app)
	if err := table.Register(7, "node-1.echo.child-1", cproto.MsgType_REQUEST); err == nil {
		t.Fatal("expected child target rejection")
	}
}

func TestActorRegistrationPublishesMethod(t *testing.T) {
	app, table := setup(t)
	msgType, found := table.MsgType(echoMethod)
	if !found || msgType != cproto.MsgType_REQUEST {
		t.Fatalf("unexpected registered message type %d, found %v", msgType, found)
	}
	if app.Methods() != table {
		t.Fatal("application should expose the Actor method table")
	}
	target, found := table.Target(echoMethod)
	if !found || target != "node-1.echo" {
		t.Fatalf("unexpected method target %q, found %v", target, found)
	}
}

func mustMarshal(t *testing.T, message proto.Message) []byte {
	t.Helper()
	data, err := proto.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
