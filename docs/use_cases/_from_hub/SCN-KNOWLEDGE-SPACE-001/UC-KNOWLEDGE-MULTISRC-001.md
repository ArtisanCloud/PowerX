---
scn_id: "SCN-KNOWLEDGE-MULTISRC-001"
scenario_name: "多源知识融合与策略组合"
slug: "knowledge-multisrc"
primary_scope: "powerx"
primary_layer: "service"
primary_domain: "knowledge"
primary_repo: "powerx"
doc_owner: "Amber Liu / Knowledge Platform Lead / knowledge@artisan-cloud.com"
last_generated_at: "2025-11-12"
---

# 多源知识融合与策略组合 Usecase Seed 生成指南

> 场景摘要：融合政策 PDF、费用 Excel 与实时额度 API，构建“条款→费用项→实时额度”知识链路，并通过 BM25 + 向量 + 图谱约束 + Cross-Encoder 重排序实现可回滚的混合检索策略，要求首轮同步 ≤1 小时、准确率提升 ≥15%、API 故障可自动降级与告警。

本文面向 `powerx` 仓库的 Knowledge Platform / AI Infra / SRE Stewards，指导如何落地 `SCN-KNOWLEDGE-MULTISRC-001` 对应的子用例 Seed。

## Seed 的定位

- 定义 fusion pipeline、策略引擎、图谱链路、监控与回滚在 PowerX Core 中的职责边界。
- 与 `docs/_data/docmap.yaml` 中 `doc_id: UC-KNOWLEDGE-MULTISRC-001` 节点保持一一对应，供 `publish:usecases` / `publish:collected` 取数。
- 为跨源管线、API 重试、混合检索策略与审计提供唯一数据来源。

## 前提条件

- 场景文档：`docs/scenarios/knowledge/SCN-KNOWLEDGE-MULTISRC-001.md`；主场景：`docs/scenarios/knowledge/SCN-KNOWLEDGE-SPACE-001.md`。
- docmap：`scope: powerx`, `layer: service`, `domain: knowledge`, `path: docs/usecases-seeds/SCN-KNOWLEDGE-SPACE-001/UC-KNOWLEDGE-MULTISRC-001.md`。
- repos：`docs/_data/repos.yaml` 中 `key: powerx` 的 `usecase_seed_root = docs/use_cases/_from_hub`、`usecase_seed_legacy_root = docs/usecases-seeds`。
- 依赖：`fusion.pipeline`, `graph.constraint`, `reranker-hotfix`, `reweighting.controls` Feature Flags；外部 API 凭证（KMS 管理）、Grafana `fusion-pipeline` dashboard、`reports/_state/fusion.json` 数据脚本。

> **额外依赖**：SRE 维护 `configs/fusion/*.yaml` 模板；AI Infra 负责 `strategy_engine` 重排序模型（cross-encoder）；Graph 团队提供 `services/graph/linker.go` 链路写入。

## 生成流程

1. **登记/更新 docmap 子节点**

   ```yaml
   - scn_id: SCN-KNOWLEDGE-SPACE-001
     title: 知识空间构建
     children:
       - doc_id: UC-KNOWLEDGE-MULTISRC-001
         scope: powerx
         layer: service
         domain: knowledge
         optional: false
         repo: powerx
         path: docs/usecases-seeds/SCN-KNOWLEDGE-SPACE-001/UC-KNOWLEDGE-MULTISRC-001.md
   ```

   - `doc_id`/`scope`/`layer`/`domain`/`path` 必须与 Seed Frontmatter 完全一致。
   - 若改用 `docs/use_cases/_from_hub`, 需同步 `path` 并确保脚本可访问。

2. **复制模板并放置到目录**

   ```bash
   mkdir -p docs/usecases-seeds/SCN-KNOWLEDGE-SPACE-001
   cp docs/usecases-seeds/_template.md \
     docs/usecases-seeds/SCN-KNOWLEDGE-SPACE-001/UC-KNOWLEDGE-MULTISRC-001.md
   ```

   - 同场景的种子共用目录，便于 `publish:usecases` 打包。
   - 推送前运行 `npm run lint` 与 `npm run docs:build` 确认链接与 frontmatter 整洁。

3. **填充 Frontmatter**

   - 保持 `doc_id`, `scn_id`, `repo_key`, `scope`, `layer`, `domain`, `optional`, `last_reviewed_at` 与 docmap 对齐。
   - `owners` 建议包含 Fusion Orchestrator（Amber）、AI Infra（策略）、SRE（API 可靠性）。
   - `linked_requirements` 可列 `SCN-KNOWLEDGE-MULTISRC-001` 或对应 OKR/PRD；`feature_flags` 填 `fusion-pipeline`, `graph-constraint`, `reranker-hotfix`。
   - `code_refs` 至少列出 `services/fusion/pipeline_manager.go`, `services/fusion/strategy_engine.go`, `services/graph/linker.go`, `pkg/monitoring/fusion_metrics.go`。

4. **完善正文章节**

   - `Usecase Overview`：强调 1h 同步 SLA、准确率提升≥15%、回滚<5m、API 故障自动重试与降级。
   - `Context & Assumptions`：说明源类型、鉴权、限流、幂等键、图谱敏感字段校验范围。
   - `Solution Blueprint`：按 Orchestrator→Source→Strategy→Graph→Audit 的流程列出步骤，并可重用场景 mermaid（Orchestrator→SourcePDF/Excel/API→Strategy）。
   - `Contracts & Interfaces`：列 `POST /fusion/pipelines`, `POST /fusion/pipelines/{id}/run`, `PATCH /fusion/weights`, Events `fusion.source.failed`, `fusion.strategy.published`，以及配置 `configs/fusion/policy-expense.yaml`。
   - `Implementation Checklist`～`Follow-ups & Risks`：结合策略版本化、API 重试、告警、审计。

5. **互链场景文档**

   - 在 Seed `References` 指向 `docs/scenarios/knowledge/SCN-KNOWLEDGE-MULTISRC-001.md`、`configs/fusion/policy-expense.yaml`、`reports/_state/fusion.json`。
   - 在场景文档 “Usecase Links” 中引用本 Seed（已存在但可加 Markdown 链接）。

## 自检清单

- [ ] docmap ←→ Seed frontmatter 字段一致，路径指向实际文件。
- [ ] Seed 正文覆盖目标、上下文、体系分解、流程、契约、清单、测试、观测、回滚、风险。
- [ ] 指标与阈值与场景一致（同步成功率、API 失败率、策略准确率提升、回滚时长）。
- [ ] Feature Flag、配置文件、脚本路径均存在且可被 `publish:usecases` 引用。
- [ ] 完成 `npm run publish:usecases -- --scn-id SCN-KNOWLEDGE-SPACE-001 --validate-only`。

## 常见问题

| 问题 | 处理方式 |
|------|----------|
| API 频繁 500 / Timeout | 在 Seed 的 `Observability`/`Rollback` 中写明自动重试策略（退避 3 次）、`fusion.source.failed` 告警与人工切换缓存数据的脚本。 |
| 权重/重排序回滚太慢 | 记录 `strategy_engine rollback` CLI (`scripts/fusion/rollback_strategy.mjs`)，并将回滚耗时纳入指标。 |
| 多源数据冲突 | 在 `Solution Blueprint` 中说明冲突检测与人工审核队列；需要新增 `fusion.conflict.queue` 配置时同步 standards。 |
| 租户/区域差异 | 若需要多版本 Seed，可在 docmap 新增 `UC-KNOWLEDGE-MULTISRC-INTL-001` 等节点；当前 Seed 须说明如何通过配置区分。 |
| 指标缺口 | 保持 `reports/_state/fusion.json` 与 `pkg/monitoring/fusion_metrics.go` 字段一致，避免仪表盘取数失败。 |

完成以上步骤后，即可提交 PR 并通知 `@knowledge-platform-stewards` 复核；发布前务必同步 docmap、Seed、场景文档链接，确保分发脚本可顺利执行。
