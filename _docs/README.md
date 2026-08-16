# ActorGo 文档

- [近期 API / 架构变更汇总（2026-07）](api-changes-2026-07.md) ← **先看这份**
- [HTTP 调用 Actor 方法](http-actor.md)
- [AGP/1 JSON/PB 协议设计](agp-protobuf-protocol-design.md)
- [分步开发计划与完成状态](agp-protobuf-development-plan.md)
- [框架设计说明](design.md)（部分章节仍含历史 Call/邮箱描述，以变更汇总为准）
- [游戏场景示例](game-scenarios.md)（部分仍写 Call，迁移时请对照 Invoke/Notify）

当前有效协议是 AGP/1。历史 Pomelo、Simple、FuncName、`Call`/`CallWait`、`PostRemote`、`SetSerializer`、`SessionSnapshot` 不再适用于新入口。