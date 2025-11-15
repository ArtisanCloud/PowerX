---
scn_id: SCN-INT-PLUGIN-LICENSE-ACTIVATE-001
title: 租户激活与运行时校验
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
    scope: license-runtime
    responsibility: 激活流程、运行时令牌、能力开关
related_usecases:
  - doc_id: UC-INT-PLUGIN-LICENSE-ACTIVATE-001
    layer: ops
    domain: integration
last_reviewed_at: 2025-02-22

---

# Executive Summary

> 租户管理员使用 License Key 激活插件，运行时在每次功能调用前校验授权令牌，防止未授权或超限使用。

# Scope & Guardrails

- **In Scope**：License 激活、运行时令牌、能力开关、越权阻断、日志。
- **Out of Scope**：发放、续费、变更审计。
- **Environment & Flags**：`PX_PLUGIN_LICENSE_RUNTIME`, `PX_PLUGIN_LICENSE_ENFORCE`。

# End-to-End Flow

1. 租户提交 License Key → License 服务校验签名、状态 → 激活并下发运行时令牌。
2. 插件运行时校验令牌与授权范围，记录使用指标。
3. 越权或超限触发能力降级/关闭并通知租户与 Vendor。

# Acceptance Criteria

1. 激活成功率 ≥ 99%；跨租户或重复激活被阻断；
2. 运行时校验覆盖所有敏感调用，越权即阻断；
3. 使用日志可追溯。

# Open Issues & Follow-ups

| 风险/事项 | 影响 | 负责人 | ETA |
|-----------|------|--------|-----|
| 离线激活流程未固化 | 私有化客户体验 | Runtime Platform Squad | 2025-03-04 |
