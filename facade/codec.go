package cfacade

const (
	// CodecProtobuf and CodecJSON are stable wire values carried by AGP and
	// cluster envelopes. Do not reuse them for another encoding.
	CodecProtobuf int32 = 1
	CodecJSON     int32 = 2
)

// IBodyCodec serializes one concrete Request, Response, or Notify body.
// Packet and cluster envelopes are always protobuf and never pass through this interface.
type IBodyCodec interface {
	ISerializer
	ID() int32
}

// IBodyCodecRegistry contains the finite set of codecs accepted by the framework protocol.
type IBodyCodecRegistry interface {
	Register(codec IBodyCodec) error
	Lookup(id int32) (IBodyCodec, bool)
	Marshal(id int32, value any) ([]byte, error)
	Unmarshal(id int32, data []byte, value any) error
	Default() int32
	SetDefault(id int32) error
}
