---
scn_id: SCN-INT-PLUGIN-LICENSE-ISSUE-001
title: License 策略配置与密钥发放
status: Draft
version: v0.1.0
owners:
  - name: Grace Lin
    role: Security & Compliance Lead
    contact: compliance@artisan-cloud.com
domains: [integration]
layers: [service]
repos:
  - key: powerx
    scope: license-service
    responsibility: 产品包配置、密钥生成、审批/分发
related_usecases:
  - doc_id: UC-INT-PLUGIN-LICENSE-ISSUE-001
    layer: service
    domain: integration
last_reviewed_at: 2025-02-22

---

# Executive Summary

> Vendor 在 License 控制台配置产品包与授权策略，通过审批后生成 License Key 并分发，保证授权与策略绑定可追溯。

# Scope & Guardrails

- **In Scope**：产品包定义、策略字段校验、审批流、密钥生成/存储、分发日志。
- **Out of Scope**：租户激活、运行时校验、续费与审计。
- **Environment & Flags**：`PX_PLUGIN_LICENSE_SERVICE`, `PX_PLUGIN_LICENSE_KEYSAFE`。

# End-to-End Flow

1. Vendor 配置产品包（租户、实例、功能、有效期）。
2. 系统校验字段并生成 License Key，状态为待激活，密钥加密存储。
3. 商务审批后分发给租户或下游系统。
4. 记录发放日志并同步财务系统。

# Key Interactions & Contracts

- `POST /licenses`, `POST /licenses/:id/approve`, `POST /licenses/:id/distribute`。
- Config：`license_product_catalog.yaml`, `license_policy_rules.yaml`。

# Acceptance Criteria

1. License Key 与策略绑定，状态准确；
2. 重复或异常申请被阻断并告警；
3. 分发日志完整可审计。

# Telemetry & Ops

- 指标：`license.issue.sla`, `license.issue.fail_rate`, `license.policy.validation_errors`。
- 告警：策略字段缺失、密钥生成失败、重复发放。

# Open Issues & Follow-ups

| 风险/事项 | 影响 | 负责人 | ETA |
|-----------|------|--------|-----|
| License 渠道加密策略待确定 | 传输安全 | Security Platform Squad | 2025-03-05 |
