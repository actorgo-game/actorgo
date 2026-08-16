package cnats

import (
	"testing"

	cprofile "github.com/actorgo-game/actorgo/profile"
)

func TestNewPoolIsIdempotentWithoutConnect(t *testing.T) {
	// Isolate from other tests that may have touched the package-level pool.
	if IsReady() {
		t.Skip("nats pool already initialized in this process")
	}
	cfg := cprofile.Wrap(map[string]any{
		"address":   "nats://127.0.0.1:4222",
		"pool_size": 1,
	})
	NewPool("actorgo-test.reply", cfg, false)
	if !IsReady() {
		t.Fatal("expected pool ready after NewPool")
	}
	size := connectSize
	NewPool("actorgo-test.reply.other", cfg, false)
	if connectSize != size {
		t.Fatalf("second NewPool must be no-op, size %d -> %d", size, connectSize)
	}
}
