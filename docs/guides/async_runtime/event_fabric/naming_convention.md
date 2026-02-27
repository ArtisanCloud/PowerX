# Event Fabric 命名规范（Topic / Subscriber / Kind）

> 统一入口：`docs/guides/async_runtime/event_fabric/README.md`
> 执行级联调步骤请配合：`docs/guides/async_runtime/event_fabric/integration_playbook.md`

## 1. 目标

- 统一区分“事件语义通道”和“消费执行方”。
- 避免 `namespace` 形态相似导致误读。

## 2. 前缀规范

- Topic 命名空间前缀：`_topic`
- Subscriber 标识前缀：`_subscriber`
- 事件通知 kind 前缀：`_kind`

## 3. Topic 规范

### 3.1 逻辑 topic（不带作用域）

- 结构：`<namespace>.<name>`
- namespace 结构：`_topic.<domain>.<subdomain>...`
- 示例：
  - `_topic.system.notification`
  - `_topic.knowledge.space.feedback.reprocess`
  - `_topic.integration.gateway.invocation.failed`

### 3.2 full_topic（带作用域）

- 结构：`<scope_id>.<namespace>.<name>`
- 示例：
  - `global._topic.system.notification`
  - `global._topic.knowledge.space.feedback.reprocess`
  - `<tenant_uuid>._topic.knowledge.space.corpuscheck.run`

## 4. Subscriber 规范

- 结构：`_subscriber.<domain>.<handler>`
- 示例：
  - `_subscriber.event_fabric.cron_dispatch`
  - `_subscriber.system.notification_dispatch`
  - `_subscriber.knowledge_space.reprocess`
  - `_subscriber.knowledge_space.corpus_check`
  - `_subscriber.authorization.challenge_timeout`

## 5. Kind 规范

- 结构：`_kind.<domain>.<action>`
- 示例：
  - `_kind.event_fabric.replay.task`
  - `_kind.authorization.challenge.timeout`

## 6. 关系说明

- Topic 表达“发生了什么事件”。
- Subscriber 表达“谁来消费处理”。
- 队列分片维度：`tenant_key + subscriber_id`（不是 topic）。

## 7. 落库与代码约定

- 所有可治理 Topic 都必须进入 `event_topics`。
- 后端内置 topic 常量在 `backend/internal/event_bus/topics.go`。
- 后端内置 subscriber 常量在 `backend/internal/event_bus/subscribers.go`。
- 前端常量在 `web-admin/app/composables/domain/eventTopic.ts`。
- 当前 subscriber 以内置常量为主（`_subscriber.*`）。
