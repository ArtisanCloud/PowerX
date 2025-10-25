# Transport Adapter Interface 设计

## 核心接口（Go）

```go
// TransportAdapter 统一抽象，所有协议实现必须满足。
type TransportAdapter interface {
    Invoke(ctx context.Context, req *TransportRequest) (*TransportResponse, error)
    Stream(ctx context.Context, req *TransportRequest, sink chan<- *StreamChunk) error
    HealthCheck(ctx context.Context, capabilityKey string) (*TransportHealthReport, error)
    Close(ctx context.Context) error
}
```

### TransportRequest

```go
type TransportRequest struct {
    RequestID        string
    TraceID          string
    TenantID         string
    ActorID          string
    CapabilityKey    string
    Version          string
    Transport        TransportKind // http/grpc/mcp/agent
    Payload          map[string]any
    Deadline         time.Time
    RetryContext     *RetryContext
    Metadata         map[string]string // 调用方额外信息
    Stream           *StreamDescriptor // 流式调用参数，可为空
    ToolGrantToken   string            // 代理调用必填
    Observability    *ObservabilityContext
}
```

### TransportResponse / StreamChunk

```go
type TransportResponse struct {
    RequestID       string
    Status          ResponseStatus
    Output          map[string]any
    Error           *CapabilityError
    ObservedVersion string
    Metrics         map[string]float64 // latency/error_rate 等
}

type StreamChunk struct {
    RequestID string
    Sequence  uint64
    Kind      StreamChunkKind // data, log, event, error
    Payload   map[string]any
    Timestamp time.Time
    Error     *CapabilityError
}
```

### 错误映射

```go
type CapabilityError struct {
    Namespace       string
    Category        string
    Code            string
    Severity        ErrorSeverity
    Stage           ErrorStage
    Message         string
    SuggestedAction string
    Cause           error
    Telemetry       map[string]string
}
```

适配器实现必须：

- 将底层协议错误映射到契约声明的 `ErrorTaxonomy`。  
- 提供 `Telemetry` 标签以便 metrics/tracing 聚合。  
- 对于 `Severity >= ERROR` 的返回需同时写入审计日志。

## 生命周期约束

1. Adapter 通过依赖注入注册到 `internal/transport/*`。  
2. `HealthCheck` 将填充 `TransportProfile.last_health_status`，Router 依据结果降级。  
3. 流式调用必须在 5 秒内提供首次 `StreamChunk` 或错误反馈。  
4. `Close` 在路由卸载或进程退出时调用，负责释放连接、会话或流。  
5. Adapter 必须遵循幂等性声明：当 `RetryContext.Idempotent = true` 时，需保证重复调用不产生副作用。

## 观测与审计

- 所有 `Invoke/Stream` 调用需写入统一的 tracing span：`transport.<protocol>.<capability>`。  
- Metrics 至少包含：`latency_ms`、`retry_count`、`error_code`、`transport_mode`。  
- 审计事件通过 EventBus 发布：`integration.capability.invocation`，包含 tenant/actor/trace。

## 配置注入

Adapter 需从 `TransportProfile` 中读取以下配置：

| 字段 | 行为 |
| --- | --- |
| `timeout_ms` | 设置调用超时，上层 Deadline 兜底 |
| `retry` | 控制退避、最大尝试次数、是否允许自动重试 |
| `streaming` | 决定是否启用流式路径 |
| `qos` | 包含并发限制、优先级、带宽等信息，需要在 Adapter 侧落实 |
| `endpoint_selector` | 由 Router 提供的端点过滤条件（区域、标签等） |

当配置变更时，Adapter 需监听 Registry 事件，动态刷新内部缓存，避免服务重启。
