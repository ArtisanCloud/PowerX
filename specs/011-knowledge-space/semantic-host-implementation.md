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

- [ ] S01 合同/OpenAPI/Schema、模型与迁移
- [ ] S02 距离/相似度方向、真实模型版本和维度绑定
- [ ] S03 原文/画像 bundle、幂等、快照与旧版本保护
- [ ] S04 严格自然语言检索、前置过滤、多分块及版本一致性
- [ ] S05 删除/资格/缓存与结构化失败
- [ ] S06 Admin JWT/RBAC + API Key/STS typed service 授权与能力治理
- [ ] S07 回归、真实模型/存储与双鉴权 API 验收
- [ ] S08 Framework 对齐清单及请求/响应/证据交付

未完成项目不能仅凭 schema、mock、构建或 completed 称作语义能力已交付。
