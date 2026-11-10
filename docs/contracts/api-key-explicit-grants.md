# API Key 权限配置

Profile 是日常统一权限入口，可配置 Core 与插件服务能力。按 Key 追加权限只用于 Profile 之外的例外场景。两种保存路径都不将服务提供方加入消费方的资源 owner 列表。正式授权源是已激活的 IAM Permission；客户端只能提交 Permission ID，不能提交任意 scope、通配资源或插件身份。

## 管理接口

以下接口均位于 `/api/v1/admin/integration/api-keys/:key_id`，要求当前租户的管理员用户 JWT。API Key、STS、普通成员和其他租户不能修改授权。

| 方法 | 相对路径 | 行为 |
| --- | --- | --- |
| GET | `/explicit-grants` | 查询该 Key 的独立授权声明 |
| POST | `/explicit-grants` | 幂等追加 `{"permission_ids":[123]}` |
| DELETE | `/explicit-grants/:permission_id` | 幂等撤销该项独立授权 |

写入前必须验证：Key 与 Profile 有效；Permission 激活且 `allow_api_key=true`；元数据具有 `api_key_explicit=true`；完整插件归属与 capability ID、scope/action/resource/effect 均合法；能力已发布；该租户最新 registration 为 published。入口支持插件显式服务声明，以及 Core 正式目录中的固定 `core_internal` 服务声明，不从 Admin 权限推导服务授权。

Core 独立授权要求 IAM 来源为 `platform_capability`、类型为 `capability_service`、capability 归属 `corex.platform`，并与发布记录中的 `service_actor + tenant`、`INVOKE core://` 绑定一致。Core grant 的 `plugin_id` 为空；这不放宽插件 grant 必须匹配插件 owner 的规则。Admin REST、推导权限、自由 HTTP endpoint、通配资源或未发布／未注册的声明均不能从此接口授权。

`GET` 返回持久声明。实际生效状态仍由该 Key 调用 `/api/v1/tenant/capabilities:grant-status` 判断，不能以管理员 JWT 或权限行存在代替验收。

## 持久化与同步

复用现有 Key 的 `plugin_owner_policy` JSON 列，不新增表或运行时迁移。有独立授权时使用 version 2，并在 `explicit_grants` 中保存规范 Permission ID、capability ID 和精确权限元组。版本 1 继续表达现有 owner 策略；旧 Core 遇到版本 2 必须拒绝策略变更，不能静默丢弃独立授权。

独立授权与 Profile 权限分开维护。单 Key 追加／撤销保留其他现有有效权限，不顺带应用尚未同步的 Profile 变更。显式修改 Profile、清空或恢复 Profile、变更 owner、轮换 Key 时，独立授权都必须保留。轮换仍按原合同使旧 Key 失效；管理独立授权本身不会轮换 Key。

撤销仅移除独立授权。如果同一权限同时由 Profile 提供，该 Profile 授权仍有效。能力下架、租户 registration 停用或 IAM 禁止 API Key 后，运行时鉴权必须拒绝调用，即使持久声明仍然存在。

插件网关按当前请求的协议绑定校验服务授权：绑定必须是 `service_actor + tenant`，插件归属、精确 grant、已发布能力和该租户最新 registration 必须同时有效。API Key 的 Profile ID 不参与用户成员 RBAC；即使后台接口使用相同业务权限码，服务 Key 也不能借此访问 `admin_user` 绑定。通过后签发带原租户、`api_key` 平台和规范权限码的插件短期令牌，不为服务身份创建用户会话票据。

租户 registration 中的 REST adapter 必须使用实际插件网关路径（包含已安装插件的 HTTP base），并保持 `is_active=true`。目录坐标不等于网络入口。管理注册接口显式接收并回传 `is_active`；新 adapter 默认启用，更新已有 adapter 时省略该字段会保留原状态，停用或重新启用必须明确传入布尔值。

策略、有效权限行和审计在同一事务中写入，失败时整体回滚。提交后统一失效权限缓存。审计包含操作者、变更前后授权和 trace ID，不包含 Key 明文。

## 验证

- Service：只变更目标 Key；批量失败不产生部分授权；只接受显式、已发布且已注册的服务权限；拒绝普通成员、STS、API Key 和跨租户操作。
- 同步：Profile 清空／恢复、owner 变更、Key 轮换后保留独立授权；单 Key 修改不增加无关 Profile 权限。
- HTTP：正式管理路径的追加、查询、撤销、严格请求校验、跨租户拒绝、服务身份拒绝及轮换回读。
- 运行时：用实际 Key 获取 `granted`，再按业务契约验证请求；不以管理员调用代替服务验收。
- 网关：真实 API Key 中间件、Core IAM authorizer 和 HTTP 反向代理联合验收；覆盖独立授权、撤权、提供方与租户隔离、下架、禁用和 Admin 接口拒绝。

## 日常使用：Profile 统一配置

设置 → API Key 管理 → 选择 Profile → 权限配置。直接勾选 Core 与插件服务能力，点击「保存权限」；「全模块全选」包括符合 API Key 声明的插件服务能力，无需在两套权限配置之间切换。该 Profile 下启用的 Key 在同一事务中获得对应精确授权。

插件服务权限的 `plugin_id` 表示服务提供方；Key 的插件 owner 表示它可操作的插件资源。二者分别处理。Profile 为 WorkspaceX Key 勾选电商套餐读取后，其有效服务 grant 的 provider 为 Ecommerce，Key 的资源 owner 仍为 WorkspaceX，不获得 Ecommerce 调度、身份或其他资源管理权限。

后台复用显式服务权限校验：完整 IAM 声明、精确 provider、capability 发布、当前租户 registration 均需有效。校验失败保留原 Profile 与所有 Key 的授权，反馈具体能力及错误码；不能通过默认 AllowAPIKey、空 provider 或管理员权限绕过。普通资源权限继续使用原有 owner 匹配，不放宽旧细分绑定。

创建 Key、保存／追加 Profile 权限、修改 owner 和轮换均使用同一套绑定流程。取消 Profile 中的服务能力并保存后，撤销其继承的授权；不把 Profile 授权复制成持久的 Key 追加授权。其他 Profile 的 Key 不受影响。

## 高级场景：为指定 Key 追加权限

只有需要在 Profile 之外追加例外权限时，才使用「API Key 管理 → 高级操作 → 按 Key 追加权限」。该入口保留已有 API／持久授权的维护能力，不属于普通 Profile 配置的必经步骤。

1. 选择当前 Profile 列表中的目标 Key（最多 50 个）。
2. 选择完整显式声明的服务能力。
3. 预览核对 Key、能力、新增与已有条数；预览不写入。
4. 确认后整批事务保存。Profile、其他 Key 与原 owner 均保留；失败全部回滚。

接口 `POST /api/v1/admin/integration/api-key-grants/batch` 的 preview/hash/commit 合同不变。请求中断时显示结果未确认，重新预览后再决定是否提交，不自动重试。

已有的 Key 追加授权继续保留；若某项权限同时来自 Profile 和 Key 追加授权，取消 Profile 的勾选仅撤销 Profile 来源，仍需通过上文的 `DELETE /explicit-grants/:permission_id` 管理接口撤销该例外授权。真实 service Key 的 grant-status 和业务调用仍需另外验证，配置保存成功不代替调用验收。
