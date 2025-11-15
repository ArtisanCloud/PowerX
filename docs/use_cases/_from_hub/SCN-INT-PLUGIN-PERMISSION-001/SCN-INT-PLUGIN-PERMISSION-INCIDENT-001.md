---
scn_id: SCN-INT-PLUGIN-PERMISSION-INCIDENT-001
title: 越权检测与审计响应
status: Draft
version: v0.1.0
owners:
  - name: Grace Lin
    role: Security & Compliance Lead
    contact: compliance@artisan-cloud.com
domains: [integration]
layers: [ops]
repos:
  - key: powerx
    scope: security-ops
    responsibility: SIEM/SOAR 越权检测、隔离、复盘
related_usecases:
  - doc_id: UC-INT-PLUGIN-PERMISSION-INCIDENT-001
    layer: ops
    domain: integration
last_reviewed_at: 2025-02-22

---

# Executive Summary

> 当插件出现越权或权限滥用时，安全团队需通过 SIEM/SOAR 快速检测、隔离并通知租户，生成复盘与整改报告。

# Scope & Guardrails

- **In Scope**：日志聚合、告警、自动化隔离、租户通知、复盘、白名单管理。
- **Out of Scope**：Manifest/授权/运行时策略事先治理。
- **Environment & Flags**：`PX_PLUGIN_PERMISSION_SIEM`, `PX_PLUGIN_PERMISSION_SOAR`, `PX_PLUGIN_PERMISSION_WHITELIST`。

# End-to-End Flow

1. SIEM 关联访问日志、租户举报、策略变更，识别越权行为。
2. SOAR Playbook 执行隔离/撤销权限、通知租户与插件方。
3. 安全团队复盘，导出调用明细、权限变更记录，更新模板或发布公告。

# Key Interactions & Contracts

- `POST /siem/alerts`, `POST /soar/playbooks/permission`, `POST /notifications/tenants`, `POST /permission/whitelist`。

# Acceptance Criteria

1. 越权事件 MTTR < 15 分钟；
2. 处置闭环包含隔离、通知、复盘；
3. 白名单/豁免可追踪。

# Telemetry & Ops

- 指标：`permission.alert.count`, `permission.mttr`, `permission.false_positive_rate`。
- 告警：持续越权、通知失败、审计缺失。

# Open Issues & Follow-ups

| 风险/事项 | 影响 | 负责人 | ETA |
|-----------|------|--------|-----|
| 告警风暴缺少去噪策略 | 噪声影响 | Security Ops Squad | 2025-03-06 |
| 审计日志未统一格式 | 复盘困难 | Observability Squad | 2025-03-08 |
