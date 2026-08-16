package parser

import (
	"encoding/binary"
	"fmt"
	"io"

	cproto "github.com/actorgo-game/actorgo/net/proto"
)

// TCPPacketFramer implements AGP's four-byte big-endian packet length prefix.
type TCPPacketFramer struct {
	MaxPacketSize uint32
}

// NewTCPPacketFramer creates a framer and applies the protocol default for zero.
func NewTCPPacketFramer(maxPacketSize uint32) *TCPPacketFramer {
	if maxPacketSize == 0 {
		maxPacketSize = cproto.DefaultMaxPacketSize
	}
	return &TCPPacketFramer{MaxPacketSize: maxPacketSize}
}

// ReadPacketBytes reads exactly one bounded AGP packet.
func (f *TCPPacketFramer) ReadPacketBytes(reader io.Reader) ([]byte, error) {
	var prefix [4]byte
	if _, err := io.ReadFull(reader, prefix[:]); err != nil {
		return nil, fmt.Errorf("agp: read packet length: %w", err)
	}
	length := binary.BigEndian.Uint32(prefix[:])
	if length == 0 || length > f.MaxPacketSize {
		return nil, fmt.Errorf("agp: invalid packet length %d", length)
	}
	packet := make([]byte, length)
	if _, err := io.ReadFull(reader, packet); err != nil {
		return nil, fmt.Errorf("agp: read packet payload: %w", err)
	}
	return packet, nil
}

// WritePacketBytes writes one length-prefixed AGP packet.
func (f *TCPPacketFramer) WritePacketBytes(writer io.Writer, packet []byte) error {
	if len(packet) == 0 || uint64(len(packet)) > uint64(f.MaxPacketSize) {
		return fmt.Errorf("agp: invalid packet length %d", len(packet))
	}
	var prefix [4]byte
	binary.BigEndian.PutUint32(prefix[:], uint32(len(packet)))
	if err := writeFull(writer, prefix[:]); err != nil {
		return fmt.Errorf("agp: write packet length: %w", err)
	}
	if err := writeFull(writer, packet); err != nil {
		return fmt.Errorf("agp: write packet payload: %w", err)
	}
	return nil
}

func writeFull(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := writer.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}
