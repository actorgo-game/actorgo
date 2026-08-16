# HTTP 调用 Actor 方法

> 包：`net/httpactor`  
> 与 AGP/NATS 共用同一套 `Methods().Register` 方法表；Body 在目标 Actor mailbox 解码。

## 1. 入口

固定路由（仅 POST）：

```text
POST /actor/{methodID}
```

- `methodID`：十进制整数，且必须 **> 5**（1–5 为系统方法保留）
- 所有顶层 Actor 已 `Methods().Register` 的业务方法都会进入该入口（按 Kind 区分 Request/Notify）
- 不支持自定义 HTTP path、不支持 GET
- 不接受 ActorPath；子 Actor 由顶层 Actor 根据 Session 或业务参数选择

Gin 路由常量：`httpactor.Route` = `/actor/:methodID`

## 2. 必填 / 常用 Header

| Header | 必填 | 说明 |
|--------|:----:|------|
| `Content-Type` | 是 | `application/json` 或 `application/x-protobuf`；同时决定请求与响应 Body 编码 |
| `X-ActorGo-Timeout-Ms` | 否 | 客户端期望超时（毫秒）；可缩短/拉长默认值，但不超过服务端 `MaxTimeout` |

响应 Header：

| Header | 说明 |
|--------|------|
| `Content-Type` | 与请求相同（错误时若非 PB 则回落 JSON） |
| `X-ActorGo-Request-ID` | 服务端分配的请求 ID（字符串） |

默认会拷贝到 `RequestContext.Metadata` 的 Header（可配置）：

- `traceparent`
- `tracestate`
- `x-request-id`

## 3. 状态码

| HTTP | 场景 |
|-----:|------|
| 200 | Request 成功，Body 为业务 Response |
| 202 | Notify 成功入队（无业务 Body） |
| 400 | 非法参数等 |
| 401 | Authenticator 失败 |
| 404 | 路由或 MethodID 未注册 |
| 405 | 非 POST |
| 415 | Content-Type 不支持，或方法不支持该 Codec |
| 429 | Body 过大等资源耗尽 |
| 499 | 取消 |
| 504 | 超时 |
| 其它 4xx/5xx | 映射自协议 `StatusCode` |

错误 Body 为 `HTTPError`：

```json
{"code":2,"message":"request body decode failed","request_id":"12"}
```

（PB 请求时错误也可按 Protobuf 编码返回。）

## 4. RequestContext（HTTP 入口）

| 字段 | 值 |
|------|-----|
| `Transport` | `TransportHTTP` |
| `Codec` | 由 `Content-Type` 决定 |
| `RequestID` | 服务端递增分配 |
| `Session` | 来自 `Authenticator`；未配置则为 `nil` |
| `Metadata` | 白名单 Header 拷贝 |

与 AGP 一样：业务 handler 签名仍是 `func(*RequestContext, *Req) ...`。

## 5. 接入方式

### 5.1 独立 HTTP 组件

```go
app.Register(httpactor.NewComponent("actor-api", "127.0.0.1:9080",
    httpactor.WithTimeout(3*time.Second, 30*time.Second),
    httpactor.WithMaxBodySize(4<<20),
    httpactor.WithAuthenticator(func(r *http.Request) (*cproto.Session, error) {
        // 返回 Session 或 (nil, nil) 表示匿名
        return &cproto.Session{Sid: "http", Uid: 1}, nil
    }),
    httpactor.WithMetadataHeaders("traceparent", "x-request-id"),
))
```

### 5.2 挂到现有 Gin

```go
actorHandler := httpactor.NewHandler(app /* , opts... */)
if err := httpServer.RegisterActorRoutes(actorHandler); err != nil {
    panic(err)
}
```

现有 Gin Controller 不受影响；只是多挂一条 `POST /actor/:methodID`。

## 6. curl 示例

### JSON Request

```bash
curl -X POST "http://127.0.0.1:9080/actor/1001" \
  -H "Content-Type: application/json" \
  -H "X-ActorGo-Timeout-Ms: 3000" \
  -d '{"account":"demo","password":"xxx"}'
```

成功：HTTP 200 + JSON Response body。

### Protobuf Request

```bash
curl -X POST "http://127.0.0.1:9080/actor/1001" \
  -H "Content-Type: application/x-protobuf" \
  --data-binary @login_request.pb
```

### Notify

若该方法注册为 Notify（`func(...) error`），成功时返回 **202**，无业务 Body。

```bash
curl -i -X POST "http://127.0.0.1:9080/actor/2103" \
  -H "Content-Type: application/json" \
  -d '{"value":"event"}'
```

## 7. Options 默认值

| 选项 | 默认 |
|------|------|
| `MaxBodySize` | 4 MiB |
| `DefaultTimeout` | 3s |
| `MaxTimeout` | 30s |
| `Authenticator` | nil（匿名） |
| `MetadataHeaders` | traceparent, tracestate, x-request-id |

## 8. 与 AGP 的关系

| | AGP | HTTP |
|--|-----|------|
| Method 表 | 同一张 | 同一张 |
| Target | 不允许客户端提供 | 不允许客户端提供 |
| Codec | Packet.codec | Content-Type |
| 解码位置 | Actor mailbox | Actor mailbox |
| Session | 连接 Session | Authenticator 注入 |

HTTP **不能**替代客户端实时长连接；适合运维、机器人、Web 后台调 Actor。
