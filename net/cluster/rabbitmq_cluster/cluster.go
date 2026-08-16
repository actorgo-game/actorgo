package crabbitmqcluster

import (
	"context"
	"fmt"
	"maps"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	cerror "github.com/actorgo-game/actorgo/error"
	cfacade "github.com/actorgo-game/actorgo/facade"
	clog "github.com/actorgo-game/actorgo/logger"
	cproto "github.com/actorgo-game/actorgo/net/proto"
	cprofile "github.com/actorgo-game/actorgo/profile"
	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/protobuf/proto"
)

const corrIDHeader = "corrID"

// Cluster transports ClusterMessage values between ActorGo nodes over RabbitMQ.
type Cluster struct {
	app      cfacade.IApplication
	prefix   string
	exchange string
	url      string

	reconnectDelay time.Duration
	requestTimeout time.Duration

	conn *amqp.Connection
	ch   *amqp.Channel

	remoteKey string
	replyKey  string

	work chan amqp.Delivery
	stop chan struct{}

	submitMu sync.Mutex
	stopping bool
	workerWG sync.WaitGroup
	stopOnce sync.Once

	seq     uint64
	waiters sync.Map // map[string]chan amqp.Delivery

	mu sync.Mutex // guards conn/ch during reconnect
}

// New creates a RabbitMQ-backed Actor cluster transport.
func New(app cfacade.IApplication) cfacade.ICluster {
	return &Cluster{app: app}
}

// Init connects RabbitMQ, declares topology, and starts workers + consumers.
func (c *Cluster) Init() {
	config := cprofile.GetConfig("cluster").GetConfig("rabbitmq")
	if config.LastError() != nil {
		panic("cluster->rabbitmq config not found")
	}
	c.url = config.GetString("url")
	if c.url == "" {
		panic("cluster->rabbitmq url is required")
	}
	c.exchange = config.GetString("exchange", "actorgo.cluster")
	c.prefix = config.GetString("prefix", "node")
	c.reconnectDelay = config.GetDuration("reconnect_delay", 2) * time.Second
	c.requestTimeout = config.GetDuration("request_timeout", 5) * time.Second

	workerCount := config.GetInt("worker_count", 8)
	queueSize := config.GetInt("queue_size", 1024)
	if workerCount <= 0 || queueSize <= 0 {
		panic("cluster->rabbitmq worker_count and queue_size must be positive")
	}

	c.remoteKey = GetRemoteRoutingKey(c.prefix, c.app.NodeType(), c.app.NodeID())
	c.replyKey = GetReplyRoutingKey(c.prefix, c.app.NodeType(), c.app.NodeID())
	c.work = make(chan amqp.Delivery, queueSize)
	c.stop = make(chan struct{})

	for range workerCount {
		c.workerWG.Add(1)
		go c.worker()
	}

	if err := c.connectAndServe(); err != nil {
		panic(fmt.Sprintf("rabbitmq cluster init failed: %v", err))
	}
	go c.watchConnection()
	clog.Info("RabbitMQ ClusterMessage cluster initialized. [exchange = %s, remote = %s]", c.exchange, c.remoteKey)
}

// Stop rejects new work, drains accepted messages, and closes AMQP once.
func (c *Cluster) Stop() {
	c.stopOnce.Do(func() {
		c.submitMu.Lock()
		c.stopping = true
		c.submitMu.Unlock()
		if c.stop != nil {
			close(c.stop)
		}
		c.workerWG.Wait()
		c.mu.Lock()
		c.closeLocked()
		c.mu.Unlock()
	})
}

func (c *Cluster) connectAndServe() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connectAndServeLocked()
}

func (c *Cluster) connectAndServeLocked() error {
	c.closeLocked()

	conn, err := amqp.Dial(c.url)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("channel: %w", err)
	}

	if err := ch.ExchangeDeclare(c.exchange, "direct", false, false, false, false, nil); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return fmt.Errorf("exchange declare: %w", err)
	}
	if err := c.declareAndBind(ch, c.remoteKey); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return err
	}
	if err := c.declareAndBind(ch, c.replyKey); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return err
	}

	remoteDels, err := ch.Consume(c.remoteKey, "", true, false, false, false, nil)
	if err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return fmt.Errorf("consume remote: %w", err)
	}
	replyDels, err := ch.Consume(c.replyKey, "", true, false, false, false, nil)
	if err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return fmt.Errorf("consume reply: %w", err)
	}

	c.conn, c.ch = conn, ch
	go c.pumpRemote(remoteDels)
	go c.pumpReply(replyDels)
	return nil
}

func (c *Cluster) declareAndBind(ch *amqp.Channel, key string) error {
	if _, err := ch.QueueDeclare(key, false, true, false, false, nil); err != nil {
		return fmt.Errorf("queue declare %s: %w", key, err)
	}
	if err := ch.QueueBind(key, key, c.exchange, false, nil); err != nil {
		return fmt.Errorf("queue bind %s: %w", key, err)
	}
	return nil
}

func (c *Cluster) closeLocked() {
	if c.ch != nil {
		_ = c.ch.Close()
		c.ch = nil
	}
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
}

func (c *Cluster) watchConnection() {
	for {
		c.mu.Lock()
		conn := c.conn
		c.mu.Unlock()
		if conn == nil {
			select {
			case <-c.stop:
				return
			case <-time.After(c.reconnectDelay):
				continue
			}
		}
		notify := conn.NotifyClose(make(chan *amqp.Error, 1))
		select {
		case <-c.stop:
			return
		case err := <-notify:
			c.submitMu.Lock()
			stopping := c.stopping
			c.submitMu.Unlock()
			if stopping {
				return
			}
			clog.Warn("rabbitmq cluster connection closed. [err = %v], reconnecting...", err)
			for {
				select {
				case <-c.stop:
					return
				default:
				}
				if err := c.connectAndServe(); err != nil {
					clog.Warn("rabbitmq cluster reconnect failed. [err = %v]", err)
					time.Sleep(c.reconnectDelay)
					continue
				}
				clog.Info("rabbitmq cluster reconnected")
				break
			}
		}
	}
}

func (c *Cluster) pumpRemote(dels <-chan amqp.Delivery) {
	for {
		select {
		case <-c.stop:
			return
		case d, ok := <-dels:
			if !ok {
				return
			}
			c.enqueue(d)
		}
	}
}

func (c *Cluster) pumpReply(dels <-chan amqp.Delivery) {
	for {
		select {
		case <-c.stop:
			return
		case d, ok := <-dels:
			if !ok {
				return
			}
			corrID := d.CorrelationId
			if corrID == "" {
				continue
			}
			if chMsg, ok := c.waiters.LoadAndDelete(corrID); ok {
				ch := chMsg.(chan amqp.Delivery)
				select {
				case ch <- d:
				default:
				}
			}
		}
	}
}

func (c *Cluster) enqueue(message amqp.Delivery) {
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

func (c *Cluster) worker() {
	defer c.workerWG.Done()
	for {
		select {
		case message := <-c.work:
			c.process(message)
		case <-c.stop:
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

func (c *Cluster) rejectOverload(message amqp.Delivery) {
	clog.Warn("cluster work queue is full. [routing = %s]", message.RoutingKey)
	if message.ReplyTo == "" {
		return
	}
	clusterMessage := new(cproto.ClusterMessage)
	if err := proto.Unmarshal(message.Body, clusterMessage); err != nil || clusterMessage.MsgType != cproto.MsgType_REQUEST {
		return
	}
	c.reply(message, clusterMessage, nil, cfacade.ErrorResult(cproto.StatusCode_STATUS_RESOURCE_EXHAUSTED, "cluster work queue is full"))
}

func (c *Cluster) process(message amqp.Delivery) {
	clusterMessage := new(cproto.ClusterMessage)
	if err := proto.Unmarshal(message.Body, clusterMessage); err != nil {
		clog.Warn("cluster message decode failed. [routing = %s, err = %v]", message.RoutingKey, err)
		return
	}
	if err := validateMessage(clusterMessage); err != nil {
		clog.Warn("cluster message rejected. [routing = %s, err = %v]", message.RoutingKey, err)
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

func (c *Cluster) reply(message amqp.Delivery, request *cproto.ClusterMessage, ctx *cfacade.RequestContext, result *cfacade.InvokeResult) {
	if request.MsgType == cproto.MsgType_NOTIFY || message.ReplyTo == "" {
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
	if err := c.publish(message.ReplyTo, data, "", message.CorrelationId); err != nil {
		clog.Warn("cluster response publish failed. [err = %v]", err)
	}
}

// Publish sends a one-way Actor cluster message to a discovered node.
func (c *Cluster) Publish(nodeID string, message *cproto.ClusterMessage) error {
	routingKey, err := c.nodeRoutingKey(nodeID)
	if err != nil {
		return err
	}
	if err := validateMessage(message); err != nil {
		return err
	}
	data, err := proto.Marshal(message)
	if err != nil {
		return fmt.Errorf("actorgo cluster: marshal message: %w", err)
	}
	if err := c.publish(routingKey, data, "", ""); err != nil {
		return fmt.Errorf("actorgo cluster: publish: %w", err)
	}
	return nil
}

// Request sends an Actor request and validates response correlation.
func (c *Cluster) Request(nodeID string, message *cproto.ClusterMessage, timeout time.Duration) (*cproto.ClusterMessage, error) {
	routingKey, err := c.nodeRoutingKey(nodeID)
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
	if timeout <= 0 {
		timeout = c.requestTimeout
	}

	corrID := strconv.FormatUint(atomic.AddUint64(&c.seq, 1), 10)
	ch := make(chan amqp.Delivery, 1)
	c.waiters.Store(corrID, ch)
	defer c.waiters.Delete(corrID)

	if err := c.publish(routingKey, data, c.replyKey, corrID); err != nil {
		return nil, fmt.Errorf("actorgo cluster: request: %w", err)
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case resp := <-ch:
		response := new(cproto.ClusterMessage)
		if err := proto.Unmarshal(resp.Body, response); err != nil {
			return nil, fmt.Errorf("actorgo cluster: unmarshal response: %w", err)
		}
		if response.MsgType != cproto.MsgType_RESPONSE || response.MessageId != message.MessageId {
			return nil, fmt.Errorf("actorgo cluster: response correlation mismatch")
		}
		return response, nil
	case <-timer.C:
		clog.Warn("rabbitmq cluster request timeout. [corrID = %s]", corrID)
		return nil, cerror.ClusterRequestTimeout
	}
}

func (c *Cluster) publish(routingKey string, data []byte, replyTo, corrID string) error {
	c.mu.Lock()
	ch := c.ch
	c.mu.Unlock()
	if ch == nil {
		return fmt.Errorf("rabbitmq channel is not ready")
	}
	pub := amqp.Publishing{
		ContentType:   "application/x-protobuf",
		DeliveryMode:  amqp.Transient,
		Body:          data,
		ReplyTo:       replyTo,
		CorrelationId: corrID,
		Headers:       amqp.Table{corrIDHeader: corrID},
	}
	return ch.PublishWithContext(context.Background(), c.exchange, routingKey, false, false, pub)
}

func (c *Cluster) nodeRoutingKey(nodeID string) (string, error) {
	if nodeID == "" {
		return "", fmt.Errorf("actorgo cluster: node id is required")
	}
	nodeType, err := c.app.Discovery().GetType(nodeID)
	if err != nil {
		return "", fmt.Errorf("actorgo cluster: node %q not found: %w", nodeID, err)
	}
	return GetRemoteRoutingKey(c.prefix, nodeType, nodeID), nil
}

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

func messageContext(message *cproto.ClusterMessage) (*cfacade.RequestContext, context.CancelFunc) {
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
