package parser

import (
	"bytes"
	"testing"
)

type shortWriter struct {
	bytes.Buffer
	limit int
}

func (w *shortWriter) Write(data []byte) (int, error) {
	if len(data) > w.limit {
		data = data[:w.limit]
	}
	return w.Buffer.Write(data)
}

func TestTCPPacketFramerRoundTrip(t *testing.T) {
	framer := NewTCPPacketFramer(1024)
	writer := &shortWriter{limit: 2}
	payload := []byte("packet-body")
	if err := framer.WritePacketBytes(writer, payload); err != nil {
		t.Fatal(err)
	}
	got, err := framer.ReadPacketBytes(bytes.NewReader(writer.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("got %q, want %q", got, payload)
	}
}

func TestTCPPacketFramerRejectsOversizedPacket(t *testing.T) {
	framer := NewTCPPacketFramer(3)
	if err := framer.WritePacketBytes(&bytes.Buffer{}, []byte("four")); err == nil {
		t.Fatal("expected size error")
	}
}
