# WS Topic Registry 与 Grant 落地规范（SOP）

## 1. 适用范围

本文用于规范 PowerX 宿主 + 插件场景下，WS 动态 topic（例如 `ai_craft.shopify.sync.progress.<tenant_uuid>`）的注册、授权、订阅与验收流程。

适用对象：
- PowerX 底座开发/运维
- 插件后端开发
- 前端联调与测试

---

## 2. 问题背景（本规范要解决什么）

在当前实现中：
- `grant` 走 `FindByComposite(tenant, namespace, name)` 精确查找；
- `subscribe` 授权也依赖 topic registry + ACL；
- 仅 fallback 成功（`topics: []`, `fallback: [...]`）不代表可订阅成功。

因此，若 topic 未在 registry 中有可命中定义，前端会出现：
- `permission_denied`
- `subscription rejected`
- `topic not allowed`

---

## 3. 统一命名规范

以 Shopify 同步进度为例：

- 运行时 topic（完整）：
  - `ai_craft.shopify.sync.progress.<tenant_uuid>`
- 拆分后：
  - `tenant_key` = `<tenant_uuid>`
  - `namespace` = `ai_craft.shopify.sync.progress`
  - `name` = `<tenant_uuid>`

说明：当前版本不依赖占位符模板匹配；按精确组合注册最稳。

---

## 3.1 字段映射（按当前代码实现）

以 `tenant_uuid=1edd...` 为例，注册时应理解为：

1. 业务 topic（插件 publish/前端 subscribe 使用）
- `ai_craft.shopify.sync.progress.1edd...`

2. Registry 命中三元组（授权查找关键）
- `tenant_key = 1edd...`
- `namespace = ai_craft.shopify.sync.progress`
- `name = 1edd...`

3. `full_topic`（数据库派生字段，不建议外部手工拼接）
- 当前模型派生规则为：`<scope_id>.<namespace>.<name>`
- 默认 `scope_id = tenant_key`
- 因此常见值：`1edd....ai_craft.shopify.sync.progress.1edd...`

4. `status` 与 `lifecycle`
- `status` 为数值字段（常用 `1` 表示有效），不是字符串 `"active"`。
- 生命周期状态使用 `lifecycle_status` 字段（如 active/deprecated 等）。

5. `source_plugin_id`
- 不是 TopicDefinition 顶层固定列。
- 建议写入 `metadata.source_plugin_id` 或 `created_by` 用于追溯。

---

## 4. 注册时机与责任边界（当前实现）

### 4.1 时机

1. 插件安装完成：可不立即注册租户 topic。
2. 插件启用（Enable）时：必须对启用租户执行 topic 注册（upsert）。
3. 插件禁用：不强制删除（避免历史任务链路中断）。
4. 插件卸载：可按策略做软删除/停用标记。

### 4.2 责任归属

- 主责任：PowerX 底座插件生命周期（Enable 流程）
- 插件仅负责发起 grant/publish，不负责绕过宿主 registry 规则。

### 4.3 当前代码接入点

1. `backend/internal/bootstrap/plugin.go`：
- `PostEnable(ctx, tenantUUID, pluginID)` 会调用 `seedPluginEventFabric`。
2. `backend/internal/transport/http/admin/plugin/tenant_handler.go`：
- `POST /admin/plugins/:id/tenant_enable` 的 `enabled=true` 路径会调用 `ensureTenantEventFabricTopics(...)`，确保租户启用场景也触发 seed。
3. seed 实现：
- `backend/internal/service/event_fabric/manifest/seed_service.go`（`FindByComposite` + `CreateTopic` 幂等）。

### 4.3 强制约束（必须满足）

1. 注册发生在 `Enable(tenant)`，不是“全局安装一次”。
2. 动态 topic 必须在 Enable 时解析为真实值（例如 `name=<tenant_uuid>`），不依赖占位符匹配。
3. 注册逻辑必须幂等：`FindByComposite -> 存在则校正/跳过 -> 不存在才 Create`。

---

## 5. 注册写入策略（必须 UPSERT）

### 5.1 唯一键建议

- 业务幂等键：`(tenant_key, namespace, name)`（服务层通过 `FindByComposite` 实现）。
- 数据库内建唯一约束：`full_topic`（最终兜底唯一）。

### 5.2 Upsert 行为

- 已存在：更新 `status/updated_at/source_plugin_id`
- 不存在：插入新记录

### 5.3 推荐字段

- `tenant_key`
- `namespace`
- `name`
- `full_topic`（由模型派生）
- `metadata.source_plugin_id`（例如 `com.powerx.plugins.ai-craft`）
- `status`（数值，常用 `1`）
- `lifecycle_status`（按生命周期管理）

---

## 6. 运行时调用顺序（必须按顺序）

1. 前端/插件先调用 grant
2. grant 成功后发起 subscribe
3. 后端 publish/emit

注意：
- grant 成功仅表示授权请求处理成功；
- 只有当 `grant.data.topics` 命中完整 topic（非空）时，才视为“可订阅授权成功”。

---

## 7. 验收标准（强制）

### 7.1 Grant 验收

必须同时满足：

1. HTTP 200
2. `data.topics` 非空
3. `data.topics` 包含目标完整 topic

若出现以下任一情况，判定失败：

- `data.topics: []`
- 仅有 `fallback`，无真实 topics 命中

### 7.2 WS 验收

必须同时满足：

1. `subscribe` 返回成功（无 `topic not allowed`）
2. 日志出现 `stage=subscribed` 且包含目标 topic
3. 日志出现 `stage=emit` 且 `emitted_count > 0`
4. 前端收到进度事件并更新 UI

---

## 8. 联调排查最短命令

### 8.1 看 grant 是否命中真实 topics

```bash
# 前端 Network 中查看 grant 响应，重点看 data.topics 是否非空
```

### 8.2 看订阅与投递

```bash
sudo journalctl -u powerx-backend --since "10 min ago" --no-pager -l | \
grep -E 'transport.wsbus|stage":"subscribed"|stage":"emit"|topic not allowed|permission_denied'
```

### 8.3 看业务 publish

```bash
sudo journalctl -u powerx-backend --since "10 min ago" --no-pager -l | \
grep -E 'SYNC_PROGRESS_PUBLISH|shopify sync progress published|wsbus publish succeeded'
```

---

## 9. 回滚策略

当注册策略上线后出现异常：

1. 保留已有 topic 数据，不做硬删。
2. 回滚仅回滚“注册调用流程代码”。
3. 若 ACL 绑定异常，优先重跑 Enable 流程触发幂等 upsert。

---

## 10. 实施清单（Checklist）

1. Enable 流程中已接入 topic upsert
2. Upsert 唯一键为 `(tenant_key, namespace, name)`
3. grant 响应 `data.topics` 已被接入自动校验
4. 前端已在 grant 后执行 subscribe
5. 监控已覆盖 `topic not allowed` 告警

---

## 11. 注册请求样例（给实现同学）

> 面向 `POST /api/v1/admin/event-fabric/topics` 的创建语义。  
> 若用于 Enable 幂等流程，建议在服务层实现“先查后更/不存在则创建”。

```json
{
  "namespace": "ai_craft.shopify.sync.progress",
  "name": "1edd4132-1644-412d-abb4-d5f1e9487052",
  "payload_format": "json",
  "max_retry": 5,
  "ack_timeout_sec": 30,
  "versioning_mode": "strict",
  "metadata": {
    "source_plugin_id": "com.powerx.plugins.ai-craft",
    "usage": "shopify_sync_progress"
  },
  "created_by": "plugin-enable"
}
```

对应租户上下文：
- 通过鉴权上下文提供 `tenant_uuid`（不是 body 字段）。

---

## 13. 落库与接口真相源

1. 主题定义表：
- `powerx.event_topics`（模型：`TopicDefinition`）。
2. ACL 绑定表：
- `powerx.event_topic_acl_bindings`（由 ACL Grant 写入）。
3. Manifest 幂等绑定表（用于避免重复授权）：
- `powerx.event_topic_manifest_bindings`
- `powerx.event_acl_manifest_bindings`
4. 宿主管理接口（人工/运维）：
- `POST /api/v1/admin/event-fabric/topics`（创建主题）
- `PATCH /api/v1/admin/event-fabric/topics/:topic_id/lifecycle`（生命周期）
5. 运行时 grant 接口（前端调用）：
- `POST /api/v1/admin/runtime/internal/ws-bus/grant`

---

## 12. 实现分工（PowerX vs 插件）

### 12.1 PowerX 底座必须实现

1. 在插件 `Enable(tenant)` 生命周期中读取插件 event-fabric topic 声明。
2. 解析动态项为真实 topic 三元组（tenant_key/namespace/name）。
3. 调用内部 DirectoryService 执行幂等注册（先 `FindByComposite`，后 `CreateTopic`）。
4. 对已存在记录执行校正（至少 lifecycle/status 合法化）。
5. 记录来源（`metadata.source_plugin_id` 或 `created_by`）。
6. 输出可观测日志：`topic_ensure_started/topic_ensure_exists/topic_ensure_created/topic_ensure_failed`。

### 12.2 插件侧必须实现

1. 在插件包内提供可被宿主读取的 topic 声明（event-fabric manifest）。
2. 运行时保持业务 topic 命名与声明一致（namespace/name 规则不能漂移）。
3. 前端调用顺序固定为：`grant -> subscribe`（不要先订阅）。
4. 当 grant 返回 `topics: []` 或 ws 返回 `topic not allowed` 时，前端必须提示“注册未命中”，不静默。

### 12.3 联调责任边界

1. 若 grant `topics` 非空但订阅失败：优先查 ACL 绑定。
2. 若 grant `topics` 为空且 fallback 有值：优先查 PowerX Enable 注册流程。
3. 若 publish 成功但无前端事件：先看 `subscribed/emit`，再查前端分发。
