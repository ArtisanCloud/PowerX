## Quickstart: Capability Registry & Router

### 前置条件
- 已部署 Postgres、Redis，并配置 EventBus（Kafka/NATS 任一）连接。
- 已完成 Capability Contract & Transport Adapter 注册（参考 `specs/002-title-unified-capability`）。
- 运行环境加载 `tenant_id`、`tool_grant` 校验组件。

### 步骤 1：启动 Registry 服务
1. 构建并启动 `internal/service/capability_registry` 相关模块。
2. 配置环境变量：
   - `REGISTRY_DB_DSN`（Postgres）
   - `REDIS_URI`
   - `EVENTBUS_BROKER_URL`
3. 运行健康检查 `GET /admin/capabilities/healthz` 确认服务启动成功。

### 步骤 2：注册能力
1. 调用 `POST /admin/capabilities`，示例请求：
   ```json
   {
     "capability_id": "capabilities.search.v1",
     "tenant_id": "tenant-001",
     "contract_ref": "contracts/search#v1.2.0",
     "adapters": [
       {
         "adapter_id": "search-http-primary",
         "transport_type": "http",
         "endpoint_url": "https://svc-primary/search",
         "weight": 80,
         "timeout_ms": 2000
       },
       {
         "adapter_id": "search-grpc-backup",
         "transport_type": "grpc",
         "endpoint_url": "grpc://svc-backup:8443",
         "weight": 20,
         "timeout_ms": 2500
       }
     ],
     "routing_policy": {
       "strategy": "weighted_round_robin",
       "cooldown_seconds": 60,
       "fallback_sequence": ["search-grpc-backup"]
     }
   }
   ```
2. 响应头会返回 `ETag: W/"<version>"`，同时在体内回显 `version` 字段；后续更新时必须携带 `If-Match`。
3. 更新时调用 `PUT /admin/capabilities/{capabilityId}/tenants/{tenantId}`，请求体需包含最新 `version`，并在 Header 中附带 `If-Match: W/"<version>"` 以触发乐观锁校验。
4. 禁用注册使用 `DELETE /admin/capabilities/{capabilityId}/tenants/{tenantId}`，支持在 Body 中附带 `reason` 便于审计。

> gRPC 等价接口：`CapabilityRegistryService/CreateCapability`、`UpdateCapability`、`DisableCapability`，请求体复用 proto `CapabilityRegistration`。版本号同样由服务返回并在后续请求中透传。

### 步骤 3：订阅能力变更
- Router 服务通过 EventBus 订阅 `capability.registry.updated` 主题，事件载荷包含：
  ```json
  {
    "capability_id": "capabilities.search.v1",
    "tenant_id": "tenant-001",
    "version": 3,
    "status": "published",
    "change_kind": "updated",
    "updated_by": "admin-user"
  }
  ```
- 如果订阅失败，可调用 `GET /admin/capabilities/{id}/tenants/{tenantId}?version=latest` 拉取最新快照；支持 `version=<数字>` 查询历史版本。
- gRPC 订阅可使用 `StreamUpdates`（规划阶段），或定期调用 `GetCapability` 进行补偿。

### 步骤 4：调用 Router
1. 调用 `POST /router/invoke`（或 gRPC `Invoke`）并提供 `capability_id`、`tenant_id`。
2. Router 根据权重、健康与策略选择适配器，返回结果。
3. 当主适配器故障，Router 在 500ms 内切换到 fallback 并记录降级事件。

### 步骤 5：客户端缓存
1. 客户端通过 `GET /discovery/{tenant}/{capability}` 拉取快照，保存到本地。
2. 遵循默认 2 分钟 TTL，TTL 将在响应头 `X-Cache-TTL` 返回。
3. 若缓存过期或 Registry 不可达，使用最后快照并记录日志。

### 故障定位
- 查看 Registry 服务日志（包含 `trace_id`、`tenant_id`）。
- 查询健康表：`SELECT * FROM capability_health_probes WHERE adapter_id = ...`.
- 关注事件通道 `capability.registry.degraded` 获取降级通知。
