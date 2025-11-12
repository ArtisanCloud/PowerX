---
scn_id: "SCN-KNOWLEDGE-SPACE-001"
scenario_name: "知识空间构建"
slug: "knowledge-space-build"
primary_scope: "powerx"
primary_layer: "service"
primary_domain: "knowledge"
primary_repo: "powerx"
doc_owner: "Amber Liu / Knowledge Platform Lead / knowledge@artisan-cloud.com"
last_generated_at: "2025-11-12"
---

# 知识空间构建 Usecase Seed 生成指南

> 场景摘要：PowerX 为租户提供从空间创建、配额治理、到多模态入库与反馈闭环的一站式能力；该 Seed 对应整条主链路，要求 2 分钟内创建空间、4 小时内完成首轮多源入库，Chunk 覆盖率 ≥95%、嵌入成功率 100%、默认安全策略启用且可审计。

本指南面向 `powerx` 仓库的 Knowledge Platform / Data Infra / Security Ops Stewards，说明如何维护 `UC-KNOWLEDGE-SPACE-BUILD-001` 种子数据并落地 `SCN-KNOWLEDGE-SPACE-001` 需求。

## Seed 的定位

- 描述空间创建、入库编排、安全治理、反馈闭环在 PowerX Core 内的责任矩阵。
- 与 `docs/_data/docmap.yaml` 中主场景 `SCN-KNOWLEDGE-SPACE-001` → `doc_id: UC-KNOWLEDGE-SPACE-BUILD-001` 一致，是 `publish:usecases`、`publish:collected` 的唯一数据源。
- 统筹五个子场景（Space Gov / Longdoc / Table / Multisrc / RAG Feedback）的跨环节依赖与指标口径。

## 前提条件

- 场景文档：`docs/scenarios/knowledge/SCN-KNOWLEDGE-SPACE-001.md`；子场景文档需保持最新。
- docmap：`scope: powerx`, `layer: service`, `domain: knowledge`, `path: docs/usecases-seeds/SCN-KNOWLEDGE-SPACE-001/UC-KNOWLEDGE-SPACE-BUILD-001.md`。
- repos：`docs/_data/repos.yaml` -> `key: powerx`，`usecase_seed_root = docs/use_cases/_from_hub`，`usecase_seed_legacy_root = docs/usecases-seeds`。
- Feature Flags：`knowledge-space-v1`, `knowledge-ingestion`, `audit-streaming`, 以及子场景所需的 `longdoc.chunking`, `structured-ingestion`, `fusion.pipeline`, `feedback.loop` 等。
- 依赖脚本：`scripts/ingestion/**`, `scripts/qa/workflow-metrics.mjs`, `reports/_state/knowledge-spaces.json`、加密对象存储、IAM/审计服务。

## 生成流程

1. **docmap 子节点**

   ```yaml
   - scn_id: SCN-KNOWLEDGE-SPACE-001
     path: docs/scenarios/knowledge/SCN-KNOWLEDGE-SPACE-001.md
     children:
       - doc_id: UC-KNOWLEDGE-SPACE-BUILD-001
         scope: powerx
         layer: service
         domain: knowledge
         optional: false
         repo: powerx
         path: docs/usecases-seeds/SCN-KNOWLEDGE-SPACE-001/UC-KNOWLEDGE-SPACE-BUILD-001.md
   ```

   - 若新增子场景，请在 `children` 列表中补充新的 `doc_id` 并创建对应 Seed；同时更新场景文档 Links。

2. **复制模板到场景目录**

   ```bash
   mkdir -p docs/usecases-seeds/SCN-KNOWLEDGE-SPACE-001
   cp docs/usecases-seeds/_template.md \
     docs/usecases-seeds/SCN-KNOWLEDGE-SPACE-001/UC-KNOWLEDGE-SPACE-BUILD-001.md
   ```

   - 同场景 Seed 共用一个目录；如切换至 `docs/use_cases/_from_hub`，需同步 docmap 与 repo 配置。

3. **填充 Frontmatter**

   - `owners` 建议包含 Product/Platform（Amber）、Data Infra、Security Ops；`contributors` 可列 RAG/Feedback 团队。
   - `code_refs` 至少列出：
     - `services/knowledge_space/service.go`
     - `services/knowledge_space/pipelines/registry.go`
     - `services/ingestion/pipeline_orchestrator.go`
     - `pkg/audit/knowledge_logger.go`
   - `feature_flags`：`knowledge-space-v1`, `knowledge-ingestion`, `audit-streaming`（可追加子场景相关 flag）。

4. **完善正文章节**

   - `Usecase Overview`：重申 2 分钟 SLA、首轮入库 4 小时、chunk 覆盖率/嵌入成功率/安全启用。
   - `Context & Assumptions`：列出租户/IAM 先决条件、输入/输出、边界（不含前端 UI/Agent 消费）。
   - `Solution Blueprint`：拆解 Stage 1~4（Provision, Acquisition, Fusion & Index, Feedback），可沿用场景 mermaid。
   - `Contracts & Interfaces`：`POST /knowledge-spaces`, `PUT /knowledge-spaces/{id}/pipelines`, `POST /ingestion/jobs`, `knowledge.ingestion.completed`, `knowledge.feedback.created` 等。
   - `Implementation Checklist`～`Observability`：覆盖空间 API、管线治理、安全策略、反馈闭环、指标/告警/仪表盘。
   - `Rollback & Risks`：记录配置回滚、重试、人工工单、开放风险（API 配额、指标口径等）。

5. **互链场景**

   - Seed `References` 指向主场景、meta 文档、关键脚本；
   - 在场景文档 “Usecase Links” 中添加 `[UC-KNOWLEDGE-SPACE-BUILD-001](docs/usecases-seeds/SCN-KNOWLEDGE-SPACE-001/UC-KNOWLEDGE-SPACE-BUILD-001.md)`；需要时在子场景中回链主 Seed。

## 自检清单

- [ ] docmap 与 Seed frontmatter 完全一致（scope/layer/domain/path/optional）。
- [ ] Seed 正文覆盖业务目标、上下文、流程、契约、清单、测试、Ops、回滚、风险。
- [ ] 指标与告警阈值与场景一致（空间创建 ≤2m、chunk 覆盖率 ≥95%、嵌入成功率 100%、脱敏漏检=0、反馈 SLA ≤24h）。
- [ ] Feature Flag / 配置 / 脚本路径存在并受版本控制。
- [ ] `npm run lint`、`npm run docs:build`、`npm run publish:usecases -- --scn-id SCN-KNOWLEDGE-SPACE-001 --validate-only` 全部通过。

## 常见问题

| 问题 | 处理方式 |
|------|----------|
| docmap/Seed 字段不一致 | 以 docmap 为准，调整 frontmatter 与路径；运行 `publish:usecases --validate-only` 提前发现。 |
| 多租户并发创建导致配额冲突 | 在 `Solution Blueprint` 中写明配额校验与审计；需要时扩展 API 返回 `409` 并生成工单。 |
| 入库任务堆积 | 配置 `ingestion.max_concurrency`，在 Observability 中跟踪 `ingestion.queue_depth`，必要时自动扩容 Workers。 |
| 安全策略禁用 | 默认禁止关闭脱敏/审计；若强制关闭需 double-confirm，Seed 中记录审批流程与审计字段。 |
| 反馈回路与空间构建脱节 | 在 Checklist 中加入“为空间开启 feedback pipeline”；缺失时 `knowledge.feedback.created` 不应抛弃。 |

完成以上内容后，可提交 PR 并通知 `@knowledge-platform-stewards` 评审，随后按发布脚本同步至下游仓库与领导力视图。EOF
