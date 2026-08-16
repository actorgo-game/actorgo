package cserializer

import (
	cerr "github.com/actorgo-game/actorgo/error"
	cfacade "github.com/actorgo-game/actorgo/facade"
	"google.golang.org/protobuf/proto"
)

// Protobuf serializes concrete generated protobuf messages.
type Protobuf struct{}

// NewProtobuf creates the binary protobuf body codec.
func NewProtobuf() *Protobuf {
	return &Protobuf{}
}

// Marshal encodes a concrete protobuf message.
func (p *Protobuf) Marshal(v any) ([]byte, error) {
	if data, ok := v.([]byte); ok {
		return data, nil
	}
	pb, ok := v.(proto.Message)
	if !ok {
		return nil, cerr.ProtobufWrongValueType
	}
	return proto.Marshal(pb)
}

// Unmarshal decodes into a concrete protobuf message.
func (p *Protobuf) Unmarshal(data []byte, v any) error {
	pb, ok := v.(proto.Message)
	if !ok {
		return cerr.ProtobufWrongValueType
	}
	return proto.Unmarshal(data, pb)
}

// Name returns the codec name.
func (p *Protobuf) Name() string {
	return "protobuf"
}

// ID returns the stable AGP protobuf codec ID.
func (p *Protobuf) ID() int32 {
	return cfacade.CodecProtobuf
}
