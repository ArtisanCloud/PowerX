---
scn_id: SCN-INT-PLUGIN-PERMISSION-RUNTIME-001
title: 运行时访问控制与动态调整
status: Draft
version: v0.1.0
owners:
  - name: Michael Hu
    role: Plugin Tech Lead
    contact: tech@artisan-cloud.com
domains: [integration]
layers: [ops]
repos:
  - key: powerx
    scope: runtime-gateway
    responsibility: Scope token、运行时校验、动态策略
related_usecases:
  - doc_id: UC-INT-PLUGIN-PERMISSION-RUNTIME-001
    layer: ops
    domain: integration
last_reviewed_at: 2025-02-22

---

# Executive Summary

> 宿主运行时与 API Gateway 必须在每次调用中校验 Scope、租户上下文与速率，支持秒级动态调整与降级，阻断越权行为。

# Scope & Guardrails

- **In Scope**：Scope Token 颁发、运行时校验、限流、动态调整、租户隔离。
- **Out of Scope**：Manifest/授权/告警响应。
- **Environment & Flags**：`PX_PLUGIN_RUNTIME_SCOPE`, `PX_PLUGIN_DYNAMIC_POLICY`, `PX_PLUGIN_RATE_LIMIT`。

# End-to-End Flow

1. 宿主颁发带 Scope 的 Token，记录租户上下文。
2. API 网关校验 Token、Scope、租户 ID、速率，并记录日志。
3. 异常调用触发策略降级、冻结或强制刷新 Token。
4. 管理员/安全团队可实时调整策略并生效。

# Key Interactions & Contracts

- `POST /runtime/tokens`, `POST /runtime/tokens/revoke`, `POST /runtime/policies/adjust`。
- Logs：`plugin.runtime.scope.pass/fail`, `plugin.runtime.policy.adjust`。

# Acceptance Criteria

1. 所有调用携带合法 Scope，越权立即阻断；
2. 动态调整秒级生效；
3. 日志包含租户、Scope、结果。

# Telemetry & Ops

- 指标：`runtime.scope.block_count`, `runtime.policy.adjust_latency`, `runtime.token.revoke.count`。
- 告警：连续越权、策略同步失败。

# Open Issues & Follow-ups

| 风险/事项 | 影响 | 负责人 | ETA |
|-----------|------|--------|-----|
| 动态策略尚未与 SOAR 联动 | 响应滞后 | Security Ops Squad | 2025-03-07 |
