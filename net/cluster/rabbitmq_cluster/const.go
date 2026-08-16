package crabbitmqcluster

import "fmt"

const (
	remoteRoutingFormat = "actorgo-%s.remote.%s.%s" // actorgo-{prefix}.remote.{nodeType}.{nodeID}
	replyRoutingFormat  = "actorgo-%s.reply.%s.%s"  // actorgo-{prefix}.reply.{nodeType}.{nodeID}
)

// GetRemoteRoutingKey returns the per-node remote routing key (aligned with NATS subject).
func GetRemoteRoutingKey(prefix, nodeType, nodeID string) string {
	return fmt.Sprintf(remoteRoutingFormat, prefix, nodeType, nodeID)
}

// GetReplyRoutingKey returns the per-node reply routing key (aligned with NATS subject).
func GetReplyRoutingKey(prefix, nodeType, nodeID string) string {
	return fmt.Sprintf(replyRoutingFormat, prefix, nodeType, nodeID)
}
