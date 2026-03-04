# API Key / Token 联调指南（Integration Gateway）

本文用于回答三个问题：

1. 现在怎么测试 API Key / Token？
2. 页面里怎么获取 `token` / `api_key`？
3. `api_key` 权限怎么配置？

> 适用版本：当前分支已实现 OpenAPI 统一入口下的双凭证模型（JWT / API Key）。  
> 说明：`token` 与 `api_key` 是按调用主体分层使用，不是业务优先级替代关系。

---

## 0. 前置条件

- Backend 已启动：`http://127.0.0.1:8077`
- API 前缀：由 `server.api_prefix` 控制（默认 `/api`，常见部署为 `/api/v1`）
- 已有可登录租户账号（普通租户用户可进入页面；创建/轮换/吊销等管理动作需具备租户管理权限）

插件侧三条硬规则（务必遵守）：

- `PX_GATEWAY_BASE_URL` 只放主机，不带前缀（例如 `http://127.0.0.1:8077`）
- `PX_GATEWAY_API_PREFIX` 必须显式配置（插件默认建议 `/api/v1`），不要用 `/` 代替“无前缀”
- 调用地址拼接固定为：`{PX_GATEWAY_BASE_URL}{PX_GATEWAY_API_PREFIX}/tenant/invocations`

建议先通过 meta 接口确定运行时前缀，再拼接所有请求路径：

```bash
curl -sS "http://127.0.0.1:8077/api/v1/admin/gateway/meta" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq .
```

从返回读取：

- `base_url`
- `api_prefix`

并约定：

- `HTTP_BASE = base_url + api_prefix`

错误示例 vs 正确示例：

- 错误：
  - `PX_GATEWAY_BASE_URL=http://127.0.0.1:8077/api/v1`
  - `PX_GATEWAY_API_PREFIX=/`
  - 最终请求：`http://127.0.0.1:8077/tenant/invocations`
- 正确：
  - `PX_GATEWAY_BASE_URL=http://127.0.0.1:8077`
  - `PX_GATEWAY_API_PREFIX=/api/v1`
  - 最终请求：`http://127.0.0.1:8077/api/v1/tenant/invocations`

> 说明：`root` 不是前置条件。只有在需要做“跨租户审计/治理”时才使用 root 账号。

### 0.1 凭证分层（按场景使用）

- OpenAPI 是统一业务入口，JWT 和 API Key 是不同主体的鉴权凭证。
- JWT（Token）：人登录后的会话凭证，适用于管理台、人机联调、宿主模式插件调用。
- API Key：系统到系统凭证，适用于第三方平台调用、插件 standalone proxy 模式调用。
- 宿主模式插件调用底座能力走 Gateway/OpenAPI 统一入口，但默认凭证是用户 JWT（宿主会话），不是 API Key。
- API Key 仅用于外部系统调用，或插件在 standalone proxy 模式下调用宿主 Gateway/OpenAPI。

### 0.2 租户边界（已生效）

- 普通租户仅能管理本租户 API Key（创建/轮换/吊销/权限变更）。
- `admin/root` 允许做跨租户“有效性治理”（如禁用/吊销），用于平台级安全处置。
- `admin/root` 不应查看其他租户明文 key（仅做状态治理与审计）。

### 0.3 凭证互斥约定（推荐）

- 业务约定上，单次请求只应携带一种凭证：`JWT` 或 `API Key`。
- 宿主模式固定 JWT；外部平台/standalone proxy 固定 API Key。
- 后端按凭证类型分流：`Authorization: ApiKey <key>` 仅走 API Key 校验，`Authorization: Bearer <token>` 仅走 JWT 校验。
- 不再采用“API Key 失败后回退 JWT”的混合兜底策略。

### 0.4 能力目录接口选型（已对齐）

- 对外统一“能力列表/筛选”主接口：`GET /api/v1/admin/capabilities`
  - 支持 `source=corex|plugin`（空值/`all` 表示全部来源）
  - 适合插件/外部系统做能力检索、筛选、联调
- 平台聚合展示接口：`GET /api/v1/admin/platform-capabilities`
  - 适合“开放能力”页面按模块聚合展示（可带 `page/page_size`）
  - 不建议作为外部统一检索入口

新增辅助接口：

- `GET /api/v1/admin/capabilities/sources`：返回 source 枚举、默认值与别名映射
- `GET /api/v1/admin/gateway/meta`：返回 gateway 元信息（`base_url`、`api_prefix`、auth schemes、常用路径示例）

### 0.5 Invoke 入口选型（避免混用）

- Selector 调用（按 `capability_id` / `intent`）：
  - `POST {HTTP_BASE}/tenant/invocations`
- Integration Route 调用（按 `route_slug`）：
  - `POST {HTTP_BASE}/tenant/integration/routes/{route_slug}/invoke`

明确说明：

- 当前代码里不存在 `POST /integration/capabilities/invoke`
- 当前代码里不存在 `POST /tenant/integration/capabilities/invoke`
- 历史讨论中若出现上述路径，请全部替换为本节两条正式入口

Selector 示例：

```bash
curl -sS -X POST "$HTTP_BASE/tenant/invocations" \
  -H "Authorization: Bearer $TENANT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "capability_id":"com.corex.media.assets.read",
    "idempotency_key":"demo-selector-001",
    "payload":{}
  }' | jq .
```

Route Invoke 示例：

```bash
curl -sS -X POST "$HTTP_BASE/tenant/integration/routes/media-assets-read/invoke" \
  -H "Authorization: Bearer $TENANT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "idempotency_key":"demo-route-001",
    "payload":{}
  }' | jq .
```

### 0.6 `source` 定义来源（后端口径）

`source` 来自后端能力记录，不是前端固定值：

1. 优先读取 `CapabilityRecord.annotations.source`
2. 若未设置，按 `plugin_id` 推断：
   - `corex.*` => `corex`
   - 其他 => `plugin`

别名与归一化：

- `platform` 归一化为 `corex`
- `all` / `any` / 空值 => 不过滤（全部来源）

可用以下接口自检当前环境 source 枚举：

```bash
curl -sS "http://127.0.0.1:8077/api/v1/admin/capabilities/sources" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq .
```

---

## 1. 获取 Token（JWT）

### 1.1 通过登录接口获取

```bash
curl -sS -X POST "http://127.0.0.1:8077/api/v1/admin/user/auth/login" \
  -H "Content-Type: application/json" \
  -d '{
    "tenant":"<你的租户标识>",
    "identifier":"<管理员账号>",
    "password":"<管理员密码>"
  }' | jq .
```

返回中的 `data.access_token` 即 `ADMIN_TOKEN`。

### 1.2 在页面里取 token

登录 Web Admin 后，在浏览器控制台执行：

```js
localStorage.getItem('access_token')
```

---

## 2. 获取并创建 API Key

API Key 需要绑定 `tenant_uuid + api_key_profile`，建议先准备 profile 再创建 key。

### 2.0 默认自动创建（已实现）

- `seed root` 时会自动为 `system` 租户创建默认 profile：`integration.default`。
- 新建租户（`POST /api/v1/admin/tenants`）时会自动创建默认 profile：`integration.default`。
- profile 新增 `owner_member_id` 字段：创建者为租户成员时自动写入，便于审计归属。
- 默认 profile 会自动勾选当前租户下 `module=integration_gateway` 且 `allow_api_key=true` 的全部权限。
- 后续若 `integration_gateway` 权限目录新增条目，重新执行 `make db-seed` 可自动补齐到默认 profile。

### 2.1 查当前租户 UUID

```bash
curl -sS "http://127.0.0.1:8077/api/v1/admin/auth/me/context" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq .
```

拿到 `data.tenant_uuid`，记为 `TENANT_UUID`。该值用于理解当前上下文与审计。

### 2.2 查可用 `api_key_profile`（推荐：API）

```bash
curl -sS "http://127.0.0.1:8077/api/v1/admin/integration/api-key-profiles" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq .
```

选择返回中 `status=1` 的 `id` 作为 `PROFILE_ID`。

### 2.3 无 `api_key_profile` 时先创建一个（API）

```bash
curl -sS -X POST "http://127.0.0.1:8077/api/v1/admin/integration/api-key-profiles" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name":"Integration Default API Key Profile"
  }' | jq .
```

创建完成后，重新执行 2.2 获取 `PROFILE_ID`。

### 2.4 先为 Profile 绑定 `permission_ids`

先查询可选权限目录（只返回可用于 API Key 的权限）：

```bash
curl -sS "http://127.0.0.1:8077/api/v1/admin/integration/permissions/catalog" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq .
```

然后把选中的权限 ID 绑定到 Profile：

```bash
curl -sS -X PUT "http://127.0.0.1:8077/api/v1/admin/integration/api-key-profiles/PROFILE_ID/permissions" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "permission_ids":[101,102,103]
  }' | jq .
```

### 2.5 创建 API Key（继承 Profile 权限）

```bash
curl -sS -X POST "http://127.0.0.1:8077/api/v1/admin/integration/api-keys" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "profile_id": PROFILE_ID,
    "name":"ws-debug-key",
    "description":"for ws-bus debug"
  }' | jq .
```

返回里：

- `data.api_key`：密钥元信息（可重复查询）
- `data.plain_key`：**明文 API Key（只返回这一次）**

请马上保存 `plain_key`，后续不会再次回显。

---

## 3. 使用 API Key 测试（不带 JWT）

先区分两个接口语义（避免混淆）：

- `POST /api/v1/admin/event-fabric/topics`：创建/登记 topic（写入 `event_topics`）。
- `POST /api/v1/internal/ws-bus/grant`：对已存在 topic 做授权绑定（ACL grant），不创建 topic。

### 3.1 ws-bus/grant

```bash
curl -sS -X POST "http://127.0.0.1:8077/api/v1/internal/ws-bus/grant" \
  -H "Authorization: ApiKey $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "topics":["_topic.system.notification"],
    "actions":["publish","subscribe"]
  }' | jq .
```

### 3.2 ws-bus/publish

```bash
curl -sS -X POST "http://127.0.0.1:8077/api/v1/internal/ws-bus/publish" \
  -H "Authorization: ApiKey $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "topic":"_topic.system.notification",
    "payload":{"msg":"hello from api key"}
  }' | jq .
```

### 3.3 使用 API Key 调用 Invoke（Selector / Route）

先约定：

- `HTTP_BASE = base_url + api_prefix`（通过 `/admin/gateway/meta` 获取）

#### Selector Invoke（按 capability_id）

```bash
curl -sS -X POST "$HTTP_BASE/tenant/invocations" \
  -H "Authorization: ApiKey $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "capability_id":"com.corex.media.assets.read",
    "idempotency_key":"demo-apikey-selector-001",
    "payload":{}
  }' | jq .
```

#### Route Invoke（按 route_slug）

```bash
curl -sS -X POST "$HTTP_BASE/tenant/integration/routes/media-assets-read/invoke" \
  -H "Authorization: ApiKey $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "idempotency_key":"demo-apikey-route-001",
    "payload":{}
  }' | jq .
```

常见返回解释：

- `401`：API Key 无效/已吊销/缺失
- `403`：API Key 已认证，但权限不足（未授权对应能力或 topic）
- `404`：路径不存在（常见是 `api_prefix` 拼错或访问了错误实例）

---

## 4. 权限配置规则（RBAC 同构）

- API Key 权限改为“勾选 `iam_permission` → 绑定到 `api_key_profile`”。
- 创建/轮换 API Key 时，不再提交权限明细，后端从 Profile 的 `permission_ids` 派生最终 key 权限。
- 页面与接口统一使用 `permission_ids`，逻辑与角色绑定权限一致（只是主体从 `role` 换成 `api_key_profile`）。
- 目录过滤规则：仅展示 `iam_permission.allow_api_key=true` 且可映射为 API Key scope 的权限。

### 4.1 可用接口

- `GET /admin/integration/permissions/catalog`：获取可用于 API Key 的权限目录。
- `GET /admin/integration/api-key-profiles/:profile_id/permissions`：读取 Profile 当前绑定权限。
- `PUT /admin/integration/api-key-profiles/:profile_id/permissions`：覆盖保存 `permission_ids`。

### 4.2 相关数据表（当前实现）

- `iam_api_key_profile`：API Key Profile 主表。
- `iam_api_key_profile_permission`：Profile 与 `iam_permission` 的多对多映射（页面勾选保存到这里）。
- `iam_permission.allow_api_key`：权限是否允许被 API Key Profile 绑定（默认 false）。
- `integration_gateway_api_keys`：API Key 主表（只存 `key_hash`，不存明文）。
- `integration_gateway_api_key_permissions`：Key 创建/轮换时，从 Profile 映射派生出的权限快照。
- `iam_api_key`：鉴权中间件查验 API Key hash 的 IAM 索引表。

说明：
- 你在页面里修改 Profile 权限后，后端会自动同步该 Profile 下所有 active key 的权限快照。
- 因权限变更导致的生效不再要求手工轮换 key（轮换仅用于密钥泄露风险治理）。

### 4.3 内置模板权限（已含组织架构只读）

系统启动/权限目录读取时会自动确保一组内置 API Key 权限模板，当前包含：

- WS / Event 通用 Topic 动作权限（不按具体 topic 拆 permission）：
  - `_scope.ws.topic.publish`
  - `_scope.ws.topic.subscribe`
  - `_scope.event.topic.publish`
  - `_scope.event.topic.subscribe`
  - `_scope.event.topic.replay`
- 组织架构只读权限（用于插件/外部系统读取组织数据）：
  - `GET:/api/v1/admin/organization/departments/tree`
  - `GET:/api/v1/admin/iam/members`
  - `GET:/api/v1/admin/iam/members/:id`

建议做法：
- 在 `api_key_profile` 勾选所需组织只读权限；
- 保存后即可自动同步到该 Profile 下 active key。

### 4.4 默认开放策略（你这次要求的对齐）

- `iam_permission` 现在采用“默认可用于 API Key，少量核心接口回收”的策略。
- 回收（默认不开放）示例：
  - 认证登录链路：`/api/v1/admin/user/auth/*`
  - API Key 自身管理链路：`/api/v1/admin/integration/api-keys*`、`/api/v1/admin/integration/api-key-profiles*`
  - 高敏感权限治理链路：`/api/v1/admin/iam/permissions*`、`/api/v1/admin/iam/roles*`
- 目录接口 `GET /admin/integration/permissions/catalog` 会按 `allow_api_key=true` 返回可勾选权限。

### 4.5 插件权限自动注册（plugin.yaml）

- 插件安装时读取 `plugin.yaml.permissions` 并同步到 `iam_permission`（`source=plugin:<plugin_id>`）。
- 若条目声明 `allow_api_key: true`（或提供 `api_key` 映射），则会进入 API Key 可选目录。
- 推荐插件声明示例：

```yaml
permissions:
  - resource: plugin.template.update
    actions: [publish, subscribe]
    label: 模板更新事件
    module: plugin_runtime
    type: event
    allow_api_key: true
    api_key:
      scope: _scope.event.topic.publish
      action: publish
      resource_type: topic
      resource_pattern: _topic.template.update
```

### 4.6 插件 Event Topic 约束（必须遵守）

- 插件侧 topic 必须在 `plugin.yaml` 声明（建议 `events.topics[]`），禁止仅在代码里硬编码后直接调用。
- 平台侧 `event_topics` 是唯一 topic 真相源；topic 先存在，后授权，最后发布/订阅。
- 创建 topic 不会按 topic 自动新增 permission；API Key 权限使用固定动作级模板（publish/subscribe/replay）。
- `POST /api/v1/admin/event-fabric/topics`：创建/登记 topic（资源创建）。
- `POST /api/v1/internal/ws-bus/grant`：绑定主体对 topic 的动作权限（授权绑定），不创建 topic。
- 插件为 standalone+proxy 模式时，proxy 启动联调前必须先执行 “ensure topic -> grant -> publish/subscribe”。

底座当前对插件事件清单的读取规则（PowerX 侧实现）：

- 读取发生在插件启用阶段（由 PowerX 底座执行播种，不是插件进程自己执行）。
- 在插件安装目录下按顺序扫描：
  - `config/event_fabric.yaml`（推荐）
  - `platform_capabilities/event_fabric.yaml`
  - `event_fabric.yaml`
- 示例安装目录：`backend/plugins/installed/<plugin_id>/<version>/config/event_fabric.yaml`

推荐插件声明结构（示例）：

```yaml
events:
  topics:
    - key: _topic.template.update
      actions: [publish, subscribe]
      description: 模板更新事件
```

---

## 5. 管理 API Key（查询 / 吊销 / 轮换）

### 5.1 列表

```bash
curl -sS "http://127.0.0.1:8077/api/v1/admin/integration/api-keys?page=1&page_size=20" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq .
```

### 5.2 详情

```bash
curl -sS "http://127.0.0.1:8077/api/v1/admin/integration/api-keys/KEY_UUID" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq .
```

### 5.3 吊销

```bash
curl -sS -X POST "http://127.0.0.1:8077/api/v1/admin/integration/api-keys/KEY_UUID/revoke" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{}' | jq .
```

### 5.4 轮换（会返回新的 `plain_key`）

```bash
curl -sS -X POST "http://127.0.0.1:8077/api/v1/admin/integration/api-keys/KEY_UUID/rotate" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name":"ws-debug-key-rotated"
  }' | jq .
```

---

## 6. 故障排查

### 6.1 `401 unauthorized`

- 检查 `Authorization` 头是否为 `ApiKey <plain_key>`（不是 Bearer）
- 检查 key 是否已吊销
- 检查 key 绑定的 `profile_id` 是否还在 `status=1`

### 6.2 `403 api key permission denied`

- 当前 key 缺少对应 `scope + action + resource_pattern`
- `resource_pattern` 不匹配实际 topic（大小写/前缀）

### 6.3 `403 topic not allowed`

- 这是 topic/ACL 层拒绝，不是鉴权层拒绝
- 先确认 topic 已在 `event_topics` 且 ACL 已授予

### 6.4 API Key 缓存策略（已启用）

- 鉴权缓存：`key_hash -> tenant/profile`（TTL 默认 120s）。
- 权限缓存：`key_hash + action + topic -> allow/deny`（TTL 默认 60s）。
- 缓存版本：使用全局版本号键，写操作（创建/吊销/轮换 key、Profile 状态变更、Profile 权限变更）会递增版本，旧缓存立即失效。
- 默认走 `pkg/cache` 驱动（生产建议 Redis）；若缓存不可用则自动回退 DB 直查。

---

## 7. 现阶段页面支持说明

- `token`：页面已有（登录后 localStorage 可取）
- `api_key`：页面路径 `Web Admin -> 系统设置 -> API Key 管理`（路由：`/settings/integration-api-keys`）
- 页面支持：按当前登录租户加载 `api_key_profile`、一键创建默认 profile、启用/停用、重命名、配置 `permission_ids`、创建 key、列表、详情、吊销、轮换
- 页面会展示 `plain_key` 一次，关闭后不再回显；建议立刻复制到安全存储
- 管理台页面用于配置与运维，不建议在浏览器长期存储 API Key。

---

## 8. 一键脚本（推荐）

已提供一键联调脚本：

- `scripts/integration_gateway/apikey_token_playbook.sh`

### 8.1 最小用法（自动登录 + 自动发现 profile_id）

```bash
scripts/integration_gateway/apikey_token_playbook.sh
```

### 8.2 指定 token / tenant_uuid / profile_id

```bash
scripts/integration_gateway/apikey_token_playbook.sh \
  --admin-token "$ADMIN_TOKEN" \
  --tenant-uuid "$TENANT_UUID" \
  --profile-id 1 \
  --topic "_topic.system.notification"
```

脚本会自动完成：

1. 获取/校验 `ADMIN_TOKEN`
2. 查询 `profile_id`（若未显式传入）
3. 为 Profile 绑定最小权限 `permission_ids`
4. 创建 API Key
5. 调用 `ws-bus/grant` + `ws-bus/publish`
6. 验证未授权 topic 的 403 行为

---

## 9. 插件调用链示例（宿主 / Standalone Proxy）

下面两条示例都走 OpenAPI 统一入口，区别只在调用主体与凭证类型。

### 9.1 宿主模式（Plugin in Host）

场景：
- 插件运行在 PowerX 宿主内，依赖当前登录用户会话。
- 插件调用 OpenAPI 能力时，使用 JWT（Bearer Token）。

建议流程：
1. 用户登录宿主后台，获取当前 JWT 会话。
2. 插件通过宿主上下文调用 OpenAPI，携带 `Authorization: Bearer <TOKEN>`。
3. 权限由用户角色（RBAC）与租户边界共同决定。

示例（读取当前用户上下文）：

```bash
curl -sS "http://127.0.0.1:8077/api/v1/admin/auth/me/context" \
  -H "Authorization: Bearer $USER_TOKEN" | jq .
```

### 9.2 Standalone Proxy 模式（Plugin -> Proxy -> PowerX）

场景：
- 插件独立部署，通过 proxy 转发调用 PowerX。
- proxy 持有租户级 API Key，对 PowerX 发起系统调用。

建议流程：
1. 租户管理员在 PowerX 配置该 proxy 的 `api_key_profile + api key`。
2. proxy 向 PowerX 转发请求时统一携带 `Authorization: ApiKey <PROXY_API_KEY>`。
3. 插件本地可继续用自身会话体系，proxy 负责与 PowerX 的鉴权边界。

示例（注册并发布通知 topic）：

```bash
curl -sS -X POST "http://127.0.0.1:8077/api/v1/internal/ws-bus/grant" \
  -H "Authorization: ApiKey $PROXY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "topics":["_topic.system.notification"],
    "actions":["publish","subscribe"]
  }' | jq .
```

```bash
curl -sS -X POST "http://127.0.0.1:8077/api/v1/internal/ws-bus/publish" \
  -H "Authorization: ApiKey $PROXY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "topic":"_topic.system.notification",
    "payload":{"title":"proxy-plugin","content":"from standalone proxy"}
  }' | jq .
```

### 9.3 边界结论（给插件团队）

- 宿主模式与 standalone proxy 都走 OpenAPI 入口，不冲突。
- 宿主模式使用 JWT；standalone proxy 与第三方集成使用 API Key。
- 不建议把 root 的 JWT/Token 下发给插件进程作为长期凭证。
- 不建议把 API Key 存在浏览器 localStorage；应由服务端或 proxy 托管。
