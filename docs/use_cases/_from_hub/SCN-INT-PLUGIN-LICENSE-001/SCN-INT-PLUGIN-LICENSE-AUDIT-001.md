---
scn_id: SCN-INT-PLUGIN-LICENSE-AUDIT-001
title: License 变更与审计
status: Draft
version: v0.1.0
owners:
  - name: Grace Lin
    role: Security & Compliance Lead
    contact: compliance@artisan-cloud.com
domains: [integration]
layers: [security]
repos:
  - key: powerx
    scope: license-service
    responsibility: 变更/撤销审批、审计日志、报表导出
related_usecases:
  - doc_id: UC-INT-PLUGIN-LICENSE-AUDIT-001
    layer: security
    domain: integration
last_reviewed_at: 2025-02-22

---

# Executive Summary

> Vendor 调整授权策略或合规巡检时需变更/撤销 License，并输出审计/对账报告，保证授权链路可追溯。

# Scope & Guardrails

- **In Scope**：变更申请、审批、回滚、审计日志、报表导出。
- **Out of Scope**：发放、激活、续费运行。
- **Environment & Flags**：`PX_PLUGIN_LICENSE_AUDIT`, `PX_PLUGIN_LICENSE_CHANGE`, `PX_PLUGIN_LICENSE_WHITELIST`。

# End-to-End Flow

1. Vendor 发起变更（功能模块、席位数、停用）。
2. 系统生成变更任务，合规审批后生效或回滚。
3. 变更同步至 License 服务与运行时，更新授权令牌。
4. 审计团队导出日志、对账并处理违规使用。

# Acceptance Criteria

1. 变更流程可审批/回滚；
2. 审计日志包含所有变更细节；
3. 违规使用时可冻结 License 并通知租户。

# Open Issues & Follow-ups

| 风险/事项 | 影响 | 负责人 | ETA |
|-----------|------|--------|-----|
| 变更审批 SLA 偏长 | 发布滞后 | Governance Squad | 2025-03-06 |
