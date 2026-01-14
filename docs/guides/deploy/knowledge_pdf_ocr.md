# 部署：知识空间 PDF 文本抽取 / OCR 依赖

## 目标

知识空间入库（PDF）支持两条路径：

1. **PDF 内嵌文本抽取（推荐优先）**：适用于“可复制/可选中文本”的正常 PDF  
2. **OCR Plan B**：适用于扫描件 PDF（没有内嵌文字）

## Docker 镜像打包（推荐）

### Debian/Ubuntu 基础镜像

在 `backend` 镜像的 `Dockerfile` 中安装系统依赖：

```dockerfile
RUN apt-get update \
  && apt-get install -y --no-install-recommends \
    poppler-utils \
    tesseract-ocr \
    tesseract-ocr-chi-sim \
  && rm -rf /var/lib/apt/lists/*
```

说明：
- `poppler-utils` 提供 `pdftotext`/`pdftoppm`（PDF 文本抽取 / PDF 渲染）
- `tesseract-ocr` + `tesseract-ocr-chi-sim` 提供 OCR 引擎与中文简体模型

## 配置（config.yaml）

对应配置在 `knowledge_space.ingestion_processors`：

- `pdf_text_available`: 是否启用 `pdftotext` PDF 文本抽取（需要镜像/机器已安装 `pdftotext`）
- `ocr_available`: 是否启用 OCR Plan B（需要镜像/机器已安装 `tesseract` + `pdftoppm` 或 `mutool`）

未显式配置时，系统会按运行环境自动探测可用性（PATH 中是否存在对应命令）。

## 相关文档

- pgvector 动态向量表（按维度分表）：`docs/guides/deploy/knowledge_pgvector_dynamic_tables.md`
