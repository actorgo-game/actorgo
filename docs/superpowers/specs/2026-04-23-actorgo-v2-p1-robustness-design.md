# actorgo v2 · P1 稳健性强化 — 设计文档

- **版本**: v2.0 子项目 P1（4 子项目中的第 1 个，地基）
- **作者**: actorgo team
- **日期**: 2026-04-23
- **状态**: Draft（待用户审阅）
- **后续子项目**: P4 云原生集成 → P2 性能极致优化 → P3 v2 API 优雅化

---

## 0. 范围与原则

- **目标**：把 actorgo 升级为 v2 的"地基"。修复已知 bug，消除并发陷阱，让框架在 OOM、panic、节点宕机、网络抖动等极端场景下行为可预期。
- **不在范围**：性能调优、API 重构、云原生集成（分别归 P2、P3、P4）。
- **breaking change 策略**：v2 大版本允许 breaking change；旧版（v1.x）仅维护安全补丁。每个模块标注 `[兼容]` / `[breaking]`，便于编写迁移指南。
- **质量基线**：所有改动需通过 `go test -race -count=10 ./...`。

---

## 1. 错误模型迁移：`int32 ccode → error`  `[breaking]`

### 1.1 新错误类型

```go
package cerror

type Error struct {
    Code    int32       // 兼容旧 ccode 数值
    Op      string      // 操作上下文，如 "actor.Call"
    Source  string      // 调用方 actor path
    Target  string      // 目标 actor path
    Func    string      // 函数名
    Cause   error       // wrapped 原始错误
    Fields  []slog.Attr // 结构化扩展字段（可选）
}

func (e *Error) Error() string  { /* 含 op/source/target/func/code 的格式化 */ }
func (e *Error) Unwrap() error  { return e.Cause }
func (e *Error) Is(target error) bool { /* 按 Code 比较 */ }

// 工厂
func New(code int32, op string) *Error
func Wrap(code int32, op string, cause error) *Error

// Sentinel 错误（每个 ccode 一个变量），可直接 errors.Is 比较
var (
    ErrActorNotFound      = &Error{Code: ccode.ActorNotFound,        Op: "actor"}
    ErrActorPathInvalid   = &Error{Code: ccode.ActorConvertPathError, Op: "actor.path"}
    ErrCallTimeout        = &Error{Code: ccode.ActorCallTimeout,     Op: "actor.callwait"}
    ErrSourceEqualTarget  = &Error{Code: ccode.ActorSourceEqualTarget, Op: "actor.callwait"}
    ErrPublishRemote      = &Error{Code: ccode.ActorPublishRemoteError, Op: "cluster.publish"}
    ErrMarshal            = &Error{Code: ccode.ActorMarshalError,    Op: "serializer.marshal"}
    ErrUnmarshal          = &Error{Code: ccode.ActorUnmarshalError,  Op: "serializer.unmarshal"}
    ErrDiscoveryNotFound  = &Error{Code: ccode.DiscoveryNotFoundNode, Op: "discovery"}
    ErrMailboxFull        = &Error{Code: ccode.ActorMailboxFull,     Op: "actor.mailbox"} // 新增
    ErrSystemShuttingDown = &Error{Code: ccode.SystemShuttingDown,   Op: "system"}        // 新增
    // ... 与现有 ccode 一一对应
)
```

### 1.2 IActorSystem / IActor v2 签名

```go
// v2: 全部返回 error
type IActorSystem interface {
    Call(source, target, funcName string, arg any) error
    CallWait(source, target, funcName string, arg, reply any) error
    CallType(nodeType, actorID, funcName string, arg any) error
    PostRemote(m *Message) error
    PostLocal(m *Message) error
    PostEvent(data IEventData) error
    // ...
}
```

业务调用从：

```go
if code := actor.Call(...); ccode.IsFail(code) {
    log.Warnf("call fail: %d", code)
}
```

变为：

```go
if err := actor.Call(...); err != nil {
    if errors.Is(err, cerror.ErrCallTimeout) {
        // 处理超时
    }
    log.Warnw("call fail", "err", err)
}
```

### 1.3 ICluster 接口同步迁移

`RequestRemote` 返回 `([]byte, error)`，`Response.Code` 字段保留用于跨节点错误码透传，本地始终包装成 `*cerror.Error`。

### 1.4 兼容层（过渡）

提供 `legacy/ccode.go` 中导出 `func ToCode(err error) int32`，业务过渡期可拿到 int32 数值：

```go
err := actor.Call(...)
if cerror.ToCode(err) == ccode.ActorCallTimeout { /* ... */ }
```

---

## 2. Panic 防御加固  `[兼容]`

### 2.1 设计原则

保持当前 `recover + log + 继续` 策略（不引入监督树），但要做到：

- **每个潜在 panic 入口都被 recover** —— 包括 `processLocal/Remote/Event/Timer`、`Push/Pop`、`MemberListener` 回调、所有 NATS 订阅 handler、`onShutdownFn`、`OnInit/OnStop`、Timer 回调等
- **panic 信息结构化**：包含 stack trace、actor path、func name、消息片段（截断到 256 字节）
- **panic 计数 metric**（接口先内置 nop counter，P4 暴露 Prometheus）
- **panic 不影响其它 Actor**：当前已满足，文档中明确该不变量

### 2.2 关键修复点

| 位置 | 当前问题 | 修复 |
|---|---|---|
| `Application.Startup()` | `recover` 强转 `string` 会二次 panic | 改用 `fmt.Sprint(r)` |
| `processLocal/Remote` | 仅在 `invokeFunc` 内 recover | 在 `processXxx` 顶层增加二级 recover |
| NATS subscribe callbacks | 无 recover，单条 panic 拖垮整个订阅 goroutine | 包装 `safeSubscribe` 工具函数 |
| `MemberListener` 回调 | 业务异常污染发现循环 | 包装 `safeListenerCall` |
| `Timer fn` | 业务异常未处理 | `timer.invokeFunc` 增加 recover |
| `cutils.Try` | 仅捕获 string 类型 panic | 重写为 `SafeExec`（见下） |

### 2.3 统一 `SafeExec` 工具

```go
package cutils

// SafeExec 执行 fn，捕获任意类型 panic 并报告
func SafeExec(fn func(), ctx string, attrs ...slog.Attr) {
    defer func() {
        if r := recover(); r != nil {
            stack := debug.Stack()
            clog.Error("panic recovered",
                slog.String("ctx", ctx),
                slog.Any("panic", r),
                slog.String("stack", string(stack)),
            )
            metrics.PanicTotal.Inc(ctx)
        }
    }()
    fn()
}
```

替换现有的 `cutils.Try`（保留兼容 alias）。

---

## 3. 可选有界邮箱与背压  `[兼容]`

### 3.1 接口设计

```go
type ActorOption func(*ActorOptions)

type ActorOptions struct {
    LocalQueueSize  int            // 0 = 无界（默认）
    RemoteQueueSize int            // 0 = 无界（默认）
    EventQueueSize  int            // 0 = 无界
    OverflowPolicy  OverflowPolicy // 仅在有界时生效
    BlockTimeout    time.Duration  // PolicyBlock 模式的最大阻塞时间
}

type OverflowPolicy int
const (
    PolicyDropNewest OverflowPolicy = iota // 丢弃新消息（默认）
    PolicyDropOldest                       // 丢弃最早消息
    PolicyBlock                            // 阻塞 Producer（带超时）
    PolicyReject                           // 立即返回 ErrMailboxFull
)

// 创建 Actor 时传入
system.CreateActor("room", &RoomActor{},
    cactor.WithLocalQueueSize(10000),
    cactor.WithOverflowPolicy(cactor.PolicyDropOldest))
```

### 3.2 内部实现策略

- **无界路径不变**：默认无界路径走当前 lock-free MPSC 队列，零性能影响
- **有界路径**：底层切换为 `chan *Message`（容量 = QueueSize），简单可靠；高水位告警（>80%）通过 atomic counter + tick 检查
- **DropOldest** 实现：当满时先 `Pop()` 一次再 `Push()`；为避免竞态，`DropOldest` 强制单生产者（多生产者场景应使用 `DropNewest` 或 `Reject`）
- **Block** 模式：默认 1s 超时（可配），超时后转为 Reject 并返回 `ErrMailboxFull`

### 3.3 Metrics 接口

```go
type MailboxMetrics interface {
    OnEnqueue(actorPath string, mailbox string)
    OnDrop(actorPath string, mailbox string, reason string)
    OnHighWater(actorPath string, mailbox string, watermark int)
    OnDepth(actorPath string, mailbox string, depth int)
}
```

P1 仅定义接口 + 内置 nop 实现，P4 接入 Prometheus。

---

## 4. 三阶段优雅关闭  `[兼容]`

### 4.1 阶段定义

```
┌──────────────────────────────────────────────────────┐
│  Phase 1: Drain Sources (停止接收新输入)               │
│   - Connector.Stop() 拒绝新连接                       │
│   - Cluster 取消订阅 remote/local subject             │
│   - Discovery deregister（防止他节点路由）             │
│   - Mailbox.SetClosed() 拒绝 PostXxx                  │
│   持续时长: 立即                                       │
├──────────────────────────────────────────────────────┤
│  Phase 2: Drain Mailboxes (排空已收到消息)             │
│   - 等待所有 Actor 处理完 mailbox 残留                 │
│   - 单个 Actor 排空后退出 goroutine                   │
│   - 全局超时由 GraceTimeout 控制（默认 30s）           │
├──────────────────────────────────────────────────────┤
│  Phase 3: Force Stop                                  │
│   - 超时则向所有 Actor 发 cancel ctx                   │
│   - 业务回调收到 ctx.Done()                            │
│   - 仍未退出则强制 close + 丢弃 + metric 上报          │
│   - 走 OnStop 但不再等待                               │
└──────────────────────────────────────────────────────┘
```

### 4.2 配置

```json
{
  "shutdown": {
    "grace_period_seconds": 30,
    "force_kill_after_seconds": 60
  }
}
```

### 4.3 与 k8s preStop 配合

- `preStop`: `sleep 5` 给 service mesh 做端点摘除
- `terminationGracePeriodSeconds`: 60+
- 框架收到 SIGTERM 立即进入 Phase 1，避免新流量
- P4 中给出完整 Helm values 示例

### 4.4 Shutdown Hook 改进

```go
type ShutdownHook struct {
    Name     string
    Priority int                              // 数字越大越先执行
    Fn       func(ctx context.Context) error
}

app.OnShutdown(ShutdownHook{
    Name:     "save-game-state",
    Priority: 100,
    Fn:       saveAllRoomState,
})
```

---

## 5. Discovery 三种实现一致升级  `[breaking on ETCD]`

### 5.1 ETCD 模式

- 启用 `Lease + KeepAlive`（修复注释的 `getLeaseId`）
- TTL 默认 10s，KeepAlive 间隔 = TTL/3
- KeepAlive chan 关闭时**自动重新 Grant + 重注册**（断线重连场景）
- `Stop()` 时显式 `Revoke(leaseID)`，确保 KV 立即清理
- nodeID 冲突检测：注册前 `Get` 看 KV 是否已存在 → 启动失败而非覆盖
- watch 增加 compaction 错误自动 `forceSync` 重建本地视图
- 启动时清理同 nodeID 的旧残留

### 5.2 Master 模式

- 心跳间隔可配（默认 1s），超时 3s 移除
- 注册请求添加幂等 token，重连不会触发重复加入
- Master 节点宕机后，client 节点检测到 NATS 重连事件 → 自动重新注册
- 增加 `MemberVersion` 字段，防止 add/remove 事件乱序

### 5.3 Default 模式

- 启动时 nodeID 全局唯一性检查（profile 中重复直接 panic 报错而非 silent drop）

### 5.4 通用增强（IDiscovery 接口扩展）

```go
type IDiscovery interface {
    // ... 现有方法保持

    // 新增
    Watch(ctx context.Context) <-chan MemberEvent  // 替代 OnAdd/OnRemove，支持取消订阅
    Health() HealthStatus                          // 当前发现服务自身的健康状态
}

type MemberEvent struct {
    Type   EventType  // Add / Remove / Update
    Member IMember
}

type HealthStatus struct {
    Healthy   bool
    LastSync  time.Time
    Detail    string
}
```

旧的 `OnAddMember/OnRemoveMember` 兼容保留，内部基于 Watch 实现。

---

## 6. 并发安全现代化  `[兼容]`

### 6.1 atomic 现代化

| 旧 | 新 |
|---|---|
| `int32` + `atomic.LoadInt32/StoreInt32` | `atomic.Int32`、`atomic.Bool` |
| `unsafe.Pointer` + `atomic.SwapPointer` | `atomic.Pointer[T]`（Go 1.19+） |
| `Application.running int32` | `atomic.Bool` |
| `Actor.state State` | `atomic.Int32`（封装 Load/Store/CAS 方法） |
| `nats.Connect.seq uint64` | `atomic.Uint64` |
| `cnats.connectSize/roundIndex` | `atomic.Uint64` |

### 6.2 已知并发 bug 修复

| Bug | 现状 | 修复 |
|---|---|---|
| `Actor.state` 读写 race | `loop()` 读 `p.state`，`PostRemote/Local` 读 `targetActor.state`，但写入未原子化 | 全部 CAS 化 |
| `Actor.Exit()` 二次调用阻塞 | `p.close <- struct{}{}` cap=1 | 用 `sync.Once` 保护 + close(chan) 替代发送 |
| `actorChild.Get + Create` race | 先 Get 再 Create 不是原子，并发可能创建两次 | 用 `LoadOrStore` + double-check |
| `actor.findChildActor` 与 `OnFindChild` race | 同时多个消息触发 OnFindChild 可能创建多个 child | 单飞（singleflight）保护 |
| `messagePool` 二次回收 | 业务可能错误调用两次 `Recycle` | Recycle 内部 `atomic.CompareAndSwap` 标志位 |
| `localMail.Push(nil)` | `mailbox.Push` 检查 nil 但 funcMap 在 Stop 后被 clear，并发 Push 期间会 nil deref | Stop 前先 SetClosed 阻止 Push |
| `connectPool` 启动期 race | `connectSize` 在循环中累加，并发读取可能为 0 | 全部初始化完毕后再发布到 atomic |

### 6.3 race detector CI

- 提供 `Makefile` target `test-race`
- 文档要求 PR 必须通过 `go test -race ./...`
- 现有测试覆盖不足的位置增加 race-friendly 测试用例（mailbox 多生产者并发 push、actor 创建/销毁并发、discovery member 增删并发）

### 6.4 Context 传播

所有阻塞 API（`CallWait`、`Discovery.Watch`、`Shutdown` 钩子）增加 `ctx context.Context` 参数版本：

```go
CallWaitContext(ctx context.Context, source, target, funcName string, arg, reply any) error
```

旧 `CallWait` 内部转调 `CallWaitContext(context.Background(), ...)`，保持调用兼容。框架内部建立 root context，shutdown 时统一 cancel。

---

## 7. 测试与验收

### 7.1 新增测试目录

```
test/
├── robustness/
│   ├── mailbox_overflow_test.go          # 4 种 OverflowPolicy 行为
│   ├── shutdown_three_phase_test.go      # 三阶段超时/正常路径
│   ├── etcd_lease_recovery_test.go       # 杀进程后 KV 是否清理
│   ├── etcd_compaction_recovery_test.go
│   ├── panic_isolation_test.go           # 单 Actor panic 不影响其他
│   └── concurrent_actor_create_test.go   # 并发创建同 ID Actor
├── chaos/
│   ├── nats_disconnect_test.go           # NATS 断网 30s 后业务能否恢复
│   ├── etcd_disconnect_test.go
│   └── flood_message_test.go             # 单 Actor 100 万消息洪水测试
```

### 7.2 验收标准

- 所有新增测试 + 现有测试通过 `go test -race -count=10 ./...`
- 长跑稳定性测试：单节点单 Actor 1 亿消息，无 OOM，p99 处理时延 < 10ms（具体数字 P2 优化前作为 baseline）
- 杀进程注入测试：30 秒内 ETCD/Master 中节点信息消失
- Memory leak 检查：跑 5 分钟混合负载后 `runtime.NumGoroutine()` 与 `runtime.MemStats.HeapAlloc` 稳定不增长

---

## 8. 实施路线（writing-plans 阶段细化）

按依赖关系分 6 个 PR：

1. **PR1** 错误模型基础设施（`cerror` 包），不动调用方
2. **PR2** 并发安全修复 + atomic 现代化（不改 API）
3. **PR3** 邮箱有界化 + 背压 metrics 接口
4. **PR4** 三阶段 shutdown
5. **PR5** Discovery 三模式一致升级（ETCD lease 修复是关键）
6. **PR6** API 全面切换为 error 返回（breaking change，集中在一个 PR 便于评审与回滚）

每个 PR 独立可合入，每个都附带对应测试用例，全程 race detector 在线。

---

## 9. 与其它子项目的衔接

| 衔接点 | 目标子项目 | 说明 |
|---|---|---|
| `MailboxMetrics` 接口 | P4 | P4 接入 Prometheus 实现 |
| `metrics.PanicTotal` 接口 | P4 | 同上 |
| `IDiscovery.Health()` | P4 | 健康检查 endpoint 复用 |
| Shutdown 阶段化 | P4 | k8s preStop / readinessProbe 设计依赖 |
| `error` 类型 | P3 | v2 API 全部基于 `error` 返回 |
| `atomic.Pointer[T]` | P2 | 性能优化时不再受 unsafe.Pointer 拖累 |
| Actor 创建并发安全 | P2 | 批量消息处理优化的前置 |

---

> 本设计文档为 actorgo v2 重构第 1 个子项目。批准后将通过 writing-plans skill 转化为可执行的 PR-level 实施计划。
