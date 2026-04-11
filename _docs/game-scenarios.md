# ActorGo 游戏品类使用场景与开发示范

## 目录

- [前言：Actor 建模思路](#前言actor-建模思路)
- [一、即时通讯 / 聊天室](#一即时通讯--聊天室)
- [二、回合制卡牌 / RPG](#二回合制卡牌--rpg)
- [三、MMORPG / 大世界](#三mmorpg--大世界)
- [四、实时对战 / MOBA / FPS](#四实时对战--moba--fps)
- [五、棋牌 / 桌游](#五棋牌--桌游)
- [六、SLG / 策略经营](#六slg--策略经营)
- [七、休闲竞技 / 派对游戏](#七休闲竞技--派对游戏)
- [八、Roguelike / 单局制游戏](#八roguelike--单局制游戏)
- [九、通用模块：跨品类可复用的 Actor 设计](#九通用模块跨品类可复用的-actor-设计)
- [十、最佳实践与注意事项](#十最佳实践与注意事项)

---

## 前言：Actor 建模思路

在使用 ActorGo 开发任何品类的游戏之前，需要理解 Actor 建模的核心思路：

### 一个 Actor 应该代表什么？

| 原则 | 说明 |
|------|------|
| **独立状态单元** | 一个 Actor 管理一份独立的、不与其他 Actor 共享的状态 |
| **串行处理边界** | 需要串行处理的逻辑放在同一个 Actor 中 |
| **生命周期一致** | Actor 的生命周期应与其管理的业务实体一致 |

### 常见建模方式

```
游戏实体          →  Actor 模型
─────────────────────────────────
单个玩家会话       →  子 Actor（Agent）
一个游戏房间       →  子 Actor
一个游戏大厅       →  父 Actor
全服排行榜         →  独立 Actor
匹配队列           →  独立 Actor
全服邮件系统       →  独立 Actor
```

### 框架核心 API 速查

```go
// 嵌入 Base 获得 Actor 能力
type MyActor struct {
    cactor.Base   // 或 pomelo.ActorBase（Pomelo 协议时）
}

// 通过 load 注册消息处理函数
func (p *MyActor) load(actor *cactor.Actor) {
    p.Actor = actor
    // 注册客户端消息（Local）
    p.Local().Register("join", p.onJoin)
    // 注册 Actor 间消息（Remote）
    p.Remote().Register("settle", p.onSettle)
    // 注册事件
    p.Event().Register("playerOffline", p.onPlayerOffline)
    // 注册定时器
    p.Timer().Add(time.Second, p.onTick)
}

// Local 函数签名：func(session, args)
func (p *MyActor) onJoin(session *cproto.Session, msg *pb.JoinReq) { ... }

// Remote 函数签名：func(args) 或 func(args) (reply, code)
func (p *MyActor) onSettle(req *pb.SettleReq) { ... }
```

---

## 一、即时通讯 / 聊天室

### 场景特点

- 多房间并行，每个房间独立广播
- 玩家动态加入/退出房间
- 消息量大但逻辑简单

### Actor 建模

```
Gate 节点（前端）                    Chat 节点（后端）
┌─────────────────┐               ┌─────────────────────┐
│ AgentActor      │               │ ChatActor (父)       │
│ ├── Agent-sid1  │──── NATS ────►│ ├── Room-1001 (子)  │
│ ├── Agent-sid2  │               │ ├── Room-1002 (子)  │
│ └── Agent-sid3  │               │ └── Room-1003 (子)  │
└─────────────────┘               └─────────────────────┘
```

### 节点与协议配置

```go
func main() {
    // Gate 节点 —— 前端，处理客户端连接
    app := actorgo.Configure("profile.json", "gate-1", true, actorgo.Cluster)

    pomeloActor := pomelo.NewActor("agent")
    pomeloActor.AddConnector(cconnector.NewWS(":9001"))
    app.SetNetParser(pomeloActor)

    app.Startup()
}
```

```go
func main() {
    // Chat 节点 —— 后端，处理聊天逻辑
    app := actorgo.Configure("profile.json", "chat-1", false, actorgo.Cluster)
    app.AddActors(&ChatActor{})
    app.Startup()
}
```

### ChatActor（父 Actor）—— 房间管理器

```go
type ChatActor struct {
    cactor.Base
    rooms map[string]*RoomState // 房间元信息（人数等）
}

func (p *ChatActor) AliasID() string { return "chat" }

func (p *ChatActor) OnInit() {
    p.rooms = make(map[string]*RoomState)
    p.Remote().Register("createRoom", p.onCreateRoom)
    p.Remote().Register("listRooms", p.onListRooms)
}

// 当消息目标为子 Actor 但子 Actor 不存在时，动态创建房间
func (p *ChatActor) OnFindChild(m *cfacade.Message) (cfacade.IActor, bool) {
    roomID := m.TargetPath().ChildID
    roomHandler := &RoomActor{roomID: roomID}
    child, err := p.Child().Create(roomID, roomHandler)
    if err != nil {
        return nil, false
    }
    p.rooms[roomID] = &RoomState{PlayerCount: 0}
    return child, true
}

func (p *ChatActor) onCreateRoom(req *pb.CreateRoomReq) (*pb.CreateRoomRsp, int32) {
    roomID := generateRoomID()
    roomHandler := &RoomActor{roomID: roomID}
    p.Child().Create(roomID, roomHandler)
    p.rooms[roomID] = &RoomState{}
    return &pb.CreateRoomRsp{RoomID: roomID}, ccode.OK
}

func (p *ChatActor) onListRooms(req *pb.ListRoomsReq) (*pb.ListRoomsRsp, int32) {
    // 直接访问 rooms，因为在同一 goroutine，无需加锁
    rsp := &pb.ListRoomsRsp{}
    for id, state := range p.rooms {
        rsp.Rooms = append(rsp.Rooms, &pb.RoomInfo{
            RoomID:      id,
            PlayerCount: state.PlayerCount,
        })
    }
    return rsp, ccode.OK
}
```

### RoomActor（子 Actor）—— 单个聊天房间

```go
type RoomActor struct {
    cactor.Base
    roomID  string
    members map[int64]*MemberInfo // uid → 成员信息
}

func (p *RoomActor) AliasID() string { return p.roomID }

func (p *RoomActor) OnInit() {
    p.members = make(map[int64]*MemberInfo)
    p.Local().Register("join", p.onJoin)
    p.Local().Register("chat", p.onChat)
    p.Local().Register("leave", p.onLeave)
}

func (p *RoomActor) onJoin(session *cproto.Session, msg *pb.JoinReq) {
    p.members[session.Uid] = &MemberInfo{
        UID:       session.Uid,
        AgentPath: session.AgentPath,
        SID:       session.Sid,
        Name:      msg.Nickname,
    }

    // 响应客户端
    pomelo.Response(p, session.AgentPath, session.Sid, session.GetMID(), &pb.JoinRsp{
        RoomID:  p.roomID,
        Members: p.memberList(),
    })

    // 广播给房间内其他人
    p.broadcastToRoom(&pb.JoinNotify{
        UID:  session.Uid,
        Name: msg.Nickname,
    }, "onMemberJoin", session.Uid)
}

func (p *RoomActor) onChat(session *cproto.Session, msg *pb.ChatReq) {
    chatMsg := &pb.ChatNotify{
        UID:     session.Uid,
        Content: msg.Content,
        Time:    time.Now().Unix(),
    }
    // 广播给房间所有人（包括发送者）
    p.broadcastToRoom(chatMsg, "onChat", 0)
}

func (p *RoomActor) onLeave(session *cproto.Session, msg *pb.LeaveReq) {
    delete(p.members, session.Uid)
    pomelo.Response(p, session.AgentPath, session.Sid, session.GetMID(), &pb.LeaveRsp{})

    if len(p.members) == 0 {
        p.Exit() // 房间空了，销毁子 Actor
        return
    }

    p.broadcastToRoom(&pb.LeaveNotify{UID: session.Uid}, "onMemberLeave", session.Uid)
}

// 向房间内所有成员推送消息
func (p *RoomActor) broadcastToRoom(v any, route string, excludeUID int64) {
    for uid, member := range p.members {
        if uid == excludeUID {
            continue
        }
        pomelo.PushWithSID(p, member.AgentPath, member.SID, route, v)
    }
}
```

**要点**：
- 每个聊天房间是一个子 Actor，天然隔离状态
- `members` map 只在 RoomActor 的 goroutine 中访问，无需加锁
- `OnFindChild` 实现按需创建房间
- 房间空了时调用 `Exit()` 自动销毁

---

## 二、回合制卡牌 / RPG

### 场景特点

- 强角色状态（属性、装备、背包、任务）
- 回合制战斗，节奏较慢
- 关注数据持久化和跨系统交互

### Actor 建模

```
Gate 节点                  Game 节点                    Center 节点
┌────────────┐            ┌─────────────────┐          ┌─────────────────┐
│ AgentActor │            │ PlayerActor (父) │          │ RankActor       │
│ ├── sid1   │── NATS ──►│ ├── uid-10001   │── NATS──►│                 │
│ └── sid2   │            │ └── uid-10002   │          │ MailActor       │
└────────────┘            │                 │          │                 │
                          │ BattleActor (父) │          │ ShopActor       │
                          │ ├── battle-1    │          └─────────────────┘
                          │ └── battle-2    │
                          └─────────────────┘
```

### PlayerActor —— 玩家管理器

```go
type PlayerActor struct {
    pomelo.ActorBase
}

func (p *PlayerActor) AliasID() string { return "player" }

func (p *PlayerActor) OnInit() {
    p.Remote().Register("login", p.onLogin)
}

// 客户端登录后，动态创建玩家子 Actor
func (p *PlayerActor) OnFindChild(m *cfacade.Message) (cfacade.IActor, bool) {
    childID := m.TargetPath().ChildID
    handler := NewPlayerChildActor(childID)
    child, err := p.Child().Create(childID, handler)
    if err != nil {
        return nil, false
    }
    return child, true
}

func (p *PlayerActor) onLogin(req *pb.LoginReq) (*pb.LoginRsp, int32) {
    // 验证账号，加载玩家数据，创建子 Actor
    playerData := loadPlayerFromDB(req.UID)
    handler := NewPlayerChildActor(playerData)
    p.Child().Create(fmt.Sprintf("%d", req.UID), handler)
    return &pb.LoginRsp{Success: true}, ccode.OK
}
```

### PlayerChildActor —— 单个玩家实体

```go
type PlayerChildActor struct {
    pomelo.ActorBase
    uid        int64
    profile    *PlayerProfile
    bag        *Bag
    equip      *Equipment
    questMgr   *QuestManager
    saveTimer  uint64
}

func NewPlayerChildActor(data *PlayerData) *PlayerChildActor {
    return &PlayerChildActor{
        uid:     data.UID,
        profile: data.Profile,
        bag:     data.Bag,
        equip:   data.Equipment,
    }
}

func (p *PlayerChildActor) AliasID() string {
    return fmt.Sprintf("%d", p.uid)
}

func (p *PlayerChildActor) OnInit() {
    // 客户端消息
    p.Local().Register("getProfile", p.onGetProfile)
    p.Local().Register("useItem", p.onUseItem)
    p.Local().Register("equipItem", p.onEquipItem)
    p.Local().Register("startBattle", p.onStartBattle)
    p.Local().Register("acceptQuest", p.onAcceptQuest)

    // Actor 间消息
    p.Remote().Register("addItem", p.onAddItem)
    p.Remote().Register("battleResult", p.onBattleResult)

    // 事件监听
    p.Event().Register("dailyReset", p.onDailyReset, p.uid)

    // 定时存档（每 60 秒）
    p.saveTimer = p.Timer().Add(60*time.Second, p.autoSave)
}

func (p *PlayerChildActor) OnStop() {
    p.saveToDatabase() // 下线时存档
}

// ─── 客户端请求处理 ───

func (p *PlayerChildActor) onGetProfile(session *cproto.Session, msg *pb.GetProfileReq) {
    p.Response(session, &pb.GetProfileRsp{
        Name:  p.profile.Name,
        Level: p.profile.Level,
        Gold:  p.profile.Gold,
    })
}

func (p *PlayerChildActor) onUseItem(session *cproto.Session, msg *pb.UseItemReq) {
    item := p.bag.Get(msg.ItemID)
    if item == nil {
        p.ResponseCode(session, ccode.ItemNotFound)
        return
    }

    effect := item.Use(p.profile)
    p.bag.Remove(msg.ItemID, 1)
    p.Response(session, &pb.UseItemRsp{Effect: effect})
}

func (p *PlayerChildActor) onStartBattle(session *cproto.Session, msg *pb.StartBattleReq) {
    // 跨 Actor 调用：通知 BattleActor 创建战斗
    battleReq := &pb.CreateBattleReq{
        UID:      p.uid,
        StageID:  msg.StageID,
        TeamData: p.equip.GetTeamData(),
    }
    // 本节点调用 battle Actor
    targetPath := p.NewNodePath("battle")
    p.Call(targetPath, "createBattle", battleReq)

    p.Response(session, &pb.StartBattleRsp{})
}

// ─── Actor 间消息 ───

func (p *PlayerChildActor) onAddItem(req *pb.AddItemReq) {
    p.bag.Add(req.ItemID, req.Count)
    // 推送给客户端
    p.Push(p.getSession(), "onItemChange", &pb.ItemChangeNotify{
        ItemID: req.ItemID,
        Count:  p.bag.GetCount(req.ItemID),
    })
}

func (p *PlayerChildActor) onBattleResult(result *pb.BattleResultReq) {
    if result.Win {
        p.profile.AddExp(result.RewardExp)
        for _, item := range result.RewardItems {
            p.bag.Add(item.ID, item.Count)
        }
    }
    // 推送战斗结果
    p.Push(p.getSession(), "onBattleResult", &pb.BattleResultNotify{Win: result.Win})
}

// ─── 事件和定时器 ───

func (p *PlayerChildActor) onDailyReset(data cfacade.IEventData) {
    p.questMgr.ResetDaily()
    p.profile.ResetDailyData()
}

func (p *PlayerChildActor) autoSave() {
    p.saveToDatabase()
}
```

### BattleActor —— 回合制战斗

```go
type BattleActor struct {
    cactor.Base
}

func (p *BattleActor) AliasID() string { return "battle" }

func (p *BattleActor) OnInit() {
    p.Remote().Register("createBattle", p.onCreateBattle)
}

func (p *BattleActor) OnFindChild(m *cfacade.Message) (cfacade.IActor, bool) {
    return nil, false
}

func (p *BattleActor) onCreateBattle(req *pb.CreateBattleReq) {
    battleID := generateBattleID()
    handler := &BattleChildActor{
        battleID: battleID,
        uid:      req.UID,
        stageID:  req.StageID,
        teamData: req.TeamData,
    }
    p.Child().Create(battleID, handler)
}
```

```go
type BattleChildActor struct {
    cactor.Base
    battleID string
    uid      int64
    stageID  int32
    teamData *pb.TeamData
    round    int32
    state    BattleState
}

func (p *BattleChildActor) AliasID() string { return p.battleID }

func (p *BattleChildActor) OnInit() {
    p.Local().Register("action", p.onPlayerAction)
    p.Remote().Register("aiAction", p.onAIAction)

    p.state = BattleRunning
    p.round = 1

    // 回合超时定时器：30 秒内未操作则自动行动
    p.Timer().AddOnce(30*time.Second, p.onRoundTimeout)
}

func (p *BattleChildActor) onPlayerAction(session *cproto.Session, msg *pb.BattleActionReq) {
    result := p.executeAction(msg.SkillID, msg.TargetID)

    if p.isBattleOver() {
        p.settleBattle()
        return
    }

    p.round++
    pomelo.Response(p, session.AgentPath, session.Sid, session.GetMID(), &pb.BattleActionRsp{
        Round:  p.round,
        Result: result,
    })
}

func (p *BattleChildActor) onRoundTimeout() {
    p.executeAutoAction()
    if p.isBattleOver() {
        p.settleBattle()
    }
}

func (p *BattleChildActor) settleBattle() {
    // 通知玩家 Actor 结算
    playerPath := p.NewChildPath("player", fmt.Sprintf("%d", p.uid))
    p.Call(playerPath, "battleResult", &pb.BattleResultReq{
        Win:         p.state == BattleWin,
        RewardExp:   100,
        RewardItems: p.calculateRewards(),
    })
    p.Exit()
}
```

**要点**：
- 玩家状态（背包、装备）在 PlayerChildActor 中串行访问，无竞态
- 战斗以独立子 Actor 运行，战斗结束后 `Exit()` 释放资源
- `autoSave` 定时器保证数据安全
- `onDailyReset` 通过事件系统触发，仅通知指定 UID（UniqueID 过滤）

---

## 三、MMORPG / 大世界

### 场景特点

- 大量玩家同场景，需要 AOI（区域兴趣）管理
- 实时移动同步、技能战斗
- 场景切换频繁

### Actor 建模

```
Gate 节点                   Scene 节点
┌────────────┐             ┌──────────────────────┐
│ AgentActor │             │ SceneActor (父)       │
│ ├── sid1   │── NATS ───►│ ├── scene-1001 (子)   │ ← 新手村
│ ├── sid2   │             │ ├── scene-2001 (子)   │ ← 主城
│ └── sid3   │             │ └── scene-3001 (子)   │ ← 副本
└────────────┘             └──────────────────────┘

Game 节点                   Center 节点
┌────────────────┐         ┌─────────────┐
│ PlayerActor    │         │ GuildActor  │
│ ├── uid-10001  │         │ TeamActor   │
│ └── uid-10002  │         │ WorldBoss   │
└────────────────┘         └─────────────┘
```

### SceneChildActor —— 单个场景实例

```go
type SceneChildActor struct {
    cactor.Base
    sceneID    int32
    mapID      int32
    entities   map[int64]*Entity    // entityID → 实体
    aoi        *AOIManager          // 九宫格 AOI
    tickTimer  uint64
}

func (p *SceneChildActor) OnInit() {
    p.Local().Register("enterScene", p.onEnterScene)
    p.Local().Register("move", p.onMove)
    p.Local().Register("castSkill", p.onCastSkill)
    p.Local().Register("leaveScene", p.onLeaveScene)

    p.Remote().Register("npcAction", p.onNpcAction)
    p.Remote().Register("spawnMonster", p.onSpawnMonster)

    p.entities = make(map[int64]*Entity)
    p.aoi = NewAOIManager(p.mapID)

    // 场景 Tick：每 100ms 驱动一次逻辑帧
    p.tickTimer = p.Timer().Add(100*time.Millisecond, p.onTick)
}

func (p *SceneChildActor) onEnterScene(session *cproto.Session, msg *pb.EnterSceneReq) {
    entity := &Entity{
        UID:       session.Uid,
        EntityID:  session.Uid,
        Position:  msg.Position,
        AgentPath: session.AgentPath,
        SID:       session.Sid,
    }

    p.entities[entity.EntityID] = entity
    p.aoi.Enter(entity, msg.Position.X, msg.Position.Y)

    // 获取视野内的实体列表
    nearbyEntities := p.aoi.GetNearby(msg.Position.X, msg.Position.Y)

    pomelo.Response(p, session.AgentPath, session.Sid, session.GetMID(), &pb.EnterSceneRsp{
        SceneID:  p.sceneID,
        Entities: entitiesToProto(nearbyEntities),
    })

    // 通知视野内其他玩家
    p.broadcastToNearby(entity, "onEntityEnter", &pb.EntityEnterNotify{
        Entity: entityToProto(entity),
    })
}

func (p *SceneChildActor) onMove(session *cproto.Session, msg *pb.MoveReq) {
    entity := p.entities[session.Uid]
    if entity == nil {
        return
    }

    oldX, oldY := entity.Position.X, entity.Position.Y
    entity.Position = msg.Position
    entity.Velocity = msg.Velocity

    // 更新 AOI 位置
    p.aoi.Move(entity, oldX, oldY, msg.Position.X, msg.Position.Y)

    // 广播给视野内的玩家
    p.broadcastToNearby(entity, "onEntityMove", &pb.EntityMoveNotify{
        EntityID: entity.EntityID,
        Position: msg.Position,
        Velocity: msg.Velocity,
    })
}

func (p *SceneChildActor) onCastSkill(session *cproto.Session, msg *pb.CastSkillReq) {
    caster := p.entities[session.Uid]
    if caster == nil {
        return
    }

    result := p.resolveSkill(caster, msg.SkillID, msg.TargetID)

    p.broadcastToNearby(caster, "onSkillEffect", &pb.SkillEffectNotify{
        CasterID: caster.EntityID,
        SkillID:  msg.SkillID,
        Targets:  result.AffectedTargets,
    })
}

func (p *SceneChildActor) onTick() {
    now := time.Now()
    // 驱动怪物 AI
    for _, entity := range p.entities {
        if entity.IsMonster() {
            entity.AI.Update(now, p)
        }
    }
    // 处理 Buff 倒计时
    for _, entity := range p.entities {
        entity.UpdateBuffs(now)
    }
}

// AOI 广播：仅向视野范围内的玩家推送
func (p *SceneChildActor) broadcastToNearby(center *Entity, route string, v any) {
    nearbyEntities := p.aoi.GetNearby(center.Position.X, center.Position.Y)
    for _, e := range nearbyEntities {
        if e.IsPlayer() && e.EntityID != center.EntityID {
            pomelo.PushWithSID(p, e.AgentPath, e.SID, route, v)
        }
    }
}
```

### 跨场景传送

```go
func (p *SceneChildActor) onLeaveScene(session *cproto.Session, msg *pb.LeaveSceneReq) {
    entity := p.entities[session.Uid]
    if entity == nil {
        return
    }

    p.aoi.Leave(entity)
    delete(p.entities, entity.EntityID)

    // 通知视野内的玩家
    p.broadcastToNearby(entity, "onEntityLeave", &pb.EntityLeaveNotify{
        EntityID: entity.EntityID,
    })

    // 跨 Actor 调用：通知目标场景加入
    targetScenePath := p.NewMyChildPath(fmt.Sprintf("scene-%d", msg.TargetSceneID))
    p.Call(targetScenePath, "transferIn", &pb.TransferInReq{
        UID:       session.Uid,
        AgentPath: session.AgentPath,
        SID:       session.Sid,
        Position:  msg.TargetPosition,
    })
}
```

**要点**：
- 场景以子 Actor 形式运行，每个场景实例独立
- 100ms 的 Tick 定时器驱动帧逻辑（AI、Buff、状态同步）
- AOI 管理在 Actor 内部实现，利用串行特性免锁
- 跨场景传送通过 `Call` 实现 Actor 间通信

---

## 四、实时对战 / MOBA / FPS

### 场景特点

- 高频状态同步（帧同步或状态同步）
- 单局制，房间生命周期短
- 匹配系统需全局协调

### Actor 建模

```
Gate 节点          Match 节点           Battle 节点
┌──────────┐      ┌─────────────┐      ┌───────────────────┐
│ Agent    │      │ MatchActor  │      │ BattleActor (父)   │
│ ├── s1   │      │             │      │ ├── battle-5001   │
│ └── s2   │      └─────────────┘      │ └── battle-5002   │
└──────────┘                           └───────────────────┘
```

### MatchActor —— 匹配系统

```go
type MatchActor struct {
    cactor.Base
    queues     map[int32]*MatchQueue  // modeID → 匹配队列
    matchTimer uint64
}

func (p *MatchActor) AliasID() string { return "match" }

func (p *MatchActor) OnInit() {
    p.queues = map[int32]*MatchQueue{
        1: NewMatchQueue(1, 10), // 5v5 模式，需要 10 人
        2: NewMatchQueue(2, 6),  // 3v3 模式，需要 6 人
    }

    p.Remote().Register("joinQueue", p.onJoinQueue)
    p.Remote().Register("cancelQueue", p.onCancelQueue)

    // 每秒执行一次匹配算法
    p.matchTimer = p.Timer().Add(time.Second, p.doMatch)
}

func (p *MatchActor) onJoinQueue(req *pb.JoinQueueReq) (*pb.JoinQueueRsp, int32) {
    queue := p.queues[req.ModeID]
    if queue == nil {
        return nil, ccode.InvalidMode
    }
    queue.Add(&MatchPlayer{
        UID:       req.UID,
        MMR:       req.MMR,
        AgentPath: req.AgentPath,
        JoinTime:  time.Now(),
    })
    return &pb.JoinQueueRsp{}, ccode.OK
}

func (p *MatchActor) doMatch() {
    for _, queue := range p.queues {
        matches := queue.TryMatch()
        for _, match := range matches {
            p.createBattle(match)
        }
    }
}

func (p *MatchActor) createBattle(match *MatchResult) {
    // 跨节点调用：在 Battle 节点创建战斗
    battleNode, found := p.App().Discovery().Random("battle")
    if !found {
        return
    }

    targetPath := cfacade.NewPath(battleNode.GetNodeID(), "battle")
    p.Call(targetPath, "createBattle", &pb.CreateBattleReq{
        BattleID: match.BattleID,
        ModeID:   match.ModeID,
        Teams:    match.Teams,
    })

    // 通知所有匹配成功的玩家
    for _, player := range match.AllPlayers() {
        pomelo.PushWithSID(p, player.AgentPath, player.SID, "onMatchSuccess", &pb.MatchSuccessNotify{
            BattleID: match.BattleID,
            Teams:    match.Teams,
        })
    }
}
```

### BattleChildActor —— 帧同步战斗房间

```go
type FrameSyncBattle struct {
    cactor.Base
    battleID    string
    modeID      int32
    players     map[int64]*BattlePlayer
    frameInputs map[int32][]*pb.PlayerInput // frame → inputs
    currentFrame int32
    frameTimer   uint64
    state        BattleState
}

func (p *FrameSyncBattle) OnInit() {
    p.Local().Register("ready", p.onReady)
    p.Local().Register("input", p.onInput)
    p.Local().Register("reconnect", p.onReconnect)

    // 等待所有玩家准备，30 秒超时
    p.Timer().AddOnce(30*time.Second, p.onReadyTimeout)
}

func (p *FrameSyncBattle) onReady(session *cproto.Session, msg *pb.ReadyReq) {
    player := p.players[session.Uid]
    if player == nil {
        return
    }
    player.Ready = true

    if p.allReady() {
        p.startBattle()
    }
}

func (p *FrameSyncBattle) startBattle() {
    p.state = BattleRunning
    p.broadcastAll("onBattleStart", &pb.BattleStartNotify{})

    // 启动帧驱动定时器：每 66ms 一帧（约 15fps 逻辑帧）
    p.frameTimer = p.Timer().Add(66*time.Millisecond, p.onFrameTick)
}

func (p *FrameSyncBattle) onInput(session *cproto.Session, msg *pb.InputReq) {
    if p.state != BattleRunning {
        return
    }
    // 收集玩家输入，在下一帧统一处理
    p.frameInputs[p.currentFrame] = append(p.frameInputs[p.currentFrame], &pb.PlayerInput{
        UID:   session.Uid,
        Input: msg.Input,
    })
}

func (p *FrameSyncBattle) onFrameTick() {
    inputs := p.frameInputs[p.currentFrame]

    // 广播本帧所有输入给所有玩家
    p.broadcastAll("onFrame", &pb.FrameNotify{
        Frame:  p.currentFrame,
        Inputs: inputs,
    })

    p.currentFrame++
    delete(p.frameInputs, p.currentFrame-1)

    // 检查战斗是否结束（例如到达最大帧数）
    if p.currentFrame > MaxFrames {
        p.endBattle()
    }
}

func (p *FrameSyncBattle) onReconnect(session *cproto.Session, msg *pb.ReconnectReq) {
    // 发送从断线帧到当前帧的所有历史帧数据
    for frame := msg.LastFrame; frame < p.currentFrame; frame++ {
        pomelo.PushWithSID(p, session.AgentPath, session.Sid, "onFrame", &pb.FrameNotify{
            Frame:  frame,
            Inputs: p.frameInputs[frame],
        })
    }
}

func (p *FrameSyncBattle) endBattle() {
    p.Timer().Remove(p.frameTimer)
    p.state = BattleEnded
    p.broadcastAll("onBattleEnd", p.calcResult())
    // 延迟 5 秒销毁，给客户端展示结算
    p.Timer().AddOnce(5*time.Second, func() { p.Exit() })
}

func (p *FrameSyncBattle) broadcastAll(route string, v any) {
    for _, player := range p.players {
        pomelo.PushWithSID(p, player.AgentPath, player.SID, route, v)
    }
}
```

**要点**：
- 匹配系统作为独立 Actor，1 秒 Tick 驱动匹配算法
- 战斗房间以子 Actor 运行，帧同步用 66ms 定时器驱动
- 帧同步核心：收集输入 → 广播帧数据 → 客户端本地计算
- 断线重连：发送历史帧数据追帧

---

## 五、棋牌 / 桌游

### 场景特点

- 严格回合顺序，操作超时自动处理
- 牌桌生命周期明确（创建→游戏→结算→销毁）
- 防作弊要求高，所有逻辑服务端运算

### Actor 建模

```
Gate 节点               Game 节点
┌────────────┐         ┌──────────────────────┐
│ AgentActor │         │ HallActor (大厅)       │
│ ├── s1     │─ NATS ─►│ TableActor (父)       │
│ ├── s2     │         │ ├── table-7001 (子)   │ ← 一桌麻将
│ └── s3     │         │ ├── table-7002 (子)   │ ← 一桌斗地主
│             │         │ └── table-7003 (子)   │
└────────────┘         └──────────────────────┘
```

### TableChildActor —— 一桌麻将

```go
type MahjongTable struct {
    cactor.Base
    tableID    string
    seats      [4]*Seat            // 四个座位
    deck       *MahjongDeck        // 牌堆
    discards   []Tile              // 弃牌区
    turnIndex  int                 // 当前出牌座位
    phase      GamePhase           // 等待/进行/结算
    turnTimer  uint64              // 出牌超时定时器
}

type Seat struct {
    UID       int64
    AgentPath string
    SID       string
    Hand      []Tile              // 手牌
    Melds     []Meld              // 明牌组合（吃/碰/杠）
    Ready     bool
}

func (p *MahjongTable) OnInit() {
    p.Local().Register("sitDown", p.onSitDown)
    p.Local().Register("ready", p.onReady)
    p.Local().Register("discard", p.onDiscard)
    p.Local().Register("action", p.onAction) // 吃/碰/杠/胡
    p.Local().Register("pass", p.onPass)

    p.Event().Register("playerOffline", p.onPlayerOffline)
}

func (p *MahjongTable) onReady(session *cproto.Session, msg *pb.ReadyReq) {
    seat := p.getSeat(session.Uid)
    if seat == nil {
        return
    }
    seat.Ready = true

    if p.allReady() {
        p.startGame()
    }
}

func (p *MahjongTable) startGame() {
    p.phase = PhaseRunning
    p.deck = NewShuffledDeck()

    // 发牌
    for i := 0; i < 4; i++ {
        p.seats[i].Hand = p.deck.Draw(13)
    }
    // 庄家多摸一张
    p.seats[0].Hand = append(p.seats[0].Hand, p.deck.DrawOne())

    // 通知每个玩家自己的手牌
    for i := 0; i < 4; i++ {
        seat := p.seats[i]
        pomelo.PushWithSID(p, seat.AgentPath, seat.SID, "onGameStart", &pb.GameStartNotify{
            Hand:      tilesToProto(seat.Hand),
            SeatIndex: int32(i),
            Dealer:    0,
        })
    }

    p.turnIndex = 0
    p.startTurn()
}

func (p *MahjongTable) startTurn() {
    // 通知当前玩家出牌
    seat := p.seats[p.turnIndex]
    p.broadcastAll("onTurnStart", &pb.TurnStartNotify{SeatIndex: int32(p.turnIndex)})

    // 15 秒出牌超时
    p.turnTimer = p.Timer().AddOnce(15*time.Second, p.onTurnTimeout)
}

func (p *MahjongTable) onDiscard(session *cproto.Session, msg *pb.DiscardReq) {
    seat := p.getSeat(session.Uid)
    if seat == nil || p.seatIndex(session.Uid) != p.turnIndex {
        return
    }

    p.Timer().Remove(p.turnTimer) // 取消超时

    tile := seat.RemoveFromHand(msg.Tile)
    p.discards = append(p.discards, tile)

    // 检查其他玩家是否可以吃/碰/杠/胡
    actions := p.checkOtherActions(tile, p.turnIndex)
    if len(actions) > 0 {
        p.waitForActions(actions)
        return
    }

    p.nextTurn()
}

func (p *MahjongTable) onTurnTimeout() {
    // 超时自动出最后一张牌
    seat := p.seats[p.turnIndex]
    autoTile := seat.Hand[len(seat.Hand)-1]
    seat.RemoveFromHand(autoTile)
    p.discards = append(p.discards, autoTile)

    p.broadcastAll("onAutoDiscard", &pb.AutoDiscardNotify{
        SeatIndex: int32(p.turnIndex),
        Tile:      tileToProto(autoTile),
    })

    p.nextTurn()
}

func (p *MahjongTable) nextTurn() {
    p.turnIndex = (p.turnIndex + 1) % 4

    // 摸牌
    if p.deck.Empty() {
        p.endGame(nil) // 流局
        return
    }

    tile := p.deck.DrawOne()
    seat := p.seats[p.turnIndex]
    seat.Hand = append(seat.Hand, tile)

    // 只通知摸牌的玩家看到牌面
    pomelo.PushWithSID(p, seat.AgentPath, seat.SID, "onDraw", &pb.DrawNotify{
        Tile: tileToProto(tile),
    })

    // 检查自摸
    if checkWin(seat.Hand, seat.Melds) {
        pomelo.PushWithSID(p, seat.AgentPath, seat.SID, "onCanWin", &pb.CanWinNotify{})
    }

    p.startTurn()
}

func (p *MahjongTable) endGame(winner *Seat) {
    p.phase = PhaseSettlement
    result := p.calculateScore(winner)
    p.broadcastAll("onGameEnd", result)

    // 5 秒后销毁牌桌
    p.Timer().AddOnce(5*time.Second, func() { p.Exit() })
}

func (p *MahjongTable) onPlayerOffline(data cfacade.IEventData) {
    // 掉线玩家自动托管
    offlineUID := data.UniqueID()
    seat := p.getSeat(offlineUID)
    if seat != nil {
        seat.AutoPlay = true
    }
}

func (p *MahjongTable) broadcastAll(route string, v any) {
    for _, seat := range p.seats {
        if seat != nil && seat.UID > 0 {
            pomelo.PushWithSID(p, seat.AgentPath, seat.SID, route, v)
        }
    }
}
```

**要点**：
- 每桌游戏是一个子 Actor，状态完全隔离
- 回合超时使用 `AddOnce` 一次性定时器，出牌后立即 `Remove`
- 所有洗牌、发牌、胡牌判定都在服务端完成，防止作弊
- 掉线托管通过事件系统通知

---

## 六、SLG / 策略经营

### 场景特点

- 大量异步操作（建造、行军、采集，均有倒计时）
- 世界地图共享状态
- 联盟/工会系统需要全局协调

### Actor 建模

```
Game 节点                          World 节点
┌──────────────────────┐          ┌──────────────────────┐
│ CityActor (父)        │          │ WorldMapActor        │
│ ├── city-uid1 (子)   │          │ (全局唯一 Actor)      │
│ └── city-uid2 (子)   │          │                      │
│                      │          │ MarchActor (父)       │
│ AllianceActor (父)    │          │ ├── march-5001 (子)  │
│ ├── alliance-1 (子)  │          │ └── march-5002 (子)  │
│ └── alliance-2 (子)  │          └──────────────────────┘
└──────────────────────┘
```

### CityChildActor —— 玩家城池

```go
type CityChildActor struct {
    cactor.Base
    uid        int64
    buildings  map[int32]*Building   // 建筑列表
    resources  *Resources            // 资源（木/石/铁/粮）
    troops     *TroopManager         // 兵力
    buildQueue []*BuildTask          // 建造队列
}

func (p *CityChildActor) OnInit() {
    p.Local().Register("build", p.onBuild)
    p.Local().Register("upgrade", p.onUpgrade)
    p.Local().Register("train", p.onTrain)
    p.Local().Register("march", p.onMarch)
    p.Local().Register("speedUp", p.onSpeedUp)

    p.Remote().Register("addResource", p.onAddResource)
    p.Remote().Register("troopReturn", p.onTroopReturn)

    // 每秒 Tick，驱动资源产出和建造倒计时
    p.Timer().Add(time.Second, p.onTick)

    // 恢复建造队列的定时器
    for _, task := range p.buildQueue {
        remaining := task.FinishTime.Sub(time.Now())
        if remaining > 0 {
            task.TimerID = p.Timer().AddOnce(remaining, func() {
                p.onBuildComplete(task)
            })
        } else {
            p.onBuildComplete(task)
        }
    }
}

func (p *CityChildActor) onBuild(session *cproto.Session, msg *pb.BuildReq) {
    cost := getBuildCost(msg.BuildingType, 1)
    if !p.resources.Enough(cost) {
        p.responseCode(session, ccode.NotEnoughResource)
        return
    }

    p.resources.Subtract(cost)

    task := &BuildTask{
        BuildingType: msg.BuildingType,
        Level:        1,
        FinishTime:   time.Now().Add(getBuildDuration(msg.BuildingType, 1)),
    }

    // 注册建造完成的一次性定时器
    task.TimerID = p.Timer().AddOnce(getBuildDuration(msg.BuildingType, 1), func() {
        p.onBuildComplete(task)
    })

    p.buildQueue = append(p.buildQueue, task)
    p.response(session, &pb.BuildRsp{Task: taskToProto(task)})
}

func (p *CityChildActor) onBuildComplete(task *BuildTask) {
    building := &Building{
        Type:  task.BuildingType,
        Level: task.Level,
    }
    p.buildings[task.BuildingType] = building
    p.removeBuildTask(task)

    // 推送建造完成通知
    p.pushToClient("onBuildComplete", &pb.BuildCompleteNotify{
        BuildingType: task.BuildingType,
        Level:        task.Level,
    })
}

func (p *CityChildActor) onSpeedUp(session *cproto.Session, msg *pb.SpeedUpReq) {
    task := p.findBuildTask(msg.TaskID)
    if task == nil {
        return
    }

    // 消耗加速道具
    // 移除旧定时器，创建新的
    p.Timer().Remove(task.TimerID)

    task.FinishTime = task.FinishTime.Add(-msg.ReduceTime)
    remaining := task.FinishTime.Sub(time.Now())

    if remaining <= 0 {
        p.onBuildComplete(task)
    } else {
        task.TimerID = p.Timer().AddOnce(remaining, func() {
            p.onBuildComplete(task)
        })
    }

    p.response(session, &pb.SpeedUpRsp{})
}

func (p *CityChildActor) onMarch(session *cproto.Session, msg *pb.MarchReq) {
    troops := p.troops.Detach(msg.TroopIDs)

    // 通知 World 节点的 MarchActor 创建行军
    worldNode, _ := p.App().Discovery().Random("world")
    targetPath := cfacade.NewPath(worldNode.GetNodeID(), "march")
    p.Call(targetPath, "createMarch", &pb.CreateMarchReq{
        UID:         p.uid,
        Troops:      troops,
        FromPos:     msg.FromPos,
        ToPos:       msg.ToPos,
        CityActorPath: p.Path().String(),
    })

    p.response(session, &pb.MarchRsp{})
}

func (p *CityChildActor) onTick() {
    // 资源产出
    for _, building := range p.buildings {
        if building.IsResourceBuilding() {
            p.resources.Add(building.GetProduction())
        }
    }
}
```

### MarchChildActor —— 行军实例

```go
type MarchChildActor struct {
    cactor.Base
    marchID       string
    uid           int64
    troops        []*Troop
    fromPos       *Position
    toPos         *Position
    arriveTime    time.Time
    cityActorPath string
    phase         MarchPhase  // Marching / Fighting / Returning
}

func (p *MarchChildActor) OnInit() {
    duration := calculateMarchDuration(p.fromPos, p.toPos)
    p.arriveTime = time.Now().Add(duration)

    // 到达定时器
    p.Timer().AddOnce(duration, p.onArrive)

    // 每 5 秒更新位置（同步给客户端地图）
    p.Timer().Add(5*time.Second, p.onUpdatePosition)
}

func (p *MarchChildActor) onArrive() {
    p.phase = Fighting
    // 执行战斗逻辑或采集逻辑
    result := p.executeBattle()

    // 通知玩家城池 Actor 部队返回
    p.Call(p.cityActorPath, "troopReturn", &pb.TroopReturnReq{
        Troops:  result.SurvivingTroops,
        Rewards: result.Rewards,
    })

    p.Exit()
}
```

**要点**：
- 大量异步操作（建造、行军）通过 `AddOnce` 定时器实现
- 加速功能：移除旧定时器 → 计算新剩余时间 → 创建新定时器
- 下线后定时器仍在运行，上线后状态自然一致
- 行军作为独立子 Actor，到达后自动销毁

---

## 七、休闲竞技 / 派对游戏

### 场景特点

- 多种小游戏模式，规则各异
- 快速开局，单局时间短
- 需要房间大厅和快速匹配

### Actor 建模

```
Gate 节点              Game 节点
┌────────────┐        ┌──────────────────────────┐
│ AgentActor │        │ LobbyActor               │
│            │─NATS──►│ RoomActor (父)            │
│            │        │ ├── room-8001 (子)        │
│            │        │ │  └─ 答题模式             │
│            │        │ ├── room-8002 (子)        │
│            │        │ │  └─ 画画猜词             │
│            │        │ └── room-8003 (子)        │
│            │        │    └─ 赛车               │
│            │        │ QuickMatchActor           │
└────────────┘        └──────────────────────────┘
```

### RoomChildActor —— 通用房间框架

```go
type RoomChildActor struct {
    cactor.Base
    roomID      string
    hostUID     int64
    players     map[int64]*RoomPlayer
    maxPlayers  int
    gameMode    GameMode
    gameHandler IGameHandler  // 可替换的游戏逻辑
    state       RoomState     // Waiting / Playing / Settlement
}

// IGameHandler —— 游戏逻辑抽象接口
type IGameHandler interface {
    OnStart(room *RoomChildActor)
    OnPlayerInput(room *RoomChildActor, uid int64, input []byte)
    OnTick(room *RoomChildActor)
    OnEnd(room *RoomChildActor) *GameResult
    TickInterval() time.Duration
}

func (p *RoomChildActor) OnInit() {
    p.Local().Register("joinRoom", p.onJoinRoom)
    p.Local().Register("leaveRoom", p.onLeaveRoom)
    p.Local().Register("startGame", p.onStartGame)
    p.Local().Register("gameInput", p.onGameInput)
}

func (p *RoomChildActor) onStartGame(session *cproto.Session, msg *pb.StartGameReq) {
    if session.Uid != p.hostUID {
        return // 只有房主能开始
    }
    if len(p.players) < 2 {
        return
    }

    p.state = RoomPlaying

    // 根据游戏模式创建对应的处理器
    switch p.gameMode {
    case ModeQuiz:
        p.gameHandler = NewQuizHandler()
    case ModeDrawGuess:
        p.gameHandler = NewDrawGuessHandler()
    case ModeRacing:
        p.gameHandler = NewRacingHandler()
    }

    p.gameHandler.OnStart(p)

    // 启动游戏 Tick
    if interval := p.gameHandler.TickInterval(); interval > 0 {
        p.Timer().Add(interval, func() {
            p.gameHandler.OnTick(p)
        })
    }
}

func (p *RoomChildActor) onGameInput(session *cproto.Session, msg *pb.GameInputReq) {
    if p.state != RoomPlaying {
        return
    }
    p.gameHandler.OnPlayerInput(p, session.Uid, msg.Data)
}
```

### QuizHandler —— 答题模式

```go
type QuizHandler struct {
    questions    []*Question
    currentIndex int
    scores       map[int64]int32
    roundTimer   uint64
}

func (h *QuizHandler) OnStart(room *RoomChildActor) {
    h.questions = loadRandomQuestions(10)
    h.scores = make(map[int64]int32)
    h.currentIndex = 0
    h.sendQuestion(room)
}

func (h *QuizHandler) sendQuestion(room *RoomChildActor) {
    q := h.questions[h.currentIndex]
    room.BroadcastAll("onQuestion", &pb.QuestionNotify{
        Index:   int32(h.currentIndex),
        Content: q.Content,
        Options: q.Options,
    })
    // 10 秒作答时间
    h.roundTimer = room.Timer().AddOnce(10*time.Second, func() {
        h.nextQuestion(room)
    })
}

func (h *QuizHandler) OnPlayerInput(room *RoomChildActor, uid int64, input []byte) {
    answer := parseAnswer(input)
    q := h.questions[h.currentIndex]
    if answer == q.CorrectAnswer {
        h.scores[uid] += 10
    }
}

func (h *QuizHandler) nextQuestion(room *RoomChildActor) {
    h.currentIndex++
    if h.currentIndex >= len(h.questions) {
        result := h.OnEnd(room)
        room.BroadcastAll("onGameEnd", result)
        room.Timer().AddOnce(5*time.Second, func() { room.Exit() })
        return
    }
    h.sendQuestion(room)
}

func (h *QuizHandler) TickInterval() time.Duration { return 0 } // 答题模式不需要 Tick
```

**要点**：
- 通过 `IGameHandler` 接口实现策略模式，一套房间框架适配多种小游戏
- 不同游戏模式的 Tick 频率不同（答题不需要，赛车需要高频）
- 房主控制游戏开始，避免非房主随意操作

---

## 八、Roguelike / 单局制游戏

### 场景特点

- 单人或小队冒险，随机生成关卡
- 关卡内有复杂状态（地图、怪物、道具）
- 关卡之间有进度保存

### Actor 建模

```
Game 节点
┌───────────────────────────────┐
│ DungeonActor (父)              │
│ ├── dungeon-uid1-run1 (子)    │ ← 玩家1的第一次冒险
│ ├── dungeon-uid2-run1 (子)    │ ← 玩家2的第一次冒险
│ └── dungeon-uid1-run2 (子)    │ ← 玩家1的第二次冒险
└───────────────────────────────┘
```

### DungeonRunActor —— 一次 Roguelike 冒险

```go
type DungeonRunActor struct {
    cactor.Base
    runID       string
    uid         int64
    seed        int64             // 随机种子
    rng         *rand.Rand
    currentFloor int32
    totalFloors  int32
    hero        *HeroState
    inventory   []*RunItem        // 本次冒险的道具
    mapData     *FloorMap
    enemies     []*Enemy
    agentPath   string
    sid         string
}

func (p *DungeonRunActor) OnInit() {
    p.Local().Register("move", p.onMove)
    p.Local().Register("attack", p.onAttack)
    p.Local().Register("useItem", p.onUseItem)
    p.Local().Register("chooseReward", p.onChooseReward)
    p.Local().Register("nextFloor", p.onNextFloor)
    p.Local().Register("abandon", p.onAbandon)

    p.Remote().Register("getState", p.onGetState)

    p.rng = rand.New(rand.NewSource(p.seed))
    p.generateFloor()
}

func (p *DungeonRunActor) generateFloor() {
    p.currentFloor++
    p.mapData = GenerateFloorMap(p.rng, p.currentFloor)
    p.enemies = SpawnEnemies(p.rng, p.currentFloor, p.mapData)

    pushToPlayer(p, "onFloorGenerated", &pb.FloorGeneratedNotify{
        Floor:   p.currentFloor,
        MapData: p.mapData.ToProto(),
        Enemies: enemiesToProto(p.enemies),
    })
}

func (p *DungeonRunActor) onAttack(session *cproto.Session, msg *pb.AttackReq) {
    target := p.findEnemy(msg.TargetID)
    if target == nil {
        return
    }

    dmg := calculateDamage(p.hero, target, msg.SkillID)
    target.HP -= dmg

    result := &pb.AttackResultNotify{
        TargetID: msg.TargetID,
        Damage:   dmg,
        TargetHP: target.HP,
    }

    if target.HP <= 0 {
        p.removeEnemy(target)
        result.Killed = true

        // 掉落随机奖励
        drops := p.generateDrops(target)
        result.Drops = dropsToProto(drops)
    }

    pomelo.Response(p, session.AgentPath, session.Sid, session.GetMID(), result)

    // 检查本层是否通关
    if len(p.enemies) == 0 {
        p.onFloorCleared()
    }
}

func (p *DungeonRunActor) onFloorCleared() {
    // 生成三选一奖励
    rewards := p.generateRewardChoices(3)
    pushToPlayer(p, "onFloorCleared", &pb.FloorClearedNotify{
        Floor:   p.currentFloor,
        Choices: rewardsToProto(rewards),
    })
}

func (p *DungeonRunActor) onChooseReward(session *cproto.Session, msg *pb.ChooseRewardReq) {
    reward := p.applyReward(msg.ChoiceIndex)
    pomelo.Response(p, session.AgentPath, session.Sid, session.GetMID(), &pb.ChooseRewardRsp{
        Reward: rewardToProto(reward),
        Hero:   p.hero.ToProto(),
    })
}

func (p *DungeonRunActor) onNextFloor(session *cproto.Session, msg *pb.NextFloorReq) {
    if p.currentFloor >= p.totalFloors {
        // 通关
        p.onDungeonComplete()
        return
    }
    p.generateFloor()
}

func (p *DungeonRunActor) onDungeonComplete() {
    // 通知玩家 Actor 发放通关奖励
    playerPath := p.NewChildPath("player", fmt.Sprintf("%d", p.uid))
    p.Call(playerPath, "dungeonReward", &pb.DungeonRewardReq{
        Floor:     p.currentFloor,
        Inventory: itemsToProto(p.inventory),
    })

    pushToPlayer(p, "onDungeonComplete", &pb.DungeonCompleteNotify{
        TotalFloors: p.currentFloor,
    })

    p.Timer().AddOnce(3*time.Second, func() { p.Exit() })
}

func (p *DungeonRunActor) onAbandon(session *cproto.Session, msg *pb.AbandonReq) {
    pomelo.Response(p, session.AgentPath, session.Sid, session.GetMID(), &pb.AbandonRsp{})
    p.Exit() // 放弃冒险，销毁 Actor
}
```

**要点**：
- 每次冒险是独立子 Actor，互不影响
- 随机种子 (`seed`) 保证相同种子生成相同地图，便于重放和验证
- Actor 销毁时冒险数据自然清理，无内存泄漏
- 通关奖励通过 `Call` 发给 PlayerActor，解耦战斗和经济系统

---

## 九、通用模块：跨品类可复用的 Actor 设计

### 9.1 排行榜 Actor

```go
type RankActor struct {
    cactor.Base
    ranks map[string]*RankList // rankType → 排行榜
}

func (p *RankActor) AliasID() string { return "rank" }

func (p *RankActor) OnInit() {
    p.ranks = make(map[string]*RankList)
    p.Remote().Register("updateScore", p.onUpdateScore)
    p.Remote().Register("getRank", p.onGetRank)
    p.Remote().Register("getMyRank", p.onGetMyRank)

    // 每天 0 点重置日榜
    p.Timer().AddFixedHour(0, 0, 0, p.onDailyReset)
}

func (p *RankActor) onUpdateScore(req *pb.UpdateScoreReq) {
    rank := p.ranks[req.RankType]
    if rank == nil {
        rank = NewRankList(req.RankType, 100) // Top 100
        p.ranks[req.RankType] = rank
    }
    rank.Update(req.UID, req.Score, req.Name)
}

func (p *RankActor) onGetRank(req *pb.GetRankReq) (*pb.GetRankRsp, int32) {
    rank := p.ranks[req.RankType]
    if rank == nil {
        return &pb.GetRankRsp{}, ccode.OK
    }
    return &pb.GetRankRsp{
        Entries: rank.TopN(int(req.Count)),
    }, ccode.OK
}
```

### 9.2 邮件 Actor

```go
type MailActor struct {
    cactor.Base
}

func (p *MailActor) AliasID() string { return "mail" }

func (p *MailActor) OnInit() {
    p.Remote().Register("sendMail", p.onSendMail)
    p.Remote().Register("getMailList", p.onGetMailList)
    p.Remote().Register("readMail", p.onReadMail)
    p.Remote().Register("claimAttachment", p.onClaimAttachment)
    p.Remote().Register("sendGlobalMail", p.onSendGlobalMail)

    // 每小时清理过期邮件
    p.Timer().Add(time.Hour, p.cleanExpiredMails)
}

func (p *MailActor) onSendMail(req *pb.SendMailReq) {
    mail := &Mail{
        ID:         generateMailID(),
        FromUID:    req.FromUID,
        ToUID:      req.ToUID,
        Title:      req.Title,
        Content:    req.Content,
        Attachment: req.Attachment,
        CreateTime: time.Now(),
        ExpireTime: time.Now().Add(7 * 24 * time.Hour),
    }
    saveMailToDB(mail)

    // 如果目标玩家在线，实时推送
    playerPath := p.NewChildPath("player", fmt.Sprintf("%d", req.ToUID))
    p.Call(playerPath, "newMailNotify", &pb.NewMailNotify{MailID: mail.ID})
}
```

### 9.3 公会 Actor

```go
type GuildChildActor struct {
    cactor.Base
    guildID   int64
    name      string
    leaderUID int64
    members   map[int64]*GuildMember
    level     int32
}

func (p *GuildChildActor) OnInit() {
    p.Remote().Register("applyJoin", p.onApplyJoin)
    p.Remote().Register("approve", p.onApprove)
    p.Remote().Register("kick", p.onKick)
    p.Remote().Register("donate", p.onDonate)
    p.Remote().Register("chat", p.onGuildChat)

    p.Event().Register("memberLevelUp", p.onMemberLevelUp)
}

func (p *GuildChildActor) onGuildChat(req *pb.GuildChatReq) {
    // 向所有在线成员推送
    for _, member := range p.members {
        if member.Online {
            playerPath := p.NewChildPath("player", fmt.Sprintf("%d", member.UID))
            p.Call(playerPath, "guildChatNotify", &pb.GuildChatNotify{
                FromUID:  req.FromUID,
                FromName: req.FromName,
                Content:  req.Content,
            })
        }
    }
}
```

### 9.4 全局事件发布 —— 每日重置

```go
// 在 Center 节点的某个 Actor 中
func (p *CenterActor) OnInit() {
    // 每天 0:00 发布每日重置事件
    p.Timer().AddFixedHour(0, 0, 0, func() {
        p.PostEvent(&DailyResetEvent{})
    })
}

type DailyResetEvent struct{}

func (e *DailyResetEvent) Name() string    { return "dailyReset" }
func (e *DailyResetEvent) UniqueID() int64 { return 0 } // 0 表示通知所有订阅者
```

---

## 十、最佳实践与注意事项

### 10.1 Actor 粒度选择

| 场景 | 推荐粒度 | 原因 |
|------|---------|------|
| 单个玩家状态 | 子 Actor（per player） | 每个玩家独立生命周期 |
| 游戏房间/牌桌 | 子 Actor（per room） | 房间内状态隔离 |
| 全服排行榜 | 独立父 Actor | 全局唯一，无需子 Actor |
| 匹配队列 | 独立父 Actor | 全局协调 |
| 世界地图格子 | 子 Actor（per region） | 区域隔离，减少锁竞争 |

### 10.2 避免 CallWait 死锁

```go
// 禁止 Actor 对自身 CallWait
// 禁止两个 Actor 互相 CallWait（A → B 和 B → A 同时发生）

// 正确做法：使用 Call（异步）替代 CallWait
p.Call(targetPath, "someFunc", req)  // 不阻塞
```

### 10.3 定时器使用规范

```go
// 循环定时器：记住 ID，退出时会自动清理
id := p.Timer().Add(time.Second, p.onTick)

// 一次性定时器：用于超时控制
timeoutID := p.Timer().AddOnce(30*time.Second, p.onTimeout)
// 在超时前完成时，主动移除
p.Timer().Remove(timeoutID)

// 每日固定时间：每天 12:00:00
p.Timer().AddFixedHour(12, 0, 0, p.onNoon)
```

### 10.4 事件使用规范

```go
// 广播给所有订阅者（UniqueID = 0）
p.PostEvent(&GlobalEvent{name: "serverShutdown"})

// 定向通知（UniqueID 匹配时才投递）
p.PostEvent(&PlayerEvent{name: "levelUp", uid: 10086})

// 订阅时指定 UniqueID 过滤
p.Event().Register("levelUp", p.onLevelUp, playerUID)
```

### 10.5 跨节点通信模式

```go
// 模式一：Fire-and-forget（最常用）
targetPath := cfacade.NewPath("game-1", "room")
p.Call(targetPath, "doSomething", req)

// 模式二：同步 RPC（谨慎使用，会阻塞当前 Actor）
var reply pb.SomeRsp
code := p.CallWait(targetPath, "getData", req, &reply)

// 模式三：按节点类型广播（通知所有同类节点）
p.CallType("game", "room", "onConfigChange", req)
```

### 10.6 子 Actor 生命周期管理

```go
// 创建（幂等：重复创建返回已有实例）
child, err := p.Child().Create("room-1001", handler)

// 按需创建（OnFindChild）
func (p *ParentActor) OnFindChild(m *cfacade.Message) (cfacade.IActor, bool) {
    childID := m.TargetPath().ChildID
    handler := createHandler(childID)
    child, err := p.Child().Create(childID, handler)
    if err != nil {
        return nil, false
    }
    return child, true
}

// 销毁：子 Actor 内部调用
p.Exit()

// 父 Actor 停止时自动级联关闭所有子 Actor
```

### 10.7 性能优化建议

| 场景 | 建议 |
|------|------|
| 高频广播（场景同步） | 使用 AOI 限制广播范围，避免全场景广播 |
| 大量子 Actor | 设置空闲超时，自动 `Exit()` 释放资源 |
| 频繁创建/销毁 | Actor 复用或预创建池 |
| 大量定时器 | 优先使用 `AddOnce`，用完即移除 |
| 数据持久化 | 定时批量写库，避免每次操作都写 |
| 消息体积 | 集群内使用 Protobuf，压缩大包体 |

### 10.8 调试与监控

```go
// 消息到达超时监控（默认 100ms 告警）
app.ActorSystem().SetArrivalTimeout(200)

// 消息执行超时监控（默认 100ms 告警）
app.ActorSystem().SetExecutionTimeout(200)

// RPC 调用超时（默认 3s）
app.ActorSystem().SetCallTimeout(5 * time.Second)
```

框架内置的超时告警日志格式：
```
[WARN] [remote] Invoke timeout.[source=game-1.player.10086, target=game-1.room.1001->join, arrival=150ms]
[WARN] [local] Invoke timeout.[source=gate-1.agent.sid123, target=gate-1.agent->hello, execution=250ms]
```

---

> 本文档覆盖了 8 种主流游戏品类的 Actor 建模方案与代码示范，以及 4 种通用可复用模块的设计。所有示例代码基于 ActorGo 框架的真实 API 编写，可作为项目开发的起点参考。
