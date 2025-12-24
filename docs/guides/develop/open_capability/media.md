# Media 模块开放能力调试指南

PowerX 底座将媒资读写能力注册为统一的能力记录，Tool Scope `media.assets`。插件或宿主只需按照下表选择协议即可调用；Root 管理员仍可通过 `/admin/media/assets` 直接 CRUD 以便排查。

| Capability ID | Intent | Prefer/Fallback | Channels |
| --- | --- | --- | --- |
| `com.corex.media.assets.read` | `media.assets.read` | Prefer REST，Fallback gRPC | `GET /media/assets`、`GET /media/assets/{uuid}`；gRPC `ListMediaAssets`、`GetMediaAsset` |
| `com.corex.media.assets.manage` | `media.assets.write` | Prefer gRPC，Fallback REST | `POST/DELETE /media/assets`、`POST /media/assets/{uuid}/presign`；gRPC `Create/Delete/PresignMediaAsset` |

> 插件调用需携带 `Authorization: Bearer <TENANT_TOKEN>` 与 `X-Tenant-UUID`，并确保租户启用了 `media.assets` Tool Grant。

## REST（开放接口 `/api/v1/media`）

OpenAPI 契约：`specs/001-docs-media-storage/contracts/http-openapi.yaml`（默认前缀 `/api/v1`）。

```bash
export API_PREFIX="http://127.0.0.1:8077/api/v1"
export TENANT_TOKEN="<tenant-jwt>"
export TENANT_UUID="<tenant-uuid>"
```

常用操作及可复制的命令如下（将占位符替换为实际值即可）：

1. **列出资产**

   ```bash
   curl -sS "$API_PREFIX/media/assets?page=1&page_size=20" \
     -H "Authorization: Bearer $TENANT_TOKEN" \
     -H "X-Tenant-UUID: $TENANT_UUID"
   ```

2. **查看单条**

   ```bash
   ASSET_UUID="d3d5d476-...-1c2a"
   curl -sS "$API_PREFIX/media/assets/$ASSET_UUID" \
     -H "Authorization: Bearer $TENANT_TOKEN" \
     -H "X-Tenant-UUID: $TENANT_UUID"
   ```

3. **创建/注册（把整条上传链路一次讲清楚）**
   1. **Step 1 · 注册元数据（所有场景共用）**

      ```bash
      curl -sS -X POST "$API_PREFIX/media/assets" \
        -H "Authorization: Bearer $TENANT_TOKEN" \
        -H "X-Tenant-UUID: $TENANT_UUID" \
        -H "Content-Type: application/json" \
        -d '{
          "name": "demo-image",
          "driver": "local",
          "ownerSubjectType": "tenant",
          "ownerSubjectId": "00000000-0000-0000-0000-000000000001",
          "uploadMethod": "presign_upload",
          "tags": ["doc"]
        }'
      ```

      - `driver` 只可填 `local` 或 `s3`（MinIO 也写 `s3`）；必须与配置里的 `storage.default_driver` 匹配。
      - `ownerSubjectType` / `ownerSubjectId` 用来追溯资源归属，推荐直接写 `tenant` + 租户 UUID。
      - `objectKey` 现在强制为 UUID；如果不填写，系统会自动生成并写入 `media_assets.storage_key`，也会作为资产 UUID，后续 PUT/访问都用它即可，不允许自定义 `demo/foo` 这类路径。
      - `sizeBytes`、`mimeType` 由后台解析得出：`external_link` 在保存前 `HEAD`/`GET` 目标 URL，`direct_upload`/`presign_upload` 在文件写入完成后回填，因此调用方不需要也不应填写这两个字段。
      - 完整字段（`CreateAssetRequest`）：`name`、`description?`、`driver`、`bucket?`、`baseUrl?`、`folder?`、`ownerSubjectType`、`ownerSubjectId`、`tags?[]`、`uploadMethod?`、`externalUrl?`、`objectKey?`、`metadata?{}`、`headers?{}`。

   2. **Step 2 · 按上传方式继续走**

      | 场景 | `uploadMethod` | 补充动作 | 说明 |
      | --- | --- | --- | --- |
      | 直接上传本地文件 | `direct_upload` | 注册后立刻 **`PUT http://127.0.0.1:8077/media/<objectKey>`**（注意：这个 PUT 端点不走 `$API_PREFIX`，是根路径 `/media/*`，见“本地上传 PUT 端点”），`objectKey` 取自创建响应 | 注册接口不会写文件；你必须自己把内容写到同名路径。 |
      | 预签名上传（推荐） | `presign_upload` | `POST /media/assets/{uuid}/presign` → 拿到 URL/Headers → 用返回的信息执行 PUT | 适合浏览器/S3 直传，**未执行 PUT 前磁盘不会有文件**；PUT 成功后后台会基于实际文件大小/MIME 自动回填资产元数据。 |
      | 引用外链 | `external_link` | 在 Step 1 的 payload 里填 `externalUrl`，无需上传 | 只同步元数据，后台会先请求外链验证可用性并提取大小/MIME，再写入 Registry。 |

      预签名示例：

      ```bash
      ASSET_UUID="刚创建成功返回的 uuid"
      curl -sS -X POST "$API_PREFIX/media/assets/$ASSET_UUID/presign" \
        -H "Authorization: Bearer $TENANT_TOKEN" \
        -H "X-Tenant-UUID: $TENANT_UUID" \
        -H "Content-Type: application/json" \
        -d '{ "action": "upload", "expiresInSeconds": 600 }'
      ```

      响应里的 `url`、`method`、`headers` 就是 PUT 所需参数，例如：

      ```bash
      OBJECT_KEY="a4e5e92e-4f97-4c57-b9d2-1b6d3d7a8f3a" # 创建或预签名响应返回
      # ⚠️ 本地 PUT 端点始终是 http://<host>:<port>/media/<objectKey>，不要带 $API_PREFIX
      curl -X PUT "http://127.0.0.1:8077/media/$OBJECT_KEY" \
        -H "PX-Upload-Token: <token>" \
        -H "PX-Upload-Expires: <ts>" \
        --upload-file ./demo.png
      ```

   3. **Step 3 · 其他变体示例（外链）**

      ```bash
      curl -sS -X POST "$API_PREFIX/media/assets" \
        -H "Authorization: Bearer $TENANT_TOKEN" \
        -H "X-Tenant-UUID: $TENANT_UUID" \
        -H "Content-Type: application/json" \
        -d '{
          "name": "banner_q4.png",
          "driver": "local",
          "ownerSubjectType": "tenant",
          "ownerSubjectId": "'$TENANT_UUID'",
          "uploadMethod": "external_link",
          "externalUrl": "https://example.com/assets/banner_q4.png",
          "objectKey": "tenants/<tenant-uuid>/campaigns/q4/banner",
          "tags": ["campaign:q4", "channel:web"],
          "metadata": {
            "alt": "Q4 Campaign Banner",
            "description": "Hero banner for web"
          }
        }'
      ```

      这种方式不会写入本地或 S3，只是在 Registry 中记录一个“外部可访问 URL + 元数据”。
4. **下载/访问资源**

   ```bash
   curl -L "$API_PREFIX/media/assets/$ASSET_UUID/resource?disposition=attachment" \
     -H "Authorization: Bearer $TENANT_TOKEN" \
     -H "X-Tenant-UUID: $TENANT_UUID" \
     -o demo.png
   ```

   ```bash
   curl -L "http://127.0.0.1:8077/media/$ASSET_UUID/resource" -o demo.png
   ```

   - 第一个示例为租户鉴权接口，第二个示例为 **公开只读入口**（`GET /media/{uuid}/resource`），适合在外部页面直接嵌入；公开入口只要 UUID 正确即可访问，无需 token，但仍会尊重 404/302 行为。
   - `disposition` 支持 `inline`（默认）与 `attachment`，可按需让浏览器直接预览或强制下载。
   - 对于外链资产，两种接口都会返回 **302** 跳转到 `externalUrl`。
   - Root 调试可使用 `GET /api/v1/admin/media/assets/{uuid}/resource`，只需把 Header 替换为 `ADMIN_TOKEN`。

5. **更新元数据**

   ```bash
   curl -sS -X PATCH "$API_PREFIX/media/assets/$ASSET_UUID" \
     -H "Authorization: Bearer $TENANT_TOKEN" \
     -H "X-Tenant-UUID: $TENANT_UUID" \
     -H "Content-Type: application/json" \
     -d '{ "tags": ["doc","reviewed"], "metadata": { "signedBy": "ops" } }'
   ```

6. **删除**

   ```bash
   curl -sS -X DELETE "$API_PREFIX/media/assets/$ASSET_UUID" \
     -H "Authorization: Bearer $TENANT_TOKEN" \
     -H "X-Tenant-UUID: $TENANT_UUID"
   ```

REST 通道足以完成完整 CRUD；**请记住：创建 (`POST /media/assets`) 只写入数据库元数据。若是 `presign_upload/direct_upload`，必须按上方步骤 2/3 或直接 PUT 上传，文件才会真正落盘**（见文末“本地上传 PUT 端点”）。

### 元数据自动校验与回填

- **外链创建**：`uploadMethod=external_link` 时，服务在写库前会主动请求该 URL（HEAD → GET Range）。如果不可达或响应 ≥400 会直接返回错误；若成功则会用探测结果填充 `sizeBytes` 与 `mimeType`，并忽略请求体中用户自带的值。
- **本地/预签名上传**：当你使用 `PUT /media/<objectKey>` 或 S3 直传写入实体文件后，服务会读取实际文件的大小与 MIME，更新 `media_assets.size_bytes/mime_type`，防止有人伪造元数据信息。
- **调用方无需额外同步**：`CreateAsset` 阶段不需要传 `sizeBytes`/`mimeType`，即使传了也会被后台解析结果覆盖，最终以真实文件为准。

## gRPC（开放接口）

契约：`backend/api/grpc/contracts/powerx/media/v1/media_asset.proto`，服务 `powerx.media.v1.MediaAssetAdminService`，默认地址 `127.0.0.1:9001`。

```bash
export GRPC_ADDR="127.0.0.1:9001"
```

- 列表：

  ```bash
  grpcurl -plaintext -H "authorization: Bearer $TENANT_TOKEN" -H "x-tenant-uuid: $TENANT_UUID" \
    -d '{ "page": 1, "page_size": 20 }' \
    $GRPC_ADDR powerx.media.v1.MediaAssetAdminService/ListMediaAssets
  ```

- 创建：

  ```bash
  grpcurl -plaintext -H "authorization: Bearer $TENANT_TOKEN" -H "x-tenant-uuid: $TENANT_UUID" \
    -d '{ "name": "demo-image", "driver": "minio", "mime_type": "image/png", "size": 2048 }' \
    $GRPC_ADDR powerx.media.v1.MediaAssetAdminService/CreateMediaAsset
  ```

- 预签名、删除等 RPC 与 REST 的操作一一对应（`PresignMediaAsset`, `DeleteMediaAsset`, `GetMediaAsset`）。

## Root 调试入口（可选）

若需要跳过租户鉴权直接 CRUD，可改调 `/admin/media/assets`（实现：`backend/internal/transport/http/admin/media/router.go`）：

```bash
export ADMIN_TOKEN="<root-admin-jwt>"
curl -sS -H "Authorization: Bearer $ADMIN_TOKEN" "http://127.0.0.1:8077/admin/media/assets/$ASSET_UUID"
curl -sS -X PATCH "http://127.0.0.1:8077/admin/media/assets/$ASSET_UUID" \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H "Content-Type: application/json" \
  -d '{ "tags": ["doc","reviewed"], "public": true }'
```

该入口与 `/api/v1/media/assets` 使用同一 schema，仅供 Root 处理异常或 CI 场景。

## 本地上传 PUT 端点（仅调试）

生产访问请一律走 `/api/v1/media/assets/{uuid}/resource` 或公开 `/media/{uuid}/resource`，下述 `/media/*` PUT 端点仅在开发/CI 调试阶段用于直传文件，默认不会对外暴露 GET（唯一例外就是上一节提到的 `GET /media/{uuid}/resource`）。

默认情况下（参见 `backend/config/defaults.go`）本地驱动的 `base_path` 为 `./storage/media`。所有 `objectKey` 都会被当作相对路径写入该目录，例如 `objectKey=a4e5e92e-4f97-4c57-b9d2-1b6d3d7a8f3a` 会落盘到 `storage/media/a4e5e92e-4f97-4c57-b9d2-1b6d3d7a8f3a`。若创建时未显式提供 `objectKey`，MediaService 会生成一个 UUID 并写入 `media_assets.storage_key`。你也可以在 `backend/config/*.yaml` 中覆盖：

```yaml
storage:
  default_driver: local
  local:
    base_path: ./storage/media
    public_base_url: http://127.0.0.1:8077/media
    upload_token_secret: "<optional-token>"
```

```bash
curl -X PUT "http://127.0.0.1:8077/media/$OBJECT_KEY" \
  -H "PX-Upload-Expires: 1735708800" \
  -H "PX-Upload-Token: <token-from-presign>" \
  --upload-file ./demo.png
```

> **提示**：当 PUT 成功写入后，底座会读取文件真实的大小与 MIME，自动更新 `media_assets.size_bytes/mime_type`。S3/MinIO 驱动的预签名链接会直接指向对象存储，无需访问该 PUT 端点。
