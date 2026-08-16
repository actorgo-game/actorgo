# Body Codec Simplify Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Collapse Application serializer/codecs/defaultBodyCodec into a single BodyCodecs registry with Default.

**Architecture:** `IBodyCodecRegistry` owns registered codecs plus default id. Application only holds `codecs`. Discovery and zero-codec paths use `BodyCodecs().Default()`.

**Tech Stack:** Go, existing `net/serializer` registry.

## Global Constraints

- Keep Protobuf + JSON registered simultaneously
- Wire codec ids unchanged (`CodecProtobuf=1`, `CodecJSON=2`)
- Default remains Protobuf unless `SetDefaultBodyCodec` is called before Startup

---

### Task 1: Registry Default API

**Files:**
- Modify: `facade/codec.go`
- Modify: `net/serializer/registry.go`
- Modify: `net/serializer/registry_test.go`

- [ ] Add `Default() int32` and `SetDefault(id int32) error` to interface and Registry (default starts as CodecProtobuf when that codec is registered, else first registered / 0)
- [ ] Test SetDefault success for JSON and reject unknown id
- [ ] Run: `go test ./net/serializer -count=1`

### Task 2: Application API

**Files:**
- Modify: `facade/application.go`
- Modify: `application.go`

- [ ] Remove `serializer`, `defaultBodyCodec`, `Serializer()`, `DefaultBodyCodec()`, `SetSerializer`
- [ ] Keep `BodyCodecs()`; add `SetDefaultBodyCodec(id int32)`
- [ ] Fix startup log that referenced serializer name

### Task 3: Call-site migration

**Files:**
- Modify: `net/discovery/discovery_nats.go`
- Modify: `net/actor/system.go`, `net/actor/actor_mailbox.go`, `net/method/table.go`, `net/parser/connection.go`
- Modify: `D:/game/actorgo/actorgo-examples/demo_chat/room/main.go`
- Modify: `_docs/agp-protobuf-protocol-design.md` (SetSerializer mention)

- [ ] Replace `app.DefaultBodyCodec()` → `app.BodyCodecs().Default()`
- [ ] Replace Discovery `Serializer()` → Marshal/Unmarshal with Default id
- [ ] demo_chat: `SetDefaultBodyCodec(cfacade.CodecJSON)`

### Task 4: Verify

- [ ] `go test ./net/serializer ./net/actor ./net/method ./net/parser ./net/httpactor ./net/proto ./net/discovery -count=1`
- [ ] `go build` demo_chat / demo_cluster as needed
