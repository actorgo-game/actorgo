# actorgo v2 · P4 云原生集成包 — 设计文档

- **版本**: v2.0 子项目 P4（4 子项目中的第 2 个，实施顺序 P1→**P4**→P2→P3）
- **作者**: actorgo team
- **日期**: 2026-04-23
- **状态**: Draft（待用户审阅）
- **依赖**: P1 稳健性强化（MailboxMetrics 接口、PanicTotal 接口、IDiscovery.Health、三阶段 shutdown）

---

## 0. 范围与原则

- **目标**：让 actorgo 应用"开箱即云原生"——单一二进制 + ConfigMap + 一行 Helm 即可部署到 k8s，且自带 Prometheus 指标、OpenTelemetry 追踪、JSON 结构化日志、健康检查、ENV 配置覆盖。
- **不在范围**：JetStream/可靠传输（推迟到 P5）、k8s Operator（后续可独立 spec）、Service Mesh 自动接入。
- **代码组织**：全部位于 actorgo 主仓库 `components/` 下，沿用现有 `IComponent` 风格；无独立 module。
- **依赖增量**：`prometheus/client_golang`、`go.opentelemetry.io/otel` 系列（zap 已存在）。这些依赖仅在启用对应组件时才走代码路径。

---

## 1. Metrics 组件 `components/metrics/`  `[新增]`

### 1.1 设计

```go
// 默认实现基于 prometheus/client_golang
type MetricsComponent struct {
    cfacade.Component
    registry *prometheus.Registry
    server   *http.Server      // /metrics endpoint，可独立端口
    port     int
}

func New(opts ...Option) *MetricsComponent
```

### 1.2 内置指标

| 指标 | 类型 | Labels | 说明 |
|---|---|---|---|
| `actorgo_actor_count` | Gauge | `node_id, node_type` | 当前 actor 数（含 child） |
| `actorgo_mailbox_depth` | Gauge | `actor_path, mailbox` | 邮箱深度（来自 P1 的 `MailboxMetrics`） |
| `actorgo_mailbox_enqueue_total` | Counter | `actor_path, mailbox` | 入队总数 |
| `actorgo_mailbox_drop_total` | Counter | `actor_path, mailbox, reason` | 丢消息计数（背压触发） |
| `actorgo_message_duration_seconds` | Histogram | `actor_type, func, mailbox` | 消息处理耗时（buckets 1ms~1s） |
| `actorgo_message_arrival_seconds` | Histogram | `actor_type, mailbox` | 入队到处理的等待时长 |
| `actorgo_panic_total` | Counter | `ctx` | panic 计数（来自 P1） |
| `actorgo_call_total` | Counter | `source_type, target_type, kind, result` | Call/CallWait/CallType 总数与结果 |
| `actorgo_call_duration_seconds` | Histogram | `kind` | RPC 时延 |
| `actorgo_cluster_publish_total` | Counter | `node_type, kind, result` | NATS publish 计数 |
| `actorgo_nats_pool_size` | Gauge | - | NATS 连接池大小 |
| `actorgo_nats_reconnects_total` | Counter | - | NATS 重连次数 |
| `actorgo_discovery_members` | Gauge | `node_type` | 当前发现的成员数 |
| `actorgo_discovery_events_total` | Counter | `event_type` | 成员变更事件数 |
| `actorgo_session_count` | Gauge | `node_type` | 当前会话数（仅前端节点） |
| `actorgo_goroutines` | Gauge | - | runtime goroutine 数 |
| `actorgo_build_info` | Gauge | `version, commit, go_version` | 构建信息 |

### 1.3 配置

```json
{
  "metrics": {
    "enabled": true,
    "port": 9100,
    "path": "/metrics",
    "include_go_runtime": true,
    "include_process": true
  }
}
```

### 1.4 与 P1 的对接

P1 定义了 `MailboxMetrics`、`metrics.PanicTotal` 接口。本组件提供 Prometheus 实现并通过 `App().SetMailboxMetrics(...)` 注入。

---

## 2. Tracing 组件 `components/tracing/`  `[新增]`

### 2.1 设计

```go
type TracingComponent struct {
    cfacade.Component
    tracer   trace.Tracer
    provider *sdktrace.TracerProvider
}

func New(opts ...Option) *TracingComponent
```

### 2.2 自动埋点点

- 每条 `Call` / `CallWait` 创建 root span（前端节点）或 child span（中间节点）
- TraceID/SpanID 通过 `Message.Header`（NATS Header 已存在）传播；序列化为 W3C `traceparent` 标准头
- Pomelo/Simple 协议握手时支持从 HTTP header 注入 trace context（前端节点入口）
- Mailbox 出入队不打 span（避免 span 爆炸），但通过 attributes 记录 mailbox 等待时长
- 消息处理函数自动开 span，span 名 = `<actor_type>.<func_name>`
- 异常（panic / 返回 error）自动标记 span status

### 2.3 配置

```json
{
  "tracing": {
    "enabled": true,
    "exporter": "otlp",
    "otlp_endpoint": "${OTEL_EXPORTER_OTLP_ENDPOINT:http://otel-collector:4317}",
    "sample_ratio": 0.1,
    "service_name": "${OTEL_SERVICE_NAME:actorgo-${ACTORGO_NODE_ID}}"
  }
}
```

### 2.4 提供给业务的 API

```go
import "github.com/actorgo-game/actorgo/components/tracing"

func (a *RoomActor) Join(session *Session, req *JoinReq) {
    ctx, span := tracing.StartSpan(a, "RoomActor.Join.business")
    defer span.End()
    // ...
}
```

`tracing.StartSpan` 自动从当前消息上下文 extract 父 span。

---

## 3. 结构化日志升级 `logger/`  `[兼容]`

### 3.1 改造点

- zap encoder 提供 `console`（开发）和 `json`（生产）两种，由 profile `logger.encoder` 切换
- 默认字段：`ts`、`level`、`caller`、`node_id`、`node_type`、`trace_id`（自动注入）、`span_id`
- 日志级别支持 SIGHUP 热改（通过 `zap.AtomicLevel`）
- `clog.Info(format, args...)` 兼容 sugared API 不变；新增 `clog.Infow(msg, fields...)` 强类型 API（在 P3 中作为推荐）

### 3.2 配置

```json
{
  "logger": {
    "encoder": "${LOGGER_ENCODER:json}",
    "level": "${LOGGER_LEVEL:info}",
    "output": ["stdout"],
    "include_caller": true,
    "include_stack_trace_at": "error"
  }
}
```

容器内日志输出到 `stdout` 由 docker/k8s 收集到 fluentd/loki，无需写文件。

---

## 4. 健康检查组件 `components/healthcheck/`  `[新增]`

### 4.1 接口

```go
type HealthChecker interface {
    Name() string
    Check(ctx context.Context) HealthResult
}

type HealthResult struct {
    Healthy bool
    Detail  string
    Latency time.Duration
}
```

### 4.2 内置 checker

| Name | 触发条件 |
|---|---|
| `actor_system` | actorSystem.Running() == true |
| `discovery` | P1 `IDiscovery.Health().Healthy` |
| `nats` | nats.Conn.IsConnected() && 最近一次心跳 < 5s |
| `mailbox_overflow` | 任一 actor 邮箱 drop rate > 阈值（可配） |

### 4.3 业务注册

```go
hc := healthcheck.New()
hc.Register(&MyDBChecker{})
hc.Register(&MyRedisChecker{})
app.Register(hc)
```

### 4.4 HTTP 端点

| Endpoint | 语义 |
|---|---|
| `GET /healthz` | liveness：仅检查 actor_system；返回 200/503 |
| `GET /readyz` | readiness：所有 checker 全过；shutdown Phase 1 后立即 503 |
| `GET /status` | 调试用 JSON，列出所有 checker 详情、uptime、版本 |

### 4.5 配置

```json
{
  "healthcheck": {
    "enabled": true,
    "port": 9101,
    "check_timeout_ms": 1000
  }
}
```

> 端口与 metrics 默认不同，避免 NetworkPolicy 同时暴露。允许配置同端口。

---

## 5. 配置 ENV 占位符  `[兼容]`

### 5.1 占位符语法

```json
{
  "node": {
    "gate": [
      {
        "node_id": "${ACTORGO_NODE_ID:gate-1}",
        "address": "${BIND_ADDR::9001}",
        "rpc_address": "${POD_IP::9101}"
      }
    ]
  },
  "cluster": {
    "nats": {
      "address": "${NATS_URL:nats://nats:4222}",
      "user": "${NATS_USER:}",
      "password": "${NATS_PASSWORD:}"
    }
  }
}
```

- 解析阶段：profile load 后，对所有 string value 用正则 `\$\{([A-Z_][A-Z0-9_]*)(:([^}]*))?\}` 替换
- 缺省值通过 `:default` 提供；未设 ENV 且无缺省值 → 启动失败
- 支持嵌套引用一层：`${VAR1:${VAR2:default}}`

### 5.2 实施位置

修改 `profile/profile.go` 的加载流程，新增 `applyEnvSubstitution(raw []byte) []byte` 步骤，置于 `include` 合并之后、json parse 之前。

### 5.3 安全

- 敏感字段（password）日志中脱敏为 `***`
- 框架启动时打印"已应用 N 个 ENV 替换"，不打具体值

---

## 6. graceful deregister  `[依赖 P1]`

P1 已规划三阶段 shutdown 中"Phase 1 Discovery deregister"。本组件保证：

- ETCD 模式：`Stop()` 调用 `Revoke(leaseID)`
- Master 模式：`Stop()` 主动发送 `unregister` 消息给 master
- `/readyz` 在 deregister 后立即 503，给 service mesh / k8s Service 一个反应窗口

---

## 7. pprof / debug 端点  `[新增]`

```json
{
  "debug": {
    "enabled": false,
    "pprof_path": "/debug/pprof"
  }
}
```

- 默认关闭
- 启用时挂载 `net/http/pprof`、`expvar`、`runtime/debug.GCSummary`
- 强烈建议仅监听 internal NetworkPolicy
- 复用 healthcheck 的 HTTP server

---

## 8. 部署交付物  `[新增]`

### 8.1 目录结构

```
deploy/
├── docker/
│   ├── Dockerfile                # 多阶段构建模板，alpine + distroless 两版本
│   ├── docker-bake.hcl           # buildx 多架构（amd64/arm64）
│   └── .dockerignore
├── helm/
│   └── actorgo/
│       ├── Chart.yaml
│       ├── values.yaml           # 完整可配参数
│       ├── values.schema.json    # 类型校验
│       ├── templates/
│       │   ├── deployment.yaml   # 按 nodeType 拆分多个 deployment
│       │   ├── service.yaml
│       │   ├── configmap.yaml    # 渲染 profile.json
│       │   ├── secret.yaml       # 敏感配置
│       │   ├── hpa.yaml          # HorizontalPodAutoscaler
│       │   ├── pdb.yaml          # PodDisruptionBudget
│       │   ├── networkpolicy.yaml
│       │   ├── servicemonitor.yaml  # Prometheus Operator CRD（可选）
│       │   └── _helpers.tpl
│       └── README.md
└── kustomize/
    └── base/
        ├── kustomization.yaml
        ├── deployment.yaml
        ├── service.yaml
        └── configmap.yaml
```

### 8.2 Dockerfile 关键要点

```dockerfile
FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w \
    -X main.version=$(git describe --tags --always) \
    -X main.commit=$(git rev-parse HEAD)" \
    -o /out/app ./cmd/app

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /out/app /app
COPY profile.json /etc/actorgo/profile.json
USER nonroot
ENTRYPOINT ["/app"]
CMD ["-profile", "/etc/actorgo/profile.json"]
```

### 8.3 Helm values.yaml 节选

```yaml
nodes:
  gate:
    replicas: 3
    image:
      repository: example/actorgo-gate
      tag: v1.0.0
    resources:
      requests: { cpu: "500m", memory: "512Mi" }
      limits:   { cpu: "2",    memory: "2Gi" }
    autoscaling:
      enabled: true
      minReplicas: 3
      maxReplicas: 20
      targetCPU: 60
    env:
      NATS_URL: "nats://nats:4222"
    service:
      type: LoadBalancer
      port: 9001
    livenessProbe:
      httpGet: { path: /healthz, port: 9101 }
      initialDelaySeconds: 5
      periodSeconds: 10
    readinessProbe:
      httpGet: { path: /readyz, port: 9101 }
      periodSeconds: 5
    lifecycle:
      preStop:
        exec: { command: ["sh", "-c", "sleep 5"] }
    terminationGracePeriodSeconds: 60
  game:
    replicas: 5
    # ...

global:
  metrics:
    serviceMonitor: { enabled: true, interval: 15s }
  tracing:
    otlpEndpoint: "http://otel-collector:4317"
```

### 8.4 GitHub Actions 工作流（参考）

`.github/workflows/release.yml`：tag 触发 → multi-arch docker build & push → helm package → push 到 OCI registry。

---

## 9. 与其它子项目的衔接

| 衔接点 | 来源/目标 | 说明 |
|---|---|---|
| `MailboxMetrics` | P1 → P4 | P4 实现 Prometheus exporter |
| `metrics.PanicTotal` | P1 → P4 | 同上 |
| `IDiscovery.Health()` | P1 → P4 | healthcheck 复用 |
| 三阶段 shutdown Phase 1 | P1 → P4 | `/readyz` 立即 503 |
| 性能指标 baseline | P4 → P2 | P2 优化时使用 P4 metrics 验证效果 |
| Tracing context 传播 | P4 → P3 | v2 新 API 自动注入 ctx |

---

## 10. 测试与验收

### 10.1 测试

```
test/cloudnative/
├── metrics_endpoint_test.go         # /metrics 内容正确
├── healthcheck_endpoint_test.go     # /healthz/readyz 状态切换
├── env_substitution_test.go         # ENV 占位符替换正确
├── tracing_propagation_test.go      # span 跨节点传递
├── graceful_deregister_test.go      # shutdown 后 ETCD/Master 立即清理
└── helm_chart_test.go               # helm template 渲染 + helm lint
```

### 10.2 验收

- 启动一个 docker-compose（NATS + ETCD + actorgo-gate + actorgo-game + Prometheus + Jaeger + Grafana），跑通完整可观测链路
- `helm install actorgo ./deploy/helm/actorgo --dry-run` 通过 schema 验证
- 杀 Pod 后 readyz 立即 unready，30s 内完成 graceful drain
- Prometheus 抓取所有内置指标无错误
- Jaeger 中能看到完整 client → gate → game 的 trace
- ENV 覆盖：`NATS_URL=nats://other:4222 ./app` 能正确连到 other

---

## 11. 实施路线（writing-plans 阶段细化）

按依赖关系分 5 个 PR：

1. **PR1** ENV 占位符 + JSON logger（无新依赖，最低风险，先上线）
2. **PR2** Metrics 组件（引入 prometheus/client_golang）
3. **PR3** healthcheck 组件 + 与 P1 shutdown Phase 1 联动
4. **PR4** Tracing 组件（引入 OTel SDK）
5. **PR5** 部署交付物（Dockerfile/Helm/Kustomize）

---

> 本设计文档为 actorgo v2 重构第 2 个子项目（实施顺序 P1→**P4**→P2→P3）。批准后将通过 writing-plans 转化为可执行 PR-level 计划。
