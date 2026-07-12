# Agent Security and Isolation Policy

> PowerX 智能体安全与隔离策略规范
>
> 本文档定义 **Agent 层在多租户、多插件、多智能体协作场景下的安全治理机制**。
> 目标是确保每个 Agent（无论属于核心系统或第三方插件）
> 都能在安全、可控、可审计的隔离环境中运行，
> 防止越权访问、数据泄漏、资源滥用与代理环路攻击。

---

## 1️⃣ 设计目标

| 目标            | 说明                                  |
| ------------- | ----------------------------------- |
| **多租户隔离**     | 每个租户的 Agent、上下文、事件、数据严格隔离           |
| **权限最小化**     | Agent 仅能调用其授予的 Capabilities（最小授权原则） |
| **安全沙箱**      | 限制文件系统、网络与内存访问范围                    |
| **可审计可追踪**    | 所有调用链具备 trace_id 与安全审计记录            |
| **代理防护（A2A）** | 防止恶意 Agent 形成无限代理调用链                |
| **资源与速率控制**   | CPU、内存、QPS、并发、消息速率受配额限制             |

---

## 2️⃣ 安全域（Security Domain）模型

```text
+--------------------------------------------------------------+
|                        PowerX Secure Runtime                 |
|--------------------------------------------------------------|
|  Tenant Domain                                               |
|   ├── Agent Domains (per plugin / per agent)                 |
|   │     ├── Sandbox (FS/Net/Env)                             |
|   │     ├── Capability Grants                                |
|   │     ├── Token Scope                                      |
|   │     ├── Resource Quota                                   |
|   │     └── Trace Context                                    |
|   └── Shared Services (Registry / Router / Gateway APIs)     |
+--------------------------------------------------------------+
```

| 级别               | 隔离对象                           | 说明              |
| ---------------- | ------------------------------ | --------------- |
| **Tenant Level** | 数据、Schema、EventBus Topic、Token | 不同租户间完全隔离       |
| **Plugin Level** | 插件侧 Agent 代码、端口、配置文件           | 独立配置命名空间        |
| **Agent Level**  | 内部上下文、缓存、沙箱、调用链                | 每个 Agent 独立运行空间 |

---

## 3️⃣ 权限与授权体系

### 3.1 Capability Grant 模型

```yaml
agent_uuid: 9d3f5c02-7a65-47ef-a9ad-f2a8f41d3ef1
tenant_uuid: 0f7d0d9a-4f7f-43de-9a97-2cbd1c3a6b4e
capabilities:
  - capability_uuid: 7ec62846-8213-41f5-b5f1-9811e43ef58f
    capability_id: crm.lead.create
    permission_code: crm.lead:create
    status: enabled
    source: manual
```

`agent_capability_grants` 表只表达 Agent 的最大可用能力边界。真实执行时不得只看 Agent grants，必须与当前登录用户 IAM 权限、租户 capability 启用状态、capability policy 取交集。

### 3.2 校验逻辑

1. 每次 Agent 调用 capability 前，运行时必须解析结构化 `permission_code`，格式为 `module.resource:action`；
2. 检查当前租户是否启用该 capability，未启用直接拒绝；
3. 检查 Agent 是否存在启用状态的 `agent_uuid + capability_uuid/capability_id + permission_code` 授权，缺失直接拒绝；
4. 使用当前登录用户/member 执行 IAM/RBAC `Enforce(module, resource, action)`，缺失直接拒绝；
5. 输出 `agent.capability_authorize` 结构化日志，至少包含 `tenant_uuid`、`agent_uuid`、`user_uuid`、`member_id`、`capability_id`、`permission_code`、`allowed`、`deny_reason`。

任何缺少 `tenant_uuid`、`agent_uuid`、`permission_code` 的调用上下文都必须失败，不允许从自由文本、旧字段或 capability 名称中猜测授权。

---

## 4️⃣ 执行沙箱策略（Runtime Sandbox）

| 控制域      | 策略                                            |
| -------- | --------------------------------------------- |
| **文件系统** | 插件/Agent 仅能访问自身挂载目录 `/data/agents/<agent_id>` |
| **网络访问** | 白名单机制，仅能访问 Registry 注册的 Provider 地址           |
| **系统命令** | 禁止 `exec / fork / os.Process` 等系统调用           |
| **环境变量** | 局部注入，仅允许 `POWERX_*` 前缀变量                      |
| **内存**   | 每个 Agent 限制最大 256MB 可用堆                       |
| **CPU**  | goroutine 并发上限 + 调度优先级                        |
| **持久化**  | 禁止直接访问数据库，必须通过 SDK（带租户上下文）                    |

> 若插件侧 Agent 运行于独立进程，可使用 **Namespace / cgroup / Firecracker microVM** 实现隔离。

---

## 5️⃣ 网络与通信隔离

| 通信方向                    | 控制策略                                       |
| ----------------------- | ------------------------------------------ |
| **PowerX → Agent**      | 仅通过受控端口（注册时动态分配）访问                         |
| **Agent → PowerX**      | 必须通过 gRPC SDK / MCP SDK 进行认证调用             |
| **Agent ↔ Agent (A2A)** | 必须通过 `AgentAdaptor` 通道，带 Token 与 ToolGrant |
| **外部网络访问**              | 需声明 `net_whitelist` 或 `proxy`，否则拒绝         |
| **流式事件**                | 仅允许向自身租户的 EventBus 主题发布                    |

---

## 6️⃣ 资源配额与速率限制

| 资源         | 策略                      |
| ---------- | ----------------------- |
| **CPU**    | 每 Agent goroutine ≤ 100 |
| **Memory** | 256 MB（可配置）             |
| **QPS**    | 每秒 ≤ 50 请求              |
| **并发调用数**  | ≤ 10                    |
| **事件发布速率** | ≤ 500 msg/s             |
| **调用超时**   | 默认 10s，最大不超过 60s        |
| **A2A 深度** | 最大代理层级 ≤ 3              |

> 超限将触发降级策略：暂停 → 降采样 → 断开 → 冻结。

---

## 7️⃣ 安全日志与审计

| 日志类型               | 说明                                       |
| ------------------ | ---------------------------------------- |
| `agent.invocation` | 每次 Agent 调用记录 trace_id、grant_id、duration |
| `agent.violation`  | 沙箱违规、资源越限、未授权调用                          |
| `security.grant`   | Grant 签发/吊销日志                            |
| `audit.event`      | 系统级安全操作（如删除、禁用 Agent）                    |

### 日志示例

```json
{
  "ts": "2025-10-12T17:03:12Z",
  "tenant": "t001",
  "agent": "crm_helper",
  "event": "violation",
  "category": "network",
  "detail": "attempt to reach unauthorized host api.openai.com",
  "trace_id": "trc_8c91a",
  "severity": "high"
}
```

> 所有日志流入 EventBus → `security.*` 主题，可由 Admin Console 订阅。

---

## 8️⃣ 防护机制与检测

| 风险                 | 防护机制                            |
| ------------------ | ------------------------------- |
| **恶意 Agent 自调用循环** | 检测 trace 栈深度 + 限制 `A2A depth`   |
| **越权调用**           | Grant 校验失败即拒绝                   |
| **敏感数据外泄**         | 输出脱敏规则（masking）                 |
| **高频调用攻击**         | Token + QPS 限流                  |
| **日志注入攻击**         | 日志内容转义 + schema 验证              |
| **插件反注册攻击**        | 注册时强制签名校验                       |
| **冒名注册**           | Agent 注册需签名 + 证书链（AgentID + 公钥） |

---

## 9️⃣ 加密与密钥管理

| 类型               | 方案                      |
| ---------------- | ----------------------- |
| **JWT Token**    | HS256 / RS256 签发，短期有效   |
| **ToolGrant 签名** | RS256（与 Capability Grant 统一） |
| **Agent 证书**     | ECDSA keypair per Agent |
| **传输层加密**        | HTTPS / gRPC over TLS   |
| **存储加密**         | Secrets in Vault / KMS  |
| **Token 刷新机制**   | 短期凭证（TTL 15min），需自动续签   |

---

## 🔟 威胁建模与响应

| 威胁类别      | 场景             | 响应策略                   |
| --------- | -------------- | ---------------------- |
| **注入攻击**  | 插件上传恶意二进制      | 构建期扫描 + 签名验证           |
| **反向访问**  | Agent 访问宿主 FS  | 沙箱 + cgroup 阻断         |
| **拒绝服务**  | 循环代理调用耗尽资源     | 限速 + Trace 检测          |
| **数据泄漏**  | Agent 输出敏感字段   | Masking Policy         |
| **伪造调用**  | 假冒 trace_id 调用 | 签名校验 + trace 白名单       |
| **侧信道攻击** | 推测租户数据         | schema 分离 + 不同 role 认证 |
| **高风险操作** | 注册/吊销 Grant    | 双因子确认 + 审计记录           |

---

## 11️⃣ 审计与回放（Audit & Replay）

PowerX 支持对每个 Agent 运行的全链路回放：

* 通过 `/api/v1/admin/security/audit?trace_id=xxx` 可查看调用轨迹；
* 每条事件包含输入、输出、耗时、授权、状态；
* 可复现代理调用链、判断越权与性能异常；
* 敏感字段由 `masking_policy` 控制展示级别。

Agent capability 授权审计使用同一套字段命名，管理操作和运行时调用不得混用 `agent_id` 作为外部关联主键。业务对象关联统一使用 UUID；数值 ID 仅可作为内部诊断辅助字段。

---

## 12️⃣ 监控与告警

| 告警项               | 阈值        | 动作        |
| ----------------- | --------- | --------- |
| Unauthorized call | 任意        | 阻断 + 邮件告警 |
| QPS overload      | 连续 10s 超限 | 自动降速      |
| Memory > 90%      | 60s 持续    | 进程重启      |
| Sandbox violation | 任意        | 停止 Agent  |
| Grant misuse      | 多次越权      | 吊销 Grant  |
| A2A depth > 3     | 立即        | 停止代理链     |
| Audit gap > 5min  | 自动检测      | 强制同步日志    |

---

## 13️⃣ 与其他模块关系

| 模块                    | 安全交互                     |
| --------------------- | ------------------------ |
| **Agent Manager**     | 管理注册、授权、生命周期与安全配置        |
| **AgentAdaptor**      | 验证 Grant 与 Token，执行安全封装  |
| **Security Layer**    | 集中验证 JWT / Grant / Scope |
| **EventBus**          | 安全事件发布、订阅与隔离             |
| **Workflow Engine**   | Step 调用前验证权限与租户          |
| **Router / Registry** | 只暴露可访问 Endpoint          |
| **Plugin Manager**    | 控制插件端 Agent 的安装与沙箱配置     |

---

## ✅ 一句话总结

> **Agent Security & Isolation Policy = PowerX 智能体的“防火墙 + 审计黑匣子”。**
> 它在多租户与多智能体协作场景下，确保所有 Agent 调用安全、边界清晰、行为可控、责任可追，
> 构成 PowerX 多智能体生态的安全基石。
