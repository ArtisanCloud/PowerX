# 媒体存储多驱动设计（统一 Presign Upload）
>
> 版本：v1  
> 目标：在 **CoreX 内核模块**中，通过 `MediaManager + StorageDriver` 抽象，统一 **Local** 与 **S3/MinIO** 的上传/下载语义。  
> 客户端用法固定为：**Create → Presign(upload) → PUT 到返回的 url（携带返回的 headers）→ PATCH 确认**；下载为 **Presign(download) → GET**。

---

## 1. 架构与协作

```

HTTP (Admin REST)
├─ handler (/api/v1/admin/media/...)
│     └─ MediaService (用例层)
│            ├─ MediaRepository (PostgreSQL：元数据)
│            └─ MediaManager (存储驱动门面：local | s3)
│                    └─ StorageDriver 接口 (LocalDriver / S3Driver)
└─ public routes (/media/** for GET/PUT in local)

````

- **MediaService**
  - 负责业务编排：建档、签名、状态流转（`draft→active`）、审计、多租户校验。
- **MediaRepository**
  - 维护 `MediaAsset` 表（详情见 §5），含软删、索引与审计字段。
- **MediaManager**
  - 选择驱动、标准化输入输出、统一错误与指标上报（latency、QPS、error）。
- **StorageDriver（接口）**
  - `Put(ctx, in PutObjectInput) error`
  - `Delete(ctx, in DeleteObjectInput) error`
  - `GenerateURL(ctx, in GenerateURLOptions) (out GenerateURLOutput, err error)`
  - 可选：`MultipartInit/MultipartPart/MultipartComplete`（大文件）

---

## 2. `config.yaml`（权威配置）

```yaml
storage:
  defaultDriver: local     # dev/local: local; staging/prod: s3
  drivers:
    local:
      basePath: "/abs/path/to/repo/storage/media"  # 绝对路径（读/写一致）
      publicBaseURL: "/media"                      # 下载前缀（与 GET 路由一致）
      tokenSecret: "dev-hmac-secret"               # 本地上传预签名 HMAC 秘钥
      # 可选：tenantSubdir: true                    # 路径带租户子目录（/base/<tenantId>/<key>）

    s3:
      endpoint: "https://s3.ap-northeast-1.amazonaws.com"  # MinIO 示例: http://127.0.0.1:9000
      region: "ap-northeast-1"
      bucket: "powerx-media-dev"
      accessKey: "<AK>"
      secretKey: "<SK>"
      pathStyle: false              # MinIO 推荐 true
      presignExpiration: 3600       # 预签名默认过期秒数
      # 可选：basePrefix: "tenant-<id>/"            # 多租户前缀
````

**要求**

- `local.basePath` 必须是**绝对路径**；下载与写入路由统一使用该目录。
- `local.publicBaseURL` 必须与对外 GET 路由前缀一致（如 `/media`）。
- 如启用租户子目录（`tenantSubdir` 或 S3 `basePrefix`），需在驱动侧与下载路由一致处理。

---

## 3. Driver 接口定义（示意）

```go
type PutObjectInput struct {
    Bucket      string
    ObjectKey   string
    Body        io.Reader
    Size        int64
    ContentType string
    Extra       map[string]string // filename, tenantId ...
}

type DeleteObjectInput struct {
    Bucket    string
    ObjectKey string
}

type GenerateURLOptions struct {
    Action      string        // "upload" | "download" | "multipart_init" | ...
    Bucket      string
    ObjectKey   string
    ContentType string        // upload 时必填
    ExpiresIn   time.Duration
    Filename    string
    Extra       map[string]string
}

type GenerateURLOutput struct {
    Method     string            // "PUT" | "POST" | "GET"
    URL        string
    Headers    map[string]string // 上传/下载必须携带的头
    Fields     map[string]string // POST 表单直传用（S3 可选）
    StorageKey string            // 客户端回填用
    ExpiresAt  time.Time
}
```

**统一约束**

- `GenerateURL(Action=upload)`：

  - **Local**：返回 `Method=PUT`、`URL=/media/:objectKey`、Headers 包含 `Content-Type`、短时校验头 `X-Corex-Upload-Token/Expires`（HMAC）。
  - **S3**：返回 `Method=PUT`、S3 直传 URL、Headers 包含必要的 `x-amz-*` 与 `Authorization`（或返回 `Fields` 走表单直传）。
- `GenerateURL(Action=download)`：

  - **Local**：返回 `Method=GET`、`URL=publicBaseURL + relativePath`。
  - **S3**：返回签名 GET URL。

---

## 4. 路由与网关（Local 必备）

### 4.1 Admin（已存在）

- `POST /api/v1/admin/media/assets`
- `GET /api/v1/admin/media/assets`
- `GET /api/v1/admin/media/assets/{uuid}`
- `PATCH /api/v1/admin/media/assets/{uuid}`
- `DELETE /api/v1/admin/media/assets/{uuid}`
- `POST /api/v1/admin/media/assets/{uuid}/presign`

> **不接收二进制文件**。仅提供元数据与签名。

### 4.2 Public（Local：必须存在）

- `PUT  /media/:objectKey`（**写入端点**，仅 dev/local/内网开启）

  - 校验 `X-Corex-Upload-Token`（HMAC(objectKey|expires|secret)）与 `X-Corex-Upload-Expires`。
  - 校验 `Content-Type` 与大小上限（可配置）。
  - 将请求体写入 `basePath[/<tenantId>]/objectKey`；成功返回 `204`。
- `GET  /media/**`（下载端点）

  - 将 `publicBaseURL` 映射到 `basePath`；若带租户目录，路由需匹配两段（`/media/:tenantId/:objectKey`）。

> 生产环境如果禁用 Local 驱动，请不要暴露 `PUT /media/:objectKey`。

---

## 5. 数据模型与索引（PostgreSQL）

```sql
-- 逻辑示意（非 DDL 最终稿）
MediaAsset (
  id              bigserial PK,
  uuid            uuid UNIQUE,
  tenant_id       bigint NOT NULL,
  driver          text   NOT NULL,  -- "local" | "s3"
  bucket          text   NULL,      -- s3 bucket 或 local 租户目录标记
  object_key      text   NOT NULL,  -- 存储键（文件名/UUID）
  name            text,
  content_type    text,
  size            bigint,
  tags            text[],           -- 或 JSONB
  meta            jsonb,
  business_status text   NOT NULL DEFAULT 'draft', -- draft/active/archived
  deleted_at      timestamptz NULL,
  created_at      timestamptz NOT NULL,
  updated_at      timestamptz NOT NULL
);

-- 索引建议
CREATE UNIQUE INDEX ux_asset_tenant_driver_key ON MediaAsset (tenant_id, driver, object_key);
CREATE INDEX ix_asset_tenant_status ON MediaAsset (tenant_id, business_status);
-- tags 如为 JSONB，建议 GIN 索引：
-- CREATE INDEX ix_asset_tags_gin ON MediaAsset USING GIN (meta jsonb_path_ops);
```

**约束**

- `storage_key`（即 `object_key`）在 `PATCH` 时需回填（与 presign 输出一致）。
- 软删使用 `deleted_at`；物理清理由清理任务执行。

---

## 6. 统一流程（客户端视角）

**Upload**

1. `POST /admin/media/assets`（建档）→ 返回 `{uuid, driver, objectKey,...}`
2. `POST /admin/media/assets/{uuid}/presign`（`action=upload`）→ 返回 `{method,url,headers/fields,storage_key}`
3. 客户端按返回执行 **PUT/POST** 到 `url`（**严格带 presign headers**）
4. `PATCH /admin/media/assets/{uuid}`（`status=active`、`size`、`storage_key`）

**Download**

1. `POST /admin/media/assets/{uuid}/presign`（`action=download`）→ 返回 `{method=GET,url}`
2. 客户端 **GET** 该 `url`

**Multipart**（可选）

- `multipart_init` → 返回 `uploadId`、partSize
- 多次 `multipart_part` → 返回每个分片的 PUT url，客户端上传并记录 `ETag`
- `multipart_complete` → 提交 `ETag[]` 合并
- `PATCH` 确认

---

## 7. 安全设计

- **Local 写入端点**：

  - 仅在 `env=dev/local` 或内网开启。
  - 采用 `X-Corex-Upload-Token/Expires`：

    - `token = HMAC_SHA256(secret, objectKey + ":" + expiresUnix)`
    - 服务端校验时间窗与 HMAC 一致性；过期拒绝。
  - 限制 `Content-Length` 与可接受 `Content-Type` 列表；必要时做病毒扫描（hook）。
- **S3/MinIO**：

  - 全部走 AWS v4 签名/表单策略；禁止公共读写桶。
  - 生产环境统一通过 **download presign** 或 CDN 私有回源。

---

## 8. 监控与日志

- 指标（建议接 OpenTelemetry + Prometheus）：

  - `media_generate_url_latency_ms{action,driver}`
  - `media_put_latency_ms{driver}`
  - `media_put_errors_total{driver,code}`
  - `media_download_404_total{driver}`
- 日志（结构化）：

  - `trace_id`、`tenant_id`、`driver`、`object_key`、`action`、`expires_at`
  - 重要错误打栈，避免日志中泄露签名/密钥

---

## 9. 测试清单（必过）

- **Local（dev）**

  - 配置 `basePath` 为绝对路径，`publicBaseURL=/media`
  - `upload presign → PUT`（204）→ `download presign → GET`（200）
  - 变更 `tenantSubdir` 时，验证 `/media/:tenantId/:objectKey`
  - 过期 token（`Expires` 过去时间）→ PUT 403
- **MinIO（本地）**

  - `endpoint=http://127.0.0.1:9000`、`pathStyle=true`、`region=us-east-1`
  - `upload presign → PUT`（200/204）→ `download presign → GET 200`
  - 桶不存在 → `NoSuchBucket`；headers 少/错 → 403
- **AWS S3（真云）**

  - 时钟偏差 ±1min 内；region 与桶一致；IAM policy 最小化
  - 表单直传（POST policy）与 PUT 均可通过

---

## 10. 迁移与兼容

- 如果历史实现曾有 **multipart Admin 上传端点** 或 **直传 /assets/:uuid/upload**：

  - 标注为 Deprecated；文档统一指向 **Presign → PUT**。
  - 旧端点在 1～2 个迭代后移除（提供迁移期）。
- 历史资源的 `downloadUrl` 生成需统一走 `GenerateURL(download)`，不要硬编码。

---

## 11. 典型错误与定位

| 现象                                | 高概率原因                                 | 排查要点                                                                        |
| --------------------------------- | ------------------------------------- | --------------------------------------------------------------------------- |
| `presign(upload)=400`             | 驱动未实现该 action                         | 检查 `storage.defaultDriver` 与对应 `drivers.*`；确认 `GenerateURL(upload)` 分支实现    |
| PUT=204 但 GET=404（Local）          | 读写根目录不一致 / 路由未注册                      | `basePath` 需绝对路径；`publicBaseURL` 与 GET 路由一致；以 `download presign` 的 url 为准核对 |
| `presign(download).data.url=null` | `publicBaseURL` 为空 / `storage_key` 缺失 | 配置 `local.publicBaseURL`；在 `PATCH` 回填 `storage_key`                         |
| S3=403                            | 签名头缺失/错误、时钟偏差、IAM 权限不足                | 全量回放 `headers/fields`；同步系统时间；最小 IAM policy 覆盖 Put/Get                       |
| S3=301/307                        | region 或 path-style 不匹配               | region 与桶一致；MinIO 开启 `pathStyle:true`                                       |
| 大文件失败                             | 未走 multipart、网关超时                     | 使用 `multipart_*` 流程；提升反向代理 `client_max_body_size` 与后端超时                     |

---

## 12. 实施备注（最小代码要点）

- Local `GenerateURL(upload)`：

  - `Method=PUT`，`URL=publicBaseURL + "/<tenantPath><objectKey>"`；
  - `Headers`：`Content-Type`、`X-Corex-Upload-Token`、`X-Corex-Upload-Expires`；
- Local 写入路由 `PUT /media/:objectKey`：

  - 校验 token/expires → `io.Copy` 到 `basePath[/(tenantId)]/objectKey` → `204`
- S3 `GenerateURL(upload)`：

  - 采用 SDK 生成 PUT 预签名（或 POST policy），填充 `headers/fields`。
- Service `Presign`：

  - 依据资产记录（`driver/bucket/objectKey/tenantId`）调用 `Manager.GenerateURL`；
  - 回写 `downloadUrl`（可选）与 `storage_key` 一致性校验。
- `PATCH /assets/{uuid}`：

  - 可允许一次性把 `status=size=storage_key=checksum` 一并写入，便于客户端确认。

---

## 13. 统一性承诺（对调用方）

- 客户端只需记住 **同一条上传/下载流程**。
- `presign` 的 `method/url/headers/fields` 是唯一真实来源，**严格按返回执行**。
- 无论 **Local** 还是 **S3/MinIO**，客户端**完全一致**；差异全部被封装在驱动内部。

---
