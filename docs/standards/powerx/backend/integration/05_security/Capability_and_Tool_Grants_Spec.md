# Capability and Tool Grants Specification

> 本规范定义 PowerX 的 **能力调用安全模型（Capability Security Model）** 与
> **Tool Grants（工具授权）机制**。
>
> 它适用于：
>
> * 智能体（Agent）调用 PowerX 能力（capability）；
> * 插件或第三方调用 PowerX API；
> * A2A（Agent-to-Agent）场景中智能体代调用外部工具；
> * Workflow / Flow / Orchestrator 体系中的能力执行治理。

---

## 1️⃣ 设计目标

| 目标                      | 说明                                    |
| ----------------------- | ------------------------------------- |
| **零信任调用模型**             | 每个调用必须显式授权（Scope + Grant）             |
| **工具授权治理（Tool Grants）** | Agent 可代理调用其他能力，但受限于授权清单              |
| **跨租户隔离**               | 能力调用限定在当前租户作用域                        |
| **最小权限原则**              | 默认无权限调用任何能力，必须声明 Scope                |
| **可审计与可吊销**             | 所有 Tool Grants 可动态吊销与回收               |
| **支持多级信任链**             | Agent ↔ Plugin ↔ Provider 调用链具备信任递归规则 |

---

## 2️⃣ 安全架构概览

```
+----------------------------------------------------------+
|                    PowerX Security Layer                 |
|----------------------------------------------------------|
|  1. Token & Scope Engine                                 |
|  2. Tool Grant Registry (capability whitelist)            |
|  3. Policy Evaluator (RBAC + Context)                    |
|  4. Signature & Proof Verifier (A2A)                     |
|  5. Audit & Revocation Manager                           |
+----------------------------------------------------------+
         ↑                     ↑                     ↑
         |                     |                     |
       Agent               Plugin               Orchestrator
```

---

## 3️⃣ 核心概念

| 概念                   | 说明                                        |
| -------------------- | ----------------------------------------- |
| **Capability**       | PowerX 内部或插件提供的原子能力（如 `crm.lead.fetch`）   |
| **Scope**            | 权限范围，描述调用者可访问的能力集合（如 `crm.*`、`ai.text.*`） |
| **Tool Grant**       | 临时工具授权，允许代理调用指定能力                         |
| **Grant Token**      | 表示 Tool Grant 的签名令牌（JWT/STS）              |
| **Trust Context**    | 描述调用链上下文（tenant、actor、agent）              |
| **Policy Evaluator** | 评估 Scope、Grant、Role、Tenant 边界的核心逻辑引擎      |

---

## 4️⃣ 调用鉴权模型

> 每一次调用在进入 Orchestrator 时，会依次经过三层安全验证：

| 层级 | 模块                       | 验证内容                         |
| -- | ------------------------ | ---------------------------- |
| L1 | **Scope Engine**         | 调用方是否拥有该 Capability 对应 Scope |
| L2 | **Tool Grant Evaluator** | 若是代理调用，是否在 Tool Grant 白名单内   |
| L3 | **Policy Evaluator**     | 上下文是否合法（租户、身份、调用链信任）         |

---

## 5️⃣ Scope 语义规范

| Scope 类型 | 示例                       | 含义             |
| -------- | ------------------------ | -------------- |
| 全局       | `*`                      | 可调用任意能力（仅系统保留） |
| 模块级      | `crm.*`                  | CRM 模块内任意能力    |
| 能力级      | `crm.lead.create`        | 指定能力           |
| 复合       | `crm.*,ai.text.generate` | 多模块组合          |
| 受限       | `crm.lead.*:readonly`    | 限定操作类型（扩展）     |

Scope 存储在 IAM 系统中（与 Role/Permission 绑定），
每个 Token（用户 / Agent / Plugin）都携带 `scopes` 数组。

---

## 6️⃣ Tool Grant 模型

Tool Grant 是 A2A 调用中的核心授权单元，定义“一个 Agent 可代调用哪些工具（能力）”。

### 6.1 数据结构（YAML）

```yaml
grant_id: grant_sales_assistant
issuer: agent:sales_copilot
subject: agent:crm_helper
tenant_id: t001
scopes:
  - crm.lead.fetch
  - dingding.message.send
constraints:
  ttl: 600        # 有效期 (s)
  max_calls: 20   # 最大调用次数
  time_budget_ms: 8000
  trace_propagation: true
signature: eyJhbGciOi...
```

### 6.2 生命周期

| 阶段        | 行为                              |
| --------- | ------------------------------- |
| **创建**    | 发起 Agent 通过 SDK 申请 Tool Grant   |
| **签发**    | Security Layer 签名生成 Grant Token |
| **传递**    | 通过 Orchestrator 注入下游 Agent 调用   |
| **验证**    | 对端 Agent 执行时校验 Grant Token      |
| **回收/吊销** | 管理员或系统自动吊销（超时/限额）               |

---

## 7️⃣ Grant Token（JWT STS 格式）

```json
{
  "sub": "agent:crm_helper",
  "iss": "agent:sales_copilot",
  "tenant": "t001",
  "scopes": ["crm.lead.fetch", "dingding.message.send"],
  "constraints": {"ttl":600,"max_calls":20},
  "iat": 1734014400,
  "exp": 1734015000,
  "trace": "trc_39d8a"
}
```

> 签名算法：`RS256`
> 验证方：Orchestrator 或 AgentAdaptor
> 失效后自动拒绝调用。

---

## 8️⃣ 权限验证时序

```
Agent-A (发起方)
   │
   ├──▶ 申请 Tool Grant (Security API)
   │
   ├──▶ 调用 Orchestrator → Router (transport=agent)
   │
   ├──▶ 附带 Grant Token → Agent-B (被调用方)
   │
   ├──▶ Agent-B 校验 Grant Token
   │
   └──▶ 调用实际能力 → Orchestrator → Provider
```

---

## 9️⃣ Policy Evaluator 行为逻辑

```go
func Evaluate(ctx ExecutionContext, cap Capability) error {
    if !ctx.HasScope(cap.Scope) {
        return errors.New("scope_denied")
    }
    if ctx.IsProxy() {
        if !ctx.Grant.Allows(cap.Scope) {
            return errors.New("grant_denied")
        }
    }
    if !TenantOwns(ctx.TenantID, cap.ProviderID) {
        return errors.New("tenant_mismatch")
    }
    return nil
}
```

> 三重验证（Scope + Grant + Tenant）缺一不可。

---

## 🔟 Grant Revocation（吊销与审计）

| 触发条件           | 动作                |
| -------------- | ----------------- |
| 到期 (`exp`)     | 自动吊销              |
| 超过 `max_calls` | 拒绝新调用             |
| 管理员手动撤销        | 标记 `revoked=true` |
| 租户回收           | 全量吊销该租户内 Token    |
| 安全事件（滥用/异常）    | 触发警报与隔离           |

吊销后立即广播事件：

```json
{
  "type": "security.revoke",
  "data": {
    "grant_id": "grant_sales_assistant",
    "reason": "expired",
    "tenant": "t001"
  }
}
```

---

## 11️⃣ 多级信任链（Trust Chain）

> 当 A 调用 B，B 调用 C（链式 A2A），信任模型如下：

```
Agent-A ─ Grant1 ─▶ Agent-B ─ Grant2 ─▶ Agent-C
```

| 规则  | 说明                                       |
| --- | ---------------------------------------- |
| 1️⃣ | 每一层都必须独立验证 Grant Token                   |
| 2️⃣ | Grant 不可转发（即 A 发给 B，B 不可复用 A 的 Grant）    |
| 3️⃣ | Trust Context 记录整个链路（trace_id + issuers） |
| 4️⃣ | 审计记录每个代理调用链的全路径                          |
| 5️⃣ | 可配置最大代理深度（`max_proxy_depth`）             |

---

## 12️⃣ 安全事件与指标

| 指标                                | 含义                |
| --------------------------------- | ----------------- |
| `security_grants_issued_total`    | 已签发 Tool Grant 数量 |
| `security_grants_revoked_total`   | 吊销总数              |
| `security_scope_denied_total`     | Scope 拒绝计数        |
| `security_tenant_violation_total` | 跨租户违规调用数          |
| `security_a2a_auth_latency_ms`    | 授权验证耗时            |
| `security_audit_events_total`     | 审计事件计数            |

---

## 13️⃣ Audit 审计格式

```json
{
  "event": "agent_proxy_call",
  "trace_id": "trc_39d8a",
  "tenant_id": "t001",
  "actor": "agent:sales_copilot",
  "proxy_target": "agent:crm_helper",
  "granted_tools": ["crm.lead.fetch"],
  "duration_ms": 820,
  "status": "success"
}
```

所有审计事件写入：

* `security_audit_log` 表；
* 也同步到 EventBus（Topic: `security:audit`）。

---

## 14️⃣ 管理与接口

| Method   | Path                                | 功能            |
| -------- | ----------------------------------- | ------------- |
| `POST`   | `/api/v1/security/tool-grants`      | 创建 Tool Grant |
| `GET`    | `/api/v1/security/tool-grants/{id}` | 查看 Grant 信息   |
| `DELETE` | `/api/v1/security/tool-grants/{id}` | 吊销 Grant      |
| `GET`    | `/api/v1/security/audit`            | 查询安全审计记录      |
| `POST`   | `/api/v1/security/validate`         | 验证 Token（A2A） |

---

## 15️⃣ 与其他模块的关系

| 模块                    | 职责                        |
| --------------------- | ------------------------- |
| **Agent Manager**     | 申请 Tool Grant、附带 Token 调用 |
| **Orchestrator**      | 统一鉴权入口，验证 Scope + Grant   |
| **Router / Registry** | 校验能力合法性与租户归属              |
| **Security Layer**    | 颁发 / 验证 / 吊销 Grant        |
| **Audit / Metrics**   | 输出安全日志与监控指标               |

---

## ✅ 一句话总结

> **Tool Grant = 智能体调用的“数字准入证”**
> 它在多智能体与插件生态中建立了 **最小授权、可追踪、可吊销** 的安全边界。
>
> PowerX 的安全层通过 **Scope + Grant + Tenant + Trace** 四维模型，
> 实现了完整的智能体调用治理体系，为 A2A 生态提供可信基石。
