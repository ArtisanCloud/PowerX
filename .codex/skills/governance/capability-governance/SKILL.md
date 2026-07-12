---
name: capability-governance
description: PowerX 底座 Capability 治理与发布准入规则。用于审计 REST/OpenAPI/gRPC/Gin 生成的能力候选、正式 platform_capabilities 目录、Capability Registry 登记、agent_usable/permission_code/risk_level 元数据、ignore 清单和缺失能力；当用户要求“重新识别能力”“能力是否能发布”“补齐能力登记”“检查 capability-gen/audit/check”“底座接口暴露给插件/agent”时使用。
---

# PowerX Capability Governance

## 目标

按统一规则判断 PowerX 底座能力候选是否可以进入 `backend/config/platform_capabilities/*.yaml`，并区分：

- 可发布为租户能力目录中的正式能力。
- 可登记但不能作为 agent 可选能力。
- 已由手写业务聚合能力覆盖，不应重复发布 raw route。
- 不应发布，只能进入带原因的 ignore。
- 缺真实实现，不能靠 YAML 声明补齐。
- 生成器或审计器误判，需要修工具。

正式 YAML 不是文档。进入 `backend/config/platform_capabilities` 后会被 `BaseCapabilitySeeder` 写入 `CapabilityRecord` 并给 active tenants 建 registration，因此必须按运行时授权事实处理。

## 使用方式

在对话里直接点名 skill。完整审计使用：

```text
用 $capability-governance 做一次完整能力治理审计
```

完整补齐使用：

```text
用 $capability-governance 做一次完整能力治理补齐
```

执行“完整能力治理补齐”时，不只解释结果，必须按下面顺序实际处理：

1. 读取本 skill 的“必读文件”。
2. 运行 `make capability-gen`，确保 `backend/config/platform_capabilities/generated.auto.yaml` 被重新生成。
3. 检查 `generated.auto.yaml` 是否作为正式 platform capability 文件存在，并包含所有生成能力的治理字段：
   - `permission_code`
   - `agent_usable`
   - `risk_level`
   - REST binding 的 `actor_context`
   - REST binding 的 `resource_scope`
   - `sts_direct` 只能显式为 true，不得默认打开
4. 检查手写聚合能力 YAML 是否同样具备 `permission_code / agent_usable / risk_level`。
5. 如果审计插件 capability descriptor，检查面向普通成员的能力是否显式声明 `default_role_grants: [role_user]`，并确认未使用非法默认角色码。
6. 运行 `make capability-check`。如果失败，先判断是：
   - 缺真实 transport/service：先补实现，再补 YAML。
   - capability_gen/audit 误判：先修工具，例如 Gin `Group("")` 丢父路径。
   - 正式能力缺元数据：补 YAML 或生成器。
   - 路由不应发布：写入带 `category/reason` 的 ignore，不允许静默忽略。
7. 修完后重新运行 `make capability-gen` 和 `make capability-check`，直到通过。
8. 需要验证运行时 Registry 时，运行 `make capability-seed`，把正式 `platform_capabilities/*.yaml` 写入 `capability_registry_records` 并为 active tenants 补齐 registrations。
9. 如果改了 Go 工具或 STS 规则，运行对应 Go 测试。

也可以按问题缩小范围：

```text
用 $capability-governance 检查 make capability-check 的失败原因
用 $capability-governance 判断这些候选能力哪些可以进 platform_capabilities
用 $capability-governance 分析 com.corex.customer.accounts.* 需要补哪些实现
```

## 必读文件

先读取这些位置，再给结论或改代码：

- `make_files/capability.mk`
- `make_files/dev.mk`
- `backend/cmd/capability_gen/main.go`
- `backend/cmd/capability_audit/main.go`
- `backend/config/platform_capabilities/*.yaml`
- `backend/config/capability_audit_required.yaml`
- `backend/internal/service/integration_gateway/base_capabilities.go`
- `backend/internal/service/integration_gateway/platform_capability_config.go`
- `backend/internal/http/auth_subject_validator.go`
- `backend/internal/http/auth_subject_validator_test.go`
- 相关 HTTP/gRPC/OpenAPI transport 源文件。

如果涉及 agent 能力授权，还要读取：

- `backend/internal/service/agent_authz/service.go`
- `backend/internal/service/agent/agent_service.go`
- `web-admin/app/pages/settings/ai/agents.vue`

## 基础命令

从仓库根目录运行：

```bash
make capability-gen
make capability-audit
make capability-check
make capability-seed
CAPABILITY_AUDIT_FIX=1 make capability-audit
```

解释命令结果时必须区分：

- `capability-gen`：从 OpenAPI/gRPC/Gin 生成底座 raw 能力，默认写入 `backend/config/platform_capabilities/generated.auto.yaml`。该文件是正式登记的一部分，但生成能力默认必须是 `agent_usable: false`，不能当成手写业务聚合能力。
- `capability-audit`：检查正式目录、required 清单和路由覆盖。
- `capability-check`：先临时生成候选，再审计正式目录覆盖关系，适合判断“现有接口是否已被正式声明或明确忽略”。
- `capability-seed`：只同步正式 platform capability YAML 到运行库，不执行 migrate，不执行全量 seed。
- `CAPABILITY_AUDIT_FIX=1`：只生成草稿，不得直接把草稿视为可发布。

当前 Make 约定：

- `make capability-gen` 默认输出：`backend/config/platform_capabilities/generated.auto.yaml`
- `make capability-check` 临时输出：`tmp/capability-check/generated.platform-capabilities.yaml`
- `make capability-check` 通过时应输出类似：

```text
capability-audit: ok, declared=<正式能力数> referenced=<引用数> rest_routes=<REST覆盖数> candidates=<临时候选数> ignored_route_rules=<忽略规则数>
```

如果 `declared` 只有几十个，而 `candidates` 有几百个，说明 raw 能力没有完整进入正式目录，不能视为完整。

## 发布准入

能力进入正式 `platform_capabilities` 前必须满足：

- 有真实 HTTP/gRPC/OpenAPI transport，不能只有 model、migration、文档或调用方引用。
- 有稳定业务语义，不是临时调试、安装、迁移、root 支持或内部运维入口。
- `capability_id` 稳定且属于 `com.corex.*`。
- `permission_code` 明确，不能依赖不可读的粗粒度推导。
- `agent_usable` 明确设置。raw route 和后台管理能力默认应为 `false`。
- `risk_level` 明确设置。
- 插件 capability 同步 IAM permission 后，Core 默认授予 `role_owner` 和 `role_admin`；如需普通成员使用，descriptor 必须显式声明 `default_role_grants: [role_user]`，不得靠手工补 `iam_role_permission`。
- `module`、`categories`、`intents`、`tool_scopes` 与业务语义一致。
- `protocols` 指向真实路径/RPC，路径参数风格要与项目规范一致。
- 用户可见标题和描述要支持 locale/i18n 元数据；技术 ID 不能作为主要展示文本。
- 需要插件或 agent 通过 STS/API key 调用时，STS direct 自动派生、blocklist、tenant registration、permission code、调用路径要一起验证。

### generated.auto.yaml 准入

`backend/config/platform_capabilities/generated.auto.yaml` 是底座 raw 能力登记文件，必须被当作正式目录的一部分读取和审计，但它与手写聚合能力语义不同：

- 必须由 `make capability-gen` 生成，不手工大段编辑。
- 每条能力必须包含 `permission_code`、`agent_usable`、`risk_level`。
- 生成能力默认 `agent_usable: false`，避免 raw route 出现在智能体能力选购页面。
- REST protocol 必须包含 `actor_context` 和 `resource_scope`。
- REST protocol 不得默认 `sts_direct: true`。
- 如果 generator 丢路径、丢 `/admin`、丢 `/tenant`、错误处理 `Group("")`，必须先修 generator，而不是手写补假能力。

## 能力与端点关系

Capability 是业务授权单元，不是 URL。REST/OpenAPI/Admin/gRPC endpoint 只是 capability 的 protocol binding。

规则：

- 同一业务语义、同一授权边界的多个入口 MUST 复用同一个 `capability_id`。
- `/api/v1/admin/<resource>` 与 `/api/v1/<resource>` 如果只是用户态后台入口和服务态开放入口的差异，应登记为同一个 capability 的不同 REST bindings。
- 不得因为路径前缀不同，把同一能力拆成多个 raw route capability。
- 只有当可操作资源范围、actor 约束、风险等级或授权开关必须独立时，才拆 capability。例如 admin 全量治理能力和插件 owner-scoped 自助能力可以拆开。
- 对 scheduler 这类能力，`/api/v1/admin/scheduler/jobs`、未来的 `/api/v1/scheduler/jobs`、`powerx.scheduler.v1.SchedulerService` 如果语义都是 Runtime Scheduler jobs，应优先归到 `com.corex.scheduler.jobs`；差异通过 binding metadata、service 层 actor/owner 校验和 tool_scope 表达。

### Actor 与资源边界

审计 capability 时必须先确认调用主体：

- `admin_user`：PowerX Admin 或插件 Admin 页面，使用用户 JWT、tenant member、RBAC。典型路径 `/api/v1/admin/*`。
- `service_actor`：插件后端、agent、skill、系统集成，使用 STS/API Key/OAuth client。典型路径 `/api/v1/tenant/invocations` 或服务态开放 REST。
- `web_user`：租户侧 web 应用用户，使用用户 JWT 或业务 session。
- `mini_app_user`：小程序用户，使用小程序会话、用户 JWT 或 customer/user token。
- `customer_actor`：客户门户或外部客户身份，使用 customer token 或 customer-scoped OAuth/API Key。

规则：

- web、mini-app、customer 入口不是后台管理入口，不得复用 admin 全量治理语义。
- customer/mini-app 自助能力默认必须 owner-scoped/self-scoped，例如 `com.corex.customer.account.self_read`，不能继承 `com.corex.customer.accounts.admin_manage`。
- 如果同一资源在不同 actor 下资源范围不同，应拆 capability；例如 admin 管理全租户 customer accounts，与 customer 只能查看自己的 account，是两个授权单元。
- 如果 actor 不同但资源范围、操作风险和授权开关完全一致，可以共享 capability，并在 protocol binding metadata 中标明 `actor_context`。
- capability 命名建议用边界后缀表达语义：`admin_manage`、`service_manage`、`self_read`、`self_update`、`owner_manage`。

## STS direct 访问规则

插件 STS token 直接访问 PowerX Core HTTP 的允许集合按下面公式计算：

```text
STS direct route policy =
  static plugin runtime contracts
  + REST endpoints in formal platform_capabilities/*.yaml
  - STS blocklist
```

规则：

- `/tenant/invocations` 和 `/tenant/invocations/stream` 是插件调用底座能力的推荐主路径。
- Direct REST 是辅助路径，只能来自正式 `platform_capabilities/*.yaml` 中的 REST protocol endpoints；`generated.auto.yaml` 当前也参与自动派生，但必须经过 blocklist。
- 自动开放必须精确到 HTTP method，不允许因同一路径开放 `GET` 就放开写操作。
- `/api/v1/admin/*` 是后台用户态 API 命名空间，不等于“禁止插件后台页面使用”。浏览器中的 PowerX Admin、插件 Admin 页面、以及任何携带用户 JWT 的后台请求，仍然按用户鉴权、租户成员、RBAC 和业务权限判断；STS direct route policy 不得影响用户态 JWT。
- STS token 是插件服务态身份，不携带 `uid/mid`，不能代表登录用户通过 `/api/v1/admin/*` 绕过用户 RBAC。插件后端如果要代表当前用户调用底座后台 API，必须引入明确的 delegated/on-behalf-of 机制，不能复用普通 `powerx:api` STS token。
- blocklist 只约束插件服务态 STS direct call，必须拦截 `/admin/*`、`/internal/*`、`/public/*`、`/auth/*`、`/setup/*`、debug、migration、root、drain、bootstrap、mock、health、根级动态路径等非服务态开放入口。
- `/admin/*` 默认不允许服务态 STS direct call。只有确认为插件服务运行时合同的少量入口，才允许进入 static allow，并必须补用途说明和 `auth_subject_validator` 测试。
- 新增开放接口不得只改 STS validator。正确顺序是：实现真实 transport/service/permission/test，登记正式 platform capability REST protocol，运行 `make capability-check`，再验证 STS direct policy。
- 后台业务能力如果需要给插件服务态调用，不要把 `/api/v1/admin/*` 改成 `sts_direct: true`。应保留 `admin_user/user_jwt` binding，并新增 `service_actor/sts` 的 `core_internal` binding 或服务态开放 REST，由 `/tenant/invocations` 在校验 Registry 与 tenant registration 后调 Core service。例如 `com.corex.customer.accounts.admin_manage` 使用 `/api/v1/admin/customers/*` 作为 payload endpoint 选择参数，但实际由 Core 内部 customer service 执行。

## 分类规则

### 可以正式发布

满足发布准入，且插件/agent/skill 有复用价值的底座业务能力，例如 media、knowledge、workflow、scheduler、event fabric、agent runtime、AI 模型调用等。

优先使用手写业务聚合能力，而不是每个 raw route 一个 capability。聚合能力可以包含多个 REST/gRPC protocol binding。

### 可登记但 agent 不可选

后台治理、观测、诊断、插件运行时辅助接口，如果确实需要被租户或插件调用，可以登记，但必须：

- `agent_usable: false`
- 权限码、风险等级和文档明确。
- UI 不把它当成 agent 能力选购项。

### 已被聚合能力覆盖

如果 raw route 已经被手写聚合能力覆盖，不要再发布重复 capability。应把 raw route 覆盖关系体现在聚合能力的 `protocols` 中，或在审计 ignore 中说明“covered_by: <capability_id>”。

### 不应发布

以下类型不得进入租户能力注册目录：

- `/internal/**` 或 `/api/v1/internal/**`
- `/api/v1/admin/root/**`
- migration/fix/bootstrap/debug/test-only 路由
- public signup/auth/icon 等公共入口
- plugin drain、tenant instance enable/disable、registry sync、安装任务等运维控制接口
- runtime task-queue、ws-bus grant/publish 等底层运行时控制接口，除非已经被明确设计为插件运行时合同并标记非 agent 可选
- `_test.go` 中的测试路由
- capability_gen 误解析出来的假路径

这些需要进入 `capability_audit_ignore.yaml` 时，每条必须有 `category` 和 `reason`，不能静默忽略。

这些类型即使被 OpenAPI/Gin/gRPC 生成器识别为候选，也不得通过服务态 STS direct 自动开放；如果确实属于插件服务运行时合同，应从 capability 发布判断中剥离，进入静态合同入口并单独测试。用户态后台页面访问 `/api/v1/admin/*` 不受这条 STS blocklist 影响，仍由用户 JWT 和 RBAC 判定。

### 缺实现

如果只有 `required_capabilities`、插件调用方或文档提到某 capability，但底座没有真实 transport，不得补 YAML 伪造能力。必须先实现 API/service/repository/transport/权限/测试，然后再登记 capability。

典型判断：`com.corex.customer.accounts.*` 如果没有真实 customer admin HTTP/gRPC transport，就属于缺实现，不能仅靠 capability YAML 解决。

## 生成器与审计器规则

发现以下情况时先修工具，不要发布候选：

- Gin group 解析丢父路径，例如 `group := adminGroup.Group("")` 后路由被生成到 `/api/v1/catalog` 而不是 `/api/v1/admin/skills/catalog`。
- 扫描 `_test.go` 产生候选。
- 生成路径缺 `/admin`、`/tenant`、`/internal` 等实际前缀。
- OpenAPI、gRPC、Gin 对同一路由生成重复或冲突 capability ID。
- `permission_code`、`agent_usable`、`risk_level` 缺省导致 UI 或授权过宽。

修工具后重新运行 `make capability-check`，用新结果重新分类。

审计器必须强制：

- 正式 YAML 中每个 capability 必须有 `permission_code`。
- 正式 YAML 中每个 capability 必须显式设置 `agent_usable`。
- 正式 YAML 中每个 capability 必须有合法 `risk_level`：`low`、`medium`、`high`、`critical`。
- 正式 YAML 中每个 REST binding 必须有 `actor_context` 和 `resource_scope`。
- `sts_direct: true` 只能用于 `actor_context: service_actor`。
- `sts_direct: true` 不得指向 `/api/v1/admin/*`。
- `generated.auto.yaml` 也必须满足上述元数据规则。

完整补齐后应能同时满足：

- `backend/config/platform_capabilities/generated.auto.yaml` 中有 700+ raw 能力。
- 手写聚合能力 YAML 中有业务语义稳定的高层能力。
- `make capability-check` 通过。
- BaseCapabilitySeeder 读取正式目录后会登记 raw 能力与手写聚合能力。

## 输出格式

给用户结论时按这个顺序：

1. 当前事实：正式能力数、候选能力数、差异数、命令结果。
2. 可以发布：列模块、代表能力、原因、需要写入哪个 YAML。
3. 不发布：列分类、代表路径、原因、是否进入 ignore。
4. 聚合覆盖：列 raw route 和覆盖它的聚合 capability。
5. 缺实现：列 capability_id、缺哪个 transport/service/permission/test。
6. 工具问题：列 capability_gen/audit/check 需要修的点。
7. STS direct 影响：列自动开放数量、被 blocklist 拦截的代表路径、是否需要静态合同入口。
8. 下一步改动：代码、配置、文档、验证命令。

不要只回答“可以”或“不可以”。必须说明运行时影响和授权边界。

## 修改约束

- 不做兼容 fallback；错误格式或缺字段要 fail fast。
- 不把草稿 YAML 直接搬进正式目录。
- 不把 root/internal/debug/migration/test 路由发布为租户能力。
- 不通过手工改 STS validator 代替正式 capability 登记；普通开放 REST 能力必须从 platform capability 自动派生。
- 不用 UUID 或 raw technical ID 做用户主展示文本。
- 业务对象关联使用 UUID；中间表可以没有 UUID，但外键映射业务对象 UUID。
- 后端启动和 migrate 分离，不把 AutoMigrate 当成运行时启动副作用。
- 新增用户可见文案必须走 i18n/locale。
- 插件 capability 的 `default_role_grants` 只能使用 `role_owner`、`role_admin`、`role_user`、`role_readonly`、`role_vendor`；非法角色码必须 fail fast。

## 验证

完成治理或代码修改后至少运行：

```bash
make capability-check
```

如果改了 Go 代码，按影响范围运行对应测试；如果新增/修改生成器或审计器，至少运行：

```bash
cd backend && go test ./cmd/capability_audit
cd backend && go test ./cmd/capability_gen
```

如果改了 STS direct 规则或 platform capability REST endpoints，还要运行：

```bash
cd backend && go test ./internal/http
```

如果对应包没有测试，也要明确说明无法运行或当前没有测试包。
