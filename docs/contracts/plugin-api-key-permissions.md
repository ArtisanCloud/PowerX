# 插件 API Key 权限合同 v1

本合同用于插件发布的独立服务 API 权限。它把权限声明、IAM 定义和实际 Key 授权关联起来，不自动授予任何 Key，也不替代 capability 发布和租户 registration。

## 声明与授权边界

```yaml
permissions:
  - type: api
    permission_code: commerce.plans_service:read
    module: commerce
    resource: plans_service
    action: read
    independent: true
    title_i18n: {zh-CN: 读取套餐, en: Read plans}
    description_i18n: {zh-CN: 读取本租户套餐, en: Read plans in the current tenant}
    risk_level: low
    data_scope: tenant
    protocol_bindings:
      - channel: rest
        method: POST
        path: /plugin/plans/query
        actor_context: service_actor
        resource_scope: tenant
    api_key:
      capability_id: com.example.plans.read
      plugin_id: com.example.plans
      scope: _scope.plans.read
      action: read
      resource_type: capability
      resource_pattern: plans
      effect: allow
```

`api_key` 必须完整，`plugin_id` 必须等于发布插件 ID，`capability_id` 必须属于该插件且与所在 catalog capability 一致。`action` 必须与 permission code 一致；scope 使用 `_scope.` 前缀，不允许空值、通配符或空白字符。

API Key 权限必须为独立的 `type: api` 声明，不能复用 `business_permission_code` 指向的 Admin 用户权限。这里的“独立”指服务权限与 Admin 用户权限分别声明，不限制管理员必须按单个 Key 授权；同一正式服务权限可以由 Profile 统一勾选并授予其下的 Key。该权限的全部 bindings 必须属于服务身份。v1 插件合同目前仅支持 `resource_scope: tenant`；Core 已有的 `customer_self` 等范围不受此插件合同限制，也不能直接由这个合同推导支持。

同一 capability 可以保留独立的 Admin 权限声明及绑定。限制作用于显式 API Key 权限，不要求整个 capability 的所有 bindings 都是 `service_actor`。

校验通过才生成 `AllowAPIKey=true`、`api_key_explicit=true` 和对应 capability/API Key 元数据。未声明的权限默认不能授予 API Key；错误的规范声明不得回退到旧 resource/actions 导入。

## 运行时授权

对插件 capability，`grant-status` 和实际 invocation 共用当前凭据的权限结果，要求：

1. IAM definition 来源为 `plugin:<owner>`，声明的 `api_key.plugin_id` 和 capability namespace 与 owner 一致。
2. Key 在可信租户中有效。
3. Key 实际权限行的 `plugin_id` 精确等于 owner，且 scope、action、resource type/resource 匹配。
4. capability 已发布、已为当前租户登记，当前 Key 拥有相应 grant。

空或通配 `plugin_id` 不授权任何插件能力。仅为插件 A 授权的权限行不能用于插件 B，即使其他匹配字段相同。移除 grant、撤销 Key 或关闭 IAM definition 的 `AllowAPIKey` 后，后续检查立即拒绝，不从 Profile 或名称推测授权。

Core 的 `com.corex.*` 能力保持既有四元组能力授权，具体可操作插件 owner 继续由对应 Core 服务检查。不能把 Core 资源 owner 与插件 capability 的归属合并成一次通用校验。相关规则见 [API Key 插件 owner](api-key-plugin-owners.md)。

## 同步优先级与冲突

- 先检查整个插件 catalog 的显式 API Key 权限归属，再发布 capability，冲突返回 `PLUGIN_API_KEY_PERMISSION_CONFLICT`。
- 同一个 `(module, resource, action)` 上，推导引用不覆盖已有显式声明、API Key 元数据或其默认角色授权。正反同步顺序和重复同步的结果必须一致。
- 独立 API Key 权限只能有一个 capability 声明 owner；另一个 capability 可以引用相同业务 permission，但不能再定义冲突的显式授权。
- 安装／启动的 manifest 同步和 catalog 同步使用同一个受保护的写入方法。唯一键和事务行锁保护并发创建／更新，批次发生冲突时回滚该批权限写入。
- 原 owner capability 显式移除 `api_key` 时允许同步为 `AllowAPIKey=false`；保护优先级不能阻止正常撤销。

## 兼容性与上线

这是跨插件的权限合同收紧。旧的 `allow_api_key: true`、不完整的 `api_key` 或依赖默认开启的 resource/actions 权限，需要由插件维护者明确迁移，不能自动保留旧授权。

发布前：

1. 收集已安装插件及其 capability descriptor，检查显式声明是否完整；同一 API Key 权限是否被多个 capability 定义。
2. 核对具体 Key 的现有权限行。插件 grant 的空／错误 `plugin_id` 必须经授权关系确认后，通过管理授权流程调整；不从 Key 名称、Profile 名称或首次访问推导 owner。
3. 完成受影响插件声明和 Key 授权清单，在同一发布窗口更新 Core 与相应插件。重新同步会收紧默认权限，不能只以构建通过作为运行时验收。
4. 通过 `make capability-check` 及相关测试。此次实现未修改平台 capability YAML 或数据库表结构；不需要为此执行数据库迁移。平台正式声明另有变化时仍按其合同执行 seed。
5. 在新 Core 进程上验证 development Host Key 的 `grant-status`、实际调用、未授权／跨插件／跨租户拒绝与撤销。不把本地单测当作已部署验收。

可先用只读 SQL 筛查不完整的插件 IAM 定义：

```sql
SELECT id, source, module, resource, action,
       meta->>'capability_id' AS capability_id,
       meta->'api_key'->>'plugin_id' AS declared_owner
FROM public.iam_permission
WHERE source LIKE 'plugin:%'
  AND allow_api_key = true
  AND (meta->>'api_key_explicit' IS DISTINCT FROM 'true'
       OR meta->'api_key'->>'plugin_id' IS DISTINCT FROM substring(source FROM 8));
```

该查询用于发现待核对项，不修改 IAM、Key 或实际授权。原始插件声明、Key grant 和运行时结果仍须分别核对。

## 验证覆盖

回归包括精确插件归属、Core 原授权语义、跨租户、错 action、撤销、双向／重复同步、显式移除声明、catalog 全局冲突、事务回滚、安装同步的冲突保护、默认角色不被推导项扩大，以及同一 capability 的独立 Admin／service 权限。
