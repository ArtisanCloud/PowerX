---
scn_id: "SCN-KNOWLEDGE-LONGDOC-001"
scenario_name: "长篇文档分层切分与入库"
slug: "knowledge-longdoc"
primary_scope: "powerx"
primary_layer: "data"
primary_domain: "knowledge"
primary_repo: "powerx"
doc_owner: "David Wen / Data Ingestion Lead / data-infra@artisan-cloud.com"
last_generated_at: "2025-11-12"
---

# 长篇文档分层切分与入库 Usecase Seed 生成指南

> 场景摘要：针对 300 页以上的 PDF/手册，通过 OCR + 结构解析 + 语义切分生成多粒度 chunk（≈800 token 摘要 / ≈300 token 段落），并同步向量、关键词、图谱实体以及 QA 报告，确保 RAG 引用可追溯且可控。

本文档面向 `powerx` 仓库的 Data Infra & Knowledge Platform Stewards，指导如何把“长篇文档分层切分与入库”场景拆解为可交付的子用例 Seed。

## Seed 的定位

- 约束 `powerx` 仓库在 `SCN-KNOWLEDGE-LONGDOC-001` 场景下需要交付的 OCR、切分、嵌入与图谱写入职责。
- 与 `docs/_data/docmap.yaml` 中 `scn_id: SCN-KNOWLEDGE-SPACE-001` → `doc_id: UC-KNOWLEDGE-LONGDOC-001` 的节点保持一一对应。
- Seed 数据会被 `npm run publish:usecases` 分发到下游、也会被 `npm run publish:collected` 纳入领导视图，因此字段必须准确。

## 前提条件

- 场景文档：`docs/scenarios/knowledge/SCN-KNOWLEDGE-LONGDOC-001.md` 已定义业务链路与指标。
- docmap: `docs/_data/docmap.yaml` 已登记 `UC-KNOWLEDGE-LONGDOC-001`，scope/layer/domain = `powerx/data/knowledge`。
- repo 定义：`docs/_data/repos.yaml` 中 `key: powerx`，`usecase_seed_root`=`docs/use_cases/_from_hub`，`usecase_seed_legacy_root`=`docs/usecases-seeds`。
- 依赖：OCR GPU 资源、`ocr.enabled`/`longdoc.chunking`/`graph.sync` Feature Flags、加密对象存储 bucket、审计日志服务。

> **补充依赖**：需要可用的 `scripts/ingestion/longdoc-batch.mjs`（批量触发）、`reports/_state/ingestion-longdoc.json`（指标抽取）以及样例文档《财务合规白皮书.pdf》。

## 生成流程

1. **登记/更新 docmap 子节点**

   ```yaml
   - scn_id: SCN-KNOWLEDGE-SPACE-001
     title: 知识空间构建
     children:
       - doc_id: UC-KNOWLEDGE-LONGDOC-001
         scope: powerx
         layer: data
         domain: knowledge
         optional: false
         repo: powerx
         path: docs/usecases-seeds/SCN-KNOWLEDGE-SPACE-001/UC-KNOWLEDGE-LONGDOC-001.md
   ```

   - `doc_id` 必须与 Seed 文件名完全一致（含大小写与破折号）。
   - `path` 指向 docs 仓内的种子文件；下游发版脚本会按该路径复制至 repo 配置的 `usecase_seed_root`。

2. **复制模板并放置到对应目录**

   ```bash
   mkdir -p docs/usecases-seeds/SCN-KNOWLEDGE-SPACE-001
   cp docs/usecases-seeds/_template.md \
     docs/usecases-seeds/SCN-KNOWLEDGE-SPACE-001/UC-KNOWLEDGE-LONGDOC-001.md
   ```

   - Knowledge 场景沿用“按场景分目录”的 legacy 结构；如需切换到 `docs/use_cases/_from_hub`, 请同步 docmap `path`。
   - 建议 commit 钩子校验 `UC-*` 命名与 docmap 字段，避免脚本找不到文件。

3. **填充 Frontmatter 区域**

   - 关键字段：`doc_id`, `scn_id`, `repo_key`, `scope`, `layer`, `domain`, `optional`, `last_reviewed_at`。
   - `owners` 建议包含 Data Ingestion Lead + Knowledge Platform 守护人；`linked_requirements` 指向场景编号或任务 ID。
   - **Notes**：保持 `feature_flags` 与场景一致（`longdoc-chunking`, `ocr-v3`, `graph-sync`），并列出核心代码路径（`services/ingestion/longdoc/*`、`services/embedding/*`、`services/graph/*`）。

4. **完善正文章节**

   - `Usecase Overview`：突出 chunk 覆盖率、嵌入成功率、图谱准确率等指标。
   - `Context & Assumptions`：写明输入（PDF、配置）、输出（chunk、向量、QA 报告）、边界（不含空间创建/表格处理）。
   - `Solution Blueprint`：拆解 Intake → Chunking → Embedding → Publish 流程，可加入 mermaid 时序（Engineer → OCR → Chunker → Embed → Graph）。
   - 需要列出关键 API（`POST /ingestion/longdoc`, `GET /ingestion/{job_id}`）、事件（`knowledge.chunk.failed`）与 schema。
   - `Testing Strategy` / `Observability` / `Rollback` / `Risks` 必须结合长文档特有的 OCR/结构解析失败、GPU 容量、人工工单等细节。

5. **与场景文档互相链接**

   - 在场景文档 “Usecase Links” 表格中引用该 Seed（路径同 docmap）。
   - Seed 中引用 `SCN-KNOWLEDGE-LONGDOC-001`，确保读者知道业务背景；如新增 ADR/标准，请同步 `docs/standards/**`。

## 自检清单

- [ ] docmap 与 Seed frontmatter 字段完全一致（尤其是 `scope/layer/domain/path`）。
- [ ] Seed 正文覆盖目标、流程、接口、测试、Ops、回滚、风险六大块，并引用真实代码/脚本。
- [ ] 指标、告警、Feature Flag 与场景文档保持一致；新增指标需更新 `reports/_state/ingestion-longdoc.json`。
- [ ] `npm run lint && npm run docs:build` 通过；`npm run publish:usecases -- --scn-id SCN-KNOWLEDGE-SPACE-001 --validate-only` 无报错。
- [ ] 需要 GPU/OCR 额度或密钥的部分在 Appendix/Dependencies 中明确说明责任团队。

## 常见问题

| 问题 | 处理方式 |
|------|----------|
| OCR 或结构解析失败频繁 | 使用 `knowledge.chunk.failed` 事件生成工单，必要时在 Seed 的 Rollback 一节加入人工校验流程，并在指标中跟踪失败率。 |
| 长文档文件过大导致上传超时 | 在 Seed 中记录分片上传脚本（如 `scripts/ingestion/upload-longdoc.sh`），并在 `Context` 中说明最大文件尺寸 & 分片策略。 |
| 需落地多租户差异化策略 | 在 docmap 中复制新的 `doc_id`（如 UC-KNOWLEDGE-LONGDOC-INTL-001），或在现有 Seed 的 `Variations` 小节注明配置矩阵。 |
| 指标/脚本缺失 | 先更新 `reports/_state/ingestion-longdoc.json` & 相关脚本，再回填 Seed；避免脚本发布后缺字段导致分发失败。 |

完成上述步骤后即可进入 Seed 分发和复核流程，若涉及新依赖或跨仓交付，请在创建 PR 时 @knowledge-platform-stewards 评审。
