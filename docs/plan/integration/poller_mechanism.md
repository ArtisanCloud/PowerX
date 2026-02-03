# 轮询机制开发计划（PowerX 底座 + 插件可复用）

## 背景与目标

当前异步接口（如即梦图像生成）需要“提交任务 → 轮询结果”。现有实现为驱动内部自带轮询，不便复用，也缺少统一的限流/上限控制策略。

本计划建立一套可复用的轮询机制：
- **PowerX 底座**可直接调用
- **插件**可复用（避免 internal 路径限制）
- **统一限流与上限控制**，避免无限轮询与网络浪费

## 设计原则

1. **可复用**：放到 `pkg/` 下，插件可直接 import
2. **安全可控**：明确最大时长 / 最大次数 / 轮询间隔
3. **可观测**：统一日志与可选指标埋点
4. **最小心智**：函数式 API 即可使用，避免复杂依赖

## 目标范围

- **适用场景**：任何“异步提交 + 轮询结果”的 API（图像生成、视频生成、任务队列等）
- **不包含**：队列系统、消息订阅系统、任务调度引擎

## 建议落点

- `pkg/corex/async/poller/`
  - `poller.go`：核心 API
  - `options.go`：配置与默认值

## API 设计草案

### 1) 函数式 API（最轻）

```go
// Result: 轮询函数返回结果
// done=true 表示结束；done=false 表示继续轮询
// err!=nil 直接结束并返回错误

type PollFunc[T any] func(ctx context.Context, attempt int) (done bool, result T, err error)

type Options struct {
    Interval        time.Duration // 轮询间隔
    Timeout         time.Duration // 最大总时长
    MaxAttempts     int           // 最大轮询次数（0=无限，推荐不使用）
    BackoffStrategy string        // fixed / exponential
    BackoffFactor   float64       // 仅 exponential
    OnRetry         func(attempt int, err error)
}

func Poll[T any](ctx context.Context, opt Options, fn PollFunc[T]) (T, error)
```

### 2) 退避策略

- **fixed**：固定间隔（默认 2s）
- **exponential**：Interval * (BackoffFactor^(attempt-1))，可设置最大上限

## 默认策略（建议）

- `poll_interval_ms = 2000`
- `poll_timeout_ms = 60000`
- `poll_max_attempts = 30`

默认行为：**任何一个条件满足即停止轮询**。

## 即梦接入方式

- `CVSync2AsyncSubmitTask` → 得到 `task_id`
- `CVSync2AsyncGetResult` → 轮询结果
- 轮询使用统一 `Poll()`，避免驱动内自定义死循环

## 对外配置建议（Manifest / Defaults）

支持将轮询策略配置放在 provider/model defaults 中：

```yaml
defaults:
  poll_interval_ms: 2000
  poll_timeout_ms: 60000
  poll_max_attempts: 30
```

驱动读取 defaults 与 runtime 参数合并后注入 `Poll()`。

## 日志与观测

统一日志标签：
- provider / model / attempt / elapsed_ms
- 失败时输出 `last_error`

可选指标（后续扩展）：
- poll_attempt_total
- poll_timeout_total
- poll_success_total

## 风险与边界

- **无限轮询风险**：若用户配置 `MaxAttempts=0` 且无超时，将造成资源浪费
- **高频轮询风险**：interval 过低可能触发上游 QPS 限制
- **超时过短风险**：会误判任务失败

## 验收标准

- 轮询次数与时长可控
- 任务超时能明确返回
- 插件可引用并复用同一 Poller
- 即梦驱动从自定义轮询切换为 Poller
