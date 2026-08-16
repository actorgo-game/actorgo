# Body Codec 收敛设计

## 问题

`Application` 同时持有：

- `serializer`：Discovery 编解码 + 历史上的“默认序列化”
- `codecs`：AGP/HTTP/集群业务 body 注册表
- `defaultBodyCodec`：`ctx.Codec == 0` / 服务端主动推送默认 id

`SetSerializer` 又会同时改前两者，职责重叠。

## 目标

Application 只保留一套编解码入口：`BodyCodecs()`。

## 设计

### Registry

`IBodyCodecRegistry` 增加默认 codec：

```go
type IBodyCodecRegistry interface {
    Register(codec IBodyCodec) error
    Lookup(id int32) (IBodyCodec, bool)
    Marshal(id int32, value any) ([]byte, error)
    Unmarshal(id int32, data []byte, value any) error
    Default() int32
    SetDefault(id int32) error // id 必须已注册
}
```

内置仍注册 Protobuf + JSON；默认 id = `CodecProtobuf`。

### Application

字段只保留 `codecs *cserializer.Registry`。

对外 API：

- 保留 `BodyCodecs() IBodyCodecRegistry`
- 新增 `SetDefaultBodyCodec(id int32)`（启动前调用；薄封装 `codecs.SetDefault`）
- 删除 `Serializer()` / `DefaultBodyCodec()` / `SetSerializer(...)`

### 调用点迁移

| 原用法 | 新用法 |
|--------|--------|
| `app.DefaultBodyCodec()` | `app.BodyCodecs().Default()` |
| `app.Serializer().Marshal/Unmarshal` | `app.BodyCodecs().Marshal/Unmarshal(app.BodyCodecs().Default(), ...)` |
| `app.SetSerializer(NewJSON())` | `app.SetDefaultBodyCodec(cfacade.CodecJSON)` |

Discovery 不再依赖独立 Serializer；仍用默认 codec（默认 Protobuf，行为不变）。

### 不改动

- 协议 wire 上的 `codec` 字段语义
- 请求/响应按 `ctx.Codec` 编解码路径
- JSON/PB 同时注册、按包选择的能力

## 验收

- 框架相关测试通过
- `demo_chat` 的 `SetSerializer(NewJSON())` 改为 `SetDefaultBodyCodec(CodecJSON)` 并可编译
