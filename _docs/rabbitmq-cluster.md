# RabbitMQ 集群传输

ActorGo 默认用 NATS 做跨节点 `ICluster` 传输。需要统一使用 RabbitMQ 时，可设 `cluster.mode=rabbitmq`，业务侧仍走 `Invoke` / `Notify`，无需改 Actor API。

Discovery（`cluster.discovery.mode`：nats / etcd）与 Cluster 传输独立配置。当 `discovery.mode=nats` 且集群传输不是 NATS 时，Discovery 会自行用 `cluster.nats` 建立共享连接池（不再依赖 `nats_cluster.Init`）。

## 配置

```json
"cluster": {
  "mode": "rabbitmq",
  "discovery": { "mode": "nats" },
  "nats": { },
  "rabbitmq": {
    "url": "amqp://guest:guest@127.0.0.1:5672/",
    "exchange": "actorgo.cluster",
    "prefix": "node",
    "worker_count": 8,
    "queue_size": 1024,
    "request_timeout": 5,
    "reconnect_delay": 2
  }
}
```

- `mode` 缺省或 `nats`：沿用现有 NATS 实现
- `request_timeout` / `reconnect_delay`：数字，单位为秒（与 NATS 配置习惯一致）

## 拓扑与语义

- Exchange：`direct`，非持久
- 每节点 remote / reply 队列（非持久、auto-delete），routing key 对齐 NATS subject：
  - `actorgo-{prefix}.remote.{nodeType}.{nodeID}`
  - `actorgo-{prefix}.reply.{nodeType}.{nodeID}`
- **Publish（Notify）**：发到目标 remote key，无 ReplyTo
- **Request（Invoke）**：`ReplyTo` + `CorrelationId`，本地 pending + 超时；响应校验 `MsgType=RESPONSE` 且 `MessageId` 一致
- **背压**：有界 worker 队列；满时 REQUEST 回 `STATUS_RESOURCE_EXHAUSTED`，NOTIFY 丢弃

第一版为**尽力投递**（无 durable / ACK / publisher confirm）。

## 代码位置

| 职责 | 路径 |
|------|------|
| 传输工厂 | `net/cluster/component.go`（`cluster.mode`） |
| NATS 实现 | `net/cluster/nats_cluster/` |
| RabbitMQ 实现 | `net/cluster/rabbitmq_cluster/` |
| 消息载体 | `net/proto/cluster_message.proto` |

## 非目标（当前版本）

- 消息持久化与消费 ACK
- RabbitMQ 版 Discovery
- 按节点类型广播
- 与 NATS 实现抽取公共 runtime

与 NATS 的对比见 [api-changes-2026-07.md](api-changes-2026-07.md) 第 10 节。
