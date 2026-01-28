# RAG 策略分支规划（以策略包为主线）

> 说明：`A_simple`（最小闭环/基础策略）已在当前分支 `011-knowledge-space` 内完成。

## 分支清单（每策略一分支）

| 分支 | 策略包 | 配置界面重点 | 备注 |
| --- | --- | --- | --- |
| `011-knowledge-space` | `A_simple` | 无独立页面（沿用当前 UI） | 当前分支落地 |
| `012-knowledge-rag-kg` | `K_kg` | KG 抽取/Schema/关系约束/融合权重 | 高复杂 |
| `013-knowledge-rag-hier` | `J_hier` | doc/section/chunk 召回比例与下钻规则 | 高复杂 |
| `014-knowledge-rag-semantic-chunking` | `B_semantic` | 语义边界阈值/兜底切分/锚点保留 | 中复杂 |
| `015-knowledge-rag-doc-augmentation` | `D_doc_augmentation` | 摘要/关键词/Q&A/实体标签产物与版本 | 中复杂 |
| `016-knowledge-rag-crag` | `O_crag` | 证据校验器/触发条件/回退策略 | 高复杂 |
| `017-knowledge-rag-self-rag` | `N_self_rag` | 自检回路/最大回路/触发策略 | 高复杂 |
| `018-knowledge-rag-adaptive` | `M_adaptive` | 路由规则/动态 topK/成本门控 | 高复杂 |
| `019-knowledge-rag-fusion` | `H_fusion` | 多索引权重/RRF/去重/归一化 | 中复杂 |
| `020-knowledge-rag-time-aware` | `A2_time_aware` | 时间字段映射/版本策略/衰减 | 中复杂 |
| `021-knowledge-rag-query-transform` | `E_query_transform` | 改写模板/候选数/触发条件 | 中复杂 |
| `022-knowledge-rag-hyde` | `I_hyde` | 触发门槛/成本上限/失败回退 | 中复杂 |
