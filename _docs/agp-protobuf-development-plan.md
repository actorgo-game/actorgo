# ActorGo AGP/1 开发计划与状态

> 目标：直接使用最终协议，不保留 Pomelo/Simple 运行时，不接入 gRPC；业务 Body 支持 JSON/PB，并支持 curl 调用 Actor。

## 阶段状态

| 阶段 | 内容 | 状态 |
|---|---|---|
| 1 | AGP Packet、Code、JSON/PB Codec | 完成 |
| 2 | Typed Actor、Method Table | 完成 |
| 3 | AGP TCP/WebSocket 与 Connection | 完成 |
| 4 | NATS ClusterMessage | 完成 |
| 5 | HTTP JSON/PB、curl、Gin 挂载 | 完成 |
| 6 | 删除 Pomelo/Simple 核心依赖 | 完成 |
| 7 | 生产环境 Race、NATS E2E、压测、SDK Golden | 待执行 |

## 1. 协议层

已实现：

- `Packet oneof { Request, Response, Notify }`；
- TCP 4 字节长度前缀；
- WebSocket `agp.v1` + Binary Message；
- Handshake、Heartbeat、Cancel、Kick、GoAway 使用保留 Method ID；
- Packet Envelope 固定 PB；
- 业务 Body 支持 PB/JSON；
- Packet 大小、Body、Metadata 和 Codec 校验。

## 2. 调度层

已实现：

- 稳定 Method ID；
- Actor `Methods().Register(MethodID, handler)` 安装 mailbox（不返回 error；失败 panic）；只有顶层 Actor 写入外部方法表，子 Actor 方法保持本地；
- PB Schema 支持 PB/ProtoJSON，普通 struct 只支持 JSON；
- RequestContext（含 Transport / Codec / Session）、code、message；
- 本地/跨节点 Actor 使用相同 Invoke/Notify 接口；
- 单一 mailbox；底层投递不暴露为业务 API；
- Body 在 Actor mailbox 解码；
- Handler 和客户端 Method ID 由业务代码显式维护。

## 2.1 Application Codec（已收敛）

- 仅保留 `BodyCodecs()` + `SetDefaultBodyCodec(id)`；
- 已删除 `Serializer()` / `DefaultBodyCodec()` / `SetSerializer`；
- Discovery 控制面固定使用 Protobuf，不受 Registry.Default() 影响。

## 2.2 ClusterMessage

- `ClusterMessage.session` 使用 `cproto.Session`（已删除 SessionSnapshot）；
- 旧 Call/CallWait/CallType 已删除，统一 Invoke/Notify。

## 3. 网络入口

AGP：

- 握手、心跳、超时、取消；
- inflight 和写队列上限；
- connection ID/UID 管理；
- Notify、Broadcast、Kick、GoAway。

HTTP：

- 固定 POST Path；
- 客户端只提交 Method ID 和 Body，不接受 Actor target；
- Content-Type 选择 JSON/PB，请求和响应使用相同 Codec；
- Request 200、Notify 202；
- 统一 HTTPError；
- Gin `RegisterActorRoutes`，现有 Controller 不变。

Cluster：

- NATS 使用 Protobuf ClusterMessage；
- Request/Reply 和 Notify Publish；
- deadline、Session（非 SessionSnapshot）、Metadata、Codec、code、message 透传；
- 已支持按配置切换 `ICluster` 实现（NATS / RabbitMQ），业务 API 不变；见 [rabbitmq-cluster.md](rabbitmq-cluster.md)。

## 4. 清理

已删除核心路径中的：

- Pomelo/Simple parser；
- INetParser 装配；
- FuncName 反射调用；
- CallWait/CallType；
- Session MID；
- ClusterPacket 调度。

为缩小改动，没有修改与 AGP 无关的历史测试、工具包和错误码。

## 5. 当前验证

```bash
# 全仓编译所有测试，但不运行历史阻塞型测试
go test ./... -run '^$' -count=1

# 执行本次核心测试
go test ./net/proto ./net/serializer ./net/method \
  ./net/parser ./net/httpactor ./net/actor ./net/connector

go vet ./...
```

已覆盖 Packet round-trip/fuzz、TCP/WS framing、握手、JSON/PB 方法分发、curl JSON、HTTP PB、Notify、Method ID 冲突和 ClusterMessage。

## 6. 上线前任务

- 在 Linux CGO 环境运行 `go test -race`；
- 真实 NATS 双节点 Request/Notify E2E；
- 连接风暴、慢消费者、写队列和 inflight 压测；
- TypeScript/C# SDK Packet Golden 互通；
- TLS、认证、授权、限流和审计配置。

这些属于部署环境验收，不再扩大当前框架改动范围。

更细的 API 对照、RequestContext、异常隔离与 NATS/RabbitMQ 对比见 [api-changes-2026-07.md](api-changes-2026-07.md)。

HTTP 调 Actor 的完整说明见 [http-actor.md](http-actor.md)。
