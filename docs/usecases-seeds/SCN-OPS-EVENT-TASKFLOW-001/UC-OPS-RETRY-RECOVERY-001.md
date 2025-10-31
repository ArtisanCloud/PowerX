doc_id: UC-OPS-RETRY-RECOVERY-001
scn_id: SCN-OPS-EVENT-TASKFLOW-001
title: 延迟队列重试与补偿闭环
status: Draft
version: v0.1.0
repo_key: powerx
scope: powerx
layer: ops
domain: ops
scenario_title: "PowerX 事件与任务流管理"
owners:
  - name: Matrix Ops
    role: Platform Ops Lead
    contact: ops@artisan-cloud.com
  - name: Eva Zhang
    role: Automation Steward
    contact: automation@artisan-cloud.com
contributors: []
linked_requirements:
  - SCN-OPS-EVENT-TASKFLOW-001-D
code_refs:
  - repo: powerx
    path: internal/tasks/retry/delay_queue.go
    description: 延迟队列入队、出队与调度实现
  - repo: powerx
    path: internal/tasks/retry/policy_engine.go
    description: 重试策略、退避算法、幂等键管理
  - repo: powerx
    path: internal/tasks/retry/dlq_handler.go
    description: 死信队列处理、人工工单生成
  - repo: powerx
    path: internal/tasks/monitoring/retry_metrics_collector.go
    description: 重试指标采集、观测与告警
  - repo: powerx
    path: pkg/ops/recovery_runbook.go
    description: 补偿脚本、Runbook 自动化入口
feature_flags:
  - task-retry-queue
  - dlq-inspector
  - audit-streaming
optional: false
last_reviewed_at: 2025-10-31

---

# Usecase Overview

- **业务目标**：为失败任务提供可配置的延迟重试、死信处理和人工补偿能力，确保关键任务在异常情况下仍可恢复，且整个过程可追踪、可审计。
- **成功度量**：自动重试成功率 ≥ 90%；死信升级告警响应时间 ≤ 5 分钟；补偿工单完成率 ≥ 95%；重复执行率 < 0.5%。
- **场景关联**：对应主场景 `SCN-OPS-EVENT-TASKFLOW-001` Stage 4，承接调度/Agent 失败任务并闭环恢复。

> 通过延迟队列、退避策略和人工补偿脚本，构建“失败→重试→升级→工单→恢复”的标准化流程。

# Context & Assumptions

- **前置条件**
  - `task-retry-queue`、`dlq-inspector`、`audit-streaming` Feature Flag 已启用。
  - Redis/Kafka 支持延迟队列功能；Ops 控制台提供重试策略配置界面。
  - 任务执行方实现幂等接口，能够根据 `retry_token` 判定重复请求。
  - PagerDuty/Slack 告警通道配置完成，工单系统可通过 API 自动创建。
- **输入/输出**
  - 输入：任务失败事件（状态、原因、上下文）、重试策略（次数、退避、阈值）、人工处理指示。
  - 输出：重试任务实例、日志、告警、工单、补偿执行结果、审计记录。
- **边界**
  - 不覆盖跨仓数据修复或业务人工补救脚本，仅调用现有 Runbook；
  - 不处理与支付、计费相关的资金补偿（另有场景）；
  - 不包括硬件/基础设施故障恢复策略。

# Solution Blueprint

## 体系分解

| 层 | 主要组件/模块 | 责任 | 代码入口 |
|----|---------------|------|---------|
| 延迟队列 | `internal/tasks/retry/delay_queue.go` | 入队、出队、退避计算、调度信号 | `services/tasks/retry` |
| 策略引擎 | `internal/tasks/retry/policy_engine.go` | 解析策略、限制尝试次数、生成幂等 token | `services/tasks/retry` |
| 死信处理 | `internal/tasks/retry/dlq_handler.go` | 死信入库、告警、工单创建、人工处理 | `services/tasks/retry` |
| 观测层 | `internal/tasks/monitoring/retry_metrics_collector.go` | 指标、日志、仪表盘、审计上报 | `services/tasks/monitoring` |
| Runbook | `pkg/ops/recovery_runbook.go` | 提供补偿脚本、人工介入流程 | `pkg/ops` |

## 流程与时序

1. **Step 1 – 失败入队**：任务执行失败，按照策略写入延迟队列并记录失败原因、幂等 token。
2. **Step 2 – 延迟重试**：到期后取出任务，重新触发执行，并将幂等 token 传递至执行方。
3. **Step 3 – 状态更新**：重试成功则更新任务状态、关闭告警；重试失败记录次数并再次入队或转入死信。
4. **Step 4 – 死信升级**：达到最大重试次数时进入 DLQ，触发 PagerDuty/Slack 告警并自动创建工单。
5. **Step 5 – 人工补偿**：运维使用 Runbook 或脚本执行补偿，记录结果并更新审计。

# Contracts & Interfaces

- **Inbound APIs / Events**
  - `EVENT task.execution.failed` — 包含错误码、可重试标记、幂等键。
  - `POST /internal/tasks/retry` — 人工触发重试或修改策略。
- **Outbound 调用**
  - `POST /plugin/runtime/{pluginId}/execute` — 重试执行。
  - `POST /ops/workorders` — 创建工单，附带失败原因、任务上下文。
  - `POST /notifications/retry-alert` — 向 PagerDuty/Slack 发送告警。
- **配置与脚本**
  - `config/tasks/retry-policies.yaml` — 默认重试策略集合。
  - `scripts/ops/retry-inspect.mjs` — 检查延迟队列与死信队列。
  - `scripts/ops/recovery-runbook.mjs` — 自动化补偿脚本。

# Implementation Checklist

| 项目 | 描述 | 完成状态 | 负责人 |
|------|------|----------|--------|
| 延迟队列实现 | 支持退避算法、幂等键、容量管理 | [ ] | Matrix Ops |
| 策略配置 | 控制台策略模板、API、权限控制 | [ ] | Eva Zhang |
| 死信治理 | 告警、工单自动化、Runbook 集成 | [ ] | Matrix Ops |
| 观测能力 | 指标、日志、报表与审计事件 | [ ] | Eva Zhang |
| 历史迁移 | 旧版失败任务数据迁移方案 | [ ] | Matrix Ops |

# Testing Strategy

- **单元测试**：退避算法、幂等 token、策略解析、死信入库、Runbook 调用。
- **集成测试**：执行用例 D-1 验证延迟重试成功；执行 D-2 验证超过阈值升级工单并停止重试；模拟策略变更生效。
- **端到端验证**：在沙箱租户触发失败任务，观察重试过程、告警、工单、Ops 控制台状态；验证补偿脚本执行与审计。
- **非功能测试**：压测延迟队列吞吐；Chaos 注入 Kafka/Redis 故障验证降级；测试大批量死信回放。

# Observability & Ops

- **指标**：`task.retry.scheduled_total`、`task.retry.success_total`、`task.retry.failure_total`、`task.retry.dlq_total`、`task.retry.escalated_total`。
- **日志**：记录 `task_id`, `retry_token`, `attempt`, `reason`, `next_retry_at`, `dlq_flag`, `workorder_id`。
- **告警**：重试失败率 >15%/10 分钟、死信队列长度超过阈值、工单创建失败；通过 PagerDuty/Slack 通知。
- **Dashboards**：Grafana `Runtime Ops / Retry & Recovery`、Datadog `task.retry.*`、Ops 控制台补偿面板。

# Rollback & Failure Handling

- **回滚步骤**：恢复旧版重试服务，关闭新 Feature Flag，将新入队任务迁移至旧结构。
- **补救措施**：人工执行 Runbook、批量重放死信、重新配置策略，通知受影响租户。
- **数据修复**：对 `task_retry_queue` 表执行一致性校验，修复重复项；使用 `retry-inspect.mjs --reconcile` 同步状态。

# Follow-ups & Risks

| 风险/事项 | 影响 | 缓解方案 | 负责人 | ETA |
|-----------|------|----------|--------|-----|
| 死信堆积清理流程未自动化 | 工单积压、补偿延迟 | 实现 dlq-inspector 批量处理、添加提醒 | Matrix Ops | 2025-11-07 |
| 退避策略与业务 SLA 未同步 | 恢复动作可能过晚 | 引入 SLA 感知策略、控制台提示 | Eva Zhang | 2025-11-14 |

# References & Links

- 主场景：`docs/scenarios/runtime-ops/SCN-OPS-EVENT-TASKFLOW-001.md`
- 子场景：`docs/scenarios/runtime-ops/SCN-OPS-RETRY-RECOVERY-001.md`
- 背景材料：`docs/meta/scenarios/powerx/core-platform/runtime-ops/event-and-taskflow-management/primary.md`
- Runbook：`scripts/ops/retry-inspect.mjs`、`scripts/ops/recovery-runbook.mjs`
