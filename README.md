# ActorGo

ActorGo 是 Go Actor 游戏服务端框架。当前协议为 AGP/1：Packet / ClusterMessage 固定 Protobuf，业务 Body 支持 JSON/PB；客户端交互只有 Request、Response、Notify。

更完整的近期变更说明见：[_docs/api-changes-2026-07.md](_docs/api-changes-2026-07.md)。

## 本次改造要点

- 删除 Pomelo/Simple 运行时与 `INetParser`
- 稳定 Method ID + Typed Handler，取消 FuncName 反射调用
- TCP：`uint32 big-endian length + Packet PB`
- WebSocket：一条 Binary Message 对应一个 Packet，子协议 `agp.v1`
- AGP / HTTP：客户端只提交 Method ID 和 Body，不接受 Actor Target
- NATS：Protobuf `ClusterMessage`（`session` 使用 `Session`，无 SessionSnapshot）
- AGP / HTTP / NATS 顶层调用共用 Actor 方法表；内部子 Actor 使用 ActorPath
- 单一 mailbox；底层 `Post` 不再暴露为业务 API
- Application 编解码收敛为 `BodyCodecs()` + `SetDefaultBodyCodec`
- 旧 `Call` / `CallWait` / `CallType` 已由 `Invoke` / `Notify` 取代

不包含 Pomelo 兼容、gRPC 和 MessagePack。

## Method 定义

业务 proto 使用标准 Protobuf：

```proto
service PlayerService {
  rpc Login(LoginRequest) returns (LoginResponse);
}
```

```bash
protoc -I . \
  --go_out=. --go_opt=paths=source_relative \
  api/player.proto
```

Actor 初始化时直接注册（无 error 返回值）：

```go
const LoginMethodID uint32 = 1001

func (a *PlayerActor) OnInit() {
    a.Methods().Register(LoginMethodID, a.Login)
}

func (a *PlayerActor) Login(
    ctx *facade.RequestContext,
    request *playerv1.LoginRequest,
) (*playerv1.LoginResponse, error) {
    return &playerv1.LoginResponse{}, nil
}
```

- Request：`func(*RequestContext, *Request) (*Response, error)`
- Notify：`func(*RequestContext, *Request) error`
- 子 Actor 的 `Register` 只安装本地 mailbox，不写入外部方法表

挂载 `httpactor` 后，所有顶层 Actor 的 `Methods().Register` 方法统一经 `POST /actor/{methodID}` 暴露。

## 启动

```go
app := actorgo.Configure(profileFilePath, nodeID, actorgo.Standalone)

// 可选：切换默认 Body Codec（JSON 与 PB 始终同时注册）
app.SetDefaultBodyCodec(cfacade.CodecJSON)

app.AddActors(playerActor)

app.Register(parser.New("client", []facade.IConnector{
    connector.NewTCP(":9000"),
    connector.NewWS(":9001"),
}))
app.Register(httpactor.NewComponent("actor-api", "127.0.0.1:9080"))
app.Startup()
```

## 跨节点调用

```go
ctx := cfacade.NewRequestContext(context.Background())
ctx.Codec = cfacade.CodecProtobuf
// 需要玩家上下文时再设置 ctx.Session
result := app.ActorSystem().InvokeNode(ctx, "center-1", methodID, req)
```

本节点顶层方法使用 `Invoke(ctx, methodID, req)`；跨节点顶层方法使用
`InvokeNode(ctx, nodeID, methodID, req)`。完整 ActorPath 仅用于服务端内部的
`InvokeTarget` / `NotifyTarget`，客户端 AGP/HTTP 都不能指定 target。

## curl JSON

```bash
curl -X POST "http://127.0.0.1:9080/actor/1001" \
  -H "Content-Type: application/json" \
  -H "X-ActorGo-Timeout-Ms: 3000" \
  -d '{"account":"demo"}'
```

PB 调用将 `Content-Type` 改为 `application/x-protobuf`，并使用 `--data-binary @request.pb`。

Notify 方法成功返回 HTTP 202；错误 Body 为 `HTTPError`。完整 Header/状态码见 [HTTP 文档](_docs/http-actor.md)。

## 与现有 Gin 共存

```go
actorHandler := httpactor.NewHandler(app)
if err := httpServer.RegisterActorRoutes(actorHandler); err != nil {
    panic(err)
}
```

## 文档与验证

- [近期变更汇总](_docs/api-changes-2026-07.md)
- [HTTP 调用 Actor](_docs/http-actor.md)
- [AGP/1 协议设计](_docs/agp-protobuf-protocol-design.md)
- [开发计划与完成状态](_docs/agp-protobuf-development-plan.md)

```bash
go test ./net/proto ./net/serializer ./net/method \
  ./net/parser ./net/httpactor ./net/actor ./net/connector
```
