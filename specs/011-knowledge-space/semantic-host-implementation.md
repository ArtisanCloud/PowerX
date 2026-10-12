# 通用语义索引与检索 Core 实施

日期：2026-10-08。范围仅 Core，Framework Go/.NET 对齐清单作为交付材料；不修改 Framework/插件仓库。

## 合同决定

- 复用文档 UUID、IndexJob 异步任务、单篇/空间重建和 ActiveIndexJobUUID 原子切换。
- 文档提交增加显式 indexing、artifacts、external_ref、idempotency_key；旧 lexical 合同不改变。
- 语义索引使用实际 Embedding，首个存储驱动 pgvector；不以 hash 或 noop 宣称成功。
- 空间索引配置 generation 与文档 bundle 发布 generation 分开；查询返回逐空间版本集合。
- 自然语言检索路径 POST /api/v1/tenant/knowledge/retrieval/query，固定 typed core://knowledge/retrieval INVOKE。
- 来源使用 Unicode codepoint 半开区间；画像定位其自己的不可变快照。
- 保留全部相关分块、raw score/type、融合 score/type及来源；业务适配分由插件负责。
- 查询/配置/撤销校验租户及可信凭据；必需通道失败明确错误。
- staged 写入后原子发布 bundle；失败保留旧版本，撤销与删除立即不可查询。

## 工作顺序与验收状态

- [x] S01 合同/OpenAPI/Schema、模型与迁移
- [x] S02 距离/相似度方向、真实模型版本和维度绑定
- [x] S03 原文/画像 bundle、幂等、快照与旧版本保护
- [x] S04 严格自然语言检索、前置过滤、多分块及版本一致性
- [x] S05 删除/资格/缓存与结构化失败
- [x] S06 Admin JWT/RBAC + API Key/STS typed service 授权与能力治理
- [x] S07 回归、真实模型/存储与双鉴权 API 验收
- [x] S08 Framework 对齐清单及请求/响应/证据交付

未完成项目不能仅凭 schema、mock、构建或 completed 称作语义能力已交付。

## 2026-10-08 验收结果

- 已执行实际开发库 migrate（模型、binding/pending configuration、任务计数、可查询资格字段及模型 env）；启动不执行迁移。
- `make capability-check`：declared=1022、referenced=60、rest_routes=936、candidates=978、ignored=6，通过。
- `make capability-seed`：1022 capability、4 个 active tenant registration 同步完成；开发 Host Key retrieval.read grant-status=granted；IAM allow_api_key/api_key_explicit 真实回读断言通过。
- Go 回归：knowledge_space、Admin/tenant transport、capability_registry、API-key permissions、HTTP auth、Ollama strict driver、Host snapshot 包通过；受影响 service/transport go vet 与 app 构建通过。
- 最终隔离服务使用真实 Ollama bge-m3/bge-large + pgvector + 开发 DB/Redis。75 条验收记录：原文/画像 640/80 实际分块与向量、同义召回、两路 hybrid、前置 top_k 过滤、多块来源、故障/超长模型输入、单篇/空间重建、pending 模型原子发布、幂等跨模型变更回读、跨租户/授权撤销、Admin JWT/RBAC 和 STS 边界。
- 原文/画像初始 bundle 2 artifacts/3 chunks/3 verified vectors；切换 bge-large 时先证明 640 超限明确失败，再以128/16完整空间发布11 vectors，失败时仍可查询旧 bge-m3 generation。
- [Core 合同](../../docs/contracts/knowledge-semantic-host.md)、[Framework 对齐](../../docs/contracts/knowledge-semantic-framework-alignment.md)、[实际响应与 Trace](../../docs/contracts/knowledge-semantic-acceptance-20261008.json)。

Core 交付项完成。部署运行需重启8077加载新代码；Framework及插件运行/浏览器验收由消费端执行，不能用此处 Core 验收替代。
