---
scn_id: "SCN-KNOWLEDGE-QA-REASON-001"
scenario_name: "智能问答与推理"
slug: "intelligent-qa-reasoning"
primary_scope: "powerx"
primary_layer: "service"
primary_domain: "knowledge"
primary_repo: "powerx"
doc_owner: "Michael Hu / Product Manager / matrix-x@artisan-cloud.com"
last_generated_at: "2025-02-14"
---

# 智能问答与推理 Usecase Seed 生成指南（治理与合规）

> 场景摘要：本 Seed 聚焦智能问答链路中的“安全合规与反馈闭环”责任，确保越权拦截率 100%、敏感命中率 ≥99%、负向反馈 24 小时内闭环、审计日志完整可追溯。

本文面向 `powerx` 仓库中负责 Security & Compliance、Feedback Intelligence 的 Stewards，阐述如何落地 `UC-KNOWLEDGE-QA-GOV-001` 种子，支撑 `SCN-KNOWLEDGE-QA-REASON-001` 场景在治理层面的交付。

## Seed 的定位

- 描述 QA 主场景在安全审计、敏感遮蔽、反馈治理方面的具体实现、指标与脚本。
- 与 docmap 中 `scn_id: SCN-KNOWLEDGE-QA-REASON-001` → `doc_id: UC-KNOWLEDGE-QA-GOV-001` 对应，是 `publish:usecases` 对下游仓库同步治理需求的唯一来源。
- 覆盖子场景 `SCN-KNOWLEDGE-QA-FEEDBACK-001` 与 `SCN-KNOWLEDGE-QA-COMPLIANCE-001` 的公共配置与接口。

## 前提条件

- 场景文档：`docs/scenarios/knowledge/SCN-KNOWLEDGE-QA-REASON-001.md`，子场景 `SCN-KNOWLEDGE-QA-FEEDBACK-001`, `SCN-KNOWLEDGE-QA-COMPLIANCE-001`。
- docmap：`scope: powerx`, `layer: service`, `domain: knowledge`, `path: docs/usecases-seeds/SCN-KNOWLEDGE-QA-REASON-001/UC-KNOWLEDGE-QA-GOV-001.md`。
- repos：`docs/_data/repos.yaml` → `key: powerx`，具备 `usecase_seed_root`, `usecase_seed_legacy_root`。
- Feature Flags：`PX_SECURITY_AUDIT_LOG`, `PX_QA_ACCESS_CHECK`, `PX_QA_SENSITIVE_MASK`, `PX_QA_FEEDBACK_LOOP`, `PX_QA_AUTOFIX`, `PX_QA_ALERT`。
- 依赖服务：`iam-service`、`security-policy`、`audit-ledger`、`feedback-service`、`knowledge-space`、`tool-runtime`。
- 运行脚本与仪表盘：`scripts/qa/workflow-metrics.mjs`, `scripts/security/audit_backfill.mjs`, `reports/_state/qa-security.json`, Grafana `QA Security & Feedback` dashboards。

> **TODO**：若需要新建安全策略、密钥或外部审计系统对接，请在此列出并发起审批。

## 生成流程

1. **docmap 子节点保持一致**

   ```yaml
   - scn_id: SCN-KNOWLEDGE-QA-REASON-001
     children:
       - doc_id: UC-KNOWLEDGE-QA-GOV-001
         scope: powerx
         layer: service
         domain: knowledge
         optional: false
         repo: powerx
         path: docs/usecases-seeds/SCN-KNOWLEDGE-QA-REASON-001/UC-KNOWLEDGE-QA-GOV-001.md
   ```

2. **复制模板**

   ```bash
   mkdir -p docs/usecases-seeds/SCN-KNOWLEDGE-QA-REASON-001
   cp docs/usecases-seeds/_template.md \
     docs/usecases-seeds/SCN-KNOWLEDGE-QA-REASON-001/UC-KNOWLEDGE-QA-GOV-001.md
   ```

3. **Frontmatter 填写建议**

   - `owners`: Security & Compliance Squad（含 Michael Hu）、Knowledge Ops（Feedback）、SRE（Audit）。
   - `feature_flags`: 上述 Flag + 指向 `backend/config/feature_flags.yaml`。
   - `code_refs`: `services/security/access/service.go`, `services/security/sensitive/guard.go`, `services/audit/logger.go`, `services/feedback/processor.go`。

4. **正文章节重点**

   - `Usecase Overview`: 说明越权拦截、敏感遮蔽、反馈闭环指标。
   - `Context & Assumptions`: 列出权限矩阵、敏感词典、审计队列、反馈入口、工单升级路径。
   - `Solution Blueprint`: 拆分 Authorization Check → Sensitive Detection → Audit Logging → Feedback Diagnose → Remediation → Regression。
   - `Contracts & Interfaces`: `POST /security/access/check`, `POST /security/sensitive/detect`, `POST /audit/logs`, `POST /alerts/security`, `POST /qa/feedback`, `POST /qa/remediation/jobs`。
   - `Implementation Checklist`: 权限矩阵同步、遮蔽策略、审计落盘、反馈工单、自动修复流水线。
   - `Testing & Validation`: 越权/敏感正逆向、反馈 SLA 演练、审计完整性校验。
   - `Observability`: 指标 `security.access.denied`, `security.sensitive.hit_rate`, `qa.feedback.loop_time`, 告警 `PX_QA_ALERT`, 报表 `reports/_state/qa-security.json`。
   - `Rollback & Failure Handling`: 权限缓存刷新、遮蔽策略回滚、审计队列重放、反馈积压处理。

5. **互链**

   - Seed 中引用主场景与子场景文档。
   - 场景文档“Usecase Links”保持指向该 Seed。

## 自检清单

- [ ] docmap 与 frontmatter 字段一致。
- [ ] Seed 覆盖安全/反馈流程、接口、Flag、测试、指标。
- [ ] 指标与场景一致：越权拦截 100%、敏感命中 ≥99%、反馈闭环 ≤24h。
- [ ] 审计与反馈脚本可在 `dev/staging/prod` 运行并记录到 `reports/_state`。
- [ ] `npm run publish:usecases -- --scn-id SCN-KNOWLEDGE-QA-REASON-001 --validate-only` 通过。

## 常见问题

| 问题 | 处理方式 |
|------|----------|
| 权限矩阵不同步导致误拦截 | 在 Checklist 中加入 IAM 同步任务，失败时触发告警并自动重试。 |
| 敏感遮蔽遗漏 | 定期运行 `scripts/security/masking_regression.mjs`，并在 Seed 中记录审计样本。 |
| 反馈量激增 | 依赖 Feedback Seed 中的告警策略，自动升级至人工审核并记录 `feedback.alert_rate`。 |
| 审计日志缺失 | 使用 `audit.replay` 工具补写，并在 Seed 说明补救流程。 |

完成后提交 PR，通知 `@security-compliance-stewards` 与 `@knowledge-ops-stewards` 评审，再执行 `publish:usecases` 分发到下游仓库。
