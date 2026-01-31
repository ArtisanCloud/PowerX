# Research

## Decision 1: 单连接多主题消息模型

- **Decision**: 采用单 WS 连接 + topic 订阅模型，客户端可订阅多个主题。
- **Rationale**: 减少连接数，降低资源消耗；满足多模块统一通知需求。
- **Alternatives considered**: 每模块独立连接（连接数膨胀，易失控）。

## Decision 2: 租户切换处理策略

- **Decision**: 切换租户时强制断开并重连，清空旧订阅。
- **Rationale**: 确保租户隔离，避免误投递旧租户消息。
- **Alternatives considered**: 同连接多租户隔离过滤（复杂且易误用）。

## Decision 3: 无权限订阅处理

- **Decision**: 拒绝订阅并返回错误。
- **Rationale**: 权限边界明确，避免静默失败导致用户误判。
- **Alternatives considered**: 静默忽略或允许订阅但不推送。

## Decision 4: 高频进度更新抖动控制

- **Decision**: 前端节流展示，最多每 1 秒更新一次。
- **Rationale**: 降低 UI 抖动与重复渲染开销。
- **Alternatives considered**: 后端合并推送或全量推送不节流。
