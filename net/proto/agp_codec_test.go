package cproto

import (
	"bytes"
	"testing"
)

func TestPacketCodecRoundTrip(t *testing.T) {
	codec := NewPacketCodec(DefaultLimits())
	input := &Packet{
		Kind: &Packet_Request{Request: &Request{
			RequestId: 7,
			MethodId:  1001,
			TimeoutMs: 3000,
			Body:      []byte(`{"name":"demo"}`),
		}},
		Metadata: map[string][]byte{"traceparent": []byte("00-test")},
		Codec:    codecJSON,
	}
	data, err := codec.Encode(input)
	if err != nil {
		t.Fatal(err)
	}
	output, err := codec.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if output.GetRequest().GetRequestId() != 7 || !bytes.Equal(output.GetRequest().GetBody(), input.GetRequest().GetBody()) {
		t.Fatalf("unexpected round trip output: %+v", output)
	}
}

func TestResponseCodeRoundTrip(t *testing.T) {
	codec := NewPacketCodec(DefaultLimits())
	input := &Packet{
		Kind: &Packet_Response{Response: &Response{
			RequestId: 7,
			Code:      int32(StatusCode_STATUS_NOT_FOUND),
			Message:   "actor not found",
		}},
		Codec: codecProtobuf,
	}
	data, err := codec.Encode(input)
	if err != nil {
		t.Fatal(err)
	}
	output, err := codec.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	response := output.GetResponse()
	if response.GetCode() != int32(StatusCode_STATUS_NOT_FOUND) || response.GetMessage() != "actor not found" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestValidatePacketRejectsInvalidRequest(t *testing.T) {
	packet := &Packet{
		Kind:  &Packet_Request{Request: &Request{MethodId: 1001}},
		Codec: codecProtobuf,
	}
	if err := ValidatePacket(packet, DefaultLimits()); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestValidatePacketRejectsUnknownCodec(t *testing.T) {
	packet := &Packet{
		Kind:  &Packet_Notify{Notify: &Notify{MethodId: 1001}},
		Codec: 0,
	}
	if err := ValidatePacket(packet, DefaultLimits()); err == nil {
		t.Fatal("expected codec validation error")
	}
}

func TestNormalizeLimitsFillsEveryUnsetField(t *testing.T) {
	limits := NormalizeLimits(Limits{MaxPacketSize: 1024})
	if limits.MaxPacketSize != 1024 || limits.MaxBodySize != 1024 {
		t.Fatalf("unexpected packet/body limits: %+v", limits)
	}
	if limits.MaxMetadataKeys <= 0 || limits.MaxMetadataKey <= 0 || limits.MaxMetadataValue <= 0 || limits.MaxMetadataSize <= 0 {
		t.Fatalf("metadata defaults were not filled: %+v", limits)
	}
	codec := NewPacketCodec(Limits{})
	if _, err := codec.Encode(&Packet{Kind: &Packet_Notify{Notify: &Notify{MethodId: 1001, Body: []byte("ok")}}, Codec: codecProtobuf}); err != nil {
		t.Fatalf("empty limits must use defaults: %v", err)
	}
}

func FuzzPacketCodecDecode(f *testing.F) {
	codec := NewPacketCodec(DefaultLimits())
	valid, err := codec.Encode(&Packet{
		Kind:  &Packet_Notify{Notify: &Notify{MethodId: 1001, Body: []byte("seed")}},
		Codec: codecProtobuf,
	})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	f.Add([]byte{0xff, 0x00, 0x01})
	f.Fuzz(func(t *testing.T, data []byte) {
		packet, err := codec.Decode(data)
		if err != nil {
			return
		}
		if _, err := codec.Encode(packet); err != nil {
			t.Fatalf("decoded packet cannot be encoded: %v", err)
		}
	})
}
