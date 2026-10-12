# dev语义索引准备验收

日期：2026-10-08。当前用户运行的8077正式API验收；范围为指定五份业务文档。

## 当前结果

- configured=true、ready=true、indexed=true。
- indexed_query_modes=[semantic,hybrid]，实际hybrid文档5份、向量/分块23个。
- 模型key：ollama/bge-m3；环境：dev；实际维度：1024。
- Embedding Profile：2ff1b71c-5fbe-4753-9c17-8ca3a85cdce2 / v1。
- 实际Ollama修订：`7907646426070047a77226ac3e684fbbe8410524f7b4a74d02837e43f2146bab`。
- configuration_generation：`5dc82f2a-89ec-4e76-b4ea-aa2c47bc7087`。
- corpus_generation：`6aaa1cae-0670-4317-b6b6-0dc963fce17f`。
- 空间：`30a6d654-c4c5-4a33-9806-6d66c6728187`；租户：`6b5d0240-9920-46da-b707-88200e0f51ea`。

## 文档与任务

| 文档 | 原Core文档UUID | 新hybrid任务UUID | 实际vector_count |
| --- | --- | --- | --- |
| framework-business-modules | f2a361a2-597d-46fc-808b-49ce6be70a0b | ed5f803e-bd47-44e0-99aa-7e1ea10de8fb | 2 |
| framework-dual-mode-business-modules | aebd5f7a-384a-4c19-9b73-0165000c5a19 | 927a248a-679a-4126-af0e-bc9973516f3b | 6 |
| framework-host-fullscreen | dc579694-39a1-43ec-9526-22bb9d107f0e | 989df2ce-68f5-4139-825b-04a615017416 | 7 |
| framework-provider-mode | 81767ec9-0199-4112-9044-51a634541344 | 592ec405-0501-4cb7-887a-8ae8935fc0cd | 2 |
| framework-release | 594f4f6c-9529-49d0-b3d9-93f5ac3cbc83 | 1b7f335f-aa76-4ce0-ae3d-372e8c054dee | 6 |

五个任务均succeeded，artifact_count=1；输入来自数据库保存的真实原文，source_chunk/source_ref.kind=raw_document，Source UUID使用Core文档UUID。插件external_ref UUID取原plugin-knowledge URI。原1024/0分块配置、Profile UUID/version和策略冻结保持一致。

## 检索验收

对全屏请求、Provider模式、模块边界、Framework发布和接入指南分别使用完整自然语言查询，semantic/hybrid各5次，共10次真实200响应。四类问题的目标文档排名1；模块边界问题的目标文档排名2，排名1为相邻的业务模块入口文档。返回真实片段、cosine原始距离/相似度或RRF、来源及精确Unicode codepoint区间，并逐条与保存的原文/checksum/version验证。

检索保留同文档多个分块。本轮验证召回与来源，生成答案和引用属于后续QA流程；插件浏览器检索由消费端验收。

## 数据保留与问题修复

- 空间所有字段与准备前完全一致：p1_general、h_fusion、product_specs、三个Profile绑定、状态、配额及原向量索引均保留。
- 五份原文、URI、Core UUID、版本、checksum、创建时间和queryable不变。
- 另外六份验收样本文档完全未变；五个旧任务和23个旧分块逐项回读一致。
- 新向量在实际pgvector表逐任务回读，数量2/6/7/2/6，维度均1024。
- 首次提交遇到历史visibility_epoch=NULL转空UUID的503。仅为这五个UUID补齐缺失的系统epoch；原文/可查询资格不变。Core保存逻辑已补充缺失epoch初始化，附回归用例，通过service/HTTP包测试与go vet。

当前8077上的数据已可用于插件浏览器检索，无需为了本轮验收重启。代码修复在下次重新编译/重启8077后生效；未替用户重启或启动额外后台服务。

## 证据与备份

- [正式请求结果、Trace与检索响应](../evidence/knowledge-dev-semantic-preparation-20261008.json)。凭证脱敏。
- 原始备份：/tmp/powerx-dev-semantic-preparation/before.json（仅本机，含真实原文、旧快照/分块）。
- 本机操作与核对脚本：/tmp/powerx-dev-semantic-preparation/prepare.py、verify.py。
- [Core正式合同](../knowledge-semantic-host.md)、[Framework对齐](../knowledge-semantic-framework-alignment.md)。
