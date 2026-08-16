# Code Review Report - 当前本地未提交修改

**审查日期**: 2026-08-15  
**审查范围**: `HEAD 68402dd0bb60b54186b9ceaa5a564cfa5787e427` 到当前 WORKTREE  
**提交总数**: 0 个（未提交工作区快照 1 份）  
**涉及开发人员**: 1 人（未提交工作区）

---

## 📊 执行摘要

### 总体评估

当前改造已基本完成“移除 Pomelo/Simple、AGP 外层固定 Protobuf、业务 Body 同时支持 JSON/PB、MethodID 路由、HTTP curl 调用”的目标。协议、序列化、方法适配、连接管理和集群入口的职责比早期版本清晰，相关新增包的定向测试与 `go vet` 均通过。

当前仍不建议直接合并。主要阻塞点是：新增节点精确匹配与运行时 NodeID 表示不一致，会使点分 Node ID 的集群节点启动失败；动态子 Actor 懒创建绕过父邮箱，破坏单线程保证；连接器和 Actor 队列在停机并发下仍存在生命周期竞态；外部 Notify 没有统一背压，存在内存耗尽风险。

本报告审查了最终工作区状态，自动生成的 `*.pb.go` 不做人工逻辑审查，但参与编译和测试。

### 问题统计

- 🔴 **严重问题**: 0 个
- 🟡 **中等问题**: 7 个
- 🔵 **轻微问题**: 6 个
- ⭐ **代码亮点**: 7 处

---

## 📋 问题汇总清单

### 🔴 严重问题（P0 - 立即修复）

无。

### 🟡 中等问题（P1 - 短期内修复）

| ID | 模块 | 问题描述 | 提交 | 负责人 | 详情 |
|----|------|----------|------|--------|------|
| P1-1 | Profile | 点分 NodeID 无法匹配配置 | WORKTREE | 未提交工作区 | [查看详情](#p1-1-node-id-normalization) |
| P1-2 | Actor/Child | 懒创建绕过父邮箱 | WORKTREE | 未提交工作区 | [查看详情](#p1-2-child-actor-concurrency) |
| P1-3 | Actor/Mailbox | 停机投递可写已销毁队列 | WORKTREE | 未提交工作区 | [查看详情](#p1-3-mailbox-stop-race) |
| P1-4 | Parser/HTTP | Notify 可无限堆积 | WORKTREE | 未提交工作区 | [查看详情](#p1-4-notify-backpressure) |
| P1-5 | Connector | Stop 后可能重新监听 | WORKTREE | 未提交工作区 | [查看详情](#p1-5-connector-start-stop) |
| P1-6 | Parser/Session | 已关闭连接仍可 Bind | WORKTREE | 未提交工作区 | [查看详情](#p1-6-bind-close-race) |
| P1-7 | Actor/Timeout | 解码后未复查 deadline | WORKTREE | 未提交工作区 | [查看详情](#p1-7-deadline-after-decode) |

### 🔵 轻微问题（P2 - 优化建议）

| ID | 模块 | 问题描述 | 提交 | 负责人 | 详情 |
|----|------|----------|------|--------|------|
| P2-1 | Parser | 超 inflight 会断开整条连接 | WORKTREE | 未提交工作区 | [查看详情](#p2-1-inflight-overload) |
| P2-2 | Actor/Method | 内部调用不校验 MethodKind | WORKTREE | 未提交工作区 | [查看详情](#p2-2-method-kind) |
| P2-3 | Actor | self-invoke 保护可绕过 | WORKTREE | 未提交工作区 | [查看详情](#p2-3-self-invoke-bypass) |
| P2-4 | Lifecycle | Actor 停在集群依赖之后 | WORKTREE | 未提交工作区 | [查看详情](#p2-4-shutdown-order) |
| P2-5 | HTTP | Notify 202 缺少请求 ID | WORKTREE | 未提交工作区 | [查看详情](#p2-5-http-notify-request-id) |
| P2-6 | Cluster | 订阅失败仍宣告初始化成功 | WORKTREE | 未提交工作区 | [查看详情](#p2-6-cluster-subscribe-error) |

---

## 🎯 主要功能模块分析

### 1. Profile 与节点发现 (Priority: P1)

**提交**: WORKTREE  
**开发者**: 未提交工作区  
**文件范围**: `profile/`、`net/discovery/`

#### 功能描述

本次改动为同一 NodeType 下的多个实例增加精确 `node_id`/正则匹配，避免始终读取第一个实例的配置。

#### 问题发现

<a id="p1-1-node-id-normalization"></a>
#### 🟡 **中等问题 P1-1: 点分 NodeID 被转为十进制后无法匹配配置**

**提交**: WORKTREE  
**作者**: 未提交工作区  
**文件位置**: `F:\actorgo-game\actorgo\profile\node.go`（约第60-86行）、`F:\actorgo-game\actorgo\profile\profile.go`（约第78-86行）  
**函数名**: `GetNodeWithConfig`、`Init`

```go
nodeId, err := cfacade.GenNodeIdByStr(nodeIdStr)
if err != nil {
	return nil, cerror.Errorf("Failed to generate node ID. [err = %v]", err)
}

nodeType := cfacade.GetNodeType(nodeId)

node, err := GetNodeWithConfig(jsonConfig, cstring.ToString(nodeId), cstring.ToString(nodeType))
```

```go
if nodeType != ndType || !findNodeID(nodeId, item.GetConfig("node_id")) {
	continue
}
```

**问题**: `Init` 把输入的 `1.10001.5.1` 编码成 `uint64`，再以十进制字符串传给新增的精确匹配；配置仍保存点分 ID 或点分正则，两种表示永远不相等。`DiscoveryNats.loadMember` 也用同样的十进制 ID 调用 `LoadNode`。新增测试直接把点分 ID 传入 `GetNodeWithConfig`，没有覆盖真实入口。

**风险**: 使用当前文档和示例配置的集群节点会在启动阶段稳定报 `ndType ... not found`，多实例配置无法使用。

**建议**: 用原始 `nodeIdStr` 匹配配置，解析后的十进制值只作为运行时 `NodeID`；或提供统一规范化函数，让点分、十进制和正则匹配遵守同一规则。补充经 `profile.Init` 的回归测试。

[返回问题清单](#-问题汇总清单)

---

### 2. Actor 与 Mailbox (Priority: P1)

**提交**: WORKTREE  
**开发者**: 未提交工作区  
**文件范围**: `net/actor/`、`net/method/`、`facade/`

#### 功能描述

Actor 调用已统一为 MethodID + 单 mailbox，增加 RequestContext 生命周期、InvokeResult、子 Actor 定向调用和初始化期方法注册。

#### 问题发现

<a id="p1-2-child-actor-concurrency"></a>
#### 🟡 **中等问题 P1-2: 动态子 Actor 懒创建绕过父 Actor mailbox**

**提交**: WORKTREE  
**作者**: 未提交工作区  
**文件位置**: `F:\actorgo-game\actorgo\net\actor\system.go`（约第176-184行）、`F:\actorgo-game\actorgo\net\actor\actor.go`（约第238-253行）、`F:\actorgo-game\actorgo\net\actor\actor_child.go`（约第41-53行）  
**函数名**: `System.InvokeTarget`、`Actor.InvokeChild`、`actorChild.Create`

```go
if targetPath.IsChild() {
	parent, found := p.GetActor(targetPath.ActorID)
	if !found {
		return cfacade.ErrorResult(cproto.StatusCode_STATUS_NOT_FOUND, "actor not found")
	}
	return parent.InvokeChild(ctx, targetPath.ChildID, methodID, payload)
}
```

```go
childActor, found := p.findChildActor(message)
if !found {
	message.Recycle()
	return cfacade.ErrorResult(cproto.StatusCode_STATUS_NOT_FOUND, "child actor not found")
}
```

**问题**: 外部调用 goroutine 直接执行父 Actor 的 `OnFindChild`。两个并发请求访问同一个不存在的 child 时，可同时通过 `Get`，分别创建并启动同路径 Actor，随后 `Store` 覆盖其中一个。

**风险**: `OnFindChild` 与父 handler 并发访问业务状态；同一逻辑 child 失去串行语义；被覆盖的 orphan child 不在 `childActors` 中，父停止时收不到 `Exit`，`System.Stop()` 可能永久等待。

**建议**: `System.InvokeTarget/NotifyTarget` 的 child path 先投递父 mailbox，由 `processTyped` 串行懒创建；仅父 handler 内部使用的 `InvokeChild/NotifyChild` 允许直投。`actorChild.Create` 仍需锁或原子 `LoadOrStore`。

[返回问题清单](#-问题汇总清单)

---

<a id="p1-3-mailbox-stop-race"></a>
#### 🟡 **中等问题 P1-3: Actor 停止与消息投递存在 TOCTOU**

**提交**: WORKTREE  
**作者**: 未提交工作区  
**文件位置**: `F:\actorgo-game\actorgo\net\actor\system.go`（约第426-448行）、`F:\actorgo-game\actorgo\net\actor\queue.go`（约第39-53、93-98行）  
**函数名**: `System.post`、`queue.Push`、`queue.Destroy`

```go
state := targetActor.State()
if state == InitState || state == WorkerState {
	targetActor.post(m)
	return true
}
```

```go
func (p *queue) Destroy() {
	close(p.C)
	p.head = nil
	p.tail = nil
	p.count = 0
}
```

**问题**: 状态检查和 `Push` 不是同一原子生命周期操作。生产者读到可投递状态后，Actor 可以完成停止并销毁队列，随后生产者继续写入。

**风险**: `head == nil` 时解引用 `prev.next`，或 `_setCount` 向已关闭的 `C` 发送，均可 panic。网络请求与节点关闭并发即可触发。

**建议**: mailbox 提供带生命周期同步的 `TryPush`；关闭先禁止新生产者，再等待已进入生产者完成并 drain/recycle 消息，不要在外部仍持有 Actor 指针时直接关闭通知 channel 和清空链表。

[返回问题清单](#-问题汇总清单)

---

<a id="p1-7-deadline-after-decode"></a>
#### 🟡 **中等问题 P1-7: Body 解码后未再次检查 deadline**

**提交**: WORKTREE  
**作者**: 未提交工作区  
**文件位置**: `F:\actorgo-game\actorgo\net\actor\actor.go`（约第132-165行）  
**函数名**: `Actor.invokeTyped`

```go
if err := ctx.Err(); err != nil {
	p.finishTyped(message, contextFailure(err, "actor invoke"))
	return
}

payload, decodeFailure := p.mail.decodePayload(ctx, entry, message.Payload)
if decodeFailure != nil {
	p.finishTyped(message, decodeFailure)
	return
}

result = entry.invoke(ctx, payload)
```

**问题**: 只在解码前检查一次 context。大 JSON/PB body 解码期间 deadline 到期后，代码仍直接进入业务 handler。

**风险**: 调用方已经收到超时，扣费、写库等尚未开始的副作用仍可能执行。

**建议**: `decodePayload` 成功后、`entry.invoke` 前再次检查 `ctx.Err()`，并增加“解码期间到期”的回归测试。

[返回问题清单](#-问题汇总清单)

---

<a id="p2-2-method-kind"></a>
#### 🔵 **轻微问题 P2-2: 内部 Invoke/Notify 不校验注册方法 Kind**

**提交**: WORKTREE  
**作者**: 未提交工作区  
**文件位置**: `F:\actorgo-game\actorgo\net\actor\system.go`（约第142-158、207-224行）、`F:\actorgo-game\actorgo\net\actor\actor_mailbox.go`（约第13-17行）

```go
type methodEntry struct {
	invoke           cfacade.TypedInvoke
	requestType      reflect.Type
	supportsProtobuf bool
}
```

**问题**: `MethodTable.Dispatch` 检查 Request/Notify kind，但 `System.Invoke/Notify/InvokeTarget/NotifyTarget` 和 child mailbox 不检查。Notify handler 可被 Invoke，Request handler 也可被 Notify，后者会静默丢弃响应和错误。

**建议**: 在 `methodEntry` 和 `Message` 中保留期望 kind，执行前统一校验；顶层调用也可先用 `Methods().Kind` 快速拒绝。

[返回问题清单](#-问题汇总清单)

---

<a id="p2-3-self-invoke-bypass"></a>
#### 🔵 **轻微问题 P2-3: self-invoke 保护可通过公开 ActorSystem 绕过**

**提交**: WORKTREE  
**作者**: 未提交工作区  
**文件位置**: `F:\actorgo-game\actorgo\net\actor\actor.go`（约第349-358行）、`F:\actorgo-game\actorgo\net\actor\system.go`（约第142-149行）

```go
func (p *Actor) Invoke(ctx *cfacade.RequestContext, methodID uint32, payload any) *cfacade.InvokeResult {
	target, failure := p.system.methodTarget(methodID)
	if failure != nil {
		return failure
	}
	if target == p.PathString() {
		return cfacade.ErrorResult(cproto.StatusCode_STATUS_FAILED_PRECONDITION, "actor cannot synchronously invoke itself")
	}
	return p.system.InvokeTarget(ctx, target, methodID, payload)
}

func (p *System) Invoke(ctx *cfacade.RequestContext, methodID uint32, payload any) *cfacade.InvokeResult {
	target, failure := p.methodTarget(methodID)
	if failure != nil {
		return failure
	}
	return p.InvokeTarget(ctx, target, methodID, payload)
}
```

**问题**: 保护只在 `Actor.Invoke`。handler 内仍可调用 `a.App().ActorSystem().Invoke(...)` 或 `a.System().InvokeTarget(a.PathString(), ...)`，向自身 mailbox 入队并同步等待。

**建议**: 在内部 context 标记当前 ActorPath，由 `System.InvokeTarget` 统一拒绝 source == target；至少应在 API 文档和测试中覆盖公开 ActorSystem 的绕过路径。

[返回问题清单](#-问题汇总清单)

---

### 3. AGP、HTTP 与连接生命周期 (Priority: P1)

**提交**: WORKTREE  
**开发者**: 未提交工作区  
**文件范围**: `net/parser/`、`net/connector/`、`net/httpactor/`

#### 功能描述

新增 AGP TCP/WebSocket Server、Connection/ConnectionManager、固定 HTTP Actor 入口、握手/心跳/cancel、Session 绑定和统一写队列。

#### 问题发现

<a id="p1-4-notify-backpressure"></a>
#### 🟡 **中等问题 P1-4: 外部 Notify 可无限写入无界 Actor mailbox**

**提交**: WORKTREE  
**作者**: 未提交工作区  
**文件位置**: `F:\actorgo-game\actorgo\net\parser\connection.go`（约第175-189行）、`F:\actorgo-game\actorgo\net\httpactor\handler.go`（约第91-103行）、`F:\actorgo-game\actorgo\net\actor\queue.go`（约第39-53行）  
**函数名**: `Connection.process`、`Handler.ServeHTTP`、`queue.Push`

```go
case *cproto.Packet_Notify:
	if c.State() != ConnectionReady {
		return false
	}
	// ...
	c.handleNotify(packet, kind.Notify)
	return true
```

```go
if kind == cfacade.MethodNotify && result != nil && result.OK() {
	writer.WriteHeader(http.StatusAccepted)
	return
}
```

**问题**: `MaxInflight` 只限制 Request。AGP/HTTP Notify 直接投递到无容量上限的链式队列，成功只表示入队。

**风险**: 攻击者持续发送接近 4 MiB 的 Notify 时，单线程 Actor 消费速度低于入口速度，每条消息都会持有 body、context 和 metadata，最终耗尽进程内存。

**建议**: 在 mailbox 层实现原子有界 `TryPush`，让所有传输共用背压。满载时 HTTP 返回 429/`RESOURCE_EXHAUSTED`；AGP Notify 明确选择丢弃、限流或关闭滥用连接。

[返回问题清单](#-问题汇总清单)

---

<a id="p1-5-connector-start-stop"></a>
#### 🟡 **中等问题 P1-5: Stop 之后仍可能创建并保留监听端口**

**提交**: WORKTREE  
**作者**: 未提交工作区  
**文件位置**: `F:\actorgo-game\actorgo\net\parser\component.go`（约第55-66行）、`F:\actorgo-game\actorgo\net\connector\connector.go`（约第120-136行）、`F:\actorgo-game\actorgo\net\connector\ws_connector.go`（约第69-82行）、`F:\actorgo-game\actorgo\net\connector\tcp_connector.go`（约第48-62行）  
**函数名**: `Component.OnAfterInit`、`Connector.Stop`、`WSConnector.Start`、`TCPConnector.Start`

```go
connector.OnConnect(c.HandleConn)
go connector.Start()
```

```go
listener, err := w.GetListener(w.certFile, w.keyFile, w.address)
// ...
w.Connector.Start()
http.Serve(listener, w)
```

**问题**: 连接器异步启动。如果 `Stop` 先运行，它会在 `listener == nil` 时永久设置 stopped；迟到的 `Start` 仍创建 listener。WebSocket 随后无条件 `http.Serve`，TCP 虽因 `Running == false` 不 Accept，也没有关闭刚创建的 listener。`listener` 的读写也未同步。

**风险**: 应用已显示停机但端口仍被占用；WebSocket 甚至继续接受 Upgrade，且幂等 `Stop` 无法再次关闭该 listener。

**建议**: listener 创建、发布和 stopped 检查使用同一生命周期锁；创建后发现已停止必须立即 Close 并返回 `net.ErrClosed`，各 Start 方法只有在成功进入 running 状态后才 Serve/Accept。

[返回问题清单](#-问题汇总清单)

---

<a id="p1-6-bind-close-race"></a>
#### 🟡 **中等问题 P1-6: Connection 已关闭或 Draining 时 Bind 仍可成功**

**提交**: WORKTREE  
**作者**: 未提交工作区  
**文件位置**: `F:\actorgo-game\actorgo\net\parser\connection_manager.go`（约第61-79行）、`F:\actorgo-game\actorgo\net\parser\connection.go`（约第435-455行）  
**函数名**: `ConnectionManager.Bind`、`Connection.Close`

```go
connection := m.connections[id]
if connection == nil {
	return fmt.Errorf("actorgo agp: connection %q not found", id)
}
// ...
connection.bind(uid, data)
m.byUID[uid] = id
return nil
```

```go
c.state.Store(int32(ConnectionClosed))
// ...
if c.manager != nil {
	c.manager.Remove(c.id)
}
```

**问题**: state 转换和 manager 索引没有共同的同步协议。`Close` 已设置 `Closed` 但尚未 `Remove`，或 `GoAway` 处于 `Draining` 时，`Bind` 仍可找到连接并成功返回。

**风险**: 登录逻辑认为绑定成功，但连接马上被删除，断线清理与“上线”业务状态可能乱序，形成幽灵在线状态或无响应登录。

**建议**: 提供与 Close 协调的 `bindIfReady`，在同一生命周期锁下确认状态仍为 `ConnectionReady` 并完成索引更新；单次无锁状态预检查不够。

[返回问题清单](#-问题汇总清单)

---

<a id="p2-1-inflight-overload"></a>
#### 🔵 **轻微问题 P2-1: 超过 MaxInflight 会关闭连接并取消全部请求**

**提交**: WORKTREE  
**作者**: 未提交工作区  
**文件位置**: `F:\actorgo-game\actorgo\net\parser\connection.go`（约第168-173、304-313行）

```go
if !c.reserve(kind.Request.RequestId, cancel) {
	cancel()
	return false
}
```

**问题**: 重复 request ID 和超过并发上限共用 bool 失败路径；两者都会让 `Run` 关闭连接并取消所有在途请求。握手也没有公布 `MaxInflight`。

**建议**: `reserve` 返回明确原因：重复 ID 作为协议错误关闭；容量不足只给当前请求返回 `RESOURCE_EXHAUSTED` 并保留连接；在握手或协议文档中公布限制。

[返回问题清单](#-问题汇总清单)

---

<a id="p2-5-http-notify-request-id"></a>
#### 🔵 **轻微问题 P2-5: HTTP Notify 的 202 响应缺少请求 ID**

**提交**: WORKTREE  
**作者**: 未提交工作区  
**文件位置**: `F:\actorgo-game\actorgo\net\httpactor\handler.go`（约第76-103行）

```go
requestIDText := strconv.FormatUint(uint64(requestID), 10)
// ...
if kind == cfacade.MethodNotify && result != nil && result.OK() {
	writer.WriteHeader(http.StatusAccepted)
	return
}
```

**问题**: 成功 Request 和错误响应都会设置 `X-ActorGo-Request-ID`，成功 Notify 已生成 ID 却在 202 分支丢失。Notify 无响应体，该 Header 是 curl 调用方关联异步日志的主要依据。

**建议**: 在写 202 前设置 `X-ActorGo-Request-ID`，并增加响应 Header 测试。

[返回问题清单](#-问题汇总清单)

---

### 4. Application 与 NATS Cluster 生命周期 (Priority: P2)

**提交**: WORKTREE  
**开发者**: 未提交工作区  
**文件范围**: `application.go`、`net/cluster/`

#### 功能描述

Application 现在强制 ActorSystem 优先初始化；NATS 集群增加有界 worker queue、过载响应、ClusterMessage 和停止排空逻辑。

#### 问题发现

<a id="p2-4-shutdown-order"></a>
#### 🔵 **轻微问题 P2-4: ActorSystem 最后停止，Actor.OnStop 已失去集群依赖**

**提交**: WORKTREE  
**作者**: 未提交工作区  
**文件位置**: `F:\actorgo-game\actorgo\application.go`（约第165-169、239-255行）

```go
if a.Find(a.actorSystem.Name()) == nil {
	a.components = append([]cfacade.IComponent{a.actorSystem}, a.components...)
}

for i := len(a.components) - 1; i >= 0; i-- {
	a.components[i].OnStop()
}
```

**问题**: Cluster 模式顺序为 `[actorSystem, cluster, discovery, custom...]`，反向停止成为 `custom → discovery → cluster → actorSystem`。ActorSystem.Stop 才会调用业务 Actor 的 `OnStop`，此时 NATS 已关闭。

**建议**: 显式分阶段为“关闭 ingress → 停 Actor → 停 discovery/cluster”，或引入依赖拓扑，不要用单一 slice 同时表达初始化和停止依赖。

[返回问题清单](#-问题汇总清单)

---

<a id="p2-6-cluster-subscribe-error"></a>
#### 🔵 **轻微问题 P2-6: 集群订阅失败仍宣告初始化成功**

**提交**: WORKTREE  
**作者**: 未提交工作区  
**文件位置**: `F:\actorgo-game\actorgo\net\cluster\nats_cluster\cluster.go`（约第47-53、72-78行）

```go
c.subscribe(GetRemoteSubject(c.prefix, c.app.NodeType(), c.app.NodeID()))
clog.Info("NATS ClusterMessage cluster initialized")
```

```go
if err != nil {
	clog.Error("cluster subscribe failed. [subject = %s, err = %v]", subject, err)
	return
}
```

**问题**: Subscribe 错误只记日志，`Init` 无条件继续并宣告成功。节点可发布却没有远程消费入口，所有远程请求只能超时。

**建议**: `subscribe` 返回 error；初始化失败时停止 workers、关闭新建连接并让 Application 启动失败。后续可把组件生命周期统一改为返回 error，以支持回滚。

[返回问题清单](#-问题汇总清单)

---

## ⭐ 代码亮点

1. **协议边界清晰**（未提交工作区 - WORKTREE）
   - AGP `Packet` 用 oneof 明确 Request/Response/Notify，外层固定 Protobuf，业务 Body 只允许 JSON/PB，客户端不再携带 ActorPath。

2. **方法注册已收敛**（未提交工作区 - WORKTREE）
   - `Methods().Register(MethodID, handler)` 只在初始化期反射适配一次；顶层 MethodID 全局冲突、保留 ID 和 child 外部暴露均有校验。

3. **请求上下文所有权明显改善**（未提交工作区 - WORKTREE）
   - Notify 使用 detached 有界 context，由 Message 回收时取消；Session、Metadata 在异步/跨节点边界进行复制。

4. **过期排队请求已有防护**（未提交工作区 - WORKTREE）
   - mailbox 执行前检查 context，新增测试覆盖调用方超时后排队消息不再执行。

5. **连接写入串行化**（未提交工作区 - WORKTREE）
   - TCP/WebSocket 共用单 writer queue，避免 WebSocket 并发写；帧长、Body 和 Metadata 均有上限。

6. **NATS 入口有界化**（未提交工作区 - WORKTREE）
   - 固定 worker 数和有界 queue 避免单个慢 Actor 串行阻塞整个订阅回调，Request 过载会返回 `RESOURCE_EXHAUSTED`。

7. **敏感错误已收口**（未提交工作区 - WORKTREE）
   - 普通 handler error 只在服务端记录，对外固定返回 `internal error`；显式 `InvokeError` 才携带业务可见状态。

---

## 🔧 改进建议

### 代码规范

先修复节点 ID 表示和生命周期并发问题，再继续做 API 精简。当前 `InvokeTarget/NotifyTarget` 等底层入口仍需明确“仅框架内部”边界。

### 测试覆盖

- 增加 `profile.Init` + 实际点分配置的测试，不能只测 `GetNodeWithConfig`。
- 增加并发创建同一 child、Stop 与 mailbox Push、Connector Stop-before-Start 的确定性测试。
- 增加 Notify mailbox 满载、MaxInflight 超限不掉线、Bind 与 Close 并发测试。
- Cluster/Discovery/NATS 当前没有包级测试，应至少用嵌入式 NATS 覆盖 subscribe、request/reply、过载和停机。

### 工程实践

Application 组件生命周期建议从“按 slice 正序启动、反序停止”升级为显式阶段或依赖拓扑，并让 `Init/Start` 返回 error，以便启动失败时回滚已创建的 goroutine、listener 和订阅。

### 安全性

HTTP/AGP Notify 必须在框架层有统一的容量和限流边界；不能依赖部署方一定配置反向代理限流。

---

## ✅ 验证结果

- `go test ./facade ./net/actor ./net/method ./net/parser ./net/connector ./net/httpactor ./net/proto ./net/serializer ./profile ./net/cluster/... ./net/discovery/... ./net/nats/... -count=1 -timeout 90s`：通过。
- `go vet ./facade ./net/actor ./net/method ./net/parser ./net/connector ./net/httpactor ./net/proto ./net/serializer ./profile ./net/cluster/nats_cluster ./net/discovery ./net/nats`：通过。
- `go test ./... -count=1 -timeout 30s`：未全绿；失败位于本次协议改造未触及的历史包 `components/cron`、`extend/mapstructure`、`extend/snowflake`、`extend/time_wheel`。本报告未把这些历史失败归因于当前修改。
- `go test -race`：当前机器缺少 C 编译器 `gcc`，无法构建 `runtime/cgo`，未获得 race 结果。
- `git diff --check`：未发现空白错误；Git 提示多个 LF 文件下次写入会转换为 CRLF，应在提交前确认仓库换行策略。

---

## 附录：完整提交列表

### WORKTREE

**作者**: 未提交工作区  
**时间**: 2026-08-15 17:32:48  
**说明**: 相对 `68402dd` 的本地 AGP/PB、Actor MethodID、HTTP、连接与集群改造  
**变更规模**: 72 个 tracked 文件，1856 insertions，6219 deletions；另有未跟踪源码、测试和文档  
**主要变更文件**:

- M `application.go`、`actorgo.go`、`facade/actor.go`、`facade/application.go`、`facade/cluster.go`、`facade/message.go`
- A `facade/codec.go`、`facade/invoke_result.go`、`facade/method_table.go`、`facade/request_context.go`
- M `net/actor/actor.go`、`net/actor/actor_mailbox.go`、`net/actor/system.go`、`net/actor/component.go`
- A `net/method/invoker.go`、`net/method/table.go`
- A `net/parser/component.go`、`connection.go`、`connection_manager.go`、`packet_transport.go`、`tcp_packet_framer.go`
- M `net/connector/connector.go`、`tcp_connector.go`、`ws_connector.go`
- A `net/httpactor/component.go`、`handler.go`、`options.go`、`components/gin/actor_handler.go`
- A `net/proto/agp.proto`、`cluster_message.proto`、`agp_codec.go`、`agp_validator.go`
- M `net/proto/proto.proto`、`net/serializer/json.go`、`net/serializer/protobuf.go`
- A `net/serializer/registry.go`
- M `net/cluster/nats_cluster/cluster.go`、`net/discovery/discovery_nats.go`、`net/nats/connect.go`
- M `profile/node.go`
- D `net/parser/pomelo/**`、`net/parser/simple/**`、`facade/net_parser.go`、`net/actor/invoke.go`
- A/M `_docs/**`、`README.md`、`docs/superpowers/**`
- A/M 对应的 `*_test.go`、生成的 `*.pb.go` 和协议构建脚本

---

## 📞 联系信息

如对本报告有疑问，请联系：

- **Code Review 负责人**: Codex
- **审查依据**: 当前 WORKTREE 最终状态、真实 diff、最终源码上下文、定向测试与静态检查
