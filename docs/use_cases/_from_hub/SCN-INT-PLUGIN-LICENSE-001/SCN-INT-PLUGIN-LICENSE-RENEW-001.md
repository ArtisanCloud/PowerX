---
scn_id: SCN-INT-PLUGIN-LICENSE-RENEW-001
title: 续费提醒与到期处理
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
    scope: license-service
    responsibility: 续费提醒、到期停用、能力恢复
related_usecases:
  - doc_id: UC-INT-PLUGIN-LICENSE-RENEW-001
    layer: ops
    domain: integration
last_reviewed_at: 2025-02-22

---

# Executive Summary

> License 服务在到期前发送提醒，续费成功后自动延长授权；若未续费则切换到期状态，插件运行时执行降级或停用，并记录恢复流程。

# Scope & Guardrails

- **In Scope**：到期提醒、支付确认、停用策略、能力恢复、日志。
- **Out of Scope**：发放、激活、变更审计。
- **Environment & Flags**：`PX_PLUGIN_LICENSE_RENEWAL`, `PX_PLUGIN_LICENSE_EXPIRY`, `PX_PLUGIN_LICENSE_RECOVER`。

# End-to-End Flow

1. 到期前 T-30/T-7/T-1 发送提醒（邮件/站内/Webhook）。
2. 续费完成 → 更新 License 有效期 → 通知插件恢复能力。
3. 到期未续费 → 状态设为过期 → 运行时降级/停用。

# Acceptance Criteria

1. 提醒触达率 ≥ 98%；
2. 续费后能力自动恢复；
3. 到期停用在 SLA 内完成，日志可追溯。

# Open Issues & Follow-ups

| 风险/事项 | 影响 | 负责人 | ETA |
|-----------|------|--------|-----|
| 离线租户提醒渠道不足 | 续费延迟 | Commercial Ops Squad | 2025-03-07 |
