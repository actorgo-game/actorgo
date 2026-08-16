# ActorGo AGP 改造代码评审报告

## 1. 评审信息

| 项目 | 内容 |
| --- | --- |
| 评审范围 | 当前未提交工作区相对 `68402dd` 的协议与 Actor 调用改造 |
| 评审日期 | 2026-08-10 |
| 重点目录 | `facade/`、`net/actor/`、`net/method/`、`net/parser/`、`net/proto/`、`net/httpactor/`、`net/cluster/`、`net/serializer/` |
| 目标 | 在仅保留 req/res/notify、同时支持 Protobuf 和 JSON 的前提下继续简化 |

## 2. 实施状态（2026-08-11）

本报告建议的第一、二阶段已落地：

- 已删除 AGP Request/Notify 和 HTTP Header 中的外部 target。
- MethodTable 只发布顶层 Actor 方法，子 Actor Handler 只保留在本地 Mailbox。
- Handler 只在 Mailbox 注册时反射适配一次。
- 已删除 `RequestContext.MethodID`、公开 Post 入口、Actor 上重复的 Node/Target 包装和无用 Adapt/错误常量。
- Discovery 控制面已固定使用 Protobuf。
- `demo_cluster` 已改为客户端只传 Method ID，由 Gate 根据 Session 路由 Game 子 Actor。

第三阶段的 Codec Registry、独立 HTTP Component、集群响应 Envelope 和握手瘦身仍保持为可选项，本轮未扩大修改范围。

## 3. 总体结论

当前实现已经去掉了 Pomelo 和 `MethodDescriptor`，核心协议也已经收敛为 `Request`、`Response`、`Notify`。下一轮最值得做的不是继续删除 `request_id`、`method_id`、`code` 等必要字段，而是收紧外部协议的路由职责：

> 外部客户端只提交 `method_id + body`，服务端根据方法注册表和已认证会话决定 Actor；完整 `target_path` 只属于框架内部的 Actor/集群调用。

这样可以同时减少外部协议字段、方法注册分支和安全校验，并消除当前“第一次调用子 Actor”与后续调用行为不一致的问题。

## 4. 问题汇总

| 优先级 | 问题 | 影响 |
| --- | --- | --- |
| P0 | 外部 AGP 客户端可提交完整 `target`，框架只校验格式后按其中的 NodeID 转发 | 客户端可能把网关当作内部 Actor 路由代理 |
| P1 | 子 Actor 方法依赖全局表的延迟注册，首次调用可绕过 kind 和签名一致性校验 | 首次与后续调用语义不一致，错误暴露较晚 |
| P1 | `RequestContext.MethodID` 在多层调用中被原地修改 | 嵌套调用后上下文含义改变，异步复用时存在竞态风险 |
| P1 | Discovery 数据使用各节点的默认业务 Codec | 节点默认 Codec 不同时，控制面数据可能无法互解 |
| P2 | Handler 被反射适配两次，注册表还保留未使用字段和返回值 | 增加理解与维护成本 |
| P2 | `Post`、`Adapt` 等底层能力暴露过多，且 `Post` 对非法 target 缺少保护 | 扩大 API 面并引入可避免的错误路径 |
| P2 | HTTP Actor 同时存在独立 Component 和 Gin 挂载方式 | 两套启动/配置入口重复 |
| P2 | 旧 Pomelo 文档和生成过程资料仍在仓库中 | 搜索结果和使用指引容易误导开发者 |

## 5. 详细发现

### P0-1：外部 `target` 穿透了客户端与内部 Actor 路由的信任边界

AGP 请求和通知允许客户端直接提供完整 target：

```protobuf
message Request {
  uint32 request_id = 1;
  uint32 method_id = 2;
  uint32 timeout_ms = 3;
  bytes body = 4;
  string target = 5;
}

message Notify {
  uint32 method_id = 1;
  bytes body = 2;
  string target = 3;
}
```

连接层只验证 target 能否被解析，然后直接交给方法表：

```go
if request.Target != "" {
	if _, err := cfacade.ToActorPath(request.Target); err != nil {
		_ = c.send(responsePacket(request.RequestId, packet.Codec,
			cfacade.ErrorResult(cproto.StatusCode_STATUS_INVALID_ARGUMENT, "invalid actor target"), nil))
		return
	}
}

result := c.methods.Dispatch(ctx, request.Target, request.MethodId, request.Body, cfacade.MethodRequest)
```

`MethodTable.Dispatch` 对显式 target 甚至允许方法尚未注册；`ActorSystem.InvokeTarget` 又会按 target 中的 NodeID 向其他节点转发。当前链路缺少“该客户端是否允许访问这个 Actor/节点”的授权边界。

建议：

- 从外部 AGP `Request`、`Notify` 和 HTTP `X-Actor-Target` 中删除 target。
- 外部请求只通过 `method_id` 定位顶层服务 Actor。
- 玩家/房间/战斗等子 Actor 的 ID 和节点位置由服务端根据 `Session` 或业务参数决定。
- 内部集群信封可以继续保留 `target_path`，但不要把它暴露为客户端协议字段。
- `InvokeTarget`、`NotifyTarget` 若仍是内部实现所需，应从日常业务接口移入内部接口；业务 Actor 使用 `Invoke`、`Notify` 和明确的 `InvokeChild`、`NotifyChild`。

### P1-1：全局注册子 Actor 方法导致首次调用和后续调用不一致

显式 target 下，即使方法不存在，只要传入的 kind 合法，分发仍会继续：

```go
entry, found := r.lookup(methodID)
if target == "" {
	if !found {
		return cfacade.ErrorResult(cproto.StatusCode_STATUS_NOT_FOUND, "method not found")
	}
	if entry.target == "" {
		return cfacade.ErrorResult(cproto.StatusCode_STATUS_INVALID_ARGUMENT,
			"child method requires an explicit target")
	}
	target = entry.target
} else {
	if _, err := cfacade.ToActorPath(target); err != nil {
		return cfacade.ErrorResult(cproto.StatusCode_STATUS_INVALID_ARGUMENT, "invalid actor target")
	}
	if !found && kind != cfacade.MethodRequest && kind != cfacade.MethodNotify {
		return cfacade.ErrorResult(cproto.StatusCode_STATUS_NOT_FOUND, "method not found")
	}
}
```

这条分支是为了让目标子 Actor 尚未创建时也能被第一次调用。但是它带来三个额外问题：

1. 第一次调用没有注册元数据，无法验证请求/通知 kind。
2. 子 Actor 创建后才把方法写入全局表，第二次调用走另一套校验路径。
3. 兄弟子 Actor共用 MethodID 时，只比较父路由，没有完整比较 kind、请求类型和返回类型。

建议把方法表恢复成单一职责：只注册外部可访问的顶层方法，结构可收敛为 `methodID -> actorID + kind`。子 Actor 的 Handler 只保存在自己的 Mailbox 中，不进入全局 MethodTable。内部调用子 Actor 时，调用方已经明确知道 target 和 kind，不需要借用外部方法表。

### P1-2：`RequestContext.MethodID` 是可变的调用栈状态

方法分发和 Actor 收信都在修改同一个上下文：

```go
ctx.MethodID = methodID
return t.actorSystem.InvokeTarget(ctx, target, methodID, body)
```

```go
ctx.MethodID = message.MethodID
result, err := a.mailbox.Invoke(ctx, message.MethodID, message.Payload)
```

当一个 Handler 使用原 ctx 嵌套调用另一个方法时，返回后 `ctx.MethodID` 已经变成被调用方法；如果同一 ctx 被异步通知继续使用，还可能产生数据竞态。当前仓库没有发现依赖该字段的有效业务读取点。

最简单的处理是删除 `RequestContext.MethodID`，方法 ID 从当前 Message/Handler 的明确参数获得。若以后中间件确实需要调用信息，应为每次调用创建只读的子上下文，而不是修改调用者对象。

### P1-3：Discovery 控制面不应依赖业务默认 Codec

Discovery 当前使用每个节点自己的默认 Codec：

```go
func (m *DiscoveryNats) marshal(value any) ([]byte, error) {
	return m.app.BodyCodecs().Marshal(m.app.BodyCodecs().Default(), value)
}

func (m *DiscoveryNats) unmarshal(data []byte, value any) error {
	return m.app.BodyCodecs().Unmarshal(m.app.BodyCodecs().Default(), data, value)
}
```

Discovery 消息本身没有携带 Codec ID。一个节点默认 JSON、另一个节点默认 Protobuf 时，双方可能不能解析彼此的注册数据。

建议将 Discovery、集群控制消息固定为 Protobuf；“默认 Codec”只用于没有显式指定 Codec 的业务调用，不能影响控制面。

### P2-1：Handler 适配和 Codec 能力校验存在重复

`ActorMailbox.Register` 先调用 `AdaptHandler`，随后又把原始 handler 交给 `MethodTable.Register`，后者再次调用 `adapt`。这意味着同一个函数在注册时执行两次反射签名分析，且 MethodTable 返回的 `TypedInvoke` 被 Mailbox 忽略。

同时，MethodTable 保存了 `requestType`，但后续没有读取；MethodTable 与 Mailbox 都检查 Codec 支持能力。

建议：

- Handler 只在 Mailbox 注册时适配一次。
- MethodTable 不再接收原始 Handler，也不返回 `TypedInvoke`。
- 顶层表只保存路由真正需要的 `actorID` 和 `kind`。
- 请求类型、PB/JSON 解码能力只由 Mailbox 保存和检查。

完成后可以删除 `registeredMethod.owner`、`target`、`requestType`、`protobuf` 中不再需要的字段，以及 `canShareMethodID`、`parentRouteTarget` 这类子 Actor 共享注册辅助逻辑。

### P2-2：公开 API 面仍可继续收缩

`IActorSystem` 和 `IActor` 当前都暴露 `Invoke`、`InvokeNode`、`InvokeTarget`、`Notify`、`NotifyNode`、`NotifyTarget`，还同时暴露底层 `Post`。其中：

- 日常业务调用只需要 targetless 的 `Invoke`、`Notify`。
- 定向节点调用是服务端内部能力，可放到较窄的内部接口。
- 子 Actor 调用已有语义更明确的 `InvokeChild`、`NotifyChild`。
- `Post` 绕过方法级路由和类型约束；当前传入非法 Message target 时还可能在解引用 `TargetPath()` 时 panic。
- `net/method.Adapt` 没有调用方，可以删除或改成包内函数。

建议面向业务保留最小调用接口：

```go
type IActor interface {
	Invoke(ctx *RequestContext, methodID uint32, payload any) *InvokeResult
	Notify(ctx *RequestContext, methodID uint32, payload any) *InvokeResult
}
```

Handler 注册继续使用当前简洁的 `a.Methods().Register(methodID, handler)`；具体 `Actor` 可保留 `InvokeChild`、`NotifyChild`。Node/Target/Post 仍可在框架内部实现，但不必出现在业务 Actor 的主接口中。

### P2-3：HTTP Actor 入口可按实际部署方式二选一

当前既有 `net/httpactor.Component` 自建 HTTP Server，也有 `components/gin.RegisterActorRoutes` 把同一个 Handler 挂到现有 Gin Server。对于已经统一使用 `components/gin` 的项目，独立 Component 会形成第二套地址、生命周期和中间件配置。

建议保留 `net/httpactor.Handler` 作为标准 `http.Handler`，由 Gin 组件挂载；如果仓库没有独立 HTTP Server 的实际使用场景，可删除 `net/httpactor/component.go`。Curl 场景只保留 JSON 即可，例如：

```text
POST /actor/{methodID}
Content-Type: application/json

{...请求体...}
```

AGP TCP 继续支持 PB/JSON，不要求 HTTP 也维护 PB 响应分支。此项属于产品取舍，不影响第一轮安全和正确性修复。

### P2-4：固定双 Codec 比可扩展 Registry 更符合当前协议

AGP Validator 只认可 Protobuf 和 JSON 两个固定 Codec ID，但 `serializer.Registry` 暴露了任意 Register/Lookup、默认值切换和锁保护，看起来像支持第三方 Codec，实际数据面会拒绝。

建议明确选择其一：

- 若最终协议固定 PB/JSON：改成两个固定 Codec，通过 ID 直接选择；发送时可按 `proto.Message -> PB`、普通 struct -> JSON` 自动决定。
- 若确实需要 MessagePack：先把 Codec ID、能力协商和所有入口的验证做成真正可扩展，再保留 Registry。

按当前“只支持 PB 和 JSON”的目标，固定双 Codec 更简单。

### P2-5：清理旧文档与构建过程产物

`_docs/design.md`、`_docs/game-scenarios.md`、`_docs/pomelo.md` 仍包含大量 Pomelo、`INetParser`、`CallWait` 示例；`docs/superpowers/` 下还存在生成过程材料和乱码文本。这些内容不会影响编译，但会明显降低后续维护者搜索代码和理解新 API 的效率。

建议更新仍有价值的架构文档，删除纯 Pomelo 说明和一次性生成过程产物。是否删除应逐文件确认，不要把所有未跟踪文档批量清理。

## 6. 推荐的精简顺序

### 第一阶段：先修正边界和行为

1. 删除外部 AGP/HTTP target；保留内部集群 `target_path`。
2. MethodTable 只注册顶层 Actor 方法，子 Actor Handler 只留在 Mailbox。
3. 删除 `RequestContext.MethodID` 原地修改。
4. Discovery 控制面固定使用 Protobuf。

### 第二阶段：删除随之失效的复杂度

1. Handler 只反射适配一次。
2. 删除 MethodTable 的 Handler 类型信息、子 Actor 共享注册逻辑和无用返回值。
3. 收窄 `IActor`，移除日常接口中的 Node/Target/Post 组合。
4. 删除未使用的 `method.Adapt`、无效常量及重复 Codec 校验。

### 第三阶段：可选瘦身

1. 固定 PB/JSON 双 Codec，取消伪扩展 Registry。
2. 若所有项目都通过 Gin 启动，删除独立 HTTP Actor Component。
3. 集群响应改为最小 `ActorReply { code, message, payload }`，利用 NATS RequestSync 自身的请求相关性，删除响应信封中重复的 message/request/method/codec 字段。
4. 因不考虑兼容期，可将握手的版本列表缩为单一协议版本；心跳、Kick、GoAway 是否合并应根据运维诊断需求决定。

## 7. 不建议继续删除的内容

以下内容已经接近必要最小集：

- AGP 的 `request_id`、`method_id`、`body`、响应 `code/message/body`。
- `Packet` 中 req/res/notify 的 oneof，它清楚表达三种交互语义。
- `Session`，它是删除外部 target 后进行服务端路由和鉴权的基础。
- `Connection` 与 `ConnectionManager`，二者分别负责单连接状态和连接集合生命周期。
- `packetTransport` 与 `TCPPacketFramer`，前者负责收发与请求关联，后者只负责 TCP 帧边界，并不重复。
- `InvokeResult` 的 `Payload + Code + Message`，字段规模已经合理；更需要解决的是本地调用返回对象、远程调用返回字节的不一致。

## 8. 验证结果

以下改造相关包测试通过：

```text
go test ./facade ./net/actor ./net/cluster/nats_cluster ./net/connector ./net/httpactor ./net/method ./net/parser ./net/proto ./net/serializer -timeout 30s
```

对应包的 `go vet` 也通过。

`go test ./...` 当前不是绿色，失败/超时集中在既有的 `components/cron`、`extend/mapstructure`、`extend/queue`、`extend/reflect`、`extend/snowflake`、`extend/time_wheel` 测试；本次评审没有将这些历史问题归因于 AGP 改造。

## 9. 最终建议

建议本轮做到第一、二阶段为止。完成后的模型会更清楚：

```text
外部客户端 -> method_id -> 顶层 Actor -> 基于 Session/业务数据选择子 Actor
                                      -> 内部 target_path/集群路由
```

这比把 target 继续放在所有 Invoke/Notify、AGP、HTTP 和 MethodTable 层中更简单，也更符合 Actor 地址属于服务端内部实现细节的边界。
