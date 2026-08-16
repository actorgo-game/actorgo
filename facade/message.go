package cfacade

import (
	"context"
	"strings"
	"sync"

	cconst "github.com/actorgo-game/actorgo/const"
	cerr "github.com/actorgo-game/actorgo/error"
	cstring "github.com/actorgo-game/actorgo/extend/string"
)

// Message is the pooled internal delivery unit shared by local, HTTP/AGP, and
// cluster entry points. Its final consumer must call Recycle exactly once.
type Message struct {
	MethodID         uint32             // 请求调用的方法id
	Target           string             // 目标actor path
	targetPath       *ActorPath         // 目标actor path对象
	Context          *RequestContext    // 请求上下文
	Payload          any                // 请求的参数
	ChanInvokeResult chan *InvokeResult // 请求结果通道
	Cancel           context.CancelFunc // 释放 Actor 持有的通知上下文
}

// ActorPath identifies a top-level Actor or one of its dynamic children.
// ActorPath = NodeID . ActorID
// ActorPath = NodeID . ActorID . ChildID
// A generated NodeID itself contains four dot-separated numeric segments.
type ActorPath struct {
	NodeID  string
	ActorID string
	ChildID string
}

var messagePool = sync.Pool{New: func() any { return &Message{} }}

// GetMessage acquires a cleared delivery message from the shared pool.
func GetMessage() *Message { return messagePool.Get().(*Message) }

// Recycle releases the message-owned context and clears references before reuse.
func (m *Message) Recycle() {
	if m.Cancel != nil {
		m.Cancel()
	}
	m.MethodID = 0
	m.Target = ""
	m.targetPath, m.Context = nil, nil
	m.Payload, m.ChanInvokeResult, m.Cancel = nil, nil, nil
	messagePool.Put(m)
}

// TargetPath parses Target once and caches the result for Actor routing.
func (m *Message) TargetPath() *ActorPath {
	if m.targetPath == nil {
		m.targetPath, _ = ToActorPath(m.Target)
	}
	return m.targetPath
}
func (p *ActorPath) IsChild() bool  { return p != nil && p.ChildID != "" }
func (p *ActorPath) IsParent() bool { return p != nil && p.ChildID == "" }

// String formats the path accepted by ToActorPath.
// String
func (p *ActorPath) String() string { return NewChildPath(p.NodeID, p.ActorID, p.ChildID) }

// NewActorPath constructs a parsed Actor path without validating its parts.
func NewActorPath(nodeID, actorID, childID string) *ActorPath {
	return &ActorPath{NodeID: nodeID, ActorID: actorID, ChildID: childID}
}

// NewChildPath formats either a parent path or a child path when childID is set.
func NewChildPath(nodeID, actorID, childID any) string {
	if childID == "" {
		return NewPath(nodeID, actorID)
	}
	return cstring.ToString(nodeID) + cconst.DOT + cstring.ToString(actorID) + cconst.DOT + cstring.ToString(childID)
}

// NewPath formats a top-level Actor path.
func NewPath(nodeID, actorID any) string {
	return cstring.ToString(nodeID) + cconst.DOT + cstring.ToString(actorID)
}

// ToActorPath accepts both short node IDs and generated four-segment node IDs.
func ToActorPath(path string) (*ActorPath, error) {
	if path == "" {
		return nil, cerr.ActorPathError
	}
	parts := strings.Split(path, cconst.DOT)
	if len(parts) == 2 {
		return NewActorPath(parts[0], parts[1], ""), nil
	}
	if len(parts) == 3 {
		return NewActorPath(parts[0], parts[1], parts[2]), nil
	}
	if len(parts) == 5 {
		return NewActorPath(strings.Join(parts[:4], cconst.DOT), parts[4], ""), nil
	}
	if len(parts) == 6 {
		return NewActorPath(strings.Join(parts[:4], cconst.DOT), parts[4], parts[5]), nil
	}
	return nil, cerr.ActorPathError
}
