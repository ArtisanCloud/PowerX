# SCN-050：SQL/配置/依赖关系库（KG 强）

> 默认推荐：**P3 KG 约束**（P1 可作为兜底）。

## 适用范围

- SQL DDL/DML、配置文件、系统依赖关系、数据字典（实体/关系是核心价值）。

## 前置条件（必须）

- 索引能力：`kg`（必须）+ `sparse` + `dense(摘要/说明)`。
- 需要实体/关系抽取产物，并能溯源到 chunk/source（KG provenance）。

## 建库策略（Ingestion/Index）

- 典型内容结构：对象（表/视图/函数/配置项）+ 依赖关系（引用/调用/影响）。
- 建议索引：`kg + sparse + dense`（dense 主要作为解释/摘要补充）

## 在线 RAG 策略（L3，来自 rag.md）

默认会启用的模块（系统实际组合见总表）：
- `K_kg`, `H_fusion`, `C_context_enriched`, `L_feedback`

参考：
- 映射总表：`docs/guides/knowledge_space/scenary/MAP_scene_bundle_rag_profiles.md`
- 模块定义：`docs/plan/AI_engineering/knowledge/rag.md#5.2`

## 分割策略（Segment Defaults）

> SQL/配置/依赖的关键是“对象/块边界 + 依赖关系”。可在入库向导第 3 步覆盖。

- 推荐分段模式：`code_block`（按块/段落切分；后续会升级为 AST/对象级切分）

| 策略包 | segmentMode | chunkSize | overlap | 说明 |
| --- | --- | ---:| ---:| --- |
| P0 | `code_block` | 1200 | 0 | 快速验收 |
| P1 | `code_block` | 900 | 120 | 通用 |
| P2 | `code_block` | 800 | 160 | 更细粒度，利于精确引用 |
| P3 | `code_block` | 900 | 180 | KG 强约束（默认） |

## UI 操作步骤（推荐路径）

1. 选择场景：`SQL/配置/依赖（KG 强）`
2. 选择策略包：`P3 KG 约束`
3. 导入一批最小样本（建议：1 个 schema + 2–5 个关键 SQL/配置）
4. Playground 验证：
   - “表 A 依赖哪些表/字段？”
   - “服务 X 的配置项来自哪里？有什么约束关系？”

## 验收检查点

- 能给出实体/关系链路，并能引用到来源。
- 如果提示 `kg_required`：说明 KG 索引/辅助表未就绪，先按系统修复指引完成建索引/跑体检。
