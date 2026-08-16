# ActorGo 近期 API / 架构变更汇总（2026-07）

> 本文汇总 AGP/1 落地后的一批收敛改动，作为当前实现的权威说明。  
> 历史文档中的 `Call` / `CallWait` / `PostRemote` / `SetSerializer` / `SessionSnapshot` / `Local()` 等已过时。

---

## 1. Actor 方法注册

### 写法

```go
func (p *ActorUser) OnInit() {
	p.Methods().Register(methodid.Login, p.login)
	p.Methods().Register(methodid.KickUID, p.kickUID)
}
```

- `Methods().Register(methodID, handler)` **不再返回 `error`**
- 注册失败（重复 ID、签名非法等）在 `OnInit` 中 **panic**，由 `CreateActor` 捕获并转为创建失败
- 不要写 `_ =` 或 `if err := ...`

### Handler 签名（固定两种）

```go
// Request
func(ctx *cfacade.RequestContext, req *Request) (*Response, error)

// Notify
func(ctx *cfacade.RequestContext, req *Request) error
```

- 第一个参数必须是 `*RequestContext`
- 第二个参数必须是指针
- 返回值个数决定 Kind（2 个返回值 = Request，1 个 `error` = Notify）

### 客户端 → A 与 A → B

注册方式**完全相同**。差别只在调用入口：

| 路径 | 入口 | 落到 Actor |
|------|------|------------|
| 客户端 → A | AGP / HTTP → `Methods().Dispatch` | `Invoke` → mailbox → handler |
| A → B | A 调 `Invoke`/`Notify` → 集群 | B 侧同样 `Dispatch` → handler |

---

## 2. RequestContext

### 字段含义

| 字段 | 作用 |
|------|------|
| `Transport` | 入口类型：AGP、HTTP、Cluster |
| `Codec` | **本次调用业务 Body** 的编解码（Protobuf=1 / JSON=2） |
| `Session` | 连接/用户快照（Sid/Uid/Ip/Data） |
| `RequestID` | 请求关联；Method ID 由当前 Message 明确持有，不写入可变 Context |
| `Metadata` | 透传元数据 |

### `Codec` 具体用途

- Actor mailbox 把 `[]byte` **Unmarshal** 成 `*Request`
- 回写客户端 / HTTP 时 **Marshal** `*Response`
- 跨节点写入 `ClusterMessage.Codec`，对端按同一值解码
- `0` 时使用 `app.BodyCodecs().Default()`

### 客户端直达 vs 节点互调（举例）

| 字段 | 客户端 → Gate | Gate → Center（`rpc.Invoke` 新建 ctx） |
|------|---------------|----------------------------------------|
| `Transport` | `TransportAGP` | B 上为 `TransportCluster` |
| `Session` | 连接真实 Session | 默认 **nil**（除非调用方显式放入再转发） |
| `RequestID` | 来自客户端包 | 多为 0 |
| `Codec` | 来自 Packet | 调用方设置（demo 多为 Protobuf） |

需要玩家身份时，A 必须把 `ctx.Session` 放进跨节点调用（或使用 `NotifySession` 一类助手）。

---

## 3. Body Codec（Application 收敛）

### 之前（已删除）

- `serializer` + `codecs` + `defaultBodyCodec` 三套重叠
- `Serializer()` / `DefaultBodyCodec()` / `SetSerializer(...)`

### 现在

Application 只保留：

- `BodyCodecs() IBodyCodecRegistry`
- `SetDefaultBodyCodec(id int32)`（启动前切换默认，如 JSON）

Registry 能力：

```go
Register / Lookup / Marshal / Unmarshal
Default() int32
SetDefault(id int32) error
```

内置同时注册 Protobuf + JSON；默认 id = Protobuf。  
Discovery 控制面固定使用 Protobuf，不受业务默认 Body Codec 影响。

```go
app.SetDefaultBodyCodec(cfacade.CodecJSON) // 仅改默认；PB 仍可用
```

---

## 4. 投递 API：框架内部化

单一 mailbox 后，底层 Message 投递不再暴露给业务接口。业务只使用 `Invoke` / `Notify`；跨节点由 `ICluster` 负责。

---

## 5. 旧 Call API 已移除

| 旧 | 新 |
|----|----|
| `IActor.Call` | `ActorSystem.Notify(ctx, methodID, req)` |
| `IActor.CallWait` | `ActorSystem.Invoke(ctx, methodID, req)` |
| `IActor.CallType` | 发现服务选节点 + `InvokeNode`/`NotifyNode` |
| `IActorChild.Call/CallWait` | 同节点使用 `InvokeChild`/`NotifyChild`；跨节点 target 仅供框架内部路由 |

`IActorChild` 仅保留生命周期：`Create` / `Get` / `Remove` / `Each`。

同进程子 Actor 内部可用 `InvokeChild` / `NotifyChild`，避免父 Actor 自死锁。

---

## 6. MethodID 自动定位 Actor

顶层 Actor 的业务 Request/Notify 只需 MethodID。方法注册时，MethodTable
同时记录该方法所属的顶层 ActorPath：

```go
app.ActorSystem().Invoke(ctx, methodID, req)                 // 本节点
app.ActorSystem().InvokeNode(ctx, nodeID, methodID, req)     // 指定节点
```

动态子 Actor 的 childID 是运行时数据。服务端内部需要跨节点定向时才显式使用完整 ActorPath：

```go
target := cfacade.NewChildPath(nodeID, "player", uid)
app.ActorSystem().InvokeTarget(ctx, target, methodID, req)
```

客户端 AGP/HTTP 都不接受 target；外部请求先到顶层 Actor，再由服务端选择子 Actor。

---



## 6.1 HTTP 调用 Actor

完整说明见 [http-actor.md](http-actor.md)。摘要：

- 固定 `POST /actor/{methodID}`（methodID > 5）
- 必填 `Content-Type`（json / x-protobuf）；客户端不提供 Actor target
- Request 成功 200；Notify 成功 202
- `TransportHTTP`；Session 来自可选 Authenticator
- 接入：`httpactor.NewComponent(...)` 或 Gin `RegisterActorRoutes`

## 7. ClusterMessage：去掉 SessionSnapshot

`ClusterMessage.session` 直接使用 `cproto.Session`（字段与旧 Snapshot 相同）。

进出集群仍 **clone** Session（含 `Data`），避免共享可变 map。

NATS 仍是当前 `ICluster` 实现；协议载体为 Protobuf `ClusterMessage`。

---

## 8. 方法适配层（`adapt` / `buildInvoke` / `requestValue`）

注册时反射适配，运行时统一调用：

1. **`adapt`**：校验签名，推断 Request/Notify，记录请求类型与是否 protobuf  
2. **`buildInvoke`**：生成 `func(ctx, payload) *InvokeResult`，反射调用业务函数  
3. **`requestValue`**：校验 payload 类型与注册时一致  

---

## 9. Actor 异常与进程

| 场景 | 行为 |
|------|------|
| 业务 handler panic | recover → 日志 → `STATUS_INTERNAL`，**Actor 继续跑** |
| Event / Timer panic | 有保护，不拖垮进程 |
| `OnInit` panic | CreateActor 失败，进程不退出 |
| 正常 `Exit()` | 只停该 Actor |
| 框架路径未捕获的 panic | Go 语义下可能退出整个进程 |

---

## 10. 传输选型：NATS vs RabbitMQ（规划参考）

Actor 层已抽象为 `ICluster.Publish/Request(ClusterMessage)`，换传输主要是新实现 + 配置，不必改业务 API。

| 维度 | NATS（现状） | RabbitMQ |
|------|--------------|----------|
| 定位 | 轻量 subject 总线 | Exchange + Queue 中间件 |
| Request/Reply | 原生好用 | 需 reply-to / correlation-id |
| 延迟吞吐 | 通常更优 | 功能换复杂度 |
| 持久化/ACK | Core 偏尽力；JetStream 更强 | 成熟 |
| 运维 | 轻 | 更重 |
| ActorGo 适配成本 | 已完成 | 中等（新 cluster 实现） |

建议：节点低延迟 RPC 继续 NATS；若公司统一 MQ / 强持久再加 `cluster.mode=rabbitmq`。Discovery 可与 Cluster 传输独立配置。

---

## 11. 相关文件速查

| 主题 | 路径 |
|------|------|
| 方法适配 | `net/method/invoker.go` |
| 方法表 | `net/method/table.go` |
| Mailbox / 内部投递 | `net/actor/actor.go`, `actor_mailbox.go` |
| Invoke/Notify | `net/actor/system.go` |
| Body Codec | `application.go`, `net/serializer/registry.go`, `facade/codec.go` |
| AGP 连接 | `net/parser/connection.go` |
| HTTP Actor | `net/httpactor/`，文档 `_docs/http-actor.md` |
| 集群 | `net/cluster/nats_cluster/`, `net/proto/cluster_message.proto` |
| 协议设计 | `_docs/agp-protobuf-protocol-design.md` |
| 开发状态 | `_docs/agp-protobuf-development-plan.md` |
