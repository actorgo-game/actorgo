package cnatscluster

import (
	"context"
	"fmt"
	"maps"
	"sync"
	"time"

	cfacade "github.com/actorgo-game/actorgo/facade"
	clog "github.com/actorgo-game/actorgo/logger"
	cnats "github.com/actorgo-game/actorgo/net/nats"
	cproto "github.com/actorgo-game/actorgo/net/proto"
	cprofile "github.com/actorgo-game/actorgo/profile"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
)

// Cluster transports ClusterMessage values between ActorGo nodes over NATS.
type Cluster struct {
	app          cfacade.IApplication
	prefix       string
	work         chan *nats.Msg
	stop         chan struct{}
	subscription *nats.Subscription
	submitMu     sync.Mutex
	stopping     bool
	workerWG     sync.WaitGroup
	stopOnce     sync.Once
}

// New creates a NATS-backed Actor cluster transport.
func New(app cfacade.IApplication) cfacade.ICluster { return &Cluster{app: app} }

// Init connects NATS, starts the bounded worker pool, and subscribes this node.
func (c *Cluster) Init() {
	config := cprofile.GetConfig("cluster").GetConfig("nats")
	if config.LastError() != nil {
		panic("cluster->nats config not found")
	}
	c.prefix = config.GetString("prefix", "node")
	cnats.NewPool(GetReplySubject(c.prefix, c.app.NodeType(), c.app.NodeID()), config, true)
	workerCount := config.GetInt("worker_count", 8)
	queueSize := config.GetInt("queue_size", 1024)
	if workerCount <= 0 || queueSize <= 0 {
		panic("cluster->nats worker_count and queue_size must be positive")
	}
	c.work, c.stop = make(chan *nats.Msg, queueSize), make(chan struct{})
	for range workerCount {
		c.workerWG.Add(1)
		go c.worker()
	}
	c.subscribe(GetRemoteSubject(c.prefix, c.app.NodeType(), c.app.NodeID()))
	clog.Info("NATS ClusterMessage cluster initialized")
}

// Stop rejects new work, drains already accepted messages, and closes NATS once.
func (c *Cluster) Stop() {
	c.stopOnce.Do(func() {
		c.submitMu.Lock()
		c.stopping = true
		c.submitMu.Unlock()
		if c.subscription != nil {
			_ = c.subscription.Unsubscribe()
		}
		if c.stop != nil {
			close(c.stop)
		}
		c.workerWG.Wait()
		cnats.ConnectClose()
	})
}

func (c *Cluster) subscribe(subject string) {
	subscription, err := cnats.GetConnect().SubscribeHandle(subject, c.enqueue)
	if err != nil {
		clog.Error("cluster subscribe failed. [subject = %s, err = %v]", subject, err)
		return
	}
	c.subscription = subscription
}

// enqueue keeps the NATS subscription callback non-blocking and applies queue backpressure.
func (c *Cluster) enqueue(message *nats.Msg) {
	c.submitMu.Lock()
	defer c.submitMu.Unlock()
	if c.stopping {
		return
	}
	select {
	case c.work <- message:
	default:
		c.rejectOverload(message)
	}
}

// worker serially handles messages assigned to it and drains accepted work on stop.
func (c *Cluster) worker() {
	defer c.workerWG.Done()
	for {
		select {
		case message := <-c.work:
			c.process(message)
		case <-c.stop:
			// Finish work accepted before Stop; no new messages can enter after
			// stopping is set while holding submitMu.
			for {
				select {
				case message := <-c.work:
					c.process(message)
				default:
					return
				}
			}
		}
	}
}

// rejectOverload returns a protocol error for requests while dropping notifications.
func (c *Cluster) rejectOverload(message *nats.Msg) {
	clog.Warn("cluster work queue is full. [subject = %s]", message.Subject)
	if message.Reply == "" {
		return
	}
	clusterMessage := new(cproto.ClusterMessage)
	if err := proto.Unmarshal(message.Data, clusterMessage); err != nil || clusterMessage.MsgType != cproto.MsgType_REQUEST {
		return
	}
	c.reply(message, clusterMessage, nil, cfacade.ErrorResult(cproto.StatusCode_STATUS_RESOURCE_EXHAUSTED, "cluster work queue is full"))
}

func (c *Cluster) process(message *nats.Msg) {
	clusterMessage := new(cproto.ClusterMessage)
	if err := proto.Unmarshal(message.Data, clusterMessage); err != nil {
		clog.Warn("cluster message decode failed. [subject = %s, err = %v]", message.Subject, err)
		return
	}
	if err := validateMessage(clusterMessage); err != nil {
		clog.Warn("cluster message rejected. [subject = %s, err = %v]", message.Subject, err)
		return
	}
	if clusterMessage.MsgType == cproto.MsgType_RESPONSE {
		return
	}

	ctx, cancel := messageContext(clusterMessage)
	defer cancel()
	var result *cfacade.InvokeResult
	if clusterMessage.TargetPath != "" {
		if clusterMessage.MsgType == cproto.MsgType_NOTIFY {
			result = c.app.ActorSystem().NotifyTarget(ctx, clusterMessage.TargetPath, clusterMessage.MethodId, clusterMessage.Payload)
		} else {
			result = c.app.ActorSystem().InvokeTarget(ctx, clusterMessage.TargetPath, clusterMessage.MethodId, clusterMessage.Payload)
		}
	} else {
		methods := c.app.Methods()
		if methods == nil {
			result = cfacade.ErrorResult(cproto.StatusCode_STATUS_UNAVAILABLE, "method table is not configured")
		} else {
			result = methods.Dispatch(ctx, clusterMessage.MethodId, clusterMessage.Payload, clusterMessage.MsgType)
		}
	}
	if result == nil {
		result = cfacade.ErrorResult(cproto.StatusCode_STATUS_INTERNAL, "dispatcher returned nil result")
	}
	c.reply(message, clusterMessage, ctx, result)
}

func (c *Cluster) reply(message *nats.Msg, request *cproto.ClusterMessage, ctx *cfacade.RequestContext, result *cfacade.InvokeResult) {
	// NATS supplies the reply subject out of band; notifications intentionally
	// never produce a ClusterMessage response.
	if request.MsgType == cproto.MsgType_NOTIFY || message.Reply == "" {
		return
	}

	var payload []byte
	if result.OK() && result.Payload != nil && ctx != nil && c.app != nil && c.app.BodyCodecs() != nil {
		var err error
		payload, err = c.app.BodyCodecs().Marshal(ctx.Codec, result.Payload)
		if err != nil {
			result = cfacade.ErrorResult(cproto.StatusCode_STATUS_INTERNAL, "response body encode failed")
			payload = nil
		}
	}
	response := &cproto.ClusterMessage{
		MessageId: request.MessageId, MsgType: cproto.MsgType_RESPONSE,
		RequestId: request.RequestId, MethodId: request.MethodId,
		Codec: request.Codec, Payload: payload, Code: result.Code, Message: result.Message,
	}
	data, err := proto.Marshal(response)
	if err != nil {
		clog.Warn("cluster response encode failed. [err = %v]", err)
		return
	}
	reply := cnats.GetMsg()
	reply.Subject, reply.Header, reply.Data = message.Reply, message.Header, data
	if err := cnats.GetConnect().PublishMsg(reply); err != nil {
		clog.Warn("cluster response publish failed. [err = %v]", err)
	}
	cnats.ReleaseMsg(reply)
}

// Publish sends a one-way Actor cluster message to a discovered node.
func (c *Cluster) Publish(nodeID string, message *cproto.ClusterMessage) error {
	subject, err := c.nodeSubject(nodeID)
	if err != nil {
		return err
	}
	return publishMessage(subject, message)
}

// Request sends an Actor request and validates response correlation.
func (c *Cluster) Request(nodeID string, message *cproto.ClusterMessage, timeout time.Duration) (*cproto.ClusterMessage, error) {
	subject, err := c.nodeSubject(nodeID)
	if err != nil {
		return nil, err
	}
	if err := validateMessage(message); err != nil {
		return nil, err
	}
	data, err := proto.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("actorgo cluster: marshal message: %w", err)
	}
	responseData, err := cnats.GetConnect().RequestSync(subject, data, timeout)
	if err != nil {
		return nil, fmt.Errorf("actorgo cluster: request: %w", err)
	}
	response := new(cproto.ClusterMessage)
	if err := proto.Unmarshal(responseData, response); err != nil {
		return nil, fmt.Errorf("actorgo cluster: unmarshal response: %w", err)
	}
	if response.MsgType != cproto.MsgType_RESPONSE || response.MessageId != message.MessageId {
		return nil, fmt.Errorf("actorgo cluster: response correlation mismatch")
	}
	return response, nil
}

func (c *Cluster) nodeSubject(nodeID string) (string, error) {
	if nodeID == "" {
		return "", fmt.Errorf("actorgo cluster: node id is required")
	}
	nodeType, err := c.app.Discovery().GetType(nodeID)
	if err != nil {
		return "", fmt.Errorf("actorgo cluster: node %q not found: %w", nodeID, err)
	}
	return GetRemoteSubject(c.prefix, nodeType, nodeID), nil
}

func publishMessage(subject string, message *cproto.ClusterMessage) error {
	if err := validateMessage(message); err != nil {
		return err
	}
	data, err := proto.Marshal(message)
	if err != nil {
		return fmt.Errorf("actorgo cluster: marshal message: %w", err)
	}
	if err := cnats.GetConnect().Publish(subject, data); err != nil {
		return fmt.Errorf("actorgo cluster: publish: %w", err)
	}
	return nil
}

// validateMessage enforces the outbound request/notify contract before NATS I/O.
func validateMessage(message *cproto.ClusterMessage) error {
	if message == nil {
		return fmt.Errorf("actorgo cluster: message is nil")
	}
	if message.MessageId == 0 || message.MethodId == 0 {
		return fmt.Errorf("actorgo cluster: message id and method id are required")
	}
	if message.TargetPath != "" {
		if _, err := cfacade.ToActorPath(message.TargetPath); err != nil {
			return fmt.Errorf("actorgo cluster: invalid actor target")
		}
	}
	if message.MsgType != cproto.MsgType_REQUEST && message.MsgType != cproto.MsgType_NOTIFY {
		return fmt.Errorf("actorgo cluster: invalid outbound message type %s", message.MsgType)
	}
	if message.Codec != cfacade.CodecJSON && message.Codec != cfacade.CodecProtobuf {
		return fmt.Errorf("actorgo cluster: unsupported codec %d", message.Codec)
	}
	return nil
}

// messageContext reconstructs request-scoped state received from another node.
func messageContext(message *cproto.ClusterMessage) (*cfacade.RequestContext, context.CancelFunc) {
	// Rebuild cancellation from the absolute wire deadline so every cluster hop
	// consumes the same end-to-end timeout budget.
	base := context.Background()
	cancel := func() {}
	if message.DeadlineUnixMs > 0 {
		base, cancel = context.WithDeadline(base, time.UnixMilli(message.DeadlineUnixMs))
	}
	ctx := cfacade.NewRequestContext(base)
	ctx.RequestID, ctx.Transport = message.RequestId, cfacade.TransportCluster
	ctx.Codec = message.Codec
	ctx.Metadata = message.Metadata
	if session := message.Session; session != nil {
		ctx.Session = &cproto.Session{Sid: session.Sid, Uid: session.Uid, Ip: session.Ip, Data: maps.Clone(session.Data)}
	}
	return ctx, cancel
}
