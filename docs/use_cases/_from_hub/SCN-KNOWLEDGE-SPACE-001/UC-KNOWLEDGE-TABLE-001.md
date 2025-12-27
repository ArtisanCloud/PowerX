---
scn_id: "SCN-KNOWLEDGE-TABLE-001"
scenario_name: "表格主题建模与实体映射"
slug: "knowledge-table-ingestion"
primary_scope: "powerx"
primary_layer: "data"
primary_domain: "knowledge"
primary_repo: "powerx"
doc_owner: "Grace Yang / Structured Data PM / structured@artisan-cloud.com"
last_generated_at: "2025-11-12"
---

# 表格主题建模与实体映射 Usecase Seed 生成指南

> 场景摘要：针对 Excel/CSV 等结构化表格，需自动识别主键/时间/枚举字段并匹配语义模板，生成“供应商-合同-费用项”等实体关系，确保字段识别准确率 ≥95%、脱敏覆盖率 100%，并把 chunk 写入向量/关键词索引与知识图谱。

## Seed 的定位

- 描述 `powerx` 仓库在 `SCN-KNOWLEDGE-TABLE-001` 场景中的职责：schema 解析、实体映射、脱敏阻断、索引/图谱写入、审计与告警。
- 与 `docs/_data/docmap.yaml` 中 `doc_id: UC-KNOWLEDGE-TABLE-001` 节点完全一致，是 `publish:usecases`、`publish:collected` 的单一来源。
- 指导各团队维护 usecase seed，保证分发脚本、指标与子场景文档对齐。

## 前提条件

- 场景文档：`docs/scenarios/knowledge/SCN-KNOWLEDGE-TABLE-001.md`；主场景：`docs/scenarios/knowledge/SCN-KNOWLEDGE-SPACE-001.md`。
- docmap：`scope: powerx`, `layer: data`, `domain: knowledge`, `path: docs/usecases-seeds/SCN-KNOWLEDGE-SPACE-001/UC-KNOWLEDGE-TABLE-001.md`。
- repos：`docs/_data/repos.yaml` 中 `key: powerx`，`usecase_seed_root = docs/use_cases/_from_hub`，`usecase_seed_legacy_root = docs/usecases-seeds`。
- Feature Flags：`structured-ingestion`, `masking-enforced`, `graph-sync`, `search.hybrid-index`。
- 外部依赖：字段模板库 `docs/knowledge/templates/structured-fields.md`、配置目录 `backend/config/structured/templates/*.yaml`、指标脚本 `reports/_state/ingestion-structured.json`、对象存储凭证、KMS 脱敏策略。

> **注意**：上传样例 `expense-2024.xlsx`（正向）与 `expense-sensitive.xlsx`（脱敏阻断）必须在 QA 环境可读。

## 生成流程

1. **docmap 子节点**

   ```yaml
   - scn_id: SCN-KNOWLEDGE-SPACE-001
     children:
       - doc_id: UC-KNOWLEDGE-TABLE-001
         scope: powerx
         layer: data
         domain: knowledge
         optional: false
         repo: powerx
         path: docs/usecases-seeds/SCN-KNOWLEDGE-SPACE-001/UC-KNOWLEDGE-TABLE-001.md
   ```

   - 若调整路径或 optional 状态，需同步 Seed frontmatter 与 docmap。

2. **复制模板**

   ```bash
   mkdir -p docs/usecases-seeds/SCN-KNOWLEDGE-SPACE-001
   cp docs/usecases-seeds/_template.md \
     docs/usecases-seeds/SCN-KNOWLEDGE-SPACE-001/UC-KNOWLEDGE-TABLE-001.md
   ```

3. **填充 Frontmatter**

   - `owners`：Structured Data PM、Data Modeling Lead；`contributors`：Security Ops（脱敏）、Knowledge Platform（图谱）。
   - `code_refs`：
     - `services/ingestion/structured/schema_detector.go`
     - `services/ingestion/structured/entity_mapper.go`
     - `services/security/masking/policies.go`
     - `services/search/index_writer.go`
     - `services/graph/entity_writer.go`
   - `feature_flags`：`structured-ingestion`, `masking-enforced`, `graph-sync`（可附 `search.hybrid-index`）。

4. **完善正文章节**

   - `Usecase Overview`：强调字段识别准确率 ≥95%、模板匹配 ≥90%、脱敏覆盖率 100%、结构化检索 ≤1s。
   - `Context & Assumptions`：列上传渠道、模板库、敏感字段清单、输入/输出、边界（不含长文/API 数据）。
   - `Solution Blueprint`：拆解 Schema Detection → Entity & Chunk Build → Security & Index → QA & Publish，可沿用场景序列图。
   - `Contracts & Interfaces`：`POST /ingestion/structured`, `POST /structured/templates/{id}/apply`, Event `knowledge.ingestion.blocked`、`knowledge.ingestion.published`，配置 `structured_templates.yaml`, `masking_policies.yaml`。
   - `Implementation Checklist`：字段解析、模板管理、脱敏策略、索引/图谱写入、审计。
   - `Testing Strategy`：正向/阻断用例、模板冲突、脱敏失败注入；`Observability`：`structured.field_match_rate`, `structured.masking_block_total`, `structured.index_latency_p95`；`Rollback`: 恢复模板/策略、重新发布 chunk。

5. **互链与发布**

   - 在 References 中链接场景文档、模板库、样例文件、脚本。
   - 主场景与子场景 “Usecase Links” 均应指向该 Seed。
   - 发布前执行：
     ```bash
     npm run lint
     npm run docs:build
     npm run publish:usecases -- --scn-id SCN-KNOWLEDGE-SPACE-001 --validate-only
     ```

## 自检清单

- [ ] docmap/Seed 字段完全一致；`optional: false`。
- [ ] Seed 正文覆盖业务目标、上下文、流程、契约、清单、测试、Ops、回滚、风险。
- [ ] 指标与告警阈值符合场景（字段识别 ≥95%、脱敏覆盖 100%、阻断提示、结构化响应 ≤1s）。
- [ ] Template/配置/脚本路径真实存在且可被仓库访问。
- [ ] 提交 PR 前已附验证命令输出。

## 常见问题

| 问题 | 处理方式 |
|------|----------|
| 模板匹配失败率高 | 在 Seed 中记录如何添加新模板（`docs/knowledge/templates`) 并启用 `structured.template-learning` Flag；必要时回滚到默认模板版本。 |
| 脱敏策略漏配 | `masking-enforced` 需设置为强制，Seed 的 Rollback 节描述人工补齐策略与审计流程。 |
| Excel 多表字段冲突 | 在 `Solution Blueprint` 中说明冲突检测与人工审核队列，必要时阻断并生成 `knowledge.ingestion.blocked` 事件。 |
| 向量/关键词索引不一致 | Observability 中加入比对指标；出现偏差时触发脚本 `scripts/ingestion/structured-compare.mjs` 重新生成。 |
| 结构化查询延迟 >1s | 在 FAQ 或 Risks 中记录扩展索引分片/缓存策略，或降级到关键词索引。 |

完成以上步骤后，可提交 PR 并 @knowledge-platform-stewards 复核，再执行 Seed 分发与站点构建。EOF
