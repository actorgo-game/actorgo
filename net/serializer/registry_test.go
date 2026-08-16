package cserializer

import (
	"strings"
	"testing"

	cfacade "github.com/actorgo-game/actorgo/facade"
	cproto "github.com/actorgo-game/actorgo/net/proto"
)

func TestRegistrySupportsJSONAndProtobuf(t *testing.T) {
	registry := NewRegistry(NewJSON(), NewProtobuf())
	input := &cproto.HeartbeatRequest{ClientTimeMs: 1234567890123}

	pb, err := registry.Marshal(cfacade.CodecProtobuf, input)
	if err != nil {
		t.Fatal(err)
	}
	pbOutput := new(cproto.HeartbeatRequest)
	if err := registry.Unmarshal(cfacade.CodecProtobuf, pb, pbOutput); err != nil {
		t.Fatal(err)
	}
	if pbOutput.ClientTimeMs != input.ClientTimeMs {
		t.Fatalf("protobuf round trip got %d", pbOutput.ClientTimeMs)
	}

	jsonBody, err := registry.Marshal(cfacade.CodecJSON, input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(jsonBody), `"1234567890123"`) {
		t.Fatalf("expected canonical ProtoJSON int64 string, got %s", jsonBody)
	}
	jsonOutput := new(cproto.HeartbeatRequest)
	if err := registry.Unmarshal(cfacade.CodecJSON, jsonBody, jsonOutput); err != nil {
		t.Fatal(err)
	}
	if jsonOutput.ClientTimeMs != input.ClientTimeMs {
		t.Fatalf("json round trip got %d", jsonOutput.ClientTimeMs)
	}
}

func TestJSONCodecSupportsPlainStruct(t *testing.T) {
	type request struct {
		Name string `json:"name"`
	}
	codec := NewJSON()
	body, err := codec.Marshal(&request{Name: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	output := new(request)
	if err := codec.Unmarshal(body, output); err != nil {
		t.Fatal(err)
	}
	if output.Name != "demo" {
		t.Fatalf("got %q", output.Name)
	}
}

func TestRegistryDefaultCodec(t *testing.T) {
	registry := NewRegistry(NewJSON(), NewProtobuf())
	if registry.Default() != cfacade.CodecProtobuf {
		t.Fatalf("default = %d, want protobuf", registry.Default())
	}
	if err := registry.SetDefault(cfacade.CodecJSON); err != nil {
		t.Fatal(err)
	}
	if registry.Default() != cfacade.CodecJSON {
		t.Fatalf("default = %d, want json", registry.Default())
	}
	if err := registry.SetDefault(99); err == nil {
		t.Fatal("expected error for unknown codec id")
	}
}
