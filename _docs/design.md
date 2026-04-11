# ActorGo 框架设计原理

## 目录

- [1. 概述](#1-概述)
- [2. 整体架构](#2-整体架构)
  - [2.1 架构分层](#21-架构分层)
  - [2.2 核心依赖](#22-核心依赖)
  - [2.3 包组织结构](#23-包组织结构)
- [3. Application 与组件生命周期](#3-application-与组件生命周期)
  - [3.1 AppBuilder 构建器模式](#31-appbuilder-构建器模式)
  - [3.2 Application 核心](#32-application-核心)
  - [3.3 组件接口与生命周期](#33-组件接口与生命周期)
  - [3.4 启动与关闭流程](#34-启动与关闭流程)
- [4. Actor 模型核心设计](#4-actor-模型核心设计)
  - [4.1 设计哲学](#41-设计哲学)
  - [4.2 Actor 路径寻址](#42-actor-路径寻址)
  - [4.3 Actor 状态机](#43-actor-状态机)
  - [4.4 Actor 主循环](#44-actor-主循环)
  - [4.5 Actor 创建与销毁](#45-actor-创建与销毁)
  - [4.6 父子 Actor 层级](#46-父子-actor-层级)
- [5. 消息系统](#5-消息系统)
  - [5.1 消息结构](#51-消息结构)
  - [5.2 四种消息通道](#52-四种消息通道)
  - [5.3 消息路由与分发](#53-消息路由与分发)
  - [5.4 函数调用机制](#54-函数调用机制)
  - [5.5 Call 与 CallWait](#55-call-与-callwait)
- [6. 邮箱（Mailbox）与无锁队列](#6-邮箱mailbox与无锁队列)
  - [6.1 无锁队列实现](#61-无锁队列实现)
  - [6.2 Mailbox 封装](#62-mailbox-封装)
  - [6.3 通知机制](#63-通知机制)
- [7. 事件系统](#7-事件系统)
  - [7.1 发布/订阅模型](#71-发布订阅模型)
  - [7.2 UniqueID 过滤](#72-uniqueid-过滤)
- [8. 定时器系统](#8-定时器系统)
  - [8.1 分层时间轮](#81-分层时间轮)
  - [8.2 Actor 定时器集成](#82-actor-定时器集成)
- [9. 网络层](#9-网络层)
  - [9.1 连接器抽象](#91-连接器抽象)
  - [9.2 TCP 连接器](#92-tcp-连接器)
  - [9.3 WebSocket 连接器](#93-websocket-连接器)
  - [9.4 网络解析器](#94-网络解析器)
- [10. 协议层](#10-协议层)
  - [10.1 Pomelo 协议](#101-pomelo-协议)
  - [10.2 Simple 协议](#102-simple-协议)
  - [10.3 序列化](#103-序列化)
- [11. 会话管理](#11-会话管理)
  - [11.1 Session 结构](#111-session-结构)
  - [11.2 Agent 模型](#112-agent-模型)
  - [11.3 SID/UID 绑定与映射](#113-siduid-绑定与映射)
- [12. 集群通信](#12-集群通信)
  - [12.1 NATS 集群架构](#121-nats-集群架构)
  - [12.2 Subject 命名规范](#122-subject-命名规范)
  - [12.3 集群消息流转](#123-集群消息流转)
  - [12.4 跨节点 RPC](#124-跨节点-rpc)
- [13. 服务发现](#13-服务发现)
  - [13.1 发现接口抽象](#131-发现接口抽象)
  - [13.2 默认模式（静态配置）](#132-默认模式静态配置)
  - [13.3 Master 模式（NATS）](#133-master-模式nats)
  - [13.4 ETCD 模式](#134-etcd-模式)
- [14. 配置系统](#14-配置系统)
  - [14.1 Profile 设计](#141-profile-设计)
  - [14.2 节点配置](#142-节点配置)
  - [14.3 数据配置组件](#143-数据配置组件)
- [15. 日志系统](#15-日志系统)
- [16. 扩展组件](#16-扩展组件)
  - [16.1 Cron 定时任务](#161-cron-定时任务)
  - [16.2 Gin HTTP 服务](#162-gin-http-服务)
  - [16.3 GORM 数据库](#163-gorm-数据库)
  - [16.4 MongoDB](#164-mongodb)
- [17. 工具库](#17-工具库)
- [18. 性能优化策略](#18-性能优化策略)
- [19. 典型游戏服务端架构](#19-典型游戏服务端架构)

---

## 1. 概述

ActorGo 是一个基于 **Actor 模型** 构建的高性能分布式 Golang 游戏服务器框架。框架采用 "每 Actor 一个 goroutine，消息串行处理" 的并发模型，天然避免了共享状态的竞态问题，同时通过 NATS 消息总线实现透明的跨节点 Actor 通信。

### 核心设计目标

| 目标 | 实现方式 |
|------|----------|
| **高并发** | 每个 Actor 独占 goroutine，消息串行处理，无锁 |
| **可伸缩** | 基于 NATS 的集群通信，节点可水平扩展 |
| **低耦合** | 组件化架构，接口驱动，可插拔 |
| **易扩展** | 统一的 Actor 编程模型，业务只需实现 Handler |
| **协议兼容** | 兼容 Pomelo 生态客户端 SDK，支持自定义 Simple 协议 |

---

## 2. 整体架构

### 2.1 架构分层

```
┌─────────────────────────────────────────────────────────────┐
│                      业务逻辑层 (Game Logic)                 │
│              IActorHandler 实现 / 子 Actor / 事件 / 定时器     │
├─────────────────────────────────────────────────────────────┤
│                     Actor 系统层 (Actor System)              │
│           Actor / Mailbox / Event / Timer / Child           │
├──────────────────┬──────────────────┬───────────────────────┤
│   协议解析层      │    集群通信层     │     服务发现层          │
│ Pomelo / Simple  │  NATS Cluster    │ Default/Master/ETCD   │
├──────────────────┴──────────────────┴───────────────────────┤
│                       网络传输层 (Network)                    │
│              TCP Connector / WebSocket Connector             │
├─────────────────────────────────────────────────────────────┤
│                    基础设施层 (Infrastructure)                │
│      Logger / Profile / Serializer / Extend Utils           │
├─────────────────────────────────────────────────────────────┤
│                    可插拔组件层 (Components)                   │
│          Cron / DataConfig / Gin / GORM / Mongo / ETCD      │
└─────────────────────────────────────────────────────────────┘
```

### 2.2 核心依赖

| 依赖 | 版本要求 | 用途 |
|------|---------|------|
| Go | 1.26+ | 编程语言 |
| NATS | - | 集群消息总线与 RPC |
| Protobuf | v2 | 集群内部序列化 |
| zap | - | 高性能日志 |
| gorilla/websocket | - | WebSocket 连接器 |
| gin | - | HTTP 服务组件 |
| gorm | - | MySQL ORM 组件 |
| mongo-driver | v2 | MongoDB 组件 |
| etcd | v3 | 服务发现组件 |

### 2.3 包组织结构

```
actorgo/
├── actorgo.go              # AppBuilder 入口
├── application.go          # Application 核心
│
├── facade/                 # 接口定义层（门面模式）
│   ├── actor.go            #   IActorSystem, IActor, IActorHandler
│   ├── application.go      #   IApplication, IComponent
│   ├── cluster.go          #   ICluster, IDiscovery, IMember
│   ├── component.go        #   Component 基类
│   ├── connector.go        #   IConnector
│   ├── message.go          #   Message, ActorPath
│   ├── net_parser.go       #   INetParser
│   ├── session.go          #   Session 扩展
│   └── serializer.go       #   ISerializer
│
├── net/                    # 网络与 Actor 实现层
│   ├── actor/              #   Actor 系统核心实现
│   ├── cluster/            #   集群通信
│   ├── connector/          #   TCP/WebSocket 连接器
│   ├── discovery/          #   服务发现
│   ├── nats/               #   NATS 连接池
│   ├── parser/             #   协议解析器
│   │   ├── pomelo/         #     Pomelo 协议
│   │   └── simple/         #     Simple 协议
│   ├── proto/              #   Protobuf 定义
│   └── serializer/         #   JSON/Protobuf 序列化
│
├── profile/                # 配置管理
├── logger/                 # 日志系统
├── components/             # 可插拔组件
│   ├── cron/               #   定时任务
│   ├── data-config/        #   策划配表
│   ├── etcd/               #   ETCD 发现
│   ├── gin/                #   HTTP 服务
│   ├── gorm/               #   MySQL
│   └── mongo/              #   MongoDB
│
├── extend/                 # 工具库
│   ├── time_wheel/         #   分层时间轮
│   ├── snowflake/          #   雪花 ID
│   ├── queue/              #   通用队列
│   └── ...                 #   加密/压缩/反射等
│
├── ccode/                  # 错误码
├── const/                  # 常量
└── error/                  # 错误定义
```

---

## 3. Application 与组件生命周期

### 3.1 AppBuilder 构建器模式

框架采用 Builder 模式构建应用，提供两种入口：

```go
// 方式一：从配置文件构建
app := actorgo.Configure(profileFilePath, nodeID, isFrontend, mode)

// 方式二：从已有 Node 构建
app := actorgo.ConfigureNode(node, isFrontend, mode)
```

`AppBuilder` 嵌入了 `*Application`，并提供链式调用：

```go
actorgo.Configure("profile.json", "game-1", true, actorgo.Cluster).
    SetNetParser(pomelo.NewParser()).     // 设置协议解析器
    AddActors(&GameActor{}, &RoomActor{}). // 添加 Actor Handler
    Register(&myComponent{}).              // 注册自定义组件
    Startup()                              // 启动
```

`Startup()` 是启动入口，它会在集群模式下自动注册 `Cluster` 和 `Discovery` 组件，然后将用户注册的组件合并，最终调用 `Application.Startup()` 完成启动。

### 3.2 Application 核心

`Application` 是框架的中枢，持有所有核心子系统的引用：

| 字段 | 类型 | 说明 |
|------|------|------|
| `INode` | 嵌入接口 | 节点信息（ID、类型、地址） |
| `isFrontend` | `bool` | 是否为前端节点（网关） |
| `nodeMode` | `NodeMode` | 集群/单机模式 |
| `components` | `[]IComponent` | 所有注册的组件 |
| `actorSystem` | `*cactor.Component` | Actor 系统 |
| `netParser` | `INetParser` | 网络协议解析器 |
| `cluster` | `ICluster` | 集群通信 |
| `discovery` | `IDiscovery` | 服务发现 |
| `serializer` | `ISerializer` | 序列化器（默认 Protobuf） |
| `dieChan` | `chan bool` | 关闭信号 |
| `running` | `int32` | 运行状态（原子操作） |

### 3.3 组件接口与生命周期

所有可插拔模块都实现 `IComponent` 接口：

```go
type IComponent interface {
    Name() string         // 组件唯一名称
    Set(app IApplication) // 注入 Application
    App() IApplication    // 获取 Application
    Init()                // 初始化
    OnAfterInit()         // 初始化后回调
    OnBeforeStop()        // 停止前回调
    OnStop()              // 停止
}
```

框架提供了 `Component` 基类，自定义组件只需嵌入并重写所需方法：

```go
type Component struct {
    app  IApplication
    name string
}
```

### 3.4 启动与关闭流程

**启动流程**（顺序执行）：

```
1. NewApp()
   ├── Profile.Init()          加载配置文件
   ├── SetNodeLogger()         初始化日志
   └── cactor.New()            创建 Actor 系统

2. AppBuilder.Startup()
   ├── 集群模式 → 注册 Cluster + Discovery
   ├── 注册用户组件
   └── Application.Startup()

3. Application.Startup()
   ├── Register(actorSystem)        注册 Actor 系统组件
   ├── Register(connectors...)      注册连接器组件
   ├── 遍历组件 → c.Set(app)        注入 Application
   ├── 遍历组件 → c.Init()          初始化
   ├── 遍历组件 → c.OnAfterInit()   后初始化（Actor 在此创建）
   ├── netParser.Load(app)          加载网络解析器（仅前端）
   ├── atomic(running = 1)          标记运行中
   └── select { dieChan | signal }  阻塞等待
```

**关闭流程**（逆序执行）：

```
收到 SIGINT/SIGTERM 或 dieChan 信号
   ├── atomic(running = 0)
   ├── onShutdownFn...              执行注册的关闭回调
   ├── 逆序遍历组件 → OnBeforeStop() 停止前处理
   ├── 逆序遍历组件 → OnStop()       停止
   └── 日志 Flush
```

逆序关闭确保了依赖关系的正确处理：先注册的组件（底层组件）最后关闭，后注册的组件（业务组件）先关闭。

---

## 4. Actor 模型核心设计

### 4.1 设计哲学

ActorGo 的 Actor 模型遵循以下核心原则：

1. **Actor 是最小的并发单元**：每个 Actor 独立运行在一个 goroutine 中
2. **消息驱动**：Actor 之间只通过消息通信，不共享内存
3. **串行处理**：单个 Actor 内部的所有消息串行处理，无需加锁
4. **位置透明**：通过 ActorPath 寻址，同节点和跨节点调用使用统一 API

这种设计带来的优势：
- 无共享状态，天然线程安全
- 消息隔离，业务逻辑简单清晰
- 容易实现水平扩展

### 4.2 Actor 路径寻址

每个 Actor 通过 `ActorPath` 唯一标识，格式为：

```
NodeID.ActorID              → 父 Actor
NodeID.ActorID.ChildID      → 子 Actor
```

示例：
- `game-1.room` — game-1 节点上的 room Actor
- `game-1.room.10086` — game-1 节点上 room Actor 的 10086 号子 Actor

`ActorPath` 的字符串形式使用 `.` 分隔，解析逻辑如下：

```go
func ToActorPath(path string) (*ActorPath, error) {
    p := strings.Split(path, ".")
    switch len(p) {
    case 2: return NewActorPath(p[0], p[1], ""), nil    // 父 Actor
    case 3: return NewActorPath(p[0], p[1], p[2]), nil  // 子 Actor
    }
    return nil, ActorPathError
}
```

路径中的 `NodeID` 决定了消息的路由策略：
- 若 `targetNodeID == localNodeID`：本地投递
- 若 `targetNodeID != localNodeID`：通过 NATS 集群转发

### 4.3 Actor 状态机

Actor 有四种状态，形成线性状态机：

```
InitState(0) → WorkerState(1) → FreeState(2) → StopState(3)
```

| 状态 | 值 | 说明 |
|------|---|------|
| `InitState` | 0 | 初始状态，正在执行 `OnInit()` |
| `WorkerState` | 1 | 工作状态，正常接收和处理消息 |
| `FreeState` | 2 | 空闲状态（预留） |
| `StopState` | 3 | 停止状态，排空队列后退出 |

只有处于 `WorkerState` 的 Actor 才会接收新消息（`PostLocal`/`PostRemote` 时会检查状态）。进入 `StopState` 后，Actor 会继续处理队列中残留的消息，确保消息不丢失，直到所有队列清空后才真正退出。

### 4.4 Actor 主循环

每个 Actor 的 goroutine 执行 `run()` 方法，包含初始化、循环处理、清理三个阶段：

```go
func (p *Actor) run() {
    p.onInit()        // 阶段一：初始化
    defer p.onStop()  // 阶段三：清理（延迟执行）

    for {
        if p.loop() { // 阶段二：消息循环
            break
        }
    }
}
```

`loop()` 方法是核心调度器，使用 Go 的 `select` 多路复用同时监听四种消息通道和关闭信号：

```go
func (p *Actor) loop() bool {
    // 若已进入停止状态且所有队列为空，返回 true 退出循环
    if p.state == StopState {
        if p.localMail.Count() < 1 &&
           p.remoteMail.Count() < 1 &&
           p.event.Count() < 1 {
            return true
        }
    }

    select {
    case <-p.localMail.C:   p.processLocal()   // 本地消息
    case <-p.remoteMail.C:  p.processRemote()   // 远程消息
    case <-p.event.C:       p.processEvent()     // 事件消息
    case <-p.timer.C:       p.processTimer()     // 定时器
    case <-p.close:         p.state = StopState  // 关闭信号
    }

    return false
}
```

**设计要点**：
- `select` 的公平性：Go 的 `select` 在多个 case 同时就绪时随机选择，保证了四种消息通道的公平调度
- 优雅关闭：收到 `close` 信号后不立即退出，而是标记为 `StopState`，继续排空队列
- 串行保证：所有消息处理都在同一个 goroutine 中，天然串行

### 4.5 Actor 创建与销毁

**创建流程**：

```go
func (p *System) CreateActor(id string, handler IActorHandler) (IActor, error) {
    // 1. 检查是否已存在同名 Actor
    if actor, found := p.GetIActor(id); found {
        return actor, nil  // 幂等设计
    }
    // 2. 构造 Actor 实例
    thisActor, err := newActor(id, "", handler, p)
    // 3. 注册到 System
    p.actorMap.Store(id, thisActor)
    // 4. 启动 goroutine
    go thisActor.run()
    return thisActor, nil
}
```

`newActor()` 内部构造过程：

```
newActor(actorID, childID, handler, system)
  ├── 创建 ActorPath{NodeID, ActorID, ChildID}
  ├── 创建 localMail（本地消息邮箱）
  ├── 创建 remoteMail（远程消息邮箱）
  ├── 创建 actorEvent（事件处理器）
  ├── 创建 actorChild（子 Actor 管理器）
  ├── 创建 actorTimer（定时器管理器）
  ├── 若 handler 实现 IActorLoader → 调用 load(&actor)
  └── system.wg.Add(1)
```

`IActorLoader` 接口允许 Handler 在创建时通过 `load(*Actor)` 访问底层 Actor 实例，用于注册函数、定时器、事件等。

**销毁流程**：

```go
func (p *Actor) onStop() {
    close(p.close)
    if p.path.IsParent() {
        p.system.removeActor(p.ActorID())  // 从 System 注销
        p.child.onStop()                    // 级联关闭所有子 Actor
    } else {
        parent.child.Remove(p.path.ChildID) // 从父 Actor 移除
    }
    p.handler.OnStop()          // 调用业务清理逻辑
    p.timer.onStop()            // 关闭定时器
    p.event.onStop()            // 关闭事件
    p.localMail.onStop()        // 关闭本地邮箱
    p.remoteMail.onStop()       // 关闭远程邮箱
    p.system.wg.Done()          // WaitGroup 计数减一
}
```

### 4.6 父子 Actor 层级

ActorGo 支持两级 Actor 层次结构：

```
System (actorMap: sync.Map)
  ├── ParentActor-A (actorID = "room")
  │   ├── ChildActor-1 (childID = "10086")
  │   ├── ChildActor-2 (childID = "10087")
  │   └── ChildActor-N
  ├── ParentActor-B (actorID = "lobby")
  └── ParentActor-C (actorID = "gate")
```

**子 Actor 管理器** (`actorChild`)：

```go
type actorChild struct {
    thisActor   *Actor
    childActors *sync.Map  // key: childID, value: *Actor
}
```

支持的操作：
- `Create(childID, handler)` — 创建子 Actor（同样启动独立 goroutine）
- `Get(childID)` — 查找子 Actor
- `Remove(childID)` — 移除子 Actor
- `Each(fn)` — 遍历所有子 Actor
- `Call/CallWait` — 调用子 Actor 函数

**消息路由**：当消息目标为子 Actor 路径时，消息首先到达父 Actor，由父 Actor 的 `processLocal`/`processRemote` 转发：

```
消息到达父 Actor
  ├── handler.OnLocalReceived(m) → 拦截/预处理
  ├── m.TargetPath().IsChild() ?
  │   ├── 是 → findChildActor(m) → childActor.PostLocal(m)
  │   └── 否 → invokeFunc(m) 直接处理
```

**动态子 Actor 创建**：如果 `childActors` 中找不到目标子 Actor，框架会调用 `handler.OnFindChild(m)`，允许业务层按需创建子 Actor（如玩家首次进入房间时动态创建）。

**约束**：子 Actor 不能再创建子 Actor（仅支持两级层次）。

---

## 5. 消息系统

### 5.1 消息结构

`Message` 是框架中所有消息的统一载体：

```go
type Message struct {
    BuildTime  int64            // 消息创建时间(毫秒)
    PostTime   int64            // 投递到 Actor 的时间(毫秒)
    Source     string           // 来源 Actor 路径
    Target     string           // 目标 Actor 路径
    targetPath *ActorPath       // 缓存的解析结果
    FuncName   string           // 要调用的函数名
    Session    *cproto.Session  // 网关会话信息
    Args       any              // 参数（本地为原始对象，集群为 []byte）
    Header     nats.Header      // NATS 消息头
    Reply      string           // NATS 回复 Subject
    IsCluster  bool             // 是否为集群消息
    ChanResult chan any          // 本地 CallWait 的结果通道
}
```

Message 使用 `sync.Pool` 进行对象复用，减少 GC 压力：

```go
var messagePool = sync.Pool{
    New: func() any { return &Message{} },
}

func GetMessage() *Message {
    msg := messagePool.Get().(*Message)
    msg.BuildTime = ctime.Now().ToMillisecond()
    return msg
}
```

### 5.2 四种消息通道

每个 Actor 内部有四种独立的消息通道，每种通道都有自己的队列和通知 channel：

| 通道 | 队列 | 用途 | 来源 |
|------|------|------|------|
| **Local** | `localMail` | 客户端请求 | Connector → Agent → Actor |
| **Remote** | `remoteMail` | Actor 间调用 | 本地 Call / 集群 NATS |
| **Event** | `event` | 发布/订阅事件 | System.PostEvent |
| **Timer** | `timer` | 定时回调 | 时间轮触发 |

四种通道的独立性确保了不同类型的消息不会互相阻塞。

### 5.3 消息路由与分发

**本地消息处理流程** (`processLocal`)：

```
1. localMail.Pop() 取出消息
2. 更新 lastAt 时间戳
3. handler.OnLocalReceived(m) → 返回 (next, invoke)
   - invoke=true  → invokeFunc() 调用注册函数
   - next=false   → 终止处理
   - next=true    → 继续路由
4. 若目标是子 Actor：
   - 当前是子 Actor → 直接调用
   - 当前是父 Actor → 查找子 Actor 并转发
5. 否则直接调用本地注册函数
```

**远程消息处理流程** (`processRemote`) 与本地类似，但使用 `remoteMail` 和 `remoteInvokeFunc`。

`OnLocalReceived` 和 `OnRemoteReceived` 的双返回值设计提供了灵活的拦截机制：
- `(true, false)` — 不在当前层处理，继续路由到子 Actor
- `(false, true)` — 在当前层处理并终止
- `(true, true)` — 先在当前层处理，然后继续路由
- `(false, false)` — 丢弃消息

### 5.4 函数调用机制

Actor 的函数调用基于 **注册表 + 反射**：

1. Actor 初始化时通过 `mailbox.Register(funcName, fn)` 注册处理函数
2. 消息到达时通过 `funcMap[funcName]` 查找 `FuncInfo`
3. 使用 `reflect.Value.Call()` 调用目标函数

**Local 函数签名**：

```go
func handler(session *cproto.Session, args *MyRequest) {
    // session 包含客户端会话信息
    // args 为反序列化后的请求参数
}
```

**Remote 函数签名**：

```go
// 无返回值
func handler(args *MyRequest) { ... }

// 有返回值（用于 CallWait）
func handler(args *MyRequest) (*MyResponse, int32) { ... }
```

**超时监控**：框架内置了两级超时检测：
- **到达超时** (`arrivalTimeout`)：消息从创建到被处理的时间，默认 100ms
- **执行超时** (`executionTimeout`)：函数执行耗时，默认 100ms

超时时会输出警告日志，帮助定位性能瓶颈。

### 5.5 Call 与 CallWait

框架提供三种 Actor 间调用方式：

| 方法 | 说明 | 是否等待返回 |
|------|------|-------------|
| `Call(target, funcName, arg)` | 单向调用 | 否 |
| `CallWait(target, funcName, arg, reply)` | 同步调用 | 是 |
| `CallType(nodeType, actorID, funcName, arg)` | 按类型广播 | 否 |

**Call 的路由逻辑**：

```
Call(source, target, funcName, arg)
  ├── 解析 targetPath
  ├── targetNodeID != localNodeID ?
  │   ├── 是（跨节点）
  │   │   ├── 序列化参数 → ClusterPacket
  │   │   └── Cluster.PublishRemote(nodeID, packet) → NATS
  │   └── 否（本节点）
  │       ├── 构造 Message
  │       └── System.PostRemote(message) → Actor.remoteMail
```

**CallWait 的实现差异**：

- **本地 CallWait**：通过 `Message.ChanResult` channel 传递结果，调用方阻塞在 `select` 等待结果或超时
- **跨节点 CallWait**：通过 NATS 的 Request/Reply 模式，`Cluster.RequestRemote()` 使用自定义的 `reqID + waiters` 同步等待机制

**CallWait 死锁防护**：框架禁止 Actor 对自身调用 `CallWait`（`source == target` 检查），因为这会导致 goroutine 自己等待自己的响应而永久阻塞。

---

## 6. 邮箱（Mailbox）与无锁队列

### 6.1 无锁队列实现

Actor 的消息队列采用 **无锁 MPSC（多生产者单消费者）队列**，基于 `atomic.SwapPointer` 实现：

```go
type queue struct {
    head, tail *queueNode
    C          chan int32      // 通知 channel
    count      int32           // 原子计数
}

type queueNode struct {
    next *queueNode
    val  any
}
```

**Push 操作**（多生产者安全）：

```go
func (p *queue) Push(v any) {
    n := queueNodePool.Get().(*queueNode)
    n.val = v
    n.next = nil
    // 原子交换 head 指针，获取前一个头节点
    prev := atomic.SwapPointer(&p.head, unsafe.Pointer(n))
    // 将前节点的 next 指向新节点，完成链接
    atomic.StorePointer(&prev.next, unsafe.Pointer(n))
    p._setCount(1)
}
```

**Pop 操作**（单消费者）：

```go
func (p *queue) Pop() any {
    tail := p.tail
    next := atomic.LoadPointer(&tail.next)
    if next != nil {
        p.tail = next
        v := next.val
        next.val = nil
        tail.next = nil
        queueNodePool.Put(tail)  // 回收节点
        p._setCount(-1)
        return v
    }
    return nil
}
```

**设计亮点**：
- 使用 `sync.Pool` 复用 `queueNode`，减少内存分配
- 基于 Michael-Scott 队列变体，使用 stub 节点简化边界处理
- Push 是 CAS 操作，支持多 goroutine 并发写入
- Pop 由 Actor 的 goroutine 独占调用，天然单消费者

### 6.2 Mailbox 封装

`mailbox` 在 `queue` 基础上增加了函数注册表：

```go
type mailbox struct {
    queue                                          // 内嵌无锁队列
    name    string                                 // 邮箱名称（"local"/"remote"）
    funcMap map[string]*creflect.FuncInfo           // 函数注册表
}
```

`funcMap` 将函数名映射到反射信息 (`FuncInfo`)，包含函数的 `reflect.Value`、参数类型列表等，用于消息到达时的动态调用。

### 6.3 通知机制

队列通过带缓冲的 channel（容量为 1）通知消费者：

```go
func (p *queue) _setCount(delta int32) {
    count := atomic.AddInt32(&p.count, delta)
    if count > 0 {
        select {
        case p.C <- count:  // 非阻塞写入
        default:            // channel 已有通知则跳过
        }
    }
}
```

这种设计确保了：
- **不丢失通知**：只要队列非空，channel 中一定有信号
- **不阻塞生产者**：使用 `select + default` 避免阻塞
- **合并通知**：多次 Push 可能只触发一次 channel 写入，减少 goroutine 唤醒次数

---

## 7. 事件系统

### 7.1 发布/订阅模型

ActorGo 的事件系统基于 Actor 内部的发布/订阅模式：

**注册**：Actor 通过 `Event().Register(eventName, handler, uniqueID)` 订阅事件

```go
type actorEvent struct {
    queue                                       // 事件队列
    thisActor *Actor
    funcMap   map[string][]IEventFunc            // eventName → handler 列表
}
```

注册时会同时在 `System.actorEventMap` 中记录映射关系：

```go
// System.actorEventMap 结构：
// map[eventName] → map[actorPath] → uniqueID
```

**发布**：通过 `System.PostEvent(eventData)` 发布事件

```
PostEvent(data)
  ├── 根据 data.Name() 在 actorEventMap 中查找订阅者
  ├── 遍历所有订阅的 Actor
  │   ├── 检查 uniqueID 过滤条件
  │   └── 投递到目标 Actor 的 event 队列
  └── Actor 在主循环中处理 processEvent()
```

### 7.2 UniqueID 过滤

事件数据实现 `IEventData` 接口：

```go
type IEventData interface {
    Name() string     // 事件名
    UniqueID() int64  // 唯一 ID
}
```

当订阅时指定了 `uniqueID`，只有事件数据的 `UniqueID()` 与之匹配时才会被投递。这用于实现定向事件（例如只通知特定玩家的 Actor）。如果未指定 `uniqueID`（设为 nil），则接收所有同名事件。

---

## 8. 定时器系统

### 8.1 分层时间轮

框架在 `extend/time_wheel` 中实现了经典的 **分层时间轮**（Hierarchical Timing Wheel）算法：

```go
type TimeWheel struct {
    tick          int64              // 最小精度
    wheelSize     int64              // 槽位数
    interval      int64              // tick × wheelSize
    currentTime   int64              // 当前时间
    buckets       []*bucket          // 时间槽数组
    queue         *DelayQueue        // 延迟队列
    overflowWheel unsafe.Pointer     // 溢出轮（上层时间轮）
}
```

**工作原理**：

1. 时间轮由固定数量的 `bucket`（时间槽）组成
2. 每个 `bucket` 维护一个定时器链表
3. 当定时器的到期时间超出当前轮的范围时，自动创建上层时间轮（溢出轮）
4. 时间推进时，高层轮的 bucket 到期后会将定时器 "降级" 到低层轮

全局定时器实例配置为 `tick=10ms, wheelSize=3600`：

```go
var globalTimer = ctimewheel.NewTimeWheel(10*time.Millisecond, 3600)
```

### 8.2 Actor 定时器集成

每个 Actor 内部的 `actorTimer` 将时间轮与 Actor 消息循环集成：

```go
type actorTimer struct {
    queue                                    // 定时器触发队列
    thisActor    *Actor
    timerInfoMap map[uint64]*timerInfo         // timerID → 定时器信息
}
```

支持的定时器类型：

| 方法 | 说明 |
|------|------|
| `Add(delay, fn, async)` | 循环定时器 |
| `AddOnce(delay, fn, async)` | 一次性定时器 |
| `AddFixedHour(h, m, s, fn)` | 每日固定时刻 |
| `AddSchedule(scheduler, fn)` | 自定义调度策略 |
| `Remove(id)` / `RemoveAll()` | 移除定时器 |

**集成原理**：时间轮到期时，将 `timerID` Push 到 Actor 的 timer 队列。Actor 主循环 `select` 监听 `<-p.timer.C`，触发时调用 `processTimer()` 在 Actor goroutine 中执行回调，保证定时器回调与消息处理在同一 goroutine 中串行执行。

`async` 参数控制回调是否异步：
- `async=false`（默认）：在 Actor goroutine 中执行，保证线程安全
- `async=true`：在独立 goroutine 中执行，适用于不需要访问 Actor 状态的操作

---

## 9. 网络层

### 9.1 连接器抽象

连接器的接口定义：

```go
type IConnector interface {
    IComponent
    OnConnect(fn func(conn net.Conn))  // 注册新连接回调
    Start()                            // 开始接受连接
    Stop()                             // 停止
}
```

### 9.2 TCP 连接器

`TCPConnector` 基于 `net.Listener` 实现：

```
TCPConnector.Start()
  ├── net.Listen("tcp", address)
  ├── goroutine: Accept() 循环
  │   └── 新连接 → connChan <- conn
  └── goroutine: 消费 connChan
      └── onConnectFunc(conn)
```

支持 TLS 配置（通过 `WithCert` 选项）。

### 9.3 WebSocket 连接器

`WSConnector` 基于 gorilla/websocket 的 `http.Handler`：

```
WSConnector.Start()
  ├── http.Handle(path, upgrader)
  ├── WebSocket 升级
  │   └── 新连接 → WSConn 适配 net.Conn
  │       └── connChan <- wsConn
  └── goroutine: 消费 connChan
      └── onConnectFunc(wsConn)
```

`WSConn` 将 WebSocket 连接适配为 `net.Conn` 接口，使上层代码对传输协议无感知。

### 9.4 网络解析器

`INetParser` 是连接器与 Actor 系统之间的桥梁：

```go
type INetParser interface {
    Load(app IApplication)                     // 加载解析器
    Connectors() []IConnector                   // 返回所有连接器
    SetOnDataRoute(fn func(...))                // 设置数据路由
}
```

框架提供两种内置解析器：
- **Pomelo 解析器**：兼容网易 Pomelo 协议，支持握手、心跳、路由压缩
- **Simple 解析器**：轻量级自定义二进制协议

---

## 10. 协议层

### 10.1 Pomelo 协议

Pomelo 协议分为 **Packet** 层和 **Message** 层：

**Packet 层**（传输帧）：

```
+--------+----------+--------+
| type   | length   |  data  |
| 1 byte | 3 bytes  | N bytes|
+--------+----------+--------+
```

Packet 类型：

| 类型 | 值 | 说明 |
|------|---|------|
| Handshake | 1 | 握手请求/响应 |
| HandshakeAck | 2 | 握手确认 |
| Heartbeat | 3 | 心跳 |
| Data | 4 | 业务数据 |
| Kick | 5 | 踢下线 |

**Message 层**（业务消息，嵌入 Data Packet 中）：

```
+-------+----------+-------+------+
| flag  | message  | route | body |
| 1byte | id(varint)|       |      |
+-------+----------+-------+------+
```

Message 类型（由 flag 高 3 位决定）：

| 类型 | 说明 | 是否有 messageID | 是否有 route |
|------|------|:---:|:---:|
| Request | 客户端请求 | 是 | 是 |
| Notify | 客户端通知 | 否 | 是 |
| Response | 服务端响应 | 是 | 否 |
| Push | 服务端推送 | 否 | 是 |

**路由格式**：`nodeType.handlerName.methodName`，例如 `game.room.join`

### 10.2 Simple 协议

Simple 协议是框架自定义的轻量级二进制协议：

```
+----------+----------+---------+
|   MID    | dataLen  |  data   |
| 4 bytes  | 4 bytes  | N bytes |
+----------+----------+---------+
```

- `MID`（Message ID）：4 字节，用于路由映射
- `dataLen`：4 字节，数据部分长度
- `data`：变长数据体

路由通过 MID 查表实现：

```go
type NodeRoute struct {
    NodeType string   // 目标节点类型
    ActorID  string   // 目标 Actor ID
    FuncName string   // 目标函数名
}

// 注册路由
AddNodeRoute(mid uint32, route *NodeRoute)
```

Simple 协议的优势是头部固定 8 字节，解析高效，适合对性能要求极高的场景。

### 10.3 序列化

框架通过 `ISerializer` 接口抽象序列化：

```go
type ISerializer interface {
    Name() string
    Marshal(v any) ([]byte, error)
    Unmarshal(data []byte, v any) error
}
```

内置两种实现：

| 实现 | 用途 |
|------|------|
| **Protobuf** | 默认序列化器，用于集群内部通信 |
| **JSON** | 基于 jsoniter，用于客户端通信或配置解析 |

集群内部统一使用 Protobuf 序列化 `ClusterPacket`，保证跨节点通信的效率。

---

## 11. 会话管理

### 11.1 Session 结构

Session 由 Protobuf 定义，承载客户端连接的上下文信息：

```protobuf
message Session {
    string sid       = 1;   // Session ID（NUID 生成）
    int64  uid       = 2;   // 用户 ID
    string agentPath = 3;   // Agent 的 Actor 路径
    string ip        = 4;   // 客户端 IP
    map<string,string> data = 5;  // 自定义数据
}
```

Session 随消息在集群间传递，使任何节点都能获取客户端的上下文信息。

### 11.2 Agent 模型

每个客户端连接对应一个 Agent，Agent 作为 Actor 的子 Actor 存在：

```
Connector (TCP/WS)
  └── OnConnect(conn)
      └── 创建 Agent
          ├── Session{sid: nuid.Next(), agentPath: parentActor.path}
          ├── BindSID(agent)    注册到 sidAgentMap
          └── agent.Run()       启动读写循环
```

Agent 的状态机：

```
AgentInit → AgentWaitAck → AgentWorking → AgentClosed
```

Agent 内部维护两个核心通道：
- `chPending`：待发送数据队列
- `chWrite`：写入 goroutine 消费队列

### 11.3 SID/UID 绑定与映射

`Agents` 管理器维护两个映射表：

```go
sidAgentMap *sync.Map    // SID → Agent
uidMap      *sync.Map    // UID → SID
```

- **Bind(sid, uid)**：绑定用户 ID 到 Session，建立 UID → SID 的映射
- **Unbind(sid)**：解除绑定
- **GetAgentWithUID(uid)**：通过 UID 查找 Agent，用于服务端主动推送

这种双映射设计支持：
- 按 SID 查找（连接级别）
- 按 UID 查找（用户级别）
- 批量广播（遍历所有 Agent）

---

## 12. 集群通信

### 12.1 NATS 集群架构

ActorGo 使用 NATS 作为集群消息总线，每个节点是一个 NATS 客户端：

```
┌──────────┐         ┌──────────┐         ┌──────────┐
│  Node A  │◄───────►│  NATS    │◄───────►│  Node B  │
│  (gate)  │         │  Server  │         │  (game)  │
└──────────┘         └──────────┘         └──────────┘
                          ▲
                          │
                     ┌──────────┐
                     │  Node C  │
                     │  (center)│
                     └──────────┘
```

NATS 连接通过连接池管理，支持：
- 连接池大小配置（`pool_size`）
- 轮询负载均衡
- 自动重连
- 消息对象池（`sync.Pool`）

### 12.2 Subject 命名规范

NATS Subject 采用分层命名，包含前缀、消息类型、节点类型和节点 ID：

```
actorgo-{prefix}.local.{nodeType}.{nodeID}       本地消息
actorgo-{prefix}.remote.{nodeType}.{nodeID}      远程单播
actorgo-{prefix}.remoteType.{nodeType}           按类型广播
actorgo-{prefix}.reply.{nodeType}.{nodeID}       回复
```

示例：
```
actorgo-game.remote.gate.gate-1       → 发给 gate-1 节点的远程消息
actorgo-game.remoteType.game          → 发给所有 game 类型节点
actorgo-game.reply.gate.gate-1        → gate-1 的 RPC 回复
```

### 12.3 集群消息流转

**发送端**：

```
Actor.Call(target, funcName, arg)
  ├── targetNodeID != localNodeID
  ├── 序列化参数 → argBytes
  ├── 构造 ClusterPacket{source, target, funcName, argBytes, session}
  ├── Protobuf 序列化 ClusterPacket → bytes
  └── NATS Publish(subject, bytes)
```

**接收端**：

```
NATS Subscribe(remoteSubject)
  ├── 收到 natsMsg
  ├── Protobuf 反序列化 → ClusterPacket
  ├── BuildClusterMessage(packet) → Message
  │   ├── IsCluster = true
  │   ├── Args = packet.ArgBytes（[]byte 形式）
  │   └── Reply = natsMsg.Reply（若有）
  └── ActorSystem.PostRemote(message)
      └── Actor.remoteMail.Push(message)
```

**ClusterPacket** 结构（Protobuf）：

```protobuf
message ClusterPacket {
    int64   buildTime  = 1;
    string  sourcePath = 2;
    string  targetPath = 3;
    string  funcName   = 4;
    bytes   argBytes   = 5;
    Session session    = 6;
}
```

### 12.4 跨节点 RPC

**Call（异步，不等待返回）**：使用 NATS 的 `Publish` 模式

**CallWait（同步，等待返回）**：使用 NATS 的 Request/Reply 模式

```
调用方                              NATS                 被调方
  │                                  │                     │
  │ RequestSync(subject, data)       │                     │
  │ ─────────────────────────────►   │                     │
  │                                  │  natsMsg.Reply 自动填充
  │                                  │ ──────────────────► │
  │                                  │                     │ 处理消息
  │                                  │                     │ fi.Value.Call()
  │                                  │  ◄────────────────  │ PublishMsg(reply)
  │  ◄─────────────────────────────  │                     │
  │  waiters[reqID] <- response      │                     │
  │                                  │                     │
```

NATS 连接池使用自定义的 `reqID + waiters` 机制替代原生 `nats.Request`，支持在连接池中使用同步请求。

---

## 13. 服务发现

### 13.1 发现接口抽象

```go
type IDiscovery interface {
    Load(app IApplication)
    Name() string
    Map() map[string]IMember                              // 所有成员
    ListByType(nodeType string, ...) []IMember            // 按类型查询
    Random(nodeType string) (IMember, bool)               // 随机选一个
    GetType(nodeID string) (string, error)                // 获取节点类型
    GetMember(nodeID string) (IMember, bool)              // 获取成员
    AddMember(member IMember)                             // 添加成员
    RemoveMember(nodeID string)                           // 移除成员
    OnAddMember(listener MemberListener)                  // 添加监听
    OnRemoveMember(listener MemberListener)               // 移除监听
    Stop()
}
```

### 13.2 默认模式（静态配置）

从 `profile.json` 中的 `node` 配置读取节点列表，适用于开发和测试环境：

```json
{
  "node": {
    "gate-1":  { "type": "gate",  "address": ":9001" },
    "game-1":  { "type": "game",  "address": ":9002" }
  }
}
```

特点：
- 配置文件驱动，无需外部依赖
- 适合固定拓扑的开发测试
- 不支持动态节点加入/退出

### 13.3 Master 模式（NATS）

基于 NATS 的主节点发现模式，适用于轻量级集群：

```
        Master 节点
       ┌──────────┐
       │  订阅     │
       │ register  │◄─── Client 发送注册请求
       │ heartbeat │◄─── Client 定期心跳
       │          │
       │  发布     │
       │ add       │───► 通知所有 Client 新节点加入
       │ remove    │───► 通知所有 Client 节点退出
       └──────────┘

        Client 节点
       ┌──────────┐
       │ Request   │───► Master 注册
       │ Subscribe │◄─── 接收 add/remove 通知
       │ Publish   │───► 定期发送心跳
       └──────────┘
```

NATS Subject 命名：
```
actorgo.{prefix}.discovery.{masterID}.register
actorgo.{prefix}.discovery.{masterID}.add
actorgo.{prefix}.discovery.{masterID}.remove
actorgo.{prefix}.discovery.{masterID}.heartbeat
```

心跳超时由 `cluster_heartbeat_timeout` 配置（默认 3 秒），Master 节点定期检测并移除超时成员。

### 13.4 ETCD 模式

基于 etcd 的强一致性服务发现（以独立组件形式提供）：

- 注册 Key：`/actorgo/node/{nodeID}`
- 使用 Lease + KeepAlive 做 TTL 续期
- 通过 Watch 前缀监听成员变化

适用于生产环境的大规模集群部署。

---

## 14. 配置系统

### 14.1 Profile 设计

框架使用 JSON 格式的 Profile 文件管理配置：

```json
{
  "env": "dev",
  "debug": true,
  "print_level": "debug",
  "include": ["logger.json", "cluster.json"],
  "node": { ... },
  "logger": { ... },
  "cluster": { ... }
}
```

特性：
- **include 机制**：支持拆分子配置文件，`include` 字段引用的文件会被合并
- **多环境切换**：通过 `env` 字段区分环境
- **动态访问**：基于 jsoniter.Any 的链式访问，如 `GetConfig("cluster").GetConfig("nats")`

### 14.2 节点配置

每个节点的配置包含：

```go
type Node struct {
    nodeID     string                // 节点唯一 ID
    nodeType   string                // 节点类型（gate/game/center...）
    address    string                // 监听地址
    rpcAddress string                // RPC 地址
    settings   cfacade.ProfileJSON   // 自定义配置
    enabled    bool                  // 是否启用
}
```

### 14.3 数据配置组件

`data-config` 组件用于管理游戏策划配表：

```go
type IDataConfig interface {
    Register(configs ...IConfig)              // 注册配表
    GetBytes(configName string) ([]byte, bool) // 获取原始数据
    GetParser(name string) IDataParser         // 获取解析器
    GetDataSource() IDataSource                // 获取数据源
}
```

支持两种数据源：
- **SourceFile**：本地文件 + 文件监听（watcher），支持热更新
- **SourceRedis**：Redis 存储，适合运营期动态修改配表

数据解析器：
- **ParserJson**：基于 jsoniter 的 JSON 解析

---

## 15. 日志系统

基于 uber/zap 封装的高性能日志系统：

```go
type ActorLogger struct {
    *zap.SugaredLogger
    *Config
}

type Config struct {
    LogLevel        string   // 日志输出级别
    StackLevel      string   // 堆栈捕获级别
    EnableConsole   bool     // 控制台输出
    EnableWriteFile bool     // 文件输出
    MaxAge          int      // 保留天数
    FilePathFormat  string   // 文件路径格式
}
```

特性：
- 支持按节点配置日志（路径中的 `%nodeid`、`%nodetype` 变量替换）
- 日志轮转：基于 `rotatelogs`，支持按时间和大小轮转
- 级别控制：通过 `profile` 的 `print_level` 统一配置
- 高性能：基于 zap 的结构化日志，零分配

---

## 16. 扩展组件

### 16.1 Cron 定时任务

基于 `robfig/cron` 封装的全局定时任务组件：

```go
type Cron struct {
    *cron.Cron
}

// 使用方式
AddFunc(spec, cmd)              // 标准 cron 表达式
AddEveryDayFunc(cmd, h, m, s)   // 每日定时
AddEveryHourFunc(cmd, m, s)     // 每小时定时
AddDurationFunc(cmd, duration)   // 固定间隔
```

与 Actor 内部的 Timer 不同，Cron 组件是全局级别的，不在 Actor goroutine 中执行。

### 16.2 Gin HTTP 服务

集成 Gin 框架的 HTTP 服务组件，支持中间件和分组路由：

```go
type GinComponent struct {
    *gin.Engine
    groups []*Group
}
```

用于提供 RESTful API、GM 工具、后台管理等 HTTP 服务。

### 16.3 GORM 数据库

集成 GORM 的 MySQL 数据库组件，支持多数据库配置：

```go
type GormComponent struct {
    dbMap map[string]*gorm.DB
}
```

### 16.4 MongoDB

集成 mongo-driver 的 MongoDB 组件，同样支持多数据库配置。

---

## 17. 工具库

`extend/` 目录提供了丰富的工具包：

| 包 | 功能 |
|---|------|
| `ctime` | 时间操作（格式化、比较、差值、时区偏移） |
| `ctimewheel` | 分层时间轮 |
| `csnowflake` | 雪花 ID 生成器 |
| `cnuid` | NATS 风格唯一 ID |
| `cqueue` | 通用队列 |
| `cmap` | 并发安全 Map |
| `cjson` | jsoniter 封装 |
| `ccrypto` | AES/RSA 加解密 |
| `ccompress` | 数据压缩 |
| `cbase58` | Base58 编码 |
| `cgob` | Gob 序列化 |
| `chttp` | HTTP 客户端 |
| `cnet` | 网络工具（获取本机 IP 等） |
| `cregex` | 正则表达式（带缓存） |
| `creflect` | 反射工具（函数信息提取） |
| `cslice` | 切片操作 |
| `cstring` | 字符串操作 |
| `csync` | 同步工具（限流器、并发控制） |
| `cutils` | 通用工具（Try/Catch、判空） |

---

## 18. 性能优化策略

ActorGo 在多个层面进行了性能优化：

### 对象池（sync.Pool）

| 池化对象 | 位置 | 说明 |
|---------|------|------|
| `Message` | `facade/message.go` | 消息对象复用 |
| `queueNode` | `net/actor/queue.go` | 队列节点复用 |
| `ClusterPacket` | `net/proto/cluster_packet.go` | 集群数据包复用 |
| `nats.Msg` | `net/nats/msg_pool.go` | NATS 消息复用 |

### 无锁设计

- Actor 消息队列使用原子操作替代互斥锁
- `actorMap` 和 `childActors` 使用 `sync.Map`
- `running` 状态使用 `atomic.Int32`

### 序列化优化

- 集群内部使用 Protobuf 二进制序列化，体积小、速度快
- JSON 使用 jsoniter 替代标准库，性能提升数倍

### 时间轮

- 全局共享一个时间轮实例，避免每个 Actor 创建独立的 timer goroutine
- 定时器到期后通过队列投递到 Actor，合并处理

### Channel 通知优化

- 队列通知 channel 容量为 1，使用非阻塞写入
- 多次 Push 合并为一次通知，减少 goroutine 切换

---

## 19. 典型游戏服务端架构

基于 ActorGo 构建的分布式游戏服务端典型架构：

```
                          ┌──────────┐
                          │ 客户端    │
                          │ Client   │
                          └────┬─────┘
                               │ TCP/WebSocket
                     ┌─────────┴─────────┐
                     │      网关服         │
                     │   Gate Node        │
                     │ ┌───────────────┐  │
                     │ │ Connector     │  │
                     │ │ Pomelo/Simple │  │
                     │ │ Agent(子Actor)│  │
                     │ └───────────────┘  │
                     └─────────┬─────────┘
                               │ NATS
              ┌────────────────┼────────────────┐
              │                │                │
     ┌────────┴───────┐  ┌────┴────────┐  ┌────┴────────┐
     │    中心服       │  │   游戏服     │  │   Web服      │
     │  Center Node   │  │  Game Node  │  │  Web Node   │
     │ ┌────────────┐ │  │ ┌─────────┐ │  │ ┌─────────┐ │
     │ │ 帐号 Actor  │ │  │ │房间 Actor│ │  │ │Gin HTTP │ │
     │ │ 排行 Actor  │ │  │ │匹配 Actor│ │  │ │GM 工具  │ │
     │ │ 邮件 Actor  │ │  │ │大厅 Actor│ │  │ └─────────┘ │
     │ └────────────┘ │  │ └─────────┘ │  └─────────────┘
     └────────────────┘  └─────────────┘
              │                │                │
              └────────────────┼────────────────┘
                               │
                    ┌──────────┴──────────┐
                    │  基础设施             │
                    │ ┌──────┐ ┌────────┐ │
                    │ │ NATS │ │ MySQL  │ │
                    │ │      │ │ Redis  │ │
                    │ │      │ │ MongoDB│ │
                    │ └──────┘ └────────┘ │
                    └─────────────────────┘
```

**节点类型与职责**：

| 节点类型 | 职责 | 是否前端 |
|---------|------|---------|
| Gate | 客户端连接管理、协议解析、路由转发 | 是 |
| Game | 游戏逻辑（房间、匹配、战斗） | 否 |
| Center | 全局服务（帐号、排行、邮件） | 否 |
| Web | HTTP API、后台管理 | 否 |

**消息流转示例**（玩家加入房间）：

```
1. 客户端 → Gate: Request("game.room.join", {roomID: 100})
2. Gate 的 Pomelo Actor 解析路由 → nodeType=game, actor=room, func=join
3. Gate → NATS → Game 节点
4. Game 的 room Actor 处理加入逻辑
5. Game → NATS → Gate: Response(joinResult)
6. Gate → 客户端: Response
```

---

> 本文档基于 ActorGo 框架源码分析生成，详细描述了框架的设计原理与实现机制。如需进一步了解具体实现细节，请参考源码目录下的各包实现。
