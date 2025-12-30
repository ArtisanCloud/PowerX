---
scn_id: "SCN-KNOWLEDGE-SPACE-GOV-001"
scenario_name: "知识空间初始化与治理策略"
slug: "knowledge-space-gov"
primary_scope: "powerx"
primary_layer: "service"
primary_domain: "knowledge"
primary_repo: "powerx"
doc_owner: "Amber Liu / Knowledge Platform Lead / knowledge@artisan-cloud.com"
last_generated_at: "2025-11-12"
---

# 知识空间初始化与治理策略 Usecase Seed 生成指南

> 场景摘要：管理员为业务部门创建知识空间时，需要在 2 分钟内完成空间元数据、配额、RAG/图谱/安全策略绑定与 IAM 同步，默认策略必须启用并可审计回溯，出现冲突需自动回滚。

## Seed 的定位

- 规定 `powerx` 仓库在 `SCN-KNOWLEDGE-SPACE-GOV-001` 场景下的责任：空间 API、策略模板、IAM 同步、审计与告警。
- 与 `docs/_data/docmap.yaml` 中 `doc_id: UC-KNOWLEDGE-SPACE-GOV-001` 节点一一对应，是 `publish:usecases`、`publish:collected` 的唯一数据源。
- 为后续生成分仓 Usecase、编排测试、观测指标提供权威字段。

## 前提条件

- 场景文档：`docs/scenarios/knowledge/SCN-KNOWLEDGE-SPACE-GOV-001.md`；主链路：`docs/scenarios/knowledge/SCN-KNOWLEDGE-SPACE-001.md`。
- docmap：`scope: powerx`, `layer: service`, `domain: knowledge`, `path: docs/usecases-seeds/SCN-KNOWLEDGE-SPACE-001/UC-KNOWLEDGE-SPACE-GOV-001.md`。
- repos：`docs/_data/repos.yaml` 中 `key: powerx`（`usecase_seed_root = docs/use_cases/_from_hub`, `usecase_seed_legacy_root = docs/usecases-seeds`）。
- 必需 Feature Flags：`knowledge-space-v1`, `iam-sync-v2`, `audit-streaming`, `default-pipeline-registry`。
- 依赖：IAM 同步服务、审计日志写入、配置模板 `backend/config/knowledge_space/default_pipelines.yaml`、指标脚本 `scripts/qa/workflow-metrics.mjs`。

> **额外依赖**：需要访问租户/部门目录、配额管理 API、KMS 加密的策略模板存储。

## 生成流程

1. **更新 docmap 节点**

   ```yaml
   - scn_id: SCN-KNOWLEDGE-SPACE-001
     children:
       - doc_id: UC-KNOWLEDGE-SPACE-GOV-001
         scope: powerx
         layer: service
         domain: knowledge
         optional: false
         repo: powerx
         path: docs/usecases-seeds/SCN-KNOWLEDGE-SPACE-001/UC-KNOWLEDGE-SPACE-GOV-001.md
   ```

   - 字段必须与 Seed frontmatter 对齐；若迁移目录需同步 `path`。

2. **复制模板**

   ```bash
   mkdir -p docs/usecases-seeds/SCN-KNOWLEDGE-SPACE-001
   cp docs/usecases-seeds/_template.md \
     docs/usecases-seeds/SCN-KNOWLEDGE-SPACE-001/UC-KNOWLEDGE-SPACE-GOV-001.md
   ```

   - 同场景 Seed 共用目录，便于脚本批量处理。

3. **填充 Frontmatter**

   - `owners`：Amber（Platform）、IAM Team；可在 `contributors` 中加入 Security Ops。
   - `feature_flags`：`knowledge-space-v1`, `iam-sync-v2`, `audit-streaming`。
   - `code_refs`：
     - `services/knowledge_space/space_controller.go`
     - `services/knowledge_space/templates/default_pipelines.yaml`
     - `services/iam/sync/knowledge_space_role_sync.go`
     - `pkg/audit/knowledge_logger.go`

4. **完善正文**

   - `Usecase Overview`：强调 120s SLA、默认策略覆盖率 100%、IAM 同步成功率 ≥99.5%。
   - `Context & Assumptions`：列出租户、部门、配额、审计、权限等前提；说明输入（空间名称、配额、策略）、输出（空间记录、策略版本、审计事件）。
   - `Solution Blueprint`：分解“配置提交 → 模板绑定 → IAM 同步 → 审计通知”步骤，可使用 mermaid。
   - `Contracts & Interfaces`：`POST /knowledge-spaces`, `PATCH /knowledge-spaces/{id}`, `POST /knowledge-spaces/{id}/owners`, 事件 `knowledge.space.created/updated`, IAM Sync API、审计流字段。
   - `Implementation Checklist`：空间 API、模板管理、IAM 同步、审计/监控。
   - `Testing / Observability / Rollback / Risks`：覆盖配额冲突、策略冲突、IAM 失败、审计缺失等场景。

5. **互链与发布**

   - References 指向场景文档、模板、脚本；
   - 场景文档 “Usecase Links” 中加入 `[UC-KNOWLEDGE-SPACE-GOV-001](docs/usecases-seeds/SCN-KNOWLEDGE-SPACE-001/UC-KNOWLEDGE-SPACE-GOV-001.md)`；
   - PR 合并前运行：
     ```bash
     npm run lint
     npm run docs:build
     npm run publish:usecases -- --scn-id SCN-KNOWLEDGE-SPACE-001 --validate-only
     ```

## 自检清单

- [ ] docmap 与 Frontmatter 一致（scope/layer/domain/path/optional）。
- [ ] Seed 覆盖业务目标、上下文、流程、接口、清单、测试、Ops、回滚、风险。
- [ ] SLA/指标：创建 ≤120s、策略覆盖率 100%、IAM 同步 ≥99.5%、审计字段完备。
- [ ] Feature Flag、配置、脚本皆可定位；敏感信息已脱敏。
- [ ] 运行验证脚本通过并在 PR 中附带结果。

## 常见问题

| 问题 | 处理方式 |
|------|----------|
| docmap/Seed 路径不一致 | 以 docmap 为准，修正 frontmatter `path` 并重跑 `publish:usecases --validate-only`。 |
| 策略模板缺失或冲突 | 在 Seed 中记录模板仓库与回滚机制；API 返回 409 并写入审计，要求管理员修复。 |
| IAM 同步失败 | 设置重试 + 告警阈值；Seed 的 Rollback 小节需提供手动补写脚本。 |
| 默认策略被禁用 | 强制二次确认 & 审计；Seed 指出审批流程与字段。 |
| SLA 无法达成 | 在 Risks 中列明依赖瓶颈（审批流、IAM 延迟），提出扩容或预配置策略。 |

完成上述内容后，提交 PR @knowledge-platform-stewards 复核，再依据发布流程同步到下游仓库与领导力视图。
