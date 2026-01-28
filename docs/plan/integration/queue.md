# Queue（统一队列）

## 目标
- 为 PowerX 底座与插件提供统一的队列抽象。
- 默认使用 Redis，允许替换为 Kafka/NATS/SQS。
- 支持延迟队列、重试、死信与可观测。

## 统一抽象
- **QueueDriver**：统一接口（push/pop/ack/nack/delay）。
- **QueueMessage**：标准消息结构（id、payload、headers、tenant）。
- **QueuePolicy**：重试/延迟/死信策略。

## 初始化流程（建议）
1) 读取 `queue` 配置块（宿主注入或插件本地）。
2) 解析驱动与连接参数（redis_url 优先）。
3) 注册 Provider（内置 + 自定义）。
4) 启动 worker 扫描到期延迟消息。
5) 注入业务侧 QueueClient。

## 默认实现（Redis）
- 使用 Redis List + ZSET 实现即时队列与延迟队列。
- 延迟消息写入 ZSET，worker 扫描到期后转入 List。
- 重试采用指数退避 + 抖动。

## 驱动与选择规则
### 支持驱动
- `redis`：默认生产驱动。
- `memory`：本地队列（开发/单体）。
- `noop`：空实现（禁用队列）。
- `custom`：自定义驱动（Provider 注册）。

### 选择优先级
1) `queue.driver`（config.yaml / host-values.yaml）
2) 环境变量 `POWERX_QUEUE_DRIVER`
3) 默认 `redis`（宿主模式） / `memory`（本地开发）

## 驱动配置规范
### Redis
```yaml
queue:
  driver: redis
  redis_url: "redis://:password@127.0.0.1:6379/4"
  host: 127.0.0.1
  port: 6379
  password: ""
  db: 4
  prefix: "powerx:{tenant_uuid}"
  default_visibility_timeout: 30s
  retry:
    max_attempts: 5
    backoff_seconds: 30
```

### Memory
```yaml
queue:
  driver: memory
  max_entries: 100000
```

### Noop
```yaml
queue:
  driver: noop
```

## 消息结构（建议）
```
QueueMessage:
  message_id: string
  tenant_uuid: string
  topic: string
  payload_json: bytes
  headers: map[string]string
  enqueue_at: timestamp
  attempt: number
```

## 投递语义与幂等
- 至少一次投递，消费者需幂等。
- 建议使用 `message_id` 或 `event_id` 做去重。

## 延迟与重试
- `Delay(topic, msg, runAt)` 写入延迟队列（ZSET）。
- 失败 `Nack` → 进入重试队列，按 backoff 计算下一次执行。
- 超过最大次数进入 DLQ（可选）。

## Worker 模型
- 多 worker 并发消费，按 `topic` 或 `tenant_uuid` 维度分区。
- 支持 `visibility_timeout` 防止重复消费。
- 丢失 ack 的消息自动回到队列。

## 宿主模式配置（PowerX 注入）
- 插件不在 `plugin.yaml` 配置队列。
- 宿主在启动插件时注入 `host-values.yaml` / `config.yaml` 的 `queue` 块。
- Standalone 模式由插件自身 `backend/etc/config.yaml` 提供 `queue` 块。

## 插件使用示例（伪代码）
```
queue.Push(ctx, "scheduler.job.triggered", payload)
msgs, _ := queue.Pop(ctx, "scheduler.job.triggered", 10)
for _, msg := range msgs {
  // handle
  queue.Ack(ctx, msg.MessageID)
}
```

## 可替换驱动
- Kafka：高吞吐、顺序保证、消费组。
- NATS：轻量消息总线。
- SQS：托管队列，适合异步任务。

## 与 Event Bus / Scheduler 关系
- Scheduler 触发事件 → 写入队列（可选） → Event Bus 投递。
- Event Bus 重试/延迟：由队列底座承载。

## API 草案（抽象接口）
```
type QueueDriver interface {
  Push(ctx, topic, msg) error
  Pop(ctx, topic, max int) ([]QueueMessage, error)
  Ack(ctx, messageID) error
  Nack(ctx, messageID, reason string) error
  Delay(ctx, topic, msg, runAt) error
}
```

## 观测
- `queue.enqueue_total` / `queue.dequeue_total`
- `queue.delay_total` / `queue.retry_total`
- `queue.dlq_total` / `queue.latency_p95`

## 错误码（建议）
- `queue.driver_unavailable`
- `queue.invalid_topic`
- `queue.rate_limited`
