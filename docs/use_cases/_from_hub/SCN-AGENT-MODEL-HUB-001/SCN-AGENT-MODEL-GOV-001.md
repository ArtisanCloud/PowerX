---
scn_id: SCN-AGENT-MODEL-GOV-001
title: 模型成本与配额治理
status: Draft
version: v0.1.0
owners:
  - name: Ops Reliability Center
    role: Automation Co-owner
    contact: ops-center@artisan-cloud.com
  - name: Agent Platform Guild
    role: Scenario Steward
    contact: agent-platform@artisan-cloud.com
domains: [agent-orchestration]
layers: [ops]
repos:
  - key: powerx
    scope: core-platform
    responsibility: 成本计量、配额策略、告警、审计
related_usecases:
  - doc_id: UC-AGENT-MODEL-GOV-001
    layer: ops
    domain: agent-orchestration
last_reviewed_at: 2025-02-18
---

# Executive Summary

该子场景集中治理模型与外部平台的用量、成本、配额与健康信号，确保调用成本透明可控，异常 5 分钟内告警，并能自动触发限流、降级或停用。

# Scope & Guardrails

- **In Scope**：用量计量、成本聚合、配额策略、异常检测、告警、报表、Runbook。
- **Out of Scope**：财务结算流程、合同管理、业务定价策略。
- **Environment & Flags**：`provider-cost-guard`、`quota-enforcer`；依赖 Cost Warehouse、Quota Service、Telemetry。

# Participants & Responsibilities

| Scope | Repository | Layer | 责任与交付物 | Owners |
|-------|------------|-------|--------------|--------|
| cost-metering | powerx | ops | Token/调用计量、成本计算、数据落地 | Ops Reliability Center |
| quota-service | powerx | ops | 配额配置、限流、停用策略 | Agent Platform Guild |
| observability | powerx | ops | 指标、报表、Runbook | Ops Reliability Center |

# End-to-End Flow

1. 采集调用计量并计算成本 → 2. 与租户/项目配额比对，超阈值触发告警或限流 → 3. 将指标推送至仪表板、生成报表 → 4. 如有异常，执行降级/停用并记录审计。

# Key Interactions & Contracts

- `POST /internal/provider-usage/report`、`GET /internal/provider-quotas`、`POST /internal/provider-quotas/enforce`、`EVENT agent.provider.cost.anomaly`。
- 配置：`config/cost/provider_rates.yaml`、`config/quotas/model_usage.yaml`。

# Acceptance Criteria

- 成本数据延迟 <1 分钟；超配额告警 5 分钟内送达；限流/停用操作 100% 记录审计；降级策略可在 2 分钟内生效。

# Telemetry & Ops

- 指标：`agent.provider.cost_total`, `agent.provider.quota_usage`, `agent.provider.alert_total`, `agent.provider.degrade_total`。
- 告警：成本突增、配额超限、降级失败。

# References

- `docs/meta/scenarios/powerx/agent-and-automation/agent-model-platform/primary.md`
- `scripts/qa/provider-drill.mjs`
