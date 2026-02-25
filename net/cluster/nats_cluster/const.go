package cnatscluster

import (
	"fmt"
)

const (
	localSubjectFormat      = "actorgo-%s.local.%s.%s"   // actorgo.{prefix}.local.{nodeType}.{nodeID}
	remoteSubjectFormat     = "actorgo-%s.remote.%s.%s"  // actorgo.{prefix}.remote.{nodeType}.{nodeID}
	remoteTypeSubjectFormat = "actorgo-%s.remoteType.%s" // actorgo.{prefix}.remoteType.{nodeType}
	replySubjectFormat      = "actorgo-%s.reply.%s.%s"   // actorgo.{prefix}.reply.{nodeType}.{nodeID}

)

// GetLocalSubject local message nats chan
func GetLocalSubject(prefix, nodeType, nodeID string) string {
	return fmt.Sprintf(localSubjectFormat, prefix, nodeType, nodeID)
}

// GetRemoteSubject remote message nats chan
func GetRemoteSubject(prefix, nodeType, nodeID string) string {
	return fmt.Sprintf(remoteSubjectFormat, prefix, nodeType, nodeID)
}

func GetRemoteTypeSubject(prefix, nodeType string) string {
	return fmt.Sprintf(remoteTypeSubjectFormat, prefix, nodeType)
}

func GetReplySubject(prefix, nodeType, nodeID string) string {
	return fmt.Sprintf(replySubjectFormat, prefix, nodeType, nodeID)
}
