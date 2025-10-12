# Security and Governance Specification
>
> PowerX Integration Framework — 安全与治理规范  
>
> 本文档定义 **PowerX 平台整体安全与治理体系**，  
> 适用于 CoreX / Integration / Plugin / Agent / Workflow / Registry 全域。  
> 它确保平台在多租户、多插件、多协议、跨智能体的复杂环境中保持可信、可控、可追踪。

---

## 1️⃣ 设计目标

| 目标 | 说明 |
|------|------|
| **统一安全架构** | 平台内所有模块遵循统一认证、授权、审计模型 |
| **多租户隔离** | 租户级数据与能力边界严格分离 |
| **精细化权限控制** | 基于 Scope/Action/Resource 的 RBAC |
| **数据分级治理** | 数据域、脱敏、加密与访问策略统一 |
| **传输安全** | 所有通道（MCP/gRPC/HTTP/Agent）TLS 加密 |
| **可观测与可追踪** | Trace / Audit / Metrics 全链路统一 |
| **合规与可治理** | 满足企业审计、隐私与监管要求 |

---

## 2️⃣ 架构分层

```text

+--------------------------------------------------------+

| PowerX Security Model                                      |
| ---------------------------------------------------------- |
| Governance Layer  —— 合规治理 / 审计策略                           |
| Policy Layer      —— 策略引擎 / 租户隔离 / RBAC                    |
| Enforcement Layer —— 鉴权中间件 / Token / Transport             |
| Runtime Layer     —— Plugin / Agent / Flow 运行时安全           |
| +--------------------------------------------------------+ |

````

---

## 3️⃣ 租户与身份体系

| 实体 | 描述 |
|------|------|
| **Tenant** | 逻辑隔离单位，每租户独立数据库 schema、RBAC、日志空间 |
| **Member / Role / Permission** | 细粒度 IAM 权限模型 |
| **Service Account** | 插件或 Agent 的身份凭证（用于机器访问） |
| **Actor Context** | 每次请求的运行上下文：`{tenant_id, actor_id, scopes, trace_id}` |

PowerX 所有模块均使用统一的 Context：

```go
type SecurityContext struct {
    TenantID string
    ActorID  string
    Scopes   []string
    TraceID  string
    Token    string
}
````

---

## 4️⃣ 认证机制（Authentication）

| 场景                 | 机制                           | 说明               |
| ------------------ | ---------------------------- | ---------------- |
| **用户访问 Web / API** | JWT + Session                | 用户登录后的访问控制       |
| **插件注册 / 调用**      | HMAC Token + TLS             | PluginManager 签发 |
| **Agent 注册 / 调用**  | ToolGrant Token + Mutual TLS | AgentAdaptor 管理  |
| **MCP 反连**         | Session Token + WS over TLS  | 双向身份验证           |
| **内部服务调用**         | Signed JWT (Service Account) | CoreX 内部服务通信     |

---

## 5️⃣ 授权机制（Authorization）

PowerX 使用**基于 Scope 的细粒度 RBAC**：

| 维度              | 示例                                          | 说明                      |
| --------------- | ------------------------------------------- | ----------------------- |
| **Scope**       | `crm.lead.write`                            | 能力级权限（Capability Scope） |
| **Resource**    | `/crm/leads/{id}`                           | API 或数据资源               |
| **Action**      | `create`、`update`、`invoke`                  | 操作行为                    |
| **Policy Rule** | `allow(role=manager, scope=crm.lead.write)` | 动态策略                    |

所有 Router / Workflow / Adaptor 调用均在执行前进行权限校验。

---

## 6️⃣ 数据分级与保护

| 等级               | 类型          | 策略                   |
| ---------------- | ----------- | -------------------- |
| **Public**       | 公共文档 / 开放数据 | 无需鉴权，审计记录            |
| **Internal**     | 平台日志 / 配置   | RBAC 控制，TLS 传输       |
| **Confidential** | CRM/订单数据    | Scope 控制 + 加密 + 脱敏   |
| **Restricted**   | 用户隐私 / 财务数据 | AES 加密 + 最小授权 + 审计必选 |

字段级脱敏策略（masking_policy）：

```yaml
masking_policy:
  - field: input.phone
    type: partial
  - field: output.email
    type: hash
```

---

## 7️⃣ 传输安全（Transport Security）

| 协议                     | 机制                       | 特性       |
| ---------------------- | ------------------------ | -------- |
| **HTTP**               | HTTPS + JWT + HMAC 签名    | 向外暴露接口   |
| **gRPC**               | mTLS + Metadata Scopes   | 内部高速通道   |
| **MCP**                | WebSocket + Token + 心跳校验 | 反连通道     |
| **Agent (A2A)**        | mTLS + ToolGrant         | 智能体间安全通信 |
| **EventBus / Gateway** | TLS + ACL                | 流式事件通道安全 |

---

## 8️⃣ 审计与可观测性

| 模块           | 审计内容    | 示例                           |
| ------------ | ------- | ---------------------------- |
| **Router**   | 能力调用记录  | capability + actor + tenant  |
| **Adaptor**  | 协议执行日志  | transport + latency + result |
| **Workflow** | Step 状态 | step_id + status             |
| **Agent**    | 决策轨迹    | goal + chosen tool + output  |
| **Registry** | 注册/下线事件 | provider + version           |

所有调用均附带 Trace ID：

```
Agent → Workflow → Router → Adaptor → Provider
             ↑
         Trace / Audit / Metrics
```

---

## 9️⃣ 策略与治理模型

PowerX 安全策略基于统一 Policy Engine：

```yaml
policies:
  - id: p001
    condition: tenant == "t001" && scope == "crm.*"
    effect: allow
  - id: p002
    condition: data_domain == "finance"
    effect: deny
```

管理接口：

```
GET /api/v1/admin/security/policies
POST /api/v1/admin/security/policies
```

策略存储于 PostgreSQL JSONB，可热更新。

---

## 🔟 多协议治理与沙箱控制

| 模块                     | 安全机制                     |
| ---------------------- | ------------------------ |
| **PluginManager**      | 每插件独立用户、schema、端口；文件系统沙箱 |
| **RuntimeManager**     | TTL、端点健康、自动下线            |
| **AgentManager**       | ToolGrant 授权、调用深度限制      |
| **Adaptor 层**          | Token 注入、防篡改签名           |
| **Workflow Engine**    | 运行隔离、并发与速率限制             |
| **EventBus / Gateway** | Topic ACL、订阅限速、租户隔离      |

> **Agent_Security_and_Isolation_Policy.md** 在此基础上定义 Agent 运行沙箱、内存/CPU 限额、Tool 调用深度等执行细节。

---

## 11️⃣ 合规与日志留存

| 类型   | 保留周期  | 存储策略             |
| ---- | ----- | ---------------- |
| 审计日志 | 90 天  | 压缩存档（S3 / OSS）   |
| 安全事件 | 180 天 | SIEM 系统汇聚        |
| 访问记录 | 30 天  | PostgreSQL / ELK |
| 调试日志 | 7 天   | 本地滚动文件           |

---

## 12️⃣ 安全事件与响应流程

```
检测 → 记录 → 通知 → 阻断 → 恢复 → 报告
```

| 事件类型            | 处理            |
| --------------- | ------------- |
| 登录异常 / Token 重放 | 阻断 Token + 警报 |
| 插件未授权访问         | 立即下线插件 + 审计记录 |
| Agent 超权调用      | 终止运行 + 限流封禁   |
| 数据泄漏风险          | 快照冻结 + 管理员通知  |
| MCP 异常断连        | 自动重连 + 审计警告   |

---

## 13️⃣ Metrics & 报表

| 指标                          | 含义       |
| --------------------------- | -------- |
| `security_violations_total` | 安全违规次数   |
| `policy_eval_latency_ms`    | 策略评估延迟   |
| `audit_events_total`        | 审计事件计数   |
| `token_issuance_total`      | 颁发凭证数    |
| `tenant_isolation_score`    | 多租户隔离健康度 |

---

## ✅ 一句话总结

> **Security_and_Governance.md = PowerX 的安全宪章。**
> 它从平台治理视角定义认证、授权、加密、审计、合规与多租户隔离，
> 是所有子安全策略（如 Agent 沙箱、防逃逸机制、MCP 会话保护）的基础规范。
