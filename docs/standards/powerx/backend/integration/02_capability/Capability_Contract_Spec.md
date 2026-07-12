# Capability Contract Specification

> 本规范定义 PowerX Integration Framework 的统一 **Capability Contract（能力契约模型）**，
> 用于描述所有插件、第三方平台与 PowerX 内核之间可调用的能力及其元信息。
> 它实现 **“声明业务能力 → PowerX 自动编排与执行”** 的核心闭环。

---

## 1️⃣ 设计目标

| 目标        | 描述                                                                |
| --------- | ----------------------------------------------------------------- |
| **统一抽象**  | 所有业务功能以 `Capability Contract` 声明，统一在 Registry 管理。                 |
| **协议无关**  | 插件仅声明逻辑能力，不关心传输协议；Transport 层自动适配 mcp / grpc / http / agent(A2A)。 |
| **运行时推导** | Endpoint 由 Registry 根据 Provider 信息与运行态配置动态生成。                     |
| **集中治理**  | Registry 维护版本、健康、租户、安全、观测与依赖关系。                                   |
| **编排友好**  | Workflow 与 Agent 可直接通过能力 ID 调用，不绑定协议实现。                           |

---

## 2️⃣ 概念模型

| 概念               | 描述                                                |
| ---------------- | ------------------------------------------------- |
| **Capability**   | 逻辑能力声明（如 `crm.lead.create`），定义语义、IO Schema 与安全规则。 |
| **Provider**     | 能力提供者（插件、第三方服务或 PowerX 内核）。                       |
| **Endpoint**     | 能力在运行时的访问端点，由 Registry 动态推导。                      |
| **Transport**    | 统一协议层抽象（mcp / grpc / http / agent）。               |
| **Registry**     | 能力与运行态端点的真相源（CoreX/integration/registry）。         |
| **Router**       | 智能选路层，根据健康、成本、延迟、策略打分选择最佳端点。                      |
| **Orchestrator** | 调度层（CoreX/integration/orchestrator），执行完整编排调用链。    |

---

## 3️⃣ 插件能力声明（最小定义）

插件仅需声明能力契约：

```yaml
capabilities:
  - id: crm.lead.create
    version: 1.0.0
    display_name: 创建线索
    description: 创建新的销售线索
    io_schema_ref: ./schemas/crm_lead_create.yaml
    security:
      permission_code: crm.lead:create
      default_role_grants:
        - role_user
      data_domain: crm
      classification: confidential
    agent:
      usable: true
      risk_level: high
```

PowerX 启动插件后，通过 PluginManager 上报并写入 Registry，
后续的 Transport 层与 Router 会根据运行态信息自动生成端点。

`security.permission_code` 是 capability 接入 IAM 与 Agent 授权的唯一结构化权限字段，格式为 `module.resource:action`。Registry 不得从 `id`、`display_name`、`description`、自由文本 `scope` 或历史字段中推断权限码。

`default_role_grants` 用于声明插件 capability 同步到租户 IAM 后，除默认 owner/admin 外还要授予哪些租户默认角色。PowerX Core 同步插件 capability 时必须注册 `permission_code` 对应 IAM permission，并自动授予当前租户的 `role_owner`、`role_admin`；`role_user`、`role_readonly`、`role_vendor` 必须由 capability descriptor 显式声明后才会授权。允许写在顶层、`security` 或 `rbac` 下，非法 role code 必须导致同步失败，不得静默忽略。

---

## 4️⃣ Provider Manifest

Provider 描述其可支持的传输协议及推导模板。
PowerX 运行时会结合该定义 + 插件实例端口生成真实端点。

```yaml
provider_id: com.powerx.plugin.crmplus
display_name: CRM Plus Plugin
transports:
  grpc:
    port_ref: "backend.grpc_port"
    service_template: "{domain}.{module}.Service"
    method_template: "{action|Pascal}"
  http:
    base_path: "/api"
    path_template: "/{domain}/{module}/{action}"
  mcp:
    tool_template: "{capability_id}"
  agent:
    adapter: "builtin"   # 或 external / bridge
defaults:
  version: 1.x
  region: cn
```

> ⚙️ PowerX 在注册阶段动态注入：
>
> * 插件启动分配的端口（来自 PluginManager runtime）
> * MCP 会话信息（session_id）
> * A2A 通信配置（agent_id, channel）

---

## 5️⃣ Registry 存储模型（运行态）

CoreX/integration/registry 会维护每个能力的完整元数据：

```yaml
id: crm.lead.create
display_name: 创建线索
version: 1.0.0
provider: com.powerx.plugin.crmplus
io_schema: {...}
security:
  scope: crm.lead.write
  data_domain: crm
observability:
  tracing: true
  audit: true
resolved_endpoints:
  - transport: grpc
    uri: grpc://127.0.0.1:50401
    service: crm.lead.Service
    method: Create
  - transport: http
    uri: http://127.0.0.1:8080/api/crm/lead/create
  - transport: agent
    channel: agent://com.powerx.plugin.crmplus/session-92af
```

这些端点由 **Registry Runtime Resolver** 动态生成并缓存。

---

## 6️⃣ Endpoint 解析逻辑（ERL）

Registry 依据 Provider Manifest + 运行时上下文推导端点。
不再手写多协议配置，只需声明能力 ID 与 Provider。

伪代码示例：

```go
func ResolveCapability(provider Provider, cap Capability) []Endpoint {
    d, m, a := Split(cap.ID)

    // gRPC
    if port := provider.GetPort("backend.grpc_port"); port > 0 {
        ep := fmt.Sprintf("grpc://127.0.0.1:%d", port)
        add(Endpoint{"grpc", ep, fmt.Sprintf("%s.%s.Service", d, m), strings.Title(a)})
    }

    // HTTP
    base := provider.BaseHTTP()
    add(Endpoint{"http", fmt.Sprintf("%s/api/%s/%s/%s", base, d, m, a)})

    // MCP
    if sess := provider.ActiveMCPSession(); sess != "" {
        add(Endpoint{"mcp", fmt.Sprintf("mcp://%s/%s", sess, cap.ID)})
    }

    // Agent (A2A)
    if agent := provider.AgentChannel(); agent != "" {
        add(Endpoint{"agent", fmt.Sprintf("agent://%s/%s", agent, cap.ID)})
    }

    return endpoints
}
```

---

## 7️⃣ IO Schema（结构化输入输出）

采用 JSON Schema Draft-07 格式定义输入输出结构：

```json
"io_schema": {
  "input": {
    "type": "object",
    "properties": {
      "name": { "type": "string" },
      "phone": { "type": "string" }
    },
    "required": ["name", "phone"]
  },
  "output": {
    "type": "object",
    "properties": {
      "lead_id": { "type": "string" },
      "created_at": { "type": "string", "format": "date-time" }
    },
    "required": ["lead_id", "created_at"]
  }
}
```

> 用于自动表单、验证与类型推断。
> 支持在前端与工作流设计器中自动生成 UI 与调用参数。

---

## 8️⃣ 安全定义

```yaml
security:
  auth_mode: delegated      # delegated | token | mtls | none
  permission_code: crm.lead:create
  default_role_grants:
    - role_user
  data_domain: crm
  classification: confidential
  masking:
    - field: input.phone
      type: partial
agent:
  usable: true
  risk_level: high
```

* `auth_mode`: 调用时采用的认证方式。
* `permission_code`: IAM/RBAC 权限码，必须为 `module.resource:action`。
* `default_role_grants`: capability 同步到租户 IAM 后额外授予的默认角色列表。`role_owner` 与 `role_admin` 由 Core 自动授予；普通成员需要显式声明 `role_user`。
* `agent.usable`: 是否允许 Agent grant 授权该能力。
* `agent.risk_level`: 能力风险等级，用于管理端提示和审批策略。
* `masking`: 审计输出脱敏规则。
* Registry 与 Orchestrator 会在运行时注入租户与用户上下文。

### Agent 授权索引字段

Registry 写入 `capability_records` 时必须保留：

| 字段 | 来源 | 用途 |
| --- | --- | --- |
| `uuid` | Registry 生成 | capability 业务对象 UUID，供 grant 外键引用 |
| `capability_id` | manifest | 调用路由稳定 ID |
| `plugin_id` / `plugin_uuid` | 插件注册 | 插件归属与审计 |
| `annotations.permission_code` 或 `annotations.permission_codes` | manifest | Agent 与 IAM 交集判断 |
| `annotations.default_role_grants` | manifest | 插件 capability 同步 IAM permission 后的额外默认角色授权 |
| `annotations.agent_usable` | manifest | 是否可授予 Agent |
| `annotations.risk_level` | manifest | 管理端展示与策略 |

缺少合法 `permission_code` 的记录可以存在于 Registry，但不得出现在 `/api/v1/admin/agents/grantable-capabilities`，也不得通过 Agent 运行时授权。

### 多租户存储语义

- **契约记录上的 `tenant_id` 代表归属域**：它标识这条契约由哪个租户创建/覆盖，而不是哪些租户在使用。
- **`tenant_id = 0`**：平台级（全局）契约，所有租户默认可见。运维方可在 0 号租户下维护公共版本。
- **`tenant_id = n (>0)`**：租户 `n` 定制的契约版本，可覆盖平台默认行为（IO Schema、传输配置、错误映射等）。
- Router/Registry 在查询时会先查找“当前租户定制”，找不到再回退到 `tenant_id = 0` 的共享版本，从而实现“共享 + 私有”并存。
- 审计与权限控制也会记录该字段，方便追踪是哪个租户或操作人改动了契约。

### GORM 模型与表映射

- **CapabilityContract** → `public.capability_contracts`（定义见 `capability_contract_gorm.go`）  
  存储契约主体：`capability_key`、`version`、`provider_id`、`display_name`、`security_scope`，以及 JSONB 字段 `observability_config`、`transport_preferences`。关联表 `capability_io_schemas`、`capability_contract_error_taxonomies` 持有 IO Schema 与错误映射明细。

- **CapabilityVersionPolicy** → `public.capability_version_policies`（定义见 `capability_version_policy_gorm.go`）  
  维护版本策略：`default_strategy`、`allowed_versions`、`compatibility_matrix`、`deprecation_policy`、`audit_config` 等 JSONB 字段，同样以 `(tenant_id, capability_key)` 唯一。

- **CapabilityTransportProfile** → `public.capability_transport_profiles`（定义见 `transport_profile_gorm.go`）  
  持久化运行时传输参数（timeout、retry、qos、endpoint_selector）。此表与契约 ID 关联，在 Transport Adapter 规范中有更详细说明。

---

## 9️⃣ 版本与生命周期

| 场景            | 策略                                          |
| ------------- | ------------------------------------------- |
| Minor 版本更新    | 向后兼容，Router 默认选最新版本。                        |
| Major 版本更新    | 不兼容，需显式声明。                                  |
| Deprecated 能力 | 标注 `lifecycle: deprecated` 与 `replaced_by`。 |

---

## 🔟 命名规范

| 维度   | 规则                                    |
| ---- | ------------------------------------- |
| 格式   | `<domain>.<module>.<action>`          |
| 示例   | `crm.lead.create`, `ai.text.generate` |
| 内核保留 | `px.*` 前缀为 PowerX 内建能力。               |
| 插件命名 | `com.<org>.<module>.<action>` 为插件能力。  |

---

## 11️⃣ Observability（可观测性）

```yaml
observability:
  tracing: true
  metrics: true
  audit: true
  sample_rate: 0.5
```

所有调用具备可追踪（Trace）、可审计（Audit）、可监控（Metrics）特性。
Router 与 Orchestrator 将自动注入 `trace_id` 与 `span_id`。

---

## 12️⃣ Registry 同步与健康机制

1. 插件启动 → 向 MCP-Server 注册能力 (`announce(capabilities)`)
2. PowerX 验证 Schema / Scope / Version
3. Registry 写入契约与运行态端点
4. PluginManager 维护心跳与健康信息
5. Router 在调用时动态选路

---

## 13️⃣ 示例流程图

```
Plugin (declare Capability)
       │
       │ announce()
       ▼
 PowerX MCP-Server
       │
       ▼
 Registry (Capability + Resolved Endpoints)
       │
       ▼
 Router → Orchestrator → Transport (grpc/mcp/http/agent)
       │
       ▼
 Flow Engine 执行
```

---

## 14️⃣ 未来扩展方向

* 动态 Schema 演化与版本对比
* Composite Capabilities（复合能力）
* GraphQL 与 MQ 异步调用能力映射
* Provider Side Cache 与地域调度
* Plugin Marketplace 能力目录同步

---

> 本规范与以下文档协同：
>
> * `Transport_Adapter_Spec.md`（协议层抽象）
> * `Capability_Registry_and_Router_Design.md`（注册与路由）
> * `PowerX_Plugin_SDK_Guide.md`（插件开发规范）

---

✅ **总结一句话：**

> Capability Contract = 「逻辑声明 + 自动推导 + 动态注册」
> 插件只描述 *能做什么*，而 PowerX 通过 CoreX/integration 决定 *如何做*。
