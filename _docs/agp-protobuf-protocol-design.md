# ActorGo AGP/1 JSON/PB 协议设计

> 状态：已实现  
> 范围：AGP TCP/WS、HTTP JSON/PB、NATS ClusterMessage  
> 非目标：Pomelo 兼容、gRPC、MessagePack  
> 配套：[近期 API 变更汇总](api-changes-2026-07.md)

## 1. 设计结论

ActorGo 不再把 Pomelo Packet、Route、MID 和 Actor FuncName 暴露给客户端。所有入口统一为：

```text
Transport -> Method Table (kind/存在性) -> Actor Mailbox (Body 解码 + Handler)
```

核心原则：

- Wire 使用稳定 uint32 Method ID；顶层 Actor 由 MethodID 自动定位
- 业务交互只有 Request、Response、Notify
- Packet / ClusterMessage 固定为 PB；JSON/PB 只影响业务 Body
- Request ID 属于请求上下文，不存入 Session
- 客户端只能提交 Method ID 和 Body，不能提交函数名或 ActorPath
- 动态子 Actor 的 ActorPath 仅存在于服务端内部调用和 ClusterMessage
- PB Schema 方法支持 PB 与 ProtoJSON；普通 struct 只支持 JSON
- Body 在目标 Actor mailbox 解码，不在 Method 表前置解码

## 2. AGP Packet

最终定义位于 `net/proto/agp.proto`：

```proto
message Packet {
  oneof kind {
    Request request = 1;
    Response response = 2;
    Notify notify = 3;
  }
  map<string, bytes> metadata = 4;
  int32 codec = 5; // 1=protobuf, 2=json
}

message Request {
  uint32 request_id = 1;
  uint32 method_id = 2;
  uint32 timeout_ms = 3;
  bytes body = 4;
}

message Response {
  uint32 request_id = 1;
  int32 code = 2;
  string message = 3;
  bytes body = 4;
}

message Notify {
  uint32 method_id = 1;
  bytes body = 2;
}
```

没有额外 AGP 固定头：

- TCP 使用 4 字节 Big Endian Packet 长度
- WebSocket 使用自身消息边界，一条 Binary Message 一个 Packet
- WebSocket 必须声明 `agp.v1`，Text Message 直接拒绝

## 3. 系统方法

| ID | 方法 | Kind | Body |
|---:|---|---|---|
| 1 | Handshake | Request | PB |
| 2 | Heartbeat | Request | PB |
| 3 | Cancel | Notify | PB |
| 4 | GoAway | Notify | PB |
| 5 | Kick | Notify | PB |

业务 ID 必须大于 5。握手只确认协议版本；系统控制 Body 固定使用 PB。
业务请求不包含 target，由目标节点的 MethodTable 按 Method ID 定位顶层 Actor。

## 4. Body Codec

AGP/1 只注册两种 Codec：

| Codec | PB Schema | 普通 struct |
|---|---:|---:|
| Protobuf | 支持 | 不支持 |
| JSON | ProtoJSON | 标准 JSON |

每个 Packet 使用一个 codec，请求和响应使用相同编码。

Application 侧：

- BodyCodecs()：注册表（含 Default() / SetDefault）
- SetDefaultBodyCodec(id)：只切换默认 Body Codec（如服务端主动 Notify），不会注销 PB
- 已删除 Serializer() / DefaultBodyCodec() / SetSerializer

RequestContext.Codec 贯穿解码、回写与跨节点 ClusterMessage。

## 5. Actor Method Table

```go
a.Methods().Register(LoginMethodID, a.Login)

// 子 Actor 方法只安装到自己的 mailbox，不进入外部方法表
child.Methods().Register(SelectMethodID, child.Select)
```

- Register 不返回 error；失败 panic（通常发生在 OnInit）
- 框架从函数签名自动得到 Request/Notify Kind、请求/响应类型
- Method 表只保留 Kind 与顶层 ActorPath，始终按 MethodID 路由
- 子 Actor 可重复 Register 同一 Method ID，因为只安装本地 mailbox

不再提供 MethodDescriptor、自定义 HTTP Path、动态 TargetResolver、每方法权限或每方法默认超时。顶层 Method ID 在一个节点内必须全局唯一。

## 6. Actor 调用

```go
Login(*facade.RequestContext, *LoginRequest) (*LoginResponse, error)
Report(*facade.RequestContext, *ReportNotify) error
```

传输层按 MethodID 把 raw body 投递到顶层 Actor；mailbox 按 Handler 参数类型解码并执行。顶层 Actor 再根据 Session 或业务参数选择动态子 Actor。

Actor 仅保留单一方法邮箱（Methods().Register）；已移除 Local mailbox。底层投递由框架内部完成，不暴露 Post 业务接口。

顶层 Actor 调用不暴露 ActorPath：

```go
app.ActorSystem().Invoke(ctx, methodID, payload)
app.ActorSystem().InvokeNode(ctx, nodeID, methodID, payload)
app.ActorSystem().Notify(ctx, methodID, payload)
app.ActorSystem().NotifyNode(ctx, nodeID, methodID, payload)
```

服务端内部动态子 Actor 使用 `InvokeChild` / `NotifyChild`；跨节点内部路由才使用 `InvokeTarget` / `NotifyTarget`。

旧 Call / CallWait / CallType 已移除。

Request 返回 InvokeResult；Notify 成功入队即结束。code == 0 表示成功。

## 7. AGP 连接

Connection 使用一个 reader 和一个 writer，并提供：

- 握手超时、心跳和 idle timeout
- inflight 上限与重复 Request ID 检查
- 有界写队列
- Request cancel
- connection ID/UID 绑定
- Notify、Broadcast、Kick、GoAway

非法长度、非法 PB、错误系统 Codec、握手前业务包和 WebSocket Text Message 都会关闭连接。

## 8. HTTP JSON/PB

HTTP 只提供固定 `POST /actor/{methodID}` 入口，与 AGP/NATS 共用方法表。详细用法、Header、状态码、Options 与 curl 示例见 [http-actor.md](http-actor.md)。

要点：

- 客户端不能提供 ActorPath；服务端根据 Method ID 和 Session/业务参数路由
- 必填 `Content-Type`：`application/json` 或 `application/x-protobuf`（同时决定请求/响应 Codec）
- 可选 `X-ActorGo-Timeout-Ms`（受服务端 MaxTimeout 限制）
- 所有顶层 Actor 的 `Methods().Register` 方法都进入 HTTP 方法表（MsgType 查询）
- Request 成功返回 200 + 业务 Body；Notify 入队返回 202
- 错误统一编码为 `HTTPError`，并带 `X-ActorGo-Request-ID`
- JSON-only 方法收到 PB 返回 415
- `Transport` = `TransportHTTP`；Session 由可选 Authenticator 注入

接入：

```go
// 独立组件
app.Register(httpactor.NewComponent("actor-api", "127.0.0.1:9080"))

// 或挂到现有 Gin（现有 Controller 不变）
actorHandler := httpactor.NewHandler(app)
_ = httpServer.RegisterActorRoutes(actorHandler)
```

`net/httpactor` 是标准 `http.Handler`，`components/gin.RegisterActorRoutes` 只负责挂载。

## 9. NATS ClusterMessage

跨节点不传 AGP Packet，而传 `net/proto/cluster_message.proto`：

```proto
enum MsgType {
  REQUEST = 0;
  RESPONSE = 1;
  NOTIFY = 2;
}

message ClusterMessage {
  uint64 message_id = 1;
  MsgType msg_type = 2;
  uint32 request_id = 3;
  uint32 method_id = 4;
  int64 deadline_unix_ms = 5;
  string target_path = 6; // 可选：仅动态子 Actor 使用
  cproto.Session session = 7; // 与本地 Session 同型；无 SessionSnapshot
  map<string, bytes> metadata = 8;
  int32 codec = 9;
  bytes payload = 10;
  int32 code = 11;
  string message = 12;
}
```

- Request 使用 NATS Request/Reply；Notify 使用 Publish
- 顶层调用进入 Actor 方法表；带内部 target_path 的子 Actor 调用直接进入 ActorSystem
- Session / Metadata / Codec / deadline / code / message 透传
- 进出集群会 clone Session，避免共享可变 Data

关于可选 RabbitMQ 传输，见 [rabbitmq-cluster.md](rabbitmq-cluster.md) 与 [变更汇总](api-changes-2026-07.md)。

## 10. 超时优先级

1. 客户端显式 timeout
2. 传输层默认 timeout
3. 最终受服务端最大值限制

Cancel 在请求 goroutine 启动前写入 inflight 表，避免取消竞态。

## 11. 异常隔离

业务 handler / Event / Timer panic 会被 recover，转换为协议错误或日志，不拖垮进程。未捕获的框架路径 panic 仍可能按 Go 语义退出进程。详见 [变更汇总](api-changes-2026-07.md)。

## 12. 验证边界

仓库测试覆盖 Packet round-trip/fuzz、TCP framing、WebSocket Binary 边界、握手、JSON/PB 方法分发、HTTP curl/PB/Notify、ClusterMessage 与 Method ID 冲突。

上线前仍需在 CI/部署环境执行：

- go test -race
- 真实 NATS 双节点 E2E
- 连接风暴、慢消费者和写队列压力测试
- TypeScript/C# SDK Golden 数据互通
