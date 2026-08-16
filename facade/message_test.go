package cfacade

import "testing"

func TestActorPathWithGeneratedNodeID(t *testing.T) {
	parent, err := ToActorPath("1.10001.5.1.player")
	if err != nil || parent.NodeID != "1.10001.5.1" || parent.ActorID != "player" || parent.IsChild() {
		t.Fatalf("unexpected parent path %#v, err=%v", parent, err)
	}
	child, err := ToActorPath("1.10001.5.1.player.42")
	if err != nil || child.NodeID != "1.10001.5.1" || child.ActorID != "player" || child.ChildID != "42" {
		t.Fatalf("unexpected child path %#v, err=%v", child, err)
	}
}
