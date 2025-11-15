# Release Guardrails

本文件记录租户灰度发布的关键守护项，可在更新 `tenant_release_matrix.yaml` 前进行评审：

- **指标阈值**：包括 `knowledge.delta.sla`, `knowledge.feedback.fix_accuracy`, `knowledge.event.latency`, `knowledge.decay.detected` 等。每个批次需定义告警触发值与误差容忍。
- **审批链**：列出策略审批人、执行人、回滚责任人，确保 PRD/Pilot 与量产阶段均有明确责任。
- **回滚手册**：明确 5 分钟内完成的回滚步骤（CLI + HTTP API），以及通知渠道（IM/PagerDuty）。
- **隔离校验**：灰度期间需持续校验跨租户访问控制与版本漂移 ≤ 1 的要求。
- **告警矩阵**：当 `knowledge.release.alerts` 触发时的升级路径与自动化脚本（暂停、回滚、重试）。

> 更新策略或 guardrail 时，请同步修改 `configs/knowledge/tenant_release_matrix.yaml` 并通过 CLI 脚本 `scripts/ops/knowledge-release-matrix.mjs`（待实现）校验。
