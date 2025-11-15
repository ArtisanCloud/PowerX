---
scn_id: SCN-INT-PLUGIN-PERMISSION-MANIFEST-001
title: 权限清单申报与安全审核
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
    scope: plugin-permission
    responsibility: Manifest 模板、审核流程、风险报告
related_usecases:
  - doc_id: UC-INT-PLUGIN-PERMISSION-MANIFEST-001
    layer: security
    domain: integration
last_reviewed_at: 2025-02-22

---

# Executive Summary

> 插件必须在 Manifest 中声明所需权限/租户资源，并通过自动校验与安全审核，方可进入发布流程，确保最小权限治理可追溯。

# Scope & Guardrails

- **In Scope**：权限模板、白名单/黑名单校验、敏感度分级、审批与审计。
- **Out of Scope**：租户授权、运行时校验、越权响应。
- **Environment & Flags**：`PX_PLUGIN_PERMISSION_MANIFEST`, `PX_PLUGIN_PERMISSION_RISK`。

# End-to-End Flow

1. 开发者上传 Manifest → 自动校验 Scope、数据分类、风险等级。
2. 安全审核员评估权限合理性并记录理由。
3. 审核通过生成权限模板 ID，同步 Marketplace 与策略库。

# Key Interactions & Contracts

- `POST /permissions/manifest`, `POST /permissions/manifest/:id/approve`, `GET /permissions/manifest/:id`。
- 风险报告：`manifest_risk_report.json`。

# Acceptance Criteria

1. Manifest 字段完整且命名规范；
2. 高危权限需要多级审批；
3. 审核过程写入审计，生成模板 ID。

# Telemetry & Ops

- 指标：`manifest.review.sla`, `manifest.reject.rate`, `manifest.sensitive.scope_ratio`。
- 告警：违规权限申请、频繁修改。

# Open Issues & Follow-ups

| 风险/事项 | 影响 | 负责人 | ETA |
|-----------|------|--------|-----|
| 风险评估缺少自动建议 | 审核效率 | Security Governance Squad | 2025-03-05 |
