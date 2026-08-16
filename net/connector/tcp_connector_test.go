package cconnector

import "testing"

func TestNewTCPConnector(t *testing.T) {
	connector := NewTCP("127.0.0.1:0", WithChanSize(8))
	if connector == nil {
		t.Fatal("expected TCP connector")
	}
	if connector.Name() != "tcp_connector" {
		t.Fatalf("unexpected connector name %q", connector.Name())
	}
	if connector.chanSize != 8 {
		t.Fatalf("unexpected channel size %d", connector.chanSize)
	}
}

func TestConnectorStopIsIdempotent(t *testing.T) {
	connector := NewTCP("127.0.0.1:0")
	if !connector.Running() {
		t.Fatal("new connector must be running")
	}
	connector.Stop()
	connector.Stop()
	if connector.Running() {
		t.Fatal("stopped connector is still running")
	}
}
