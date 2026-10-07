# Knowledge Lab：Core 策略目录与空间创建合同

适用：Framework Local+Host proxy（开发 API Key）及 Delegated（安装插件 STS）。Framework 需实现 typed catalog/create 并声明支持操作，插件再接表单。本合同不表示浏览器或安装插件验收已经完成。

## 路由及授权

| 操作 | Capability | 固定 binding | Tenant Host REST |
|---|---|---|---|
| 目录 | `com.corex.knowledge.catalog.read` | `INVOKE core://knowledge/catalog` | `GET /api/v1/tenant/knowledge/catalog` |
| 创建 | `com.corex.knowledge.space.create` | `INVOKE core://knowledge/spaces` | `POST /api/v1/tenant/knowledge/spaces` |

Admin 目录：`GET /api/v1/admin/knowledge-spaces/catalog`。Admin 创建沿用 `POST /api/v1/admin/knowledge-spaces` 和既有 camelCase DTO；Framework 使用 service DTO。

显式 Key 权限：`_scope.knowledge.catalog.read` + `read api/catalog`；`_scope.knowledge.space.create` + `create api/space`。同时验证正式发布、租户注册、有效 Key 和 live grant。STS 验证认证上下文和实例 `auth.credentials.allowed_capabilities` 精确 grant。无授权 403；无认证由认证层返回 401。租户和 actor 只取可信凭证。能力发布版本变化后，已有租户版本锁可能需管理员确认升级。

网关使用既有 `/api/v1/tenant/invocations` 包装，以下是 `payload`：

```json
{"method":"INVOKE","endpoint":"core://knowledge/catalog","body":{"operation":"catalog"}}
```

```json
{
  "method":"INVOKE",
  "endpoint":"core://knowledge/spaces",
  "body":{
    "operation":"create",
    "name":"产品知识库",
    "department_uuid":"<当前租户有效部门 UUID>",
    "strategy_key":"A_simple",
    "scene_key":"support_faq",
    "policy_template_uuid":"<目录返回 UUID>"
  }
}
```

REST 创建 body 同上但不带 operation。禁止任意 method/endpoint/query/header/raw proxy，以及 tenant_uuid/actor 字段。

## 目录与创建字段

目录返回 REST `data.catalog` / 网关 `payload.catalog`：

- version、source=powerx_core；scenes 含 key/label/description/default_bundle/allowed_bundles。
- strategy_packages 含 key/label/summary/recommended_profile_key/recommended_scenes/dependencies（index/runtime/assets）。
- profiles 返回当前租户对应 key 最新 published 的 ingestion/index/rag，含 uuid/key/version；缺失则省略。
- available 表示可创建，unavailable_reasons 明确缺少 Profile、embedding、模板或 runtime dependency。未知 runtime dependency 不假定支持。
- activation_dependencies 单列 index/assets；这些依赖可在后续导入和激活生成。可创建不代表索引已就绪。
- policy_templates 是 Core 全局策略模板 uuid/name/version；default_policy_template_uuid 仅在 default/v1 实际存在时返回。
- quota_defaults 为 cpu_cores=4、storage_gb=200、ingestion_concurrency=2；quota_minimums 为 1、50、1。来源为 Core 创建合同默认值和既有最小约束，不是计费额度。quota_override_allowed=true 表示本服务 grant 可设置合法配额。

目录只读，不创建模板/Profile、不补历史 UUID、不用前端默认目录掩盖失败。

创建必填 name（非空，最多 128 bytes）、department_uuid、strategy_key。可选：

- scene_key 省略取策略第一个推荐场景，显式值必须属于 recommended_scenes。
- policy_template_uuid 省略选择实际 default/v1；无默认时必须显式选择。
- ingestion_profile_uuid/index_profile_uuid/rag_profile_uuid 省略取目录映射；显式值必须匹配最新 published 映射，否则刷新目录。
- quotas 整体省略取默认，传入时三项均须满足最小值。

部门必须属于凭证租户且 active；跨租户/失效部门 403。事务内再次验证 Department/Profile。内部 code 保留兼容快照。策略 key、场景和 Profile key 不可混用。

沿用现有 CreateSpace 事务创建空间、IAM 任务、审计，任一步失败全部回滚；重复同租户名称 409。返回 REST `data.item` / 网关 `payload.item` 包含 space_uuid/name/status/department_uuid/strategy_key/scene_key/policy_template_uuid/profiles/quotas。status 初始为 pending_iam；后续沿用 IAM 同步及索引激活，再用现有目录复读。

Profile UUID 持久化为创建时快照；运行时仍遵循既有 Profile key 解析，本轮不改变为全生命周期版本锁定。

## 错误

400：非法字段/UUID/策略/场景/模板/Profile/配额或自由代理 payload。
403：无 grant、跨租户或不可用部门。
409 KNOWLEDGE_SPACE_CONFLICT：重复名称。
412 KNOWLEDGE_STRATEGY_UNAVAILABLE：目录列出的创建依赖未满足。
503：目录、存储或 UUID 迁移依赖不具备。

Framework 保留原状态、machine code 和 Core Trace ID，不能转换为 unsupported capability。

## 部署及验收

**需要标准 Core migrate**：新增策略模板公开 UUID、空间 Department/Profile UUID nullable 快照。集中迁移幂等补齐历史模板 UUID，保留 numeric 主键和历史引用。迁移前按现有流程备份。回滚应用可保留新增列，不重新生成 UUID。

1. Core migrate → make capability-check → make capability-seed。
2. 开发 Key 两项精确 grant；开发 seed 已包含 scope，但 capability-seed 本身不授予 Key。
3. STS 安装实例声明并授权两项能力，保留现有空间目录 grant。
4. 重启 Core，真实 grant-status 检查，需要时管理员确认能力版本升级。
5. API Key/STS 分别 catalog → 真实部门和 available 策略 → create → 目录复读，记录 Trace ID。
6. 验证无 grant/跨租户 403、非法 Profile 400、重复名称 409；失败无空间/IAM/audit 残留。
7. Framework OperationCatalog + typed create 接入后，插件解除禁用并完成浏览器验收。

代码：host_provisioning.go、provisioning_capability_invoker.go、Tenant knowledge_space/routes.go、platform_capabilities/knowledge.yaml。OpenAPI 见 specs/011-knowledge-space/contracts/host-provisioning.openapi.yaml。

## 本轮验证状态（2026-10-04）

相关 8 个 Go 包测试通过，覆盖两种可信凭证上下文的 typed catalog/create/复读、精确 grant、跨租户拒绝、非法字段/自由 endpoint 拒绝、事务审计故障回滚、迁移幂等和已注册 HTTP 路由。make capability-check 及 capability-seed 成功；seed 只发布能力和权限元数据，不签发或轮换运行凭证。

真实开发 Host Key grant-status HTTP 200，两项当前均为 not_granted，request_id：6f45d092-2d9b-4784-8076-209f883afdf9。尚未执行新增 schema 的全库 migrate、重启用户 Core 或授予现有 Key/安装实例的新 grant。真实 API Key/STS 创建及浏览器验收仍待完成，测试中的可信上下文不能视为真实 STS Exchange 验收。

### 实际迁移及目录复验（2026-10-04）

按用户授权，使用 8077 进程的 POWERX_CONFIG 执行标准 `cmd/database migrate`，退出码 0。数据库为 localhost:5432/powerx；策略模板 1 条，空或零值 UUID 为 0。
配置仍含公开默认 JWT 值，迁移进程单独使用临时随机环境值通过校验；没有修改配置文件或运行服务的签名密钥。
真实开发 API Key 调用 `GET /api/v1/tenant/knowledge/catalog` 返回 HTTP 200，source=powerx_core，策略包 18 项、模板 1 项。X-Trace-ID：cf7f1e4c-623e-470f-9e36-7ade6978596c；request_id：b5c72d0c-9855-411b-acf4-9b42b1c022fb。
本次没有重复授权或重启 Core；上述记录取代前文“尚未 migrate”的状态。STS 创建和插件浏览器创建验收仍未在本次执行。
