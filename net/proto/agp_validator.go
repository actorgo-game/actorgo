package cproto

import "fmt"

const (
	// System method IDs are reserved for connection control and cannot be
	// registered by business Actors.
	SystemMethodHandshake uint32 = 1
	SystemMethodHeartbeat uint32 = 2
	SystemMethodCancel    uint32 = 3
	SystemMethodGoAway    uint32 = 4
	SystemMethodKick      uint32 = 5

	codecProtobuf int32 = 1
	codecJSON     int32 = 2
)

// Limits bounds memory allocated while decoding one untrusted AGP packet.
type Limits struct {
	MaxPacketSize    int
	MaxBodySize      int
	MaxMetadataKeys  int
	MaxMetadataKey   int
	MaxMetadataValue int
	MaxMetadataSize  int
}

// DefaultLimits returns the server's default packet and metadata limits.
func DefaultLimits() Limits {
	return Limits{
		MaxPacketSize:    DefaultMaxPacketSize,
		MaxBodySize:      DefaultMaxPacketSize - 1024,
		MaxMetadataKeys:  32,
		MaxMetadataKey:   64,
		MaxMetadataValue: 1024,
		MaxMetadataSize:  8 << 10,
	}
}

// NormalizeLimits fills each unset field independently and keeps body limits
// within the packet limit. Transports and PacketCodec must share this result.
func NormalizeLimits(limits Limits) Limits {
	defaults := DefaultLimits()
	if limits.MaxPacketSize <= 0 {
		limits.MaxPacketSize = defaults.MaxPacketSize
	}
	if limits.MaxBodySize <= 0 {
		limits.MaxBodySize = defaults.MaxBodySize
	}
	if limits.MaxBodySize > limits.MaxPacketSize {
		limits.MaxBodySize = limits.MaxPacketSize
	}
	if limits.MaxMetadataKeys <= 0 {
		limits.MaxMetadataKeys = defaults.MaxMetadataKeys
	}
	if limits.MaxMetadataKey <= 0 {
		limits.MaxMetadataKey = defaults.MaxMetadataKey
	}
	if limits.MaxMetadataValue <= 0 {
		limits.MaxMetadataValue = defaults.MaxMetadataValue
	}
	if limits.MaxMetadataSize <= 0 {
		limits.MaxMetadataSize = defaults.MaxMetadataSize
	}
	return limits
}

// ValidatePacket checks structural invariants before a packet reaches routing.
func ValidatePacket(packet *Packet, limits Limits) error {
	if packet == nil {
		return fmt.Errorf("agp: packet is nil")
	}
	limits = NormalizeLimits(limits)
	if packet.Kind == nil {
		return fmt.Errorf("agp: packet kind is missing")
	}
	if packet.Codec != codecProtobuf && packet.Codec != codecJSON {
		return fmt.Errorf("agp: unsupported codec %d", packet.Codec)
	}

	var body []byte
	switch kind := packet.Kind.(type) {
	case *Packet_Request:
		if kind.Request == nil || kind.Request.RequestId == 0 || kind.Request.MethodId == 0 {
			return fmt.Errorf("agp: request_id and method_id must be non-zero")
		}
		body = kind.Request.Body
	case *Packet_Response:
		if kind.Response == nil || kind.Response.RequestId == 0 {
			return fmt.Errorf("agp: response request_id must be non-zero")
		}
		body = kind.Response.Body
	case *Packet_Notify:
		if kind.Notify == nil || kind.Notify.MethodId == 0 {
			return fmt.Errorf("agp: notify method_id must be non-zero")
		}
		body = kind.Notify.Body
	default:
		return fmt.Errorf("agp: unknown packet kind %T", packet.Kind)
	}
	if len(body) > limits.MaxBodySize {
		return fmt.Errorf("agp: body exceeds %d bytes", limits.MaxBodySize)
	}
	if len(packet.Metadata) > limits.MaxMetadataKeys {
		return fmt.Errorf("agp: too many metadata entries")
	}
	metadataSize := 0
	for key, value := range packet.Metadata {
		if len(key) == 0 || len(key) > limits.MaxMetadataKey {
			return fmt.Errorf("agp: invalid metadata key length")
		}
		if len(value) > limits.MaxMetadataValue {
			return fmt.Errorf("agp: metadata value for %q is too large", key)
		}
		metadataSize += len(key) + len(value)
	}
	if metadataSize > limits.MaxMetadataSize {
		return fmt.Errorf("agp: metadata exceeds %d bytes", limits.MaxMetadataSize)
	}
	return nil
}
