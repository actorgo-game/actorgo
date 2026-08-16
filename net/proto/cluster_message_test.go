package cproto

import (
	"testing"

	"google.golang.org/protobuf/proto"
)

func TestClusterMessageRoundTrip(t *testing.T) {
	input := &ClusterMessage{
		MessageId: 9, MsgType: MsgType_REQUEST,
		RequestId: 7, MethodId: 4101, DeadlineUnixMs: 123456,
		TargetPath: "node-b.echo",
		Metadata:   map[string][]byte{"traceparent": []byte("00-test")},
		Codec:      2,
		Payload:    []byte(`{"value":"hello"}`),
		Session:    &Session{Sid: "connection-1", Uid: 42, Data: map[string]string{"role": "player"}},
	}
	wire, err := proto.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	output := new(ClusterMessage)
	if err := proto.Unmarshal(wire, output); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(input, output) {
		t.Fatalf("round trip mismatch: got %v want %v", output, input)
	}
}

func TestClusterMessageResultRoundTrip(t *testing.T) {
	input := &ClusterMessage{
		MessageId: 9,
		MsgType:   MsgType_RESPONSE,
		RequestId: 7,
		MethodId:  4101,
		Code:      int32(StatusCode_STATUS_NOT_FOUND),
		Message:   "actor not found",
	}
	wire, err := proto.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	output := new(ClusterMessage)
	if err := proto.Unmarshal(wire, output); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(input, output) {
		t.Fatalf("round trip mismatch: got %v want %v", output, input)
	}
}
