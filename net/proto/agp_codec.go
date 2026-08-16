package cproto

import (
	"fmt"

	"google.golang.org/protobuf/proto"
)

// DefaultMaxPacketSize is the maximum encoded AGP packet size (4 MiB).
const DefaultMaxPacketSize = 4 << 20

// PacketCodec validates and protobuf-encodes the AGP packet envelope.
type PacketCodec struct {
	Limits Limits
}

// NewPacketCodec creates a codec and applies DefaultLimits when limits are empty.
func NewPacketCodec(limits Limits) *PacketCodec {
	return &PacketCodec{Limits: NormalizeLimits(limits)}
}

// Encode validates packet invariants before protobuf marshaling.
func (c *PacketCodec) Encode(packet *Packet) ([]byte, error) {
	if err := ValidatePacket(packet, c.Limits); err != nil {
		return nil, err
	}
	data, err := proto.Marshal(packet)
	if err != nil {
		return nil, fmt.Errorf("agp: marshal packet: %w", err)
	}
	if len(data) > c.Limits.MaxPacketSize {
		return nil, fmt.Errorf("agp: encoded packet exceeds %d bytes", c.Limits.MaxPacketSize)
	}
	return data, nil
}

// Decode bounds input size, unmarshals protobuf and validates the result.
func (c *PacketCodec) Decode(data []byte) (*Packet, error) {
	if len(data) == 0 || len(data) > c.Limits.MaxPacketSize {
		return nil, fmt.Errorf("agp: invalid packet size %d", len(data))
	}
	packet := new(Packet)
	if err := proto.Unmarshal(data, packet); err != nil {
		return nil, fmt.Errorf("agp: unmarshal packet: %w", err)
	}
	if err := ValidatePacket(packet, c.Limits); err != nil {
		return nil, err
	}
	return packet, nil
}
