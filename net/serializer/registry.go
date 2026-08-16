package cserializer

import (
	"fmt"
	"sync"

	cfacade "github.com/actorgo-game/actorgo/facade"
)

// Registry is the application-scoped allow-list of protocol body codecs.
type Registry struct {
	mu        sync.RWMutex
	codecs    map[int32]cfacade.IBodyCodec
	defaultID int32
}

// NewRegistry creates a registry and fails fast on conflicting built-in codecs.
// When protobuf is registered it becomes the default codec.
func NewRegistry(codecs ...cfacade.IBodyCodec) *Registry {
	r := &Registry{codecs: make(map[int32]cfacade.IBodyCodec, len(codecs))}
	for _, codec := range codecs {
		if err := r.Register(codec); err != nil {
			panic(err)
		}
	}
	if _, ok := r.codecs[cfacade.CodecProtobuf]; ok {
		r.defaultID = cfacade.CodecProtobuf
	}
	return r
}

// Register adds or replaces a codec with the same ID and name. Reusing an ID
// for a different encoding is rejected because codec IDs are wire values.
func (r *Registry) Register(codec cfacade.IBodyCodec) error {
	if codec == nil || codec.ID() == 0 {
		return fmt.Errorf("actorgo: invalid body codec")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if current, exists := r.codecs[codec.ID()]; exists && current.Name() != codec.Name() {
		return fmt.Errorf("actorgo: body codec id %d already registered as %q", codec.ID(), current.Name())
	}
	r.codecs[codec.ID()] = codec
	if r.defaultID == 0 {
		r.defaultID = codec.ID()
	}
	return nil
}

// Lookup returns the codec registered for id.
func (r *Registry) Lookup(id int32) (cfacade.IBodyCodec, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	codec, ok := r.codecs[id]
	return codec, ok
}

// Default returns the codec id used when a call does not specify one.
func (r *Registry) Default() int32 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.defaultID
}

// SetDefault selects the default codec. The id must already be registered.
func (r *Registry) SetDefault(id int32) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.codecs[id]; !ok {
		return fmt.Errorf("actorgo: body codec %d is not registered", id)
	}
	r.defaultID = id
	return nil
}

// Marshal encodes a concrete request, response or notification body.
func (r *Registry) Marshal(id int32, value any) ([]byte, error) {
	codec, ok := r.Lookup(id)
	if !ok {
		return nil, fmt.Errorf("actorgo: unsupported body codec %d", id)
	}
	return codec.Marshal(value)
}

// Unmarshal decodes a body using the codec selected by the packet.
func (r *Registry) Unmarshal(id int32, data []byte, value any) error {
	codec, ok := r.Lookup(id)
	if !ok {
		return fmt.Errorf("actorgo: unsupported body codec %d", id)
	}
	return codec.Unmarshal(data, value)
}
