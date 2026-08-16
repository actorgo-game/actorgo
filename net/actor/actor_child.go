package cactor

import (
	cfacade "github.com/actorgo-game/actorgo/facade"
	"strings"
	"sync"
)

// actorChild owns the runtime children of one top-level Actor.
type actorChild struct {
	thisActor   *Actor
	childActors *sync.Map // key:childActorID, value:*actor
}

func newChild(thisActor *Actor) actorChild {
	return actorChild{
		thisActor:   thisActor,
		childActors: &sync.Map{},
	}
}

func (p *actorChild) onStop() {
	p.childActors.Range(func(key, value any) bool {
		if childActor, ok := value.(*Actor); ok {
			childActor.Exit()
		}
		return true
	})

	p.thisActor = nil
}

// Create starts a dynamic child and waits until its OnInit completes.
func (p *actorChild) Create(childID string, handler cfacade.IActorHandler) (cfacade.IActor, error) {
	if p.thisActor.path.IsChild() {
		return nil, ErrForbiddenCreateChildActor
	}

	if strings.TrimSpace(childID) == "" {
		return nil, ErrActorIDIsNil
	}

	if thisActor, ok := p.Get(childID); ok {
		return thisActor, nil
	}

	childActor, err := newActor(p.thisActor.ActorID(), childID, handler, p.thisActor.system)
	if err != nil {
		return nil, err
	}

	p.childActors.Store(childID, childActor)
	p.thisActor.system.wg.Add(1)
	go childActor.run()
	if err := <-childActor.initDone; err != nil {
		return nil, err
	}

	return childActor, nil
}

// Get returns a child by its ID.
func (p *actorChild) Get(childID string) (cfacade.IActor, bool) {
	return p.GetActor(childID)
}

func (p *actorChild) GetActor(childID string) (*Actor, bool) {
	if actorValue, ok := p.childActors.Load(childID); ok {
		actor, found := actorValue.(*Actor)
		return actor, found
	}

	return nil, false
}

// Remove forgets a child after its shutdown path has completed.
func (p *actorChild) Remove(childID string) {
	p.childActors.Delete(childID)
}

// Each visits the currently registered children.
func (p *actorChild) Each(fn func(cfacade.IActor)) {
	p.childActors.Range(func(key, value any) bool {
		if actor, found := value.(*Actor); found {
			fn(actor)
		}
		return true
	})
}
