package cserializer

import (
	cfacade "github.com/actorgo-game/actorgo/facade"
	jsoniter "github.com/json-iterator/go"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// JSON implements the shared body codec for ordinary Go values and protobuf messages.
type JSON struct {
	marshalOptions   protojson.MarshalOptions
	unmarshalOptions protojson.UnmarshalOptions
}

// NewJSON creates a codec for ordinary structs and canonical ProtoJSON messages.
func NewJSON() *JSON {
	return &JSON{
		marshalOptions: protojson.MarshalOptions{
			UseProtoNames:   false,
			EmitUnpopulated: false,
		},
		unmarshalOptions: protojson.UnmarshalOptions{
			DiscardUnknown: false,
		},
	}
}

// Marshal uses canonical ProtoJSON for protobuf messages and jsoniter for plain structs.
func (j *JSON) Marshal(v any) ([]byte, error) {
	if data, ok := v.([]byte); ok {
		return data, nil
	}
	if message, ok := v.(proto.Message); ok {
		return j.marshalOptions.Marshal(message)
	}
	return jsoniter.Marshal(v)
}

// Unmarshal uses canonical ProtoJSON for protobuf messages and jsoniter for plain structs.
func (j *JSON) Unmarshal(data []byte, v any) error {
	if message, ok := v.(proto.Message); ok {
		return j.unmarshalOptions.Unmarshal(data, message)
	}
	return jsoniter.Unmarshal(data, v)
}

// Name returns the codec name.
func (j *JSON) Name() string {
	return "json"
}

// ID returns the stable AGP JSON codec ID.
func (j *JSON) ID() int32 {
	return cfacade.CodecJSON
}
