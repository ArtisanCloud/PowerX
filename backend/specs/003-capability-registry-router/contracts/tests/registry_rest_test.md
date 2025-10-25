# Registry REST 合同测试用例

## 场景一：创建能力注册
- **前置条件**：Registry 服务运行，租户 `tenant-corex` 已存在，Tool Grant 和契约引用均有效。
- **请求**：`POST /admin/capabilities`
  ```json
  {
    "capability_id": "capabilities.text.translate",
    "tenant_id": "tenant-corex",
    "contract_ref": "contracts.text.translate@1.0.0",
    "status": "published",
    "tool_grant_ids": ["grant-text-translate"],
    "adapters": [
      {
        "adapter_id": "adapter-grpc-1",
        "transport_type": "grpc",
        "service_ref": "grpc://translator.corex.svc:443",
        "weight": 80,
        "timeout_ms": 4000,
        "labels": {"region": "ap-sg"}
      },
      {
        "adapter_id": "adapter-http-2",
        "transport_type": "http",
        "endpoint_url": "https://translator.corex/api",
        "weight": 20,
        "timeout_ms": 2500
      }
    ],
    "routing_policy": {
      "strategy": "weighted_round_robin",
      "cooldown_seconds": 60,
      "fallback_sequence": ["adapter-http-2"]
    }
  }
  ```
- **期望响应**：`201 Created`，返回 `capability_id / tenant_id / version / status`，响应头包含 `ETag: W/"<version>"`。

## 场景二：查询能力注册快照
- **请求**：`GET /admin/capabilities/capabilities.text.translate/tenants/tenant-corex`
- **期望响应**：`200 OK`，返回完整的 `CapabilityRegistration` JSON，字段 `version` 与创建成功后返回一致，响应头包含 `ETag`。

## 场景三：乐观锁更新能力注册
- **请求**：
  - Header: `If-Match: W/"<version>"`
  - `PUT /admin/capabilities/capabilities.text.translate/tenants/tenant-corex`
  - Body 中调整 `adapters[0].weight = 60`、`adapters[1].weight = 40`
- **期望响应**：`200 OK`，响应体中 `version` 递增 `+1`，并回显权重调整后的配置。

## 场景四：版本冲突
- **请求**：缺少 `If-Match` 头或使用过期版本调用 `PUT`
- **期望响应**：`412 Precondition Failed`，错误体包含 `"code": "registry.version_conflict"`。

## 场景五：禁用能力注册
- **请求**：`DELETE /admin/capabilities/capabilities.text.translate/tenants/tenant-corex`，Body `{"reason":"deprecated capability"}`。
- **期望响应**：`202 Accepted`，并触发 `capability.registry.updated` 事件。

## 场景六：禁用后查询
- **请求**：`GET /admin/capabilities/capabilities.text.translate/tenants/tenant-corex?version=latest`
- **期望响应**：`404 Not Found`。
