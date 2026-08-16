package httpactor_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	actorgo "github.com/actorgo-game/actorgo"
	cfacade "github.com/actorgo-game/actorgo/facade"
	cactor "github.com/actorgo-game/actorgo/net/actor"
	httpactor "github.com/actorgo-game/actorgo/net/httpactor"
	cproto "github.com/actorgo-game/actorgo/net/proto"
	cprofile "github.com/actorgo-game/actorgo/profile"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	jsonMethod    uint32 = 2101
	pbMethod      uint32 = 2102
	notifyMethod  uint32 = 2103
	sessionMethod uint32 = 2104
)

type testNode struct{}

func (testNode) NodeID() string                { return "http-node" }
func (testNode) NodeType() string              { return "test" }
func (testNode) Address() string               { return "127.0.0.1:0" }
func (testNode) RpcAddress() string            { return "127.0.0.1:0" }
func (testNode) Settings() cfacade.ProfileJSON { return cprofile.Wrap(map[string]any{}) }
func (testNode) Enabled() bool                 { return true }

type jsonRequest struct {
	Value string `json:"value"`
}
type jsonResponse struct {
	Value string `json:"value"`
}
type notifyRequest struct {
	Value string `json:"value"`
}

type httpActor struct {
	cactor.Base
	ready    chan struct{}
	notified chan string
}

func (a *httpActor) AliasID() string { return "http-echo" }
func (a *httpActor) OnInit() {
	a.Methods().Register(jsonMethod, func(_ *cfacade.RequestContext, request *jsonRequest) (*jsonResponse, error) {
		return &jsonResponse{Value: strings.ToUpper(request.Value)}, nil
	})
	a.Methods().Register(pbMethod, func(_ *cfacade.RequestContext, request *wrapperspb.StringValue) (*wrapperspb.StringValue, error) {
		return wrapperspb.String(strings.ToUpper(request.Value)), nil
	})
	a.Methods().Register(notifyMethod, func(_ *cfacade.RequestContext, request *notifyRequest) error {
		a.notified <- request.Value
		return nil
	})
	a.Methods().Register(sessionMethod, func(ctx *cfacade.RequestContext, _ *jsonRequest) (*jsonResponse, error) {
		return &jsonResponse{Value: ctx.Session.GetString("role")}, nil
	})
	close(a.ready)
}

func setup(t *testing.T, opts ...httpactor.Option) (*httpactor.Handler, *httpActor) {
	app := actorgo.NewAppNode(testNode{}, actorgo.Standalone)
	component := app.ActorSystem().(*cactor.Component)
	component.Set(app)
	component.Init()
	actor := &httpActor{ready: make(chan struct{}), notified: make(chan string, 1)}
	if _, err := component.CreateActor(actor.AliasID(), actor); err != nil {
		t.Fatal(err)
	}
	select {
	case <-actor.ready:
	case <-time.After(time.Second):
		t.Fatal("actor did not initialize")
	}
	t.Cleanup(component.OnStop)
	return httpactor.NewHandler(app, opts...), actor
}

func TestHTTPAuthenticatorProvidesSession(t *testing.T) {
	handler, _ := setup(t, httpactor.WithAuthenticator(func(_ *http.Request) (*cproto.Session, error) {
		return &cproto.Session{Sid: "http-1", Uid: 42, Data: map[string]string{"role": "player"}}, nil
	}))
	request := httptest.NewRequest(http.MethodPost, "/actor/2104", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", httpactor.ContentTypeJSON)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body.String())
	}
	if got := strings.TrimSpace(response.Body.String()); got != `{"value":"player"}` {
		t.Fatalf("unexpected body %s", got)
	}
}

func TestCurlStyleJSONActorCall(t *testing.T) {
	handler, _ := setup(t)
	request := httptest.NewRequest(http.MethodPost, "/actor/2101", strings.NewReader(`{"value":"hello"}`))
	request.Header.Set("Content-Type", httpactor.ContentTypeJSON)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body.String())
	}
	if got := strings.TrimSpace(response.Body.String()); got != `{"value":"HELLO"}` {
		t.Fatalf("unexpected body %s", got)
	}
}

func TestHTTPProtobufRoundTrip(t *testing.T) {
	handler, _ := setup(t)
	body, _ := proto.Marshal(wrapperspb.String("hello"))
	request := httptest.NewRequest(http.MethodPost, "/actor/2102", bytes.NewReader(body))
	request.Header.Set("Content-Type", httpactor.ContentTypeProtobuf)
	request.Header.Set("Accept", httpactor.ContentTypeProtobuf)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body.String())
	}
	message := new(wrapperspb.StringValue)
	if err := proto.Unmarshal(response.Body.Bytes(), message); err != nil {
		t.Fatal(err)
	}
	if message.Value != "HELLO" {
		t.Fatalf("unexpected value %q", message.Value)
	}
}

func TestHTTPNotifyAccepted(t *testing.T) {
	handler, actor := setup(t)
	request := httptest.NewRequest(http.MethodPost, "/actor/2103", strings.NewReader(`{"value":"event"}`))
	request.Header.Set("Content-Type", httpactor.ContentTypeJSON)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("status %d: %s", response.Code, body)
	}
	select {
	case value := <-actor.notified:
		if value != "event" {
			t.Fatalf("unexpected notify %q", value)
		}
	case <-time.After(time.Second):
		t.Fatal("notify not delivered")
	}
}

func TestHTTPResponseUsesRequestCodec(t *testing.T) {
	handler, _ := setup(t)
	request := httptest.NewRequest(http.MethodPost, "/actor/2101", strings.NewReader(`{"value":"hello"}`))
	request.Header.Set("Content-Type", httpactor.ContentTypeJSON)
	request.Header.Set("Accept", httpactor.ContentTypeProtobuf)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != httpactor.ContentTypeJSON {
		t.Fatalf("unexpected content type %q", got)
	}
	if got := strings.TrimSpace(response.Body.String()); got != `{"value":"HELLO"}` {
		t.Fatalf("unexpected body %s", got)
	}
}

func TestHTTPRejectsUnknownMethod(t *testing.T) {
	handler, _ := setup(t)
	request := httptest.NewRequest(http.MethodPost, "/actor/9999", strings.NewReader(`{"value":"hello"}`))
	request.Header.Set("Content-Type", httpactor.ContentTypeJSON)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status %d: %s", response.Code, response.Body.String())
	}
}
