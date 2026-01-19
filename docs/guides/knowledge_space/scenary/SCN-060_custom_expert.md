# SCN-060：自定义场景（专家）

> 目标：把“场景定义”和“策略包选择”开放给专家用户，但仍受依赖校验与成本护栏约束。

## 适用范围

- 你明确知道：内容结构、主要任务意图、需要哪些索引通道，以及如何验收质量/成本。

## 三者关联（L1/L2/L3）

- L1（场景）：你自行定义“内容结构 × 任务意图”
- L2（策略包）：选择 P0–P3 的成本/护栏级别
- L3（模块）：按需组合 `rag.md` 的模块（A–O），但必须满足索引/资产依赖  
  参考：`docs/guides/knowledge_space/scenary/MAP_scene_bundle_rag_profiles.md`

## 分割策略（Segment Defaults）

> 自定义（专家）场景默认尽量“不自作主张”，优先保留处理器输出，让专家根据数据源特征自行调参。

- 推荐分段模式：`unit`（沿用处理器输出；必要时再切换到 `heading/semantic/table_row/...`）

| 策略包 | segmentMode | chunkSize | overlap | 说明 |
| --- | --- | ---:| ---:| --- |
| P0 | `unit` | 0 | 0 | 最小干预 |
| P1 | `unit` | 0 | 0 | 默认（推荐） |
| P2 | `unit` | 0 | 0 | 需要更强护栏时再配细粒度分段 |
| P3 | `unit` | 0 | 0 | KG 强约束通常需要专用 processor/profile |

## UI 操作步骤（推荐路径）

1. 场景选择：`自定义场景（专家）`
2. 选择策略包：可选 `P0/P1/P2/P3`
3. 如果需要更细粒度配置：进入 profile（Ingestion/Index/RAG）做参数级调整（chunk_size/overlap/topK/预算等）
4. 必须跑 Corpus Check，并且通过依赖校验（否则不允许发布为 active）

## 验收检查点

- 任何高级能力（KG/sparse/hier/time-aware）未就绪时，UI 必须阻止发布并给出修复指引。
