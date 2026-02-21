# 知识空间（Knowledge Space）UI 使用指南

> 面向租户业务用户与运营同学：按界面操作完成「创建空间 → 入库 → Playground 验证」的最短路径，并附常见问题排查入口。

## 1. 你应该先看哪篇文档？

- **界面怎么用（本页）**：`docs/guides/knowledge_space/ui_guide.md`
- **运维 / 排障（SRE）**：`docs/guides/knowledge_space/runbook.md`
- **发布前冒烟清单**：`docs/guides/knowledge_space/smoke_checklist.md`
- **性能 / 弹性验证**：`docs/guides/knowledge_space/perf_validation.md`
- **规格与合同（研发对齐）**：`specs/011-knowledge-space/quickstart.md`、`specs/011-knowledge-space/spec.md`

## 1.1 新增能力提示（Dense 向量索引动态分表）

从 011-knowledge-space 的最新实现开始：
- Dense 向量表采用 **按维度分表**（例如 `knowledge_vectors_v1_1536`、`knowledge_vectors_v1_1024`），并且是 **全局共享表**（通过 `space_uuid` 隔离）。
- 只有当某个 space 显式 **激活向量索引（Dense）** 后，该 space 的入库才会写入向量；否则入库会以 “degraded（无向量）” 完成。
- AI Settings 的 “测试连接” **会建表**：完成 probe 后会创建 `knowledge_vectors_v1_<D>`（若不存在），并写回 `ai_model_profiles.cap_cache`（`probed_at`/`dimensions`）。

## 2. 创建知识空间（/knowledge-spaces/create）

目标：只创建“空间容器”，不在此页做入库。

1. 进入「知识空间」→ 点击 **新建空间**
2. 填写 **空间名称**
3. 选择 **所属部门**
   - 这里是“选择”，不是输入（部门来自组织架构）
4. 点击 **创建空间**
5. 创建成功后，点击 **返回总览**（下一步入库在总览页完成）

说明：
- **不会要求你输入租户 UUID**：系统使用当前登录上下文（租户侧自助创建场景）。

## 2.1 空间很多时怎么找（50+ 空间）

建议使用总览页的三件事：
- **搜索**：按空间名/部门/状态/ID 前缀搜索
- **筛选**：按部门、状态筛选
- **分页**：10/20/50 条每页

## 3. 入库导入（/knowledge-spaces → 打开入库）

目标：将文档或远程 URL 作为来源，触发一次入库作业。

1. 回到「知识空间总览」
2. 在目标空间那一行点击 **入库**（会以 Modal 弹层打开）
3. 选择：
   - **空间**（space）
   - **来源类型**（如 PDF / Markdown / 表格）
   - **来源方式**（本地上传 / 远程 URL）
4. 提供来源：
   - 本地上传：选择文件即可
   - 远程 URL：填写 `sourceUri`（必须是后端可直接抓取的公开地址或预签名 URL）
5. 点击 **立即入库**
6. 成功后会看到“最新任务 / 最近任务”摘要（jobId、状态、覆盖率等）

说明（重要）：
- 当前 UI **不会在入库表单里收集 API Token/鉴权 Header**。如要对接私有 API、企业网盘、受控站点等，需要走后续的「连接器/同步任务/凭据管理」能力（与 `docs/plan/AI_engineering/knowledge/knowledage_base.md` 的多源接入规划一致）。

## 3.1 对接 Notion / 飞书（API 鉴权接入）

目标：把 Notion/飞书这类“需要鉴权”的系统接入到某个知识空间，并通过**定时增量同步任务**持续更新。

入口：
1. 回到「知识空间总览」
2. 在目标空间那一行点击 **连接数据源**
3. 按向导完成：选择数据源 → 授权（租户级复用）→ 选择同步范围 → 创建增量同步任务

说明：
- 该路径是“API 接入”的正确形态：**连接器 + 凭据 + 同步任务**，而不是在入库表单里输入 URL/Token。

## 4. Playground 验证（/knowledge-spaces/playground）

目标：用同一 query 对比不同策略/Profile 的检索效果（耗时、候选数、降级原因、citations 等）。

1. 进入 Playground
2. 选择 space / profile
3. 输入 query，观察：
   - latency
   - candidates
   - degrade_reason
   - citations 覆盖率

## 4.1 策略配置 + 激活向量索引（Dense）（/knowledge-spaces/strategy）

目标：把 “场景/策略包（L1/L2）” 写入 space，并在需要时激活 Dense 向量索引。

1. 在「知识空间总览」中选择目标空间，点击 **策略**
2. 选择业务场景（L1）与策略包（L2），点击 **保存到空间**
3. 在同一页面的「激活向量索引（Dense）」面板：
   - 填写 `EmbeddingProfileKey`（格式：`provider/model`，例如 `openai/text-embedding-3-small`）
   - 点击 **激活**

激活时后端会自动执行：
- probe embedding 维度（真实调用 embedding）
- `CREATE TABLE IF NOT EXISTS public.knowledge_vectors_v1_<D>`
- 写入 `powerx.knowledge_vector_indexes`
- 更新 `powerx.knowledge_spaces.embedding_profile_key` 与 `active_vector_index_key`

如果你看到“Dense 依赖未满足（dense_required）”，通常代表：
- 该 space 还没激活向量索引（未绑定 embedding profile / 未创建表）
 - embedding profile 尚未完成 probe（`cap_cache.probed_at`/`dimensions` 为空）

## 5. 常见问题（先看现象 → 再看入口）

### 5.1 部门树/部门选择为空

现象：
- 组织架构里创建部门成功，但列表不刷新
- 接口返回空 / `data: null`

建议：
- 先刷新页面或重新打开「组织架构」
- 如果刚执行过 `make db-refresh`：请 **退出重新登录**（旧 token / cookie 可能仍指向旧租户，导致部门树为空但不一定报错）
- 若用 root 账号：确认请求带了正确的租户上下文（Web 管理台会通过 `JWT claims（tid/tenant_uuid）` 注入）
- 后端排查入口：`docs/guides/knowledge_space/runbook.md`

### 5.2 控制台出现 `[intlify] Not found ...`（缺少文案 key）

现象：
- 控制台提示 `knowledgeSpaces.ingestion.*` 等 key 不存在

建议：
- 这是前端 i18n 文案缺失或 key 不一致导致的告警
- 先更新到最新代码并重启 `web-admin`

### 5.3 `unexpected status 503 Service Unavailable: {"message":"No available OpenAI account"}`

含义：
- 系统在执行某些需要模型/Provider 的能力时，**没有找到可用的 OpenAI（或兼容）账号/路由**（账号池为空、被禁用、配额耗尽、或策略未命中）。

建议排查顺序：
1. 在管理台 **AI 设置 / Provider 配置** 里确认有可用账号（并处于启用状态）
2. 检查当前租户是否有匹配到路由/策略（如果系统是多租户隔离 provider）
3. 查看后端日志定位触发 503 的具体接口与 trace_id
