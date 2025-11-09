---
scn_id: SCN-AGENT-MODEL-PROVIDER-001
title: 基础模型 Provider 接入与治理
status: Draft
version: v0.1.0
owners:
  - name: Agent Platform Guild
    role: Scenario Steward
    contact: agent-platform@artisan-cloud.com
domains: [agent-orchestration]
layers: [service]
repos:
  - key: powerx
    scope: core-platform
    responsibility: Provider Registry、密钥托管、健康验证、租户映射
related_usecases:
  - doc_id: UC-AGENT-MODEL-PROVIDER-001
    layer: service
    domain: agent-orchestration
last_reviewed_at: 2025-02-18
---

# Executive Summary

该子场景关注 LLM/VLM/TTS/Embeddings 等基础模型在 PowerX 中的标准化接入流程，涵盖资质审核、`backend/config/agents/providers/*.yaml` 注册、密钥托管、自动化验证与租户/环境映射，确保新增 provider 在 24 小时内上线并满足合规与可观测要求。

# Scope & Guardrails

- **In Scope**：Provider 资料采集、参数模板、密钥托管、租户/环境绑定、健康检测、灰度发布、回滚。
- **Out of Scope**：模型训练/评估、成本治理（由治理子场景负责）、业务级 Prompt。
- **Environment & Flags**：`model-provider-registry`、`provider-health-check`；依赖 Vault、Feature Flag、自动化验证脚本。

# Participants & Responsibilities

| Scope | Repository | Layer | 责任与交付物 | Owners |
|-------|------------|-------|--------------|--------|
| provider-registry | powerx | service | Provider 模板、配置校验、变更发布 | Agent Platform Guild |
| security | powerx | service | 密钥加密、轮换、访问审计 | Ops Reliability Center |
| automation | powerx | service | 接入验证脚本、健康信号上报 | Agent Platform Guild |

# End-to-End Flow

1. Provider 提交基本信息与密钥 → 2. Registry 生成配置并校验 → 3. 运行健康验证/沙箱调用 → 4. 发布到租户配置中心并支持灰度/回滚。

# Key Interactions & Contracts

- `POST /internal/providers/register`、`GET /internal/providers/{id}`、`POST /internal/providers/{id}/rotate-secret`。
- 配置模板：`backend/config/agents/providers/*.yaml`、`config/feature_flags/provider.yaml`。

# Acceptance Criteria

- 接入时长 ≤24 小时；密钥 100% 托管；自动化验证覆盖率 ≥99%；回滚 5 分钟内完成。

# Telemetry & Ops

- 指标：`agent.provider.onboard_duration`, `agent.provider.health_success_total`, `agent.provider.secret_rotation_total`。
- 告警：密钥即将过期、健康验证失败、发布异常。

# References

- `docs/meta/scenarios/powerx/agent-and-automation/agent-model-platform/primary.md`
- `backend/config/agents/providers/*.yaml`
