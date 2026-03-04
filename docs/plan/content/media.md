# 媒体管理 UI（Web Admin）开发方案

本文档用于指导在 `web-admin`（Nuxt）中搭建“媒体库/媒体管理”UI，以管理 PowerX 底座（CoreX）媒体资产（MediaAsset）。

> 适用范围：管理员/运营在控制台内进行上传、检索、编辑、下线与分发链接（预签名/资源链接）管理。

---

## 1. 背景与目标

### 1.1 背景

后端已提供 MediaAsset 的 Admin/OpenAPI 能力（创建、列表、详情、更新、删除、预签名、资源访问），以及本地驱动的“预签名写入端点”用于文件落盘与元数据回填。

### 1.2 目标（UI）

- 在 Web Admin 提供统一的“媒体库”入口，覆盖：
  - 媒体资产列表与筛选（分页、关键字、标签、状态、驱动、回收站）
  - 资产详情查看与编辑（名称、描述、标签、业务状态、扩展元数据）
  - 上传/入库（优先走预签名上传闭环；支持外链入库）
  - 下载/预览、复制链接（可控的资源访问方式）
  - 批量操作（第一期可先支持批量删除/批量加标签）

### 1.3 非目标（第一期）

- 不做复杂的素材编目体系（文件夹树、智能标签、相似度检索、OCR、转码、缩略图服务等）
- 不做媒体恢复/物理删除（当前后端未提供 Admin HTTP 的 restore/force-delete 接口；可在后续迭代补齐）
- 不实现“direct_upload”文件直传（后端 gRPC proto 有 `file_content/file_name` 字段，但当前 HTTP handler 未实现该上传方式的二进制直传）

---

## 2. 依赖能力与接口映射

### 2.1 认证与租户上下文

- Web Admin 通过 `Authorization: Bearer <access_token>` 登录态访问管理接口
- 请求需携带租户上下文头：`JWT claims（tid/tenant_uuid）: <tenant_uuid>`
  - `web-admin/app/composables/api/index.ts` 已默认注入 `Authorization`（租户由 JWT claims 提供）（来自本地存储/上下文）

### 2.2 后端接口（Admin）

统一前缀：Web Admin 侧以 `runtimeConfig.public.apiBase` 为准（见 `web-admin/nuxt.config.ts`；默认 `/api/v1`，可通过 `NUXT_PUBLIC_API_BASE` 或 `UPSTREAM` path 推断/覆盖）。本文下文用 `<apiBase>` 代表该前缀。

- 列表：`GET <apiBase>/admin/media/assets`
- 创建：`POST <apiBase>/admin/media/assets`
- 详情：`GET <apiBase>/admin/media/assets/:uuid`
- 更新：`PATCH <apiBase>/admin/media/assets/:uuid`
- 删除（软删）：`DELETE <apiBase>/admin/media/assets/:uuid`
- 预签名：`POST <apiBase>/admin/media/assets/:uuid/presign`
- 资源（鉴权）：`GET <apiBase>/admin/media/assets/:uuid/resource?disposition=inline|attachment`
- 说明：
  - 该资源接口需要 `Authorization: Bearer <access_token>`（Admin）以及 `JWT claims（tid/tenant_uuid）`，**直接在浏览器地址栏打开通常会报 `missing or invalid Authorization header` 属正常现象**。
  - 若希望“复制后可直接打开/外部分发”，应使用 **预签名下载链接**（`POST <apiBase>/admin/media/assets/:uuid/presign`，`action=download`）或公开入口（见 2.3）。
- 本地上传写入端点（鉴权）：`PUT <apiBase>/media/assets/:uuid`
  - 说明：上传写入端点的最终 URL 以“预签名接口返回”为准，UI 不应硬编码 `/api` 或 `/api/v1`。
  - 需透传预签名返回的头：`X-CoreX-Upload-Expires`、`X-CoreX-Upload-Token`（若配置了 `storage.local.upload_token_secret`）

### 2.3 资源公开入口（安全提醒）

后端存在公开资源入口：`GET /media/:uuid/resource`。  
默认策略：**仅 `published` 允许匿名访问**；`draft/under_review/archived` 需要携带 `token+exp`（由 `presign(action=download)` 生成）才可短期访问，用于安全分发/预览。
UI 第一阶段建议 **默认走鉴权资源接口**（`<apiBase>/admin/.../resource`）进行预览/下载；对外分发则使用 `presign(download)` 返回的链接或公开入口（仅 published）。

---

## 3. 信息架构（IA）与路由

建议挂在现有“内容管理（Content）”模块下（`/content`）：

- `GET /content/media`：媒体库（列表/网格 + 筛选 + 上传）
- `GET /content/media/:uuid`：资产详情（预览 + 编辑）
- `GET /content/media?onlyDeleted=1`：回收站视图（仅展示 + 批量清理提示）

> 当前 `web-admin/app/pages/content/index.vue` 已存在“媒体库”入口，但未实现对应页面，需要补齐 `web-admin/app/pages/content/media/*`。

---

## 4. 页面与交互设计

### 4.1 媒体库（/content/media）

#### 4.1.1 视图与布局

- 顶部：标题 + 上传按钮 + 视图切换（网格/表格）
- 筛选条（可折叠高级项）
- 内容区：
  - 网格：以“预览卡片”展示（图片可直接用资源 URL；其他 mime 显示 icon）
  - 表格：信息密度高（名称、类型、大小、标签、状态、更新时间、操作）
- 右侧抽屉：上传/入库（见 4.3）

#### 4.1.2 筛选项（后端已支持）

- `keyword`：按名称模糊匹配
- `businessStatus[]`：draft/under_review/published/archived（字符串）
- `driver`：local/s3（可先写死 options，也可从配置/后续接口拉取）
- `tags[]`：全包含语义（AND）
- `includeDeleted` / `onlyDeleted`：包含软删、仅软删（回收站）
- 高级（可选）：`ownerSubjectType`、`ownerSubjectId`

#### 4.1.3 列表操作

单条：
- 打开详情
- 复制链接（默认复制鉴权资源 URL；若启用公开入口，可复制公开 URL）
- 复制链接建议拆分为两类：
  - 复制下载链接（推荐）：通过 `presign(action=download)` 获取可直接访问的 URL（外链/S3 为带签名 URL；local 驱动通常返回 `/media/:uuid/resource`）。
  - 复制鉴权链接（调试用）：`<apiBase>/admin/media/assets/:uuid/resource`，需要带 `Authorization/JWT claims（tid/tenant_uuid）`。

> 本地开发常见坑：`presign(action=download)` 在 local 驱动下返回的 `/media/:uuid/resource` 是 **后端服务**的相对路径；若 Web Admin 与后端不在同一域（例如 3030 ↔ 8077），需要用后端 `UPSTREAM` 的 origin 去拼接，否则会在前端站点上 404。
- 下载（`disposition=attachment`）
- 改状态（按状态机约束）
- 删除（软删）

批量（第一期建议）：
- 批量删除（软删）
- 批量打标签/移除标签（如果后端暂不支持批量，可先前端串行调用 PATCH）

### 4.2 资产详情（/content/media/:uuid）

#### 4.2.1 预览区域

- 图片：`<img>` 预览（默认走鉴权资源；可用 `fetch(blob)` + `URL.createObjectURL`）
- 视频/音频：HTML5 播放器（同上）
- 其他：文件卡片 + 下载按钮

#### 4.2.2 详情与编辑

基础字段：
- `name`（必填）
- `description`（写入 meta.description）
- `tags[]`
- `businessStatus`（必须遵循后端状态机，避免无效流转）

存储信息（只读）：
- `driver`、`bucket`、`objectKey(storageKey)`、`sizeBytes`、`mimeType`
- `externalUrl`（外链入库）
- `downloadUrl`/`downloadExpiredAt`（后端可能记录最近一次预签名信息）

扩展元数据：
- 展示 `metadata/meta`（key-value）
- 第一阶段建议：只读展示 + “添加/删除键值”轻量编辑（调用 PATCH metadata）

### 4.3 上传/入库（Upload）

第一阶段建议提供 3 种入口，但只启用其中 2 种：

#### 4.3.1 预签名上传（推荐，闭环完整）

1) 创建资产（进入 draft）  
`POST <apiBase>/admin/media/assets`，`uploadMethod=presign_upload`

2) 生成上传预签名  
`POST <apiBase>/admin/media/assets/:uuid/presign`  
请求：`{ action: 'upload', method: 'PUT', content_type: file.type, expiresInSeconds?: number }`

3) 执行上传  
- 若返回 URL 为站内相对路径（例如 `<apiBase>/media/assets/:uuid`）：  
  - 以 `PUT` 上传文件内容，合并预签名返回的 `headers`（包含 `X-CoreX-Upload-*`）并带上 Authorization
- 若返回 URL 为外部对象存储（S3/MinIO）绝对地址：  
  - 使用返回的 `method + headers` 直传，不附加自定义 Authorization

4) 成功后刷新详情/列表  
本地驱动落盘后会回写 `sizeBytes/mimeType`（由上传中间件探测并同步）

#### 4.3.2 外链入库（External Link）

`POST <apiBase>/admin/media/assets`，`uploadMethod=external_link` + `externalUrl`  
后端会探测 size/mime（HEAD/Range GET），UI 直接展示即可。

#### 4.3.3 直传（Direct Upload）

目前不建议在 UI 中开放（后端未提供可用的 HTTP 二进制直传实现）；UI 可显示为“未启用/预留能力”。

---

## 5. 组件拆分建议（Web Admin）

- `MediaFilterBar`：筛选条（keyword/status/tags/driver/回收站）
- `MediaUploadDrawer`：上传抽屉（预签名上传/外链入库）
- `MediaGrid` / `MediaTable`：列表视图
- `MediaAssetCard`：网格卡片
- `MediaPreview`：按 mime 渲染预览
- `MediaAssetDetailPanel`：编辑区域（name/desc/tags/status/metadata）

---

## 6. 状态、错误与可观测性

### 6.1 状态管理

- 列表查询：分页加载、筛选变更触发刷新、空态/错误态
- 上传流程：分步状态（创建 → 预签名 → 上传 → 回填/刷新），需展示进度与失败可重试

### 6.2 错误呈现

- 统一显示后端 `message/error`（web-admin 的 api client 已做错误归一化）
- 对常见错误做明确提示：
  - `invalid media asset status transition`：提示“状态流转不合法”
  - 413：提示“文件过大（受驱动限制）”
  - 403：提示“上传 token 失效/无权限”

### 6.3 安全与默认策略

- 默认使用鉴权资源接口进行预览/下载
- 复制链接默认复制鉴权 URL；若后续确认公开入口安全可控，再提供“公开链接”复制

---

## 7. 验收清单（UI）

- 能在 `/content/media` 浏览媒体列表并筛选（keyword/tags/status/driver/回收站）
- 能创建资产并通过“预签名上传”完成文件上传，上传完成后 size/mime 有回填
- 能通过“外链入库”创建资产并可预览/跳转
- 能在详情页编辑 name/description/tags/status 并正确保存
- 能删除资产（软删）并在回收站筛选到记录
- 默认不依赖匿名 `/media/:uuid/resource` 进行预览/下载
