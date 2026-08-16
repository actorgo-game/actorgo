package cconnector

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// websocket client http://www.websocket-test.com/
func TestNewWSConnector(t *testing.T) {
	connector := NewWS("127.0.0.1:0", WithChanSize(8))
	if connector == nil {
		t.Fatal("expected WebSocket connector")
	}
	if connector.Name() != "websocket_connector" {
		t.Fatalf("unexpected connector name %q", connector.Name())
	}
	if connector.chanSize != 8 {
		t.Fatalf("unexpected channel size %d", connector.chanSize)
	}
}

func TestWebSocketRequiredSubprotocol(t *testing.T) {
	connector := NewWS("127.0.0.1:0")
	connector.SetRequiredSubprotocol("agp.v1")

	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	recorder := httptest.NewRecorder()
	connector.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUpgradeRequired {
		t.Fatalf("expected status %d, got %d", http.StatusUpgradeRequired, recorder.Code)
	}
	if got := recorder.Header().Get("Sec-WebSocket-Protocol"); got != "agp.v1" {
		t.Fatalf("expected required subprotocol header, got %q", got)
	}
}
