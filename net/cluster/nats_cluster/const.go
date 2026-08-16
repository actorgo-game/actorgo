package cnatscluster

import (
	"fmt"
)

const (
	remoteSubjectFormat = "actorgo-%s.remote.%s.%s" // actorgo.{prefix}.remote.{nodeType}.{nodeID}
	replySubjectFormat  = "actorgo-%s.reply.%s.%s"  // actorgo.{prefix}.reply.{nodeType}.{nodeID}
)

// GetRemoteSubject remote message nats chan
func GetRemoteSubject(prefix, nodeType, nodeID string) string {
	return fmt.Sprintf(remoteSubjectFormat, prefix, nodeType, nodeID)
}

// GetReplySubject returns the node-scoped inbox subject used by cluster requests.
func GetReplySubject(prefix, nodeType, nodeID string) string {
	return fmt.Sprintf(replySubjectFormat, prefix, nodeType, nodeID)
}
