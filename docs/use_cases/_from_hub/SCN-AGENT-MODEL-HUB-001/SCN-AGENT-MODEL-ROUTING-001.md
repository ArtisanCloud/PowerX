---
scn_id: SCN-AGENT-MODEL-ROUTING-001
title: 多模型路由与策略编排
status: Draft
version: v0.1.0
owners:
  - name: Agent Platform Guild
    role: Scenario Steward
    contact: agent-platform@artisan-cloud.com
domains: [agent-orchestration]
layers: [integration]
repos:
  - key: powerx
    scope: core-platform
    responsibility: Multi-Model Router、策略权重、A/B、fallback、Telemetry
related_usecases:
  - doc_id: UC-AGENT-MODEL-ROUTING-001
    layer: integration
    domain: agent-orchestration
last_reviewed_at: 2025-02-18
---

# Executive Summary

本子场景聚焦 Planner / Orchestrator 在多模型、多租户环境下的路由策略：依据任务标签、成本、延迟、风险等级自动选择主模型与备用模型，支持 A/B 与灰度，并在异常时快速回滚或降级，保障命中率与体验。

# Scope & Guardrails

- **In Scope**：能力标签、策略配置、决策 API、fallback、安全模式、策略版本管理、回滚。
- **Out of Scope**：Provider 接入（由 Provider 子场景负责）、成本计费细则（治理子场景）。
- **Environment & Flags**：`multi-model-router`、`routing-safe-mode`；依赖 Capability Graph、Feature Flag、Telemetry。

# Participants & Responsibilities

| Scope | Repository | Layer | 责任与交付物 | Owners |
|-------|------------|-------|--------------|--------|
| planner-integration | powerx | integration | 决策 API、Trace、策略执行 | Agent Platform Guild |
| policy-center | powerx | integration | 策略模板、版本化、审计 | Agent Platform Guild |
| ops | powerx | ops | 监控命中率、触发安全模式 | Ops Reliability Center |

# End-to-End Flow

1. Planner 传入任务标签 → 2. 路由器根据策略权重与健康分选择主/备模型 → 3. 输出决策/Trace/成本估算 → 4. 监控命中率与 fallback，必要时回滚策略。

# Key Interactions & Contracts

- `POST /internal/model-routing/route`、`POST /internal/model-routing/rollback`、`EVENT agent.routing.policy.updated`。
- 策略文件：`backend/config/agents/routing/*.yaml`、`config/policies/model-routing.json`。

# Acceptance Criteria

- 命中率 ≥90%、fallback 成功率 ≥95%；策略变更到生效 <5 分钟；安全模式可在 1 分钟内开启。

# Telemetry & Ops

- 指标：`agent.routing.hit_rate`, `agent.routing.fallback_total`, `agent.routing.policy_rollback_duration`。
- 告警：命中率骤降、fallback 失败、延迟超阈。

# References

- `docs/meta/scenarios/powerx/agent-and-automation/agent-model-platform/primary.md`
- `backend/config/agents/routing/*.yaml`
