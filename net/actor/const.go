package cactor

import (
	cerror "github.com/actorgo-game/actorgo/error"
)

var (
	ErrForbiddenToCallSelf       = cerror.Errorf("SendActorID cannot be equal to TargetActorID")
	ErrForbiddenCreateChildActor = cerror.Errorf("Forbidden create child actor")
	ErrActorIDIsNil              = cerror.Error("actorID is nil.")
)

const (
	LocalName  = "local"
	RemoteName = "remote"
)
