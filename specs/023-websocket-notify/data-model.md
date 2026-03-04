# Data Model

## 实体：实时消息连接（WebSocketSession）

- **含义**: 单个用户在某一租户环境下建立的 WS 连接实例。
- **关键属性**:
  - session_id（连接标识）
  - tenant_uuid（租户上下文）
  - user_id / member_id（用户或成员标识）
  - subscribed_topics（已订阅主题列表）
  - last_seen_at（最近心跳时间）

## 实体：主题订阅（TopicSubscription）

- **含义**: 连接与主题的订阅关系。
- **关键属性**:
  - session_id
  - topic
  - created_at

## 实体：消息事件（MessageEnvelope）

- **含义**: 服务端推送的统一消息结构。
- **关键属性**:
  - topic
  - type（snapshot/delta/event）
  - payload（业务数据）
  - ts（时间戳）
  - trace_id（可选）

## 实体：入库进度（IngestionProgress）

- **含义**: 入库任务的阶段与进度快照。
- **关键属性**:
  - space_id
  - job_id
  - status
  - stage
  - progress（0-100）
  - chunk_total / embedding_pct / masking_pct
  - updated_at
