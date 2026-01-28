# 入库产物、文件存储与 Embedding 落地说明

本文回答几个“入库到底做了什么”的关键问题：文件放哪、会不会删、OCR 怎么做、Embedding/索引落在哪、pgvector 需要怎么准备与如何自检。

## 1) 文件（原始 PDF/Doc 等）保存在哪里？

### 目标形态（推荐）
- **原始文件**：进入 **Media 资产**（MinIO/S3 对象存储），形成可追溯的 `source_uri`（例如 `minio://...` 或可抓取的 HTTPS URL）。
- **入库任务**：`KnowledgeSpace` 触发 `IngestionJob`，`IngestionJob.source_uri` 指向上述可访问地址，后端 Worker 负责抓取与处理。
- **产物清单（manifest）**：处理后生成 `ArtifactBundle`，其中记录：
  - `chunk_manifest_uri`：切块清单（MinIO/S3）
  - `vector_manifest_uri`：向量清单（MinIO/S3）
  - `graph_manifest_uri`：图谱清单（MinIO/S3，可选）

### 当前实现状态（你现在看到的现象原因）
Web Admin 的“上传”会先把文件 **上传到 Media（对象存储）**，再为入库 Worker 生成一个 **可抓取的 presign 下载 URL** 作为 `source_uri`。

- “保留到媒体库”开关：
  - 开启：作为媒体资产保留（便于追溯、复用与审计）
  - 关闭：仍会临时上传用于入库抓取（浏览器文件无法被后端直接读取），后续按策略补齐自动清理

## 2) OCR 怎么做？怎么抽取内容并切块？

处理原则（从便宜到昂贵）：
1. **优先读取文本层**：PDF/Doc/HTML/Markdown 等若本身含文本层，直接抽取。
2. **缺失或质量差时启用 OCR**：扫描件/图片型 PDF 才需要 OCR（你在 UI 里看到的 OCR 建议就是这个逻辑的入口）。
3. **切块策略**：
   - 默认会产出 `doc_summary / section_summary / chunk` 三类向量块（见 `backend/tests/integration/knowledge_space/ingestion_flow_test.go`）。
   - chunk 的段长、delta（overlap）等属于“建库策略/processor profile”的一部分，会随着“场景+策略包”推荐而变化（后续会逐步做成可解释的可配置项）。

> 说明：OCR 与切块实现由后端的 processor/profile 体系承担；UI 的场景/策略包主要决定“推荐的 processor/索引前置/线上 RAG 模块组合”。

### 2.1 扫描件（图片型 PDF）推荐方案：Plan B（Tesseract + bbox）

当扫描件占比高且需要“叠框验收 + 可人工修订”时，推荐采用 `specs/011-knowledge-space/ocr_scan_pdf_plan_b.md` 描述的 Plan B：
- PDF 逐页渲染为图片
- Tesseract 输出 TSV/hOCR（包含文字 + bbox）
- 内容级（段落/条款）跨页合并，再执行 chunking（**不是按页切**）
- chunk 的 `metadata.provenance` 写入 `page_number + bbox_norm`，用于 Web Admin 预览叠框与引用定位

Web Admin 当前已提供最小验收链路：
- 空间 → 入库记录：`/knowledge-spaces/:spaceId/ingestions`
- 任务 → 切块预览/编辑：`/knowledge-spaces/:spaceId/ingestions/:jobId`

> 注意：页预览叠框与 `knowledge_chunks` 持久化仍在任务清单中（见 `specs/011-knowledge-space/tasks.md` 的 `T106B~T106G`）。

## 3) Embedding / 向量 / 图谱等数据落在哪里？

### 3.1 元数据（Postgres）
当前最核心的两张表（见 `specs/011-knowledge-space/data-model.md` 与模型定义）：
- `powerx.knowledge_ingestion_jobs`（`backend/pkg/corex/db/persistence/model/knowledge/ingestion_job.go`）
- `powerx.knowledge_artifact_bundles`（`backend/pkg/corex/db/persistence/model/knowledge/artifact_bundle.go`）

### 3.2 产物（MinIO/S3）
- `ArtifactBundle.*_manifest_uri` 以 `minio://` 开头（集成测试已断言），用于承载切块/向量/图谱等产物文件的“可回放清单”。

### 3.3 Dense 向量（pgvector in Postgres）
- Dense embedding 写入 pgvector 表（由 `backend/pkg/corex/db/persistence/vectorstore/pgvector/store.go` 管理），采用 **“全局共享 + 按维度分表（带版本）”** 的策略：
  - 表结构：`(space_uuid, chunk_uuid, embedding vector(D), metadata jsonb, updated_at)`
  - Upsert 主键：`(space_uuid, chunk_uuid)`（通过 `space_uuid` 隔离不同空间）
  - 召回：`embedding <=> query_vector`（L2 距离）

#### 3.3.1 表命名规则（重要）

- 默认/推荐表：`public.knowledge_vectors_v1_1536`
- 动态表族：`public.knowledge_vectors_v1_<D>`（例如 `public.knowledge_vectors_v1_1024`）
  - `D` 为 embedding 维度；不同维度必须落在不同表（因为 `vector(D)` 类型不同）。

#### 3.3.2 索引登记表（SSOT）：`knowledge_vector_indexes`

系统会为每个 space 维护一张登记表（用于路由、回滚与治理）：
- 表：`powerx.knowledge_vector_indexes`
- 关键字段：
  - `space_uuid`
  - `index_key`（例如 `dense_v1_1536_xxxxxxxx`）
  - `table_name`（例如 `knowledge_vectors_v1_1536`）
  - `dimensions`
  - `embedding_provider / embedding_model / embedding_profile_ref`
  - `status`（`creating|active|retired|failed`）

#### 3.3.3 “AI Settings 测试连接” vs “Space 激活向量索引”

为避免“测试连接就建表/垃圾表爆炸”，这两步被强制拆分：
- **AI Settings（测试连接）**：执行一次 **probe** 得到 `dimensions`，写回 `ai_model_profiles.cap_cache`（`probed_at`/`dimensions`），并创建 `knowledge_vectors_v1_<D>`（若不存在）。
- **Space（激活向量索引）**：才会执行 `probe → CREATE TABLE IF NOT EXISTS → 写入 knowledge_vector_indexes → 更新 knowledge_spaces.embedding_profile_key/active_vector_index_key`。

> 结果：只有在空间明确绑定 embedding profile 之后，该空间的入库才会写入向量；否则会以 “degraded（无向量）” 方式完成入库，避免误判为“成功写入”。

### 3.4 KG / Sparse / Hier 等
- **KG（知识图谱）**：当前以 `ArtifactBundle.graph_manifest_uri` 为“产物入口”预留；若启用 KG 模块，会在产物与检索侧增加实体/关系约束与召回（实现会按任务逐步落地）。
- **Sparse/BM25、Hier（层次索引）**：同样属于“线上 RAG 模块（L3）”与索引前置的组合；落地形式可能是 Postgres 表、专用索引或产物清单（具体以对应模块实现为准）。

## 4) pgvector 需要先安装吗？有没有检查？

### 需要
如果你使用 Postgres 作为向量库，需要数据库安装 **pgvector** 扩展（extname=`vector`）。

### 自动创建（可选）
- 若配置 `EnableMigrations=true`，后端会执行 `CREATE EXTENSION IF NOT EXISTS vector`（需要 DB 用户有权限）。

### 默认建表（db-migrate/db-refresh）
- `make db-migrate` / `make db-refresh` 会按 `knowledge_space.vector_store.pgvector.schema/table/dimensions` 创建默认向量表（通常是 `public.knowledge_vectors_v1_1536`）。
- 其他维度表（如 `knowledge_vectors_v1_1024`）不会在迁移阶段批量创建，而是在 **Space 激活** 时按需幂等创建。

### 运行时检查（已增强）
- `pgvector` store 的 `Health()` 现在会在 `Ping()` 之后额外检查 `pg_extension` 是否存在 `vector`：
  - 缺失会报错：`pgvector: extension "vector" not found ...`
  - 权限/兼容性导致无法查询时不会误报（仅以 Ping 为准）

## 4.1 “不保留源文件”为什么也要上传？临时文件放哪、怎么清理？

- 浏览器选中的本地文件，后端 **无法直接读取**（`file://...` 仅对本机浏览器有效）。
- 我们的入库是异步作业（会有 Worker/重试/回放），因此也不适合把文件“临时放内存里”：
  - 内存不可持久化、不可跨进程/跨节点复用
  - 大文件会导致内存压力与 OOM 风险
- 所以即使你关闭“保留到媒体库”，系统仍会把文件上传到对象存储，用于入库抓取。

当前阶段的“临时”语义是：
- Media 资产会打上 `ephemeral` 标签并设置为 `archived`（可按标签筛选与人工清理）
- 自动清理（按策略/TTL）会在后续任务中补齐

## 5) 你现在先怎么测“第一个场景”？

建议先用 `docs/guides/knowledge_space/strategy/scenes/SCN-001_basic_ingestion_and_playground.md` 跑通最小闭环（空间→入库→Playground），再按你的 PDF 类型切到：
- 合同/报价类：`SCN-020_contract_quote.md`（更偏 P2/P3 的护栏与 provenance）
- 论文/长报告：`SCN-030_research_report.md`（更偏 J_hier/B_semantic_chunking 等）

如果你的目标是 **Knowledge Graph**，请直接选择支持 KG 的场景（如 `SCN-050_sql_config_kg.md`）并使用 `PROFILE-P3_kg_constrained.md` 对应策略包。
