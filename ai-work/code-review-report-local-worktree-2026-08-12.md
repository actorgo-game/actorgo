# Code Review Report - 本地 AGP/PB 最终协议改造

**审查日期**: 2026-08-12  
**审查范围**: `F:\actorgo-game\actorgo` 相对 `68402dd`、`D:\game\actorgo\actorgo-examples` 相对 `09d9eb2` 的全部未提交改动  
**提交总数**: 0 个（2 个未提交工作区）  
**涉及开发人员**: 本地未提交改动（当前 Git 配置：`burncomyang`、`actorgo-game`）

---

## 📊 执行摘要

### 总体评估

- 改造方向是合理的：删除 Pomelo/Simple，统一为轻量 AGP 包层，业务体支持 JSON/PB，外部请求仅携带 `MethodID`，Actor 通过 `Register(MethodID, handler)` 注册。
- 框架仓库可以全量编译，关键包测试和 `go vet` 均通过；`demo_cluster`、`demo_chat` 也可独立编译。
- 当前仍不适合直接合并。主要问题不是协议字段复杂度，而是请求/通知的上下文所有权、Actor 单线程重入、集群回调串行化、连接停机竞态，以及示例业务状态字段混用。
- 本报告排除了自动生成的 `*.pb.go` 的人工逻辑审查，但验证了它们参与的编译结果。

### 问题统计

- 🔴 **严重问题**: 0 个
- 🟡 **中等问题**: 13 个
- 🔵 **轻微问题**: 7 个
- ⭐ **代码亮点**: 5 处

---

## 📋 问题汇总清单

### 🔴 严重问题（P0 - 立即修复）

无。

### 🟡 中等问题（P1 - 短期内修复）

| ID | 模块 | 问题描述 | 提交 | 负责人 | 详情 |
|----|------|----------|------|--------|------|
| P1-1 | Actor | 超时请求仍会执行 | WORKTREE@68402dd | 本地未提交 | [查看详情](#p1-1-stale-request-executes) |
| P1-2 | Actor/Transport | Notify 上下文过早取消 | WORKTREE@68402dd | 本地未提交 | [查看详情](#p1-2-notify-context-cancelled) |
| P1-3 | Actor | 同 Actor Invoke 自阻塞 | WORKTREE@68402dd | 本地未提交 | [查看详情](#p1-3-self-invoke-deadlock) |
| P1-4 | Method/HTTP | 内部错误文本泄露 | WORKTREE@68402dd | 本地未提交 | [查看详情](#p1-4-internal-error-leak) |
| P1-5 | Cluster | 慢请求阻塞 NATS 入口 | WORKTREE@68402dd | 本地未提交 | [查看详情](#p1-5-nats-serial-callback) |
| P1-6 | Parser/Connector | 停机期间仍接收并泄漏连接 | WORKTREE@68402dd | 本地未提交 | [查看详情](#p1-6-shutdown-connection-race) |
| P1-7 | Parser | OnDisconnect 可永久阻塞停机 | WORKTREE@68402dd | 本地未提交 | [查看详情](#p1-7-disconnect-blocks-shutdown) |
| P1-8 | demo_cluster | 区服 ID 与运行时 NodeID 混用 | WORKTREE@09d9eb2 | 本地未提交 | [查看详情](#p1-8-server-node-id-conflict) |
| P1-9 | demo_cluster | Web 客户端未发送心跳 | WORKTREE@09d9eb2 | 本地未提交 | [查看详情](#p1-9-web-client-no-heartbeat) |
| P1-10 | demo_chat | 断线不清理且 Exit 可越权 | WORKTREE@09d9eb2 | 本地未提交 | [查看详情](#p1-10-chat-exit-authorization) |
| P1-11 | examples | 全仓无法编译 | WORKTREE@09d9eb2 | 本地未提交 | [查看详情](#p1-11-examples-build-broken) |
| P1-12 | examples | go.mod 含本机绝对路径 | WORKTREE@09d9eb2 | 本地未提交 | [查看详情](#p1-12-absolute-replace) |
| P1-13 | demo_cluster | Snowflake 节点号不唯一 | WORKTREE@09d9eb2 | 本地未提交 | [查看详情](#p1-13-snowflake-node-collision) |

### 🔵 轻微问题（P2 - 优化建议）

| ID | 模块 | 问题描述 | 提交 | 负责人 | 详情 |
|----|------|----------|------|--------|------|
| P2-1 | Actor/Cluster | 本地与远端 Payload 类型不一致 | WORKTREE@68402dd | 本地未提交 | [查看详情](#p2-1-payload-type-inconsistent) |
| P2-2 | HTTP/Actor | HTTP 超时配置被 3 秒截断 | WORKTREE@68402dd | 本地未提交 | [查看详情](#p2-2-timeout-contract-mismatch) |
| P2-3 | Actor | 运行期 Register 有 map 竞态 | WORKTREE@68402dd | 本地未提交 | [查看详情](#p2-3-runtime-register-race) |
| P2-4 | Parser | Session.Ip 包含端口 | WORKTREE@68402dd | 本地未提交 | [查看详情](#p2-4-session-ip-port) |
| P2-5 | Parser | 空 Limits 在 TCP/WS 行为不同 | WORKTREE@68402dd | 本地未提交 | [查看详情](#p2-5-empty-limits-inconsistent) |
| P2-6 | demo_chat | 客户端仍编码已删除的 target | WORKTREE@09d9eb2 | 本地未提交 | [查看详情](#p2-6-stale-target-field) |
| P2-7 | examples | 格式及原注释保留未收口 | WORKTREE@09d9eb2 | 本地未提交 | [查看详情](#p2-7-format-comments) |

---

## 🎯 主要功能模块分析

### 1. Actor 调用与上下文生命周期 (Priority: P1)

**提交**: WORKTREE@68402dd  
**开发者**: 本地未提交  
**文件范围**: `facade/`、`net/actor/`、`net/method/`、`net/httpactor/`

#### 功能描述

- 以 `MethodID` 取代字符串函数名和外部 ActorPath。
- 将 handler 适配为统一 `TypedInvoke`，在 Actor mailbox 内完成 JSON/PB 解码。
- 支持本地、指定节点、动态子 Actor 的 Request/Notify。

<a id="p1-1-stale-request-executes"></a>
#### 🟡 **中等问题 P1-1: 调用方超时后，排队请求仍会执行**

**提交**: WORKTREE@68402dd  
**作者**: 本地未提交（Git 配置：burncomyang）  
**文件位置**: `F:\actorgo-game\actorgo\net\actor\system.go`（约第186-205行）、`F:\actorgo-game\actorgo\net\actor\actor.go`（约第141-164行）  
**函数名**: `System.InvokeTarget`、`Actor.invokeTyped`

```go
message := typedMessage(ctx, target, methodID, payload)
resultCh := make(chan *cfacade.InvokeResult, 1)
message.ChanInvokeResult = resultCh
if !p.post(message) {
	message.Recycle()
	return cfacade.ErrorResult(cproto.StatusCode_STATUS_NOT_FOUND, "actor not found")
}

timer := time.NewTimer(p.callTimeout)
defer timer.Stop()
select {
case result := <-resultCh:
	return result
case <-ctx.Context.Done():
	return cfacade.ErrorResult(cproto.StatusCode_STATUS_CANCELLED, "actor invoke cancelled")
case <-timer.C:
	return cfacade.ErrorResult(cproto.StatusCode_STATUS_DEADLINE_EXCEEDED, "actor invoke timeout") // ❌ 只返回，不撤销已排队消息
}
```

```go
payload, decodeFailure := p.mail.decodePayload(ctx, entry, message.Payload)
if decodeFailure != nil {
	p.finishTyped(message, decodeFailure)
	return
}

// ❌ 调用 handler 前没有检查 ctx.Err()
result = entry.invoke(ctx, payload)
```

**问题**: Actor 忙碌时，调用方从 deadline 或框架 `callTimeout` 返回，但消息仍留在 mailbox。Actor 稍后会无条件执行 handler。`InvokeChild` 也存在相同行为。

**风险**: 客户端收到超时后重试，原请求仍可能稍后扣费、发奖或写库，形成重复副作用和状态不一致。

**建议**: 为每次 Invoke 创建 Actor 自己拥有的可取消 context；调用超时时取消；Actor 真正执行前再次检查 `ctx.Err()`，对尚未开始的过期消息直接结束。

```go
if err := ctx.Err(); err != nil {
	p.finishTyped(message, contextFailure(err))
	return
}
```

[返回问题清单](#-问题汇总清单)

---

<a id="p1-2-notify-context-cancelled"></a>
#### 🟡 **中等问题 P1-2: Notify 的上下文在 Actor 执行前被取消**

**提交**: WORKTREE@68402dd  
**作者**: 本地未提交（Git 配置：burncomyang）  
**文件位置**: `F:\actorgo-game\actorgo\net\parser\connection.go`（约第251-255行）、`F:\actorgo-game\actorgo\net\httpactor\handler.go`（约第91-101行）、`F:\actorgo-game\actorgo\net\cluster\nats_cluster\cluster.go`（约第61-84行）  
**函数名**: `Connection.handleNotify`、`Handler.ServeHTTP`、`Cluster.process`

```go
func (c *Connection) handleNotify(packet *cproto.Packet, notify *cproto.Notify) {
	ctx, cancel := c.requestContext(packet, 0, 0)
	defer cancel() // ❌ Dispatch 只入队；函数返回时 Actor 通常尚未执行

	result := c.methods.Dispatch(ctx, notify.MethodId, notify.Body, cfacade.MethodNotify)
	if result != nil && !result.OK() {
		clog.Warn("agp notify rejected. [connectionID = %s, methodID = %d, code = %d]", c.id, notify.MethodId, result.Code)
	}
}
```

**问题**: `NotifyTarget` 成功入队就返回，入口函数随后立即 `cancel()`。HTTP 与 NATS Notify 同样把入口 context 直接交给异步 mailbox。

**风险**: Notify handler 中使用 context 的数据库、HTTP/RPC 操作会收到 `context canceled`；加入 P1-1 的执行前取消检查后，现有 Notify 甚至会全部被丢弃。

**建议**: Notify 入队时复制 `RequestContext` 的 Session/Metadata/Codec，并创建独立、有界的 context；cancel 由 Actor 在消息处理完成后调用，不由入口函数释放。

```go
notifyCtx, cancel := cloneAsyncContext(ctx, notifyTimeout)
message.Context = notifyCtx
message.Cancel = cancel // finishTyped/notify finish 时释放
```

[返回问题清单](#-问题汇总清单)

---

<a id="p1-3-self-invoke-deadlock"></a>
#### 🟡 **中等问题 P1-3: Actor 在自身 handler 内 Invoke 会确定性自阻塞**

**提交**: WORKTREE@68402dd  
**作者**: 本地未提交（Git 配置：burncomyang）  
**文件位置**: `F:\actorgo-game\actorgo\net\actor\actor.go`（约第344-347行）、`F:\actorgo-game\actorgo\net\actor\system.go`（约第186-205行）  
**函数名**: `Actor.Invoke`、`System.InvokeTarget`

```go
// Invoke calls a top-level Actor method on the current node by MethodID.
func (p *Actor) Invoke(ctx *cfacade.RequestContext, methodID uint32, payload any) *cfacade.InvokeResult {
	return p.system.Invoke(ctx, methodID, payload) // ❌ 若 MethodID 属于自己，会入自己的 mailbox 后同步等待
}
```

**问题**: Actor handler 正占用唯一消费 goroutine，新消息进入同一 mailbox 后当前 handler 同步等待结果，只有等默认 3 秒超时后 Actor 才能继续消费。旧 `CallWait` 明确拒绝 source 与 target 相同，新 API 丢失了保护。

**风险**: 确定性延迟、超时和超时后的延迟副作用。

**建议**: `Actor.Invoke` 在路由后发现目标等于 `p.PathString()` 时返回 `FAILED_PRECONDITION`；同 Actor 的逻辑复用抽成普通私有函数。

```go
if target == p.PathString() {
	return cfacade.ErrorResult(cproto.StatusCode_STATUS_FAILED_PRECONDITION, "actor cannot synchronously invoke itself")
}
```

[返回问题清单](#-问题汇总清单)

---

<a id="p1-4-internal-error-leak"></a>
#### 🟡 **中等问题 P1-4: 普通 handler error 原样返回客户端**

**提交**: WORKTREE@68402dd  
**作者**: 本地未提交（Git 配置：burncomyang）  
**文件位置**: `F:\actorgo-game\actorgo\net\method\invoker.go`（约第41-50行）  
**函数名**: `resultFromError`

```go
func resultFromError(err error) *cfacade.InvokeResult {
	if err == nil {
		return cfacade.OKResult(nil)
	}
	var invokeError *InvokeError
	if errors.As(err, &invokeError) {
		return cfacade.ErrorResult(invokeError.Code, invokeError.Message)
	}
	return cfacade.ErrorResult(cproto.StatusCode_STATUS_INTERNAL, err.Error()) // ❌
}
```

**问题**: 数据库、文件系统或网络库的原始错误通过 AGP/HTTP `message` 直接返回。

**风险**: 泄露数据库结构、内部地址、文件路径或凭证片段。

**建议**: 普通 error 只在服务端记录完整文本，对客户端固定返回 `internal error`；只有显式 `InvokeError` 可以携带业务可见信息。

```go
clog.Error("actor handler failed: %v", err)
return cfacade.ErrorResult(cproto.StatusCode_STATUS_INTERNAL, "internal error")
```

[返回问题清单](#-问题汇总清单)

---

### 2. 集群与连接生命周期 (Priority: P1)

**提交**: WORKTREE@68402dd  
**开发者**: 本地未提交  
**文件范围**: `net/cluster/nats_cluster/`、`net/parser/`、`net/connector/`

<a id="p1-5-nats-serial-callback"></a>
#### 🟡 **中等问题 P1-5: 单个慢 Actor 请求会阻塞整个节点的 NATS 入口**

**提交**: WORKTREE@68402dd  
**作者**: 本地未提交（Git 配置：burncomyang）  
**文件位置**: `F:\actorgo-game\actorgo\net\cluster\nats_cluster\cluster.go`（约第40-42、64-80行）  
**函数名**: `Cluster.subscribe`、`Cluster.process`

```go
func (c *Cluster) subscribe(subject string) {
	err := cnats.GetConnect().Subscribe(subject, func(message *nats.Msg) { c.process(message) })
	if err != nil {
		clog.Error("cluster subscribe failed. [subject = %s, err = %v]", subject, err)
	}
}

// process 中同步等待 Actor Invoke
result = methods.Dispatch(ctx, envelope.MethodId, envelope.Payload, kind) // ❌
```

**问题**: nats.go 的同一异步 Subscription 逐条调用 callback，`process` 又同步等待 Actor Request。一个慢 handler 会阻止下一条消息进入任何其他 Actor。

**风险**: 节点级 head-of-line blocking，积压触发 NATS pending limit，最终造成跨节点批量超时。

**建议**: 订阅 callback 只完成校验并投递到有界 worker 队列；按容量并发处理 Request。不要为每条消息无限制启动 goroutine。

```go
func (c *Cluster) onMessage(msg *nats.Msg) {
	select {
	case c.work <- msg:
	default:
		c.replyResourceExhausted(msg)
	}
}
```

[返回问题清单](#-问题汇总清单)

---

<a id="p1-6-shutdown-connection-race"></a>
#### 🟡 **中等问题 P1-6: 停机期间仍接收请求，并可能遗留新 socket**

**提交**: WORKTREE@68402dd  
**作者**: 本地未提交（Git 配置：burncomyang）  
**文件位置**: `F:\actorgo-game\actorgo\net\parser\component.go`（约第77-101行）、`F:\actorgo-game\actorgo\net\connector\connector.go`（约第38-48行）、`F:\actorgo-game\actorgo\net\parser\connection.go`（约第142-161行）  
**函数名**: `Component.OnBeforeStop`、`Connector.InChan`、`Connection.process`

```go
func (c *Component) OnBeforeStop() {
	// ❌ listener 此时还未 Stop，新连接仍可在 CloseAll 快照后加入
	var wait sync.WaitGroup
	c.connections.Range(func(connection *Connection) bool {
		wait.Add(1)
		go func() { defer wait.Done(); _ = connection.GoAway(0, 0) }()
		return true
	})
	// ...
	c.connections.CloseAll()
}

func (c *Connector) InChan(conn net.Conn) {
	select {
	case c.connChan <- conn: // ❌ done 已关闭且 channel 有容量时，Go 可能随机选中该分支
	case <-c.done:
		_ = conn.Close()
	}
}
```

**问题**: Connector 到 `OnStop` 才停止；`OnBeforeStop` 的连接快照之后仍可接入新 socket。`Draining` 状态也未在业务 Request 分支拒绝请求。Stop 与 InChan 竞态下，socket 还可能被投递到已退出的 dispatcher。

**风险**: 优雅停机期间继续执行新业务、泄漏连接，并导致进程退出行为不确定。

**建议**: `OnBeforeStop` 第一件事先幂等停止 listener；随后统一标记所有连接 draining 并拒绝业务帧；再对稳定快照 GoAway/Close。Connector 关闭时必须 drain 并关闭排队 socket，不能依赖 select 优先级。

```go
for _, connector := range c.connectors {
	connector.Stop()
}
c.connections.MarkDraining()
c.connections.GoAwayAndClose(c.options.WriteTimeout)
```

[返回问题清单](#-问题汇总清单)

---

<a id="p1-7-disconnect-blocks-shutdown"></a>
#### 🟡 **中等问题 P1-7: OnDisconnect 位于 closeOnce 临界段，可让停机永久阻塞**

**提交**: WORKTREE@68402dd  
**作者**: 本地未提交（Git 配置：burncomyang）  
**文件位置**: `F:\actorgo-game\actorgo\net\parser\connection.go`（约第421-449行）  
**函数名**: `Connection.Close`

```go
func (c *Connection) Close() {
	c.closeOnce.Do(func() {
		// ... cancel、close(done)、socket.Close
		if c.options.OnDisconnect != nil {
			func() {
				defer func() { /* recover */ }()
				c.options.OnDisconnect(c) // ❌ 用户回调同步占用 sync.Once.Do
			}()
		}
		if c.manager != nil {
			c.manager.Remove(c.id)
		}
	})
}
```

**问题**: `OnBeforeStop` 的 GoAway goroutine即使超时返回等待，后续 `CloseAll` 再次调用 `Close` 时会阻塞等待第一次 `sync.Once.Do` 完成。demo_cluster 的回调还执行跨节点 SessionClose RPC，是实际慢路径。

**风险**: 一个卡住的业务回调即可让整个应用停机永久挂住。

**建议**: `closeOnce` 内只做不可阻塞的状态清理、socket close 和 manager remove；先复制 Session，再把用户回调交给独立 once/有界执行器。

```go
snapshot := c.Session()
c.closeOnce.Do(func() {
	close(c.done)
	_ = c.conn.Close()
	c.manager.Remove(c.id)
})
c.disconnectOnce.Do(func() { c.disconnectExecutor.Submit(snapshot) })
```

[返回问题清单](#-问题汇总清单)

---

### 3. 示例集成与业务一致性 (Priority: P1)

**提交**: WORKTREE@09d9eb2  
**开发者**: 本地未提交  
**文件范围**: `D:\game\actorgo\actorgo-examples\demo_cluster\`、`demo_chat\`、`go.mod`

<a id="p1-8-server-node-id-conflict"></a>
#### 🟡 **中等问题 P1-8: 业务区服 ID 与 ActorGo 运行时 NodeID 使用同一个 Session key**

**提交**: WORKTREE@09d9eb2  
**作者**: 本地未提交（Git 配置：actorgo-game）  
**文件位置**: `D:\game\actorgo\actorgo-examples\demo_cluster\nodes\gate\actor_user.go`（约第61-64行）、`D:\game\actorgo\actorgo-examples\demo_cluster\nodes\game\module\player\actor_player.go`（约第80行）  
**函数名**: `ActorUser.login`、`actorPlayer.playerCreate`

```go
sessionData := map[string]string{
	sessionKey.ServerID: constant.GameNodeID(req.ServerId), // ❌ 保存的是 64 位运行时 NodeID
	sessionKey.PID:      cstring.ToString(userToken.PID),
	sessionKey.OpenID:   userToken.OpenID,
}
```

```go
serverId := ctx.Session.GetInt32(sessionKey.ServerID) // ❌ 仍按 int32 业务区服 ID 读取
newPlayerTable, errCode := db.CreatePlayer(ctx.Session, req.PlayerName, serverId, playerInitRow)
```

**问题**: `sessionKey.ServerID` 注释和原业务含义是 `int32 游戏服务器ID`，但登录后改存 `1.10001.5.1` 对应的十进制运行时 NodeID `18701661997629441`。`GetInt32` 解析失败得到 0。

**风险**: 创角会把 `PlayerTable.ServerId`、`MergedServerId` 写为 0，后续按服查询和合服逻辑错误。

**建议**: 拆分两个 key：`ServerID` 保存 `req.ServerId`，`GameNodeID` 保存 `constant.GameNodeID(req.ServerId)`；路由只读后者，业务表只读前者。

```go
sessionData := map[string]string{
	sessionKey.ServerID:     strconv.FormatInt(int64(req.ServerId), 10),
	sessionKey.GameNodeID:   constant.GameNodeID(req.ServerId),
	// ...
}
```

[返回问题清单](#-问题汇总清单)

---

<a id="p1-9-web-client-no-heartbeat"></a>
#### 🟡 **中等问题 P1-9: demo_cluster Web AGP 客户端未实现心跳**

**提交**: WORKTREE@09d9eb2  
**作者**: 本地未提交（Git 配置：actorgo-game）  
**文件位置**: `D:\game\actorgo\actorgo-examples\demo_cluster\nodes\web\static\agp-client.js`（约第123-180行）  
**函数名**: `AGPClient.prototype.connect`、`AGPClient.prototype.handshake`

```javascript
this.ws.onopen = function () {
  self.handshake().then(function () { cb && cb(); }).catch(function (err) {
    console.error(err);
  });
};

AGPClient.prototype.handshake = function () {
  var body = new Uint8Array(tag(1, 0).concat(uvarint(1)));
  return this.requestRaw(1, body, 10000).then(function () { return true; }); // ❌ 丢弃 HandshakeResponse
};
```

**问题**: 客户端没有解析 `HeartbeatIntervalMs`，也没有定时发送 MethodID 2。

**风险**: 框架默认 `IdleTimeout=90s`，网页完成登录后空闲约 90 秒即被服务端关闭。

**建议**: 解析握手响应，按服务端返回周期发送 PB `HeartbeatRequest`，close 时清理 timer。

```javascript
return this.requestRaw(1, body, 10000).then(function (response) {
  var hs = decodeMessage(response.body);
  self.startHeartbeat(hs[2] || 30000);
  return hs;
});
```

[返回问题清单](#-问题汇总清单)

---

<a id="p1-10-chat-exit-authorization"></a>
#### 🟡 **中等问题 P1-10: demo_chat 异常断线不清理，Exit 可删除其他用户**

**提交**: WORKTREE@09d9eb2  
**作者**: 本地未提交（Git 配置：actorgo-game）  
**文件位置**: `D:\game\actorgo\actorgo-examples\demo_chat\room\main.go`（约第20-24行）、`D:\game\actorgo\actorgo-examples\demo_chat\room\actor_room.go`（约第76-88行）  
**函数名**: `main`、`actorRoom.exit`

```go
// 创建 AGP 服务组件（替代旧的 pomelo parser）
agpServer := parser.New("chat", []cfacade.IConnector{wsConnector}) // ❌ 没有 WithOnDisconnect
```

```go
func (p *actorRoom) exit(ctx *cfacade.RequestContext, req *ExitRequest) error {
	uid := req.UID // ❌ 优先信任外部请求中的 UID
	if uid < 1 && ctx != nil && ctx.Session != nil {
		uid = ctx.Session.Uid
	}
	if uid < 1 {
		return nil
	}
	delete(p.userMap, uid)
	return nil
}
```

**问题**: 旧 Pomelo `AddOnClose` 的清理没有迁移，断网/浏览器崩溃不会删除 `userMap`；主动 Exit 又允许客户端提交任意 UID。

**风险**: 内存和在线状态长期残留；任意连接可以删除其他用户的房间状态。

**建议**: 外部 Exit 只能使用 `ctx.Session.Uid`；配置 `WithOnDisconnect`，用内部上下文向 Actor 投递清理通知。

```go
uid := int64(0)
if ctx != nil && ctx.Session != nil {
	uid = ctx.Session.Uid
}
// 不读取 req.UID
```

[返回问题清单](#-问题汇总清单)

---

<a id="p1-11-examples-build-broken"></a>
#### 🟡 **中等问题 P1-11: 框架最终 API 已切换，但 examples 全仓未完成同步**

**提交**: WORKTREE@09d9eb2  
**作者**: 本地未提交（Git 配置：actorgo-game）  
**文件位置**: `D:\game\actorgo\actorgo-examples\test_gob\gob_test.go`（第13行）、`test_actor\actor.go`（第27行）、`test_actor\child_actor.go`（第16行）、`test_actor\main.go`（约第14-18行）及多个旧示例  
**函数名**: 仓库级构建

```text
test_gob\gob_test.go:13:2: no required module provides package .../net/parser/pomelo/message
test_actor\actor.go:27:4: p.CallWait undefined
test_actor\child_actor.go:16:23: cannot use "hello" as uint32 value in argument to p.Methods().Register
test_actor\main.go:18:3: too many arguments in call to actorgo.Configure
```

**问题**: `go test -vet=off ./... -run '^$'` 失败；`demo_gorm`、`test_gin`、`test_discovery`、`test_data_config` 也保留旧四参数 `Configure/NewApp`。

**风险**: examples 仓库 CI 失败，使用者会继续复制已经删除的 API。

**建议**: 既然不保留迁移兼容，应一次性升级或删除所有旧示例，合并门槛设为 examples 全仓编译通过。

```text
go test -vet=off ./... -run '^$' -count=1
```

[返回问题清单](#-问题汇总清单)

---

<a id="p1-12-absolute-replace"></a>
#### 🟡 **中等问题 P1-12: examples/go.mod 提交了本机绝对 replace**

**提交**: WORKTREE@09d9eb2  
**作者**: 本地未提交（Git 配置：actorgo-game）  
**文件位置**: `D:\game\actorgo\actorgo-examples\go.mod`（约第20行）  
**函数名**: Go module 解析

```go
replace github.com/actorgo-game/actorgo => F:\actorgo-game\actorgo // ❌ 仅本机有效
```

**问题**: 该路径在 Linux、CI 和其他开发者机器均不存在。

**风险**: 仓库在离开当前电脑后不可复现构建。

**建议**: 本地联调使用不提交的 `go.work`；提交时引用已发布版本或由 CI 显式创建 workspace。

```text
go work init ./actorgo ./actorgo-examples
```

[返回问题清单](#-问题汇总清单)

---

<a id="p1-13-snowflake-node-collision"></a>
#### 🟡 **中等问题 P1-13: Snowflake 节点号忽略进程实例维度**

**提交**: WORKTREE@09d9eb2  
**作者**: 本地未提交（Git 配置：actorgo-game）  
**文件位置**: `D:\game\actorgo\actorgo-examples\demo_cluster\nodes\game\game.go`（约第15-21行）  
**函数名**: `Run`

```go
numericNodeID, err := cfacade.GenNodeIdByStr(nodeID)
if err != nil {
	panic("node parameter must use bigWorld.world.type.instance format")
}
cherrySnowflake.SetDefaultNode(int64(cfacade.GetWorldId(numericNodeID))) // ❌ 仅使用 WorldId
```

**问题**: `1.10001.5.1` 与 `1.10001.5.2` 都使用 Snowflake node 10001。

**风险**: 同一区服多 game 实例并行生成 ID 时可能发生跨进程重复 ID。

**建议**: 从 `WorldId + NodeInst` 映射出 16 bit 内唯一值并校验，或通过部署配置显式分配 Snowflake node。

```go
snowflakeNode := allocateSnowflakeNode(cfacade.GetWorldId(numericNodeID), cfacade.GetNodeInst(numericNodeID))
cherrySnowflake.SetDefaultNode(int64(snowflakeNode))
```

[返回问题清单](#-问题汇总清单)

---

### 4. API 契约与低风险收口 (Priority: P2)

<a id="p2-1-payload-type-inconsistent"></a>
#### 🔵 **轻微问题 P2-1: 本地与跨节点 InvokeResult.Payload 类型不同**

**提交**: WORKTREE@68402dd  
**作者**: 本地未提交（Git 配置：burncomyang）  
**文件位置**: `F:\actorgo-game\actorgo\facade\invoke_result.go`、`F:\actorgo-game\actorgo\net\actor\system.go`（约第299-306行）

```go
// 本地返回 handler 的 *Response；远端返回原始 []byte
return &cfacade.InvokeResult{Payload: response.Payload, Code: response.Code, Message: response.Message}
```

**问题**: `result.Payload.(*LoginResponse)` 本地成功，切到远端会 panic，且与 `Payload remains a concrete response` 注释冲突。

**建议**: 明确单一契约。保持当前简化目标时，可正式定义“远端为编码字节”，并提供统一 `DecodeResult(result, rsp)` helper；不要让业务直接断言 Payload。

[返回问题清单](#-问题汇总清单)

---

<a id="p2-2-timeout-contract-mismatch"></a>
#### 🔵 **轻微问题 P2-2: HTTP 配置允许 30 秒，Actor 固定 3 秒提前返回**

**提交**: WORKTREE@68402dd  
**作者**: 本地未提交（Git 配置：burncomyang）  
**文件位置**: `F:\actorgo-game\actorgo\net\httpactor\options.go`、`F:\actorgo-game\actorgo\net\actor\system.go`（约第34、194-205行）

```go
DefaultTimeout: 3 * time.Second,
MaxTimeout:     30 * time.Second,
// ActorSystem 仍固定：
callTimeout: 3 * time.Second,
```

**问题**: HTTP 请求头设置 5/10 秒看似有效，实际 Actor 在 3 秒返回 timeout。

**建议**: 统一端到端 deadline；有 RequestContext deadline 时以其为准，框架 `callTimeout` 只作为无 deadline 的默认值，或明确取最小值并同步文档。

[返回问题清单](#-问题汇总清单)

---

<a id="p2-3-runtime-register-race"></a>
#### 🔵 **轻微问题 P2-3: Methods().Register 未限制在 OnInit，存在并发 map 风险**

**提交**: WORKTREE@68402dd  
**作者**: 本地未提交（Git 配置：burncomyang）  
**文件位置**: `F:\actorgo-game\actorgo\net\actor\actor_mailbox.go`（约第33-65行）

```go
if _, exists := m.methodMap[methodID]; exists {
	panic(fmt.Errorf("actorgo: method id %d is already registered in %s mailbox", methodID, m.name))
}
// ...
m.methodMap[methodID] = &methodEntry{ /* ... */ }
```

**问题**: `methodMap` 无锁，接口只写“typically during OnInit”，运行期从其他 goroutine Register 会与 Actor 读取并发。

**建议**: 为保持简洁，直接强制只有 `InitState` 可注册，其他状态 panic；不必为了不支持的动态注册引入锁。

[返回问题清单](#-问题汇总清单)

---

<a id="p2-4-session-ip-port"></a>
#### 🔵 **轻微问题 P2-4: Session.Ip 实际保存 endpoint**

**提交**: WORKTREE@68402dd  
**作者**: 本地未提交（Git 配置：burncomyang）  
**文件位置**: `F:\actorgo-game\actorgo\net\parser\connection.go`（约第55-64行）

```go
remoteIP := ""
if conn != nil && conn.RemoteAddr() != nil {
	remoteIP = conn.RemoteAddr().String()
}
```

**问题**: TCP/WS 得到 `192.0.2.1:54321` 或 `[::1]:54321`，不再是纯 IP。

**建议**: 使用 `net.SplitHostPort`；解析失败时再回退 `String()`。若有意保存 endpoint，应重命名字段。

[返回问题清单](#-问题汇总清单)

---

<a id="p2-5-empty-limits-inconsistent"></a>
#### 🔵 **轻微问题 P2-5: WithLimits(零值) 在 TCP 与 WebSocket 行为不同**

**提交**: WORKTREE@68402dd  
**作者**: 本地未提交（Git 配置：burncomyang）  
**文件位置**: `F:\actorgo-game\actorgo\net\parser\packet_transport.go`（约第87-92行）、`F:\actorgo-game\actorgo\net\proto\agp_codec.go`（约第17-22行）

```go
if ws, ok := conn.(websocketMessageConn); ok {
	ws.SetReadLimit(int64(options.Limits.MaxPacketSize))
	return &websocketPacketTransport{conn: ws, maxPacketSize: options.Limits.MaxPacketSize} // 0 会拒绝所有非空帧
}
return &tcpPacketTransport{Conn: conn, framer: NewTCPPacketFramer(uint32(options.Limits.MaxPacketSize))} // 0 被改成默认 4MiB
```

**问题**: 同一 Options 对 TCP 与 WS 产生不同协议行为。

**建议**: `parser.New` 时一次性规范化并校验全部 Limits，PacketCodec 与 transport 使用同一份值。

[返回问题清单](#-问题汇总清单)

---

<a id="p2-6-stale-target-field"></a>
#### 🔵 **轻微问题 P2-6: demo_chat 客户端仍编码已删除的 target 字段**

**提交**: WORKTREE@09d9eb2  
**作者**: 本地未提交（Git 配置：actorgo-game）  
**文件位置**: `D:\game\actorgo\actorgo-examples\demo_chat\static\agp-client.js`（约第47-78行）、`demo_chat\static\index.html`（约第37行）

```javascript
function encodeRequest(requestId, methodId, timeoutMs, body, target) {
    return [].concat(
        encodeVarint(1, requestId),
        encodeVarint(2, methodId),
        encodeVarint(3, timeoutMs || 0),
        encodeBytes(4, body),
        encodeString(5, target || '') // 已从 agp.proto 删除
    );
}
```

**问题**: Protobuf unknown field 会被服务端忽略，所以暂不导致失败，但示例仍向使用者展示已删除的 target 设计。

**建议**: 删除 `target` 参数、field 5/3 编码和 `ROOM_TARGET`，与 demo_cluster 客户端保持一致。

[返回问题清单](#-问题汇总清单)

---

<a id="p2-7-format-comments"></a>
#### 🔵 **轻微问题 P2-7: 示例格式与“保留原有注释”要求尚未收口**

**提交**: WORKTREE@09d9eb2  
**作者**: 本地未提交（Git 配置：actorgo-game）  
**文件位置**: `D:\game\actorgo\actorgo-examples\demo_cluster\internal\methodid\methodid.go`、`demo_chat\room\main.go` 等

```go
const (
	Login        uint32 = 1001
	KickUID      uint32 = 1002
	PlayerSelect uint32 = 2001
	// ... 当前未 gofmt 对齐
)
```

**问题**: `gofmt -d` 对 `methodid.go` 仍有差异；部分迁移文件删除了原有业务说明注释，不符合此前明确提出的保留要求。

**建议**: 提交前执行 `gofmt`，并从 HEAD 对照恢复仍有业务意义的原注释，只改写已经失效的 Pomelo API 描述。

[返回问题清单](#-问题汇总清单)

---

## ⭐ 代码亮点

1. **协议包层已明显收敛**（本地未提交 - WORKTREE@68402dd）
   - `Request` 只保留 `request_id/method_id/timeout_ms/body`，`Notify` 只保留 `method_id/body`，外部不再控制 ActorPath。

2. **MethodID 注册方式符合简化目标**（本地未提交 - WORKTREE@68402dd）
   - `Methods().Register(MethodID, handler)` 在初始化时完成一次反射适配，业务调用面比 descriptor/codegen 方案简单。

3. **`methodEntry.protobuf` 保留合理**（本地未提交 - WORKTREE@68402dd）
   - 它只是 handler 请求类型是否实现 `proto.Message` 的一次性缓存，用于禁止普通 struct 走 PB codec；不承担路由职责，也避免每次请求重复反射。

4. **传输边界具备基本防护**（本地未提交 - WORKTREE@68402dd）
   - AGP 包层统一 PB，TCP 有长度前缀，WS 只接收 binary，Packet/Body/Metadata 均有限制，写操作由单一队列串行化。

5. **外部路由权限已改善**（本地未提交 - WORKTREE@09d9eb2）
   - demo_cluster 客户端不再提交 target，Gate 根据已认证 Session 派生动态 Player ActorPath；`KickUID` 也检查 `TransportCluster`。

---

## 🔧 改进建议

### 代码规范

- 明确 `Register` 只能在 `OnInit` 调用，保持无锁实现。
- 所有手写 Go 文件统一执行 `gofmt`；生成文件继续通过 proto 源重新生成，不手改。
- 恢复仍有效的原业务注释，删除只描述旧 Pomelo API 的失效注释。

### 测试覆盖

- 增加“请求排队后超时不执行”的 Actor 测试。
- 增加 HTTP/AGP/NATS Notify handler 读取 `ctx.Err()` 的生命周期测试。
- 增加同 Actor Invoke 拒绝测试。
- 增加 Connector Stop/InChan 并发和 Draining 拒绝请求测试。
- 增加 demo_cluster 登录→选角→创角断言 `ServerId != 0` 的集成测试。
- CI 同时执行框架和 examples 的 `go test ./... -run '^$'`。

### 工程实践

- 修复顺序建议：P1-1/P1-2/P1-3 → P1-6/P1-7 → P1-5 → 示例 P1-8/P1-9/P1-10 → 全仓编译与 go.mod。
- 不提交个人绝对路径 replace；使用 `go.work` 管理双仓本地开发。
- 集群入口采用有界并发与过载返回，避免从串行回调直接切换为无限 goroutine。

### 安全性

- 普通内部 error 不返回客户端。
- 外部方法使用 Session 身份，不信任 body 中的 UID、NodeID 或 ActorPath。
- 继续保留内部方法的 `TransportCluster` 检查；长期可增加节点间身份校验。

---

## 验证结果

| 工作区 | 命令 | 结果 |
|--------|------|------|
| actorgo | `go test ./... -run '^$' -count=1 -timeout 90s` | 通过 |
| actorgo | `go test ./facade ./net/proto ./net/serializer ./net/method ./net/actor ./net/parser ./net/httpactor ./net/connector ./net/cluster/nats_cluster ./net/discovery` | 通过 |
| actorgo | `go vet ./...` | 通过 |
| examples | `go test ./demo_cluster/... ./demo_chat/...` | 通过 |
| examples | `go vet ./demo_cluster/... ./demo_chat/...` | 通过 |
| examples | `node --check` 两个新增 AGP 客户端 | 通过 |
| examples | `go test -vet=off ./... -run '^$'` | **失败**，见 P1-11 |
| 两仓 | `git diff --check` | 无 whitespace error；仅有 Git LF/CRLF 转换提示 |
| actorgo | `go test -race ...` | 未执行：当前 `CGO_ENABLED=0`，Go race detector 要求 CGO |

---

## 附录：完整提交列表

### WORKTREE@68402dd

**作者**: 本地未提交（当前 Git 配置：burncomyang）  
**基线时间**: 2026-06-03 23:44:40 +0800  
**说明**: ActorGo AGP/PB 最终协议、MethodID 路由、JSON/PB body codec、HTTP Actor、Connection/ConnectionManager 改造  
**变更规模**: 70 个跟踪文件，约 1648 行新增、6045 行删除，另有新增文件  
**主要变更文件**:

- M `application.go`, `actorgo.go`, `facade/*`
- M/A `net/actor/*`, `net/method/*`, `net/parser/*`, `net/httpactor/*`
- M/A `net/proto/agp.proto`, `net/proto/cluster_message.proto`, `net/serializer/*`
- M `net/cluster/nats_cluster/*`, `net/connector/*`, `net/discovery/*`
- D `net/parser/pomelo/*`, `net/parser/simple/*`, `net/actor/invoke.go`

### WORKTREE@09d9eb2

**作者**: 本地未提交（当前 Git 配置：actorgo-game）  
**基线时间**: 2026-03-17 23:57:24 +0800  
**说明**: demo_cluster/demo_chat 适配 MethodID + AGP/PB，新增 Go/JS 客户端和 Gate ActorUser  
**变更规模**: 39 个跟踪文件，约 1091 行新增、2038 行删除，另有新增文件  
**主要变更文件**:

- M/A `demo_cluster/internal/methodid/*`, `internal/rpc/*`, `nodes/gate/*`
- M/A `demo_cluster/robot_client/*`, `nodes/web/static/agp-client.js`
- M/A `demo_chat/room/*`, `demo_chat/static/agp-client.js`
- M `go.mod`, `go.sum`, `config/demo-*.json`
- D `demo_cluster/nodes/gate/actor_agent.go`, `route.go`

---

## 📞 联系信息

如对本报告有疑问，请联系：

- **Code Review负责人**: Codex
- **报告生成日期**: 2026-08-12
