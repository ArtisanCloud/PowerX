# Plan B 设计：扫描版 PDF 的 OCR（Tesseract）+ 跨页内容切分 + bbox 预览验收

> 目标：扫描件/图片型 PDF 占比高时，保证入库“可读、可检索、可定位、可人工校正”，并且**切分按内容而不是按页**。

## 1. 背景与问题

扫描版 PDF 的本质是“每页一张或多张图片”。没有文本层时：
- 无法直接向量化（embedding 没意义）
- 无法做段落/条款级切分（segment 质量差）
- 无法做引用定位（无法回到原文页/坐标验收）

因此需要 OCR 抽取文字，并把“文字在页面上的位置（bbox）”作为 provenance 一并落地，供 Web Admin 预览叠框验收与人工修订。

## 2. 范围（Scope）

### In Scope
- PDF（扫描件/图片型）OCR：将 PDF 渲染为逐页图片，使用 Tesseract 输出结构化结果（TSV/hOCR），再聚合为“内容级 segment/chunk”。
- provenance：支持跨页 chunk（一个 chunk 覆盖多页多框），支持 bbox 叠加预览。
- 验收链路：空间 → 入库记录 → chunk 列表预览 → 定位（页预览叠框）→ 编辑 chunk → 仅该 chunk 重建向量索引。

### Out of Scope（本阶段不做）
- 完整的版面理解（两栏/图表/公式深度结构化）。
- 以 token/字符级 bbox 进行逐字定位（粒度过细，成本过高）。
- 自动纠错/人机协同标注系统（先落地最小闭环）。

## 3. 核心结论（关键设计决策）

1) **切分按内容，不按页**  
页码仅用于 provenance（定位），不是 segment 边界。段落跨 2～3 页是常态。

2) **bbox 坐标采用归一化坐标（0..1），原点左上（top-left）**  
前端叠框时按图片实际尺寸换算像素，避免渲染分辨率变化导致错位。

3) **bbox 的权威来源来自 OCR 引擎的结构化输出（Tesseract TSV/hOCR）**  
可搜索 PDF（text layer）可以作为可选产物，但 bbox/定位不依赖它。

4) **图片与 OCR 产物放对象存储（MinIO/S3），DB 存引用与可编辑的 chunk 文本**  
大文件不进 DB；DB 负责可查询/可编辑/可审计。

## 4. 数据流（Pipeline）

以 “PDF 扫描件入库” 为例：

1. 上传 PDF → Media 资产（对象存储）  
2. Ingestion job 触发（format=pdf, ocrRequired=true 或自动判定）  
3. **PDF → page images**：渲染每页图片（PNG/JPG），产物写入 ArtifactStore  
4. **page images → OCR**：Tesseract 输出 TSV/hOCR（逐页），产物写入 ArtifactStore  
5. **OCR 聚合**：将 TSV/hOCR 解析为 `DocumentUnit` 序列（line/block），每个 unit 携带 provenance（page+bbox）  
6. **内容级 segment/chunk**：按场景策略合并为段落/条款（可跨页）并执行 chunking（chunkSize/overlap/separators/anchors）  
7. 写入：
   - `knowledge_chunks`（可编辑文本 + metadata/provenance）
   - `knowledge_vectors`（向量库：chunk_id 对应 embedding）
   - `ArtifactBundle`（离线清单：chunk_manifest/vector_manifest/ocr_pages/ocr_raw/page_images）
8. Web Admin：
   - 预览：chunk 列表 + 叠框定位
   - 编辑：修改 chunk 文本 → 单 chunk 重建向量索引（不重跑全文档）

## 5. provenance 与 bbox 数据结构（建议）

chunk 的 `metadata.provenance` 建议使用如下结构：

```json
{
  "source_uri": "minio://.../file.pdf",
  "pages": [
    {
      "page_number": 12,
      "page_image_uri": "minio://.../pages/012.png",
      "regions": [
        { "x1": 0.12, "y1": 0.34, "x2": 0.88, "y2": 0.41, "confidence": 0.93 }
      ]
    },
    {
      "page_number": 13,
      "page_image_uri": "minio://.../pages/013.png",
      "regions": [
        { "x1": 0.10, "y1": 0.05, "x2": 0.90, "y2": 0.12, "confidence": 0.91 }
      ]
    }
  ]
}
```

说明：
- `pages[]` 支持跨页 chunk；
- `regions[]` 是 line/block 的合并结果（不是 token 级 bbox）；
- 归一化坐标：`x1,y1,x2,y2` ∈ [0,1]，原点左上。

## 6. 存储布局（ArtifactStore / MinIO/S3）

建议把与 OCR 相关的新增产物写入同一 job bundle 目录：

```
knowledge/<space_uuid>/<job_uuid>/
  chunk_manifest.json
  vector_manifest.json
  masking_report.json
  ocr/
    pages/
      001.png
      002.png
    raw/
      001.tsv   (或 001.hocr)
      002.tsv
    searchable.pdf (可选)
```

并在 `ArtifactBundle` 增加（或复用 `graph_manifest_uri` 之外的字段）记录 OCR 相关 URI（方案见 tasks.md）。

## 7. API 设计（Admin）

### 7.1 Chunk 预览（已落地：最小闭环）
- `GET /api/v1/admin/knowledge-spaces/:spaceId/ingestion-jobs/:jobId/chunks`
  - 分页返回 chunk 文本 + metadata（含 provenance）

### 7.2 Chunk 编辑（已落地：最小闭环）
- `PATCH /api/v1/admin/knowledge-spaces/:spaceId/ingestion-jobs/:jobId/chunks/:chunkId`
  - 更新 chunk 文本并对该 chunk 重新写入向量索引（Upsert）
  - 记录 `edited_at/edited_by/edit_reason`

### 7.3 页预览叠框（待落地）
为减少前端拼装复杂度，建议提供：
- `GET /api/v1/admin/knowledge-spaces/:spaceId/ingestion-jobs/:jobId/pages/:pageNumber`
  - 返回该页 `page_image_uri`（或 presign 可访问 URL）及页面尺寸信息
- `GET /api/v1/admin/knowledge-spaces/:spaceId/chunks/:chunkId/locate`
  - 返回该 chunk 覆盖的 `pages[] + regions[]`（归一化 bbox）

## 8. Web Admin 验收链路（UX）

必备链路：
1) 空间列表 → 入库记录（job 列表）  
2) job → chunk 列表预览（分页、过滤、复制 id）  
3) chunk → 定位：打开页预览并叠框高亮（支持跨页）  
4) chunk → 编辑：保存后立即重建索引，并在 UI 标记“已人工修订”  
5) 反馈闭环：从 chunk 预览页一键跳转 feedback，自动带 spaceId + chunkId

## 9. 安全、资源与性能

扫描 PDF OCR 属于重计算且处理不可信输入，建议：
- OCR 在 worker/processor 侧执行，不在主 API handler 内执行；
- `exec.CommandContext` 设置超时、最大并发、CPU/内存限制（容器级）；
- 对上传文件做类型校验与大小上限；
- 产物写入与索引写入分阶段提交，保证任务可回放；
- 关键指标：OCR 耗时、页数、失败率、bbox 覆盖率、chunk 覆盖率、编辑重建耗时。

## 10. 迁移与回滚

- 允许通过配置显式启用/禁用处理器能力（优先级高于自动探测）：
  - `knowledge_space.ingestion_processors.ocr_available: true/false`
  - `knowledge_space.ingestion_processors.pdf_text_available: true/false`
- 对扫描件 OCR 路径：触发入库时设置 `processorProfile=builtin/ocr_plan_b` 且 `ocrRequired=true`，确保 OCR 不可用时进入 `blocked/ocr_required`，便于运维兜底；
- 对同一 source 的 reprocess 支持“仅重跑 OCR + 复用 chunking 配置”；
- 支持 bundle 版本化与回滚：编辑/重建后写入新的 bundle（或更新 manifest checksum），保持可审计。
