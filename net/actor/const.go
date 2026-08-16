package cactor

import (
	cerror "github.com/actorgo-game/actorgo/error"
)

var (
	ErrForbiddenCreateChildActor = cerror.Errorf("Forbidden create child actor")
	ErrActorIDIsNil              = cerror.Error("actorID is nil.")
)

const (
	MailName = "mail"
)
