# AI 请求时限与配置对齐

业务推理时限以主配置 `ai.defaults.llm.request_timeout` 为准。`ai.runtime.queue_wait_timeout` 控制取得资源前的总排队时间；租户／Profile 限额和 Redis 物理模型池共享这一个排队时限，不能分别重新计时。真正取得调用资格后才开始推理时限，排队不占用推理额度。

Core 启动时将同一份主配置传给模型调用与内部 HTTP 代理。AI 代理等待时间采用：

```text
max(capability_registry.ai_multimodal_http_timeout_seconds,
    ai.runtime.queue_wait_timeout + ai.defaults.llm.request_timeout + 10 秒)
```

代理配置表示等待时间下限。配置更长的值继续保留；较短的旧值自动对齐，并在启动日志输出 configured/effective/request/queue 时长。10 秒只用于接收、整理和返回结果或结构化错误，不增加业务推理时间。普通能力 HTTP 请求继续使用 `default_http_timeout_seconds`。代理的额外等待不反向扩大 Embedding 等非 LLM Handler 原有的配置期限；LLM 会话流与普通 LLM 入口统一交由模型调用层管理排队和推理，不叠加旧的固定 Handler 截止时间。

物理模型池租约同样按推理配置对齐：

```text
max(physical_model_pools[].lease_ttl,
    ai.defaults.llm.request_timeout + 10 秒)
```

租约保持原有正数和一小时上限校验；推理配置无法在一小时内留下租约窗口时明确拒绝启动。较短的正数旧租约按安全下限补齐，并记录 configured/effective 日志。该时长只用于槽位持有与续期，不扩大推理时限或并发容量。

例如：推理配置 10 分钟、排队配置 10 分钟、旧代理配置 310 秒、旧租约 6 分钟，实际推理仍为 10 分钟；AI 代理最多等待 20 分 10 秒；物理槽位租约为 10 分 10 秒，并继续续期和释放。

客户端、插件业务接口或整轮 Run 若设置了更早的 deadline，仍按最早的期限取消。Core 不能延长调用方的 deadline。插件客户端应留足排队、推理和回包时间；长任务适合采用异步任务与推送，不以延长同步 HTTP 连接代替任务状态管理。

排队耗尽返回独立的模型排队超时；调用方取消或调用方 deadline 不伪装成排队超时。Provider 超时保留其结构化原因，避免内部代理先断开而丢失错误。

修改配置后，按原启动方式重新启动 Core，使配置和计算后的代理／租约参数一起加载。Agent 不重启用户拥有的服务。
