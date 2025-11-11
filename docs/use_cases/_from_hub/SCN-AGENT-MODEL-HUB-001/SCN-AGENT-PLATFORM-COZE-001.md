---
scn_id: SCN-AGENT-PLATFORM-COZE-001
title: 外部智能体平台接入（Coze / n8n）
status: Draft
version: v0.1.0
owners:
  - name: Plugin Guild
    role: Platform Connector Lead
    contact: plugin-guild@artisan-cloud.com
  - name: Ops Reliability Center
    role: Automation Co-owner
    contact: ops-center@artisan-cloud.com
domains: [agent-orchestration]
layers: [integration]
repos:
  - key: powerx-plugin
    scope: plugin-ecosystem
    responsibility: 平台连接器、OAuth/Token、上下文映射、Webhooks
  - key: powerx
    scope: core-platform
    responsibility: 身份/审计集成、限流、告警
related_usecases:
  - doc_id: UC-AGENT-PLATFORM-COZE-001
    layer: integration
    domain: agent-orchestration
last_reviewed_at: 2025-02-18
---

# Executive Summary

该子场景负责将 Coze、n8n 等外部智能体/工作流平台纳入 PowerX 编排，统一身份、密钥、上下文映射与回调治理，使主 Agent 可以像调用内部插件一样触达外部平台，同时保证审计与限流。

# Scope & Guardrails

- **In Scope**：平台注册、OAuth/Token 管理、上下文映射、Webhook/回调签名、状态同步、异常降级。
- **Out of Scope**：外部平台内部节点配置、第三方审批流程、非开放 API 的私有接入。
- **Environment & Flags**：`agent-platform-connectors`、`platform-webhook-guard`；依赖 Identity Service、Webhook Gateway、Audit。

# Participants & Responsibilities

| Scope | Repository | Layer | 责任与交付物 | Owners |
|-------|------------|-------|--------------|--------|
| connector-sdk | powerx-plugin | integration | 平台适配器、上下文映射、回调处理 | Plugin Guild |
| security | powerx | integration | OAuth/Token、签名验证、租户隔离 | Agent Platform Guild |
| ops | powerx | ops | 监控延迟/错误、触发降级和告警 | Ops Reliability Center |

# End-to-End Flow

1. 注册平台实例并完成 OAuth/Token → 2. 配置上下文映射与回调签名 → 3. 主 Agent 通过连接器触发工作流 → 4. 回调同步结果、写入任务看板与审计，异常触发重试或降级。

# Key Interactions & Contracts

- `POST /platform/connectors/{platform}/instances`、`POST /platform/connectors/{platform}/invoke`、`EVENT platform.connector.callback`。
- 配置：`config/platform/connectors/*.yaml`、Webhook 签名脚本。

# Acceptance Criteria

- 平台调用成功率 ≥98%；回调延迟 <3s；签名失败自动阻断并告警；平台不可用时自动降级或人工审批。

# Telemetry & Ops

- 指标：`agent.platform.latency_p95`, `agent.platform.callback_failure_total`, `agent.platform.degrade_total`。
- 告警：回调失败率 >5%、平台响应超时、签名验证失败。

# References

- `docs/meta/scenarios/powerx/agent-and-automation/agent-model-platform/primary.md`
- `docs/scenarios/agent-orchestration/SCN-AGENT-TASK-EXEC-001.md`
