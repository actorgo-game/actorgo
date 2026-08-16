package crabbitmqcluster

import "testing"

func TestRoutingKeysAlignWithNATSStyle(t *testing.T) {
	remote := GetRemoteRoutingKey("node", "gate", "gate-1")
	reply := GetReplyRoutingKey("node", "gate", "gate-1")
	if remote != "actorgo-node.remote.gate.gate-1" {
		t.Fatalf("remote = %s", remote)
	}
	if reply != "actorgo-node.reply.gate.gate-1" {
		t.Fatalf("reply = %s", reply)
	}
}
