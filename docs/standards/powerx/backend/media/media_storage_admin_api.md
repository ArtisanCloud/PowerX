# 媒体管理后台 API（Admin）— 统一 Presign Upload 流
>
> 版本：v1  
> 目标：客户端对 **Local** 与 **S3/MinIO** 的用法完全一致：  
> **Create → Presign(upload) → PUT 到返回的 url（携带返回的 headers）→ PATCH 确认**  
> 下载使用：**Presign(download) → GET 返回的 url**

---

## 0. 前置要求（基于 `config.yaml`）

```yaml
storage:
  defaultDriver: local   # 开发：local；线上：s3
  drivers:
    local:
      basePath: "/abs/path/to/your/repo/storage/media"  # 绝对路径（读/写同源）
      publicBaseURL: "/media"                           # 下载前缀，与 GET 路由一致
      tokenSecret: "dev-hmac-secret"                    # 生成 X-Corex-Upload-Token 的 HMAC 秘钥
    s3:
      endpoint: "https://s3.ap-northeast-1.amazonaws.com"  # MinIO 可填: http://127.0.0.1:9000
      region: "ap-northeast-1"
      bucket: "powerx-media-dev"
      accessKey: "<AK>"
      secretKey: "<SK>"
      pathStyle: false          # MinIO 推荐 true
      presignExpiration: 3600   # 预签名默认过期秒数
````

> **注意**
>
> * Admin 接口（/api/v1/admin/**）需要平台鉴权（如 `Authorization: "Bearer ...`、`X-Tenant-UUID: ...`）。"
> * **上传 PUT/POST 到 presign 返回的 `url` 时，不要带平台 JWT/租户头**；只带 presign 返回的 headers。
> * Local 环境必须有下载 GET 路由（如 `GET /media/**`）且其根目录与 `local.basePath` 一致。

---

## 1. 路由总览

| 方法     | 路径                                          | 说明                                |          |               |
| ------ | ------------------------------------------- | --------------------------------- | -------- | ------------- |
| POST   | `/api/v1/admin/media/assets`                | 创建媒资记录（建档，初始 `draft`）             |          |               |
| GET    | `/api/v1/admin/media/assets`                | 列表/筛选                             |          |               |
| GET    | `/api/v1/admin/media/assets/{uuid}`         | 详情                                |          |               |
| PATCH  | `/api/v1/admin/media/assets/{uuid}`         | 更新业务字段 / 状态流转（如 `draft→active`）   |          |               |
| DELETE | `/api/v1/admin/media/assets/{uuid}`         | 软删除                               |          |               |
| POST   | `/api/v1/admin/media/assets/{uuid}/presign` | 生成 **上传/下载** 预签名票据（`action=upload | download | multipart_*`） |

> **无**“接收文件二进制”的 Admin 路由；文件字节通过 presign 返回的 `url` 直接 PUT/POST。

---

## 2. 统一上传流程（示例：curl）

### 步骤 1）创建媒资（建档）

```bash
curl -s -X POST "http://127.0.0.1:8077/api/v1/admin/media/assets" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <JWT>" \
  -H "X-Tenant-UUID: 1" \
  -d '{
        "name":"logo-green.jpg",
        "mime_type":"image/jpeg",
        "size":123456,
        "tags":["channel:web"],
        "source": { "kind":"upload" }
      }'
```

**成功响应（示例）**

```json
{
  "code": 201,
  "message": "success",
  "data": {
    "uuid": "f9cc2061-1c61-4ad5-b6e6-c7d28f86640f",
    "tenantId": "1",
    "driver": "local",                    // 或 "s3"
    "objectKey": "f05fa64c-e25c-40fc-a08e-1401cd8b411f",
    "businessStatus": "draft",
    "name": "logo-green.jpg",
    "mimeType": "image/jpeg",
    "tags": ["channel:web"]
  },
  "timestamp": 1760187000
}
```

---

### 步骤 2）获取上传预签名（upload）

```bash
UUID="f9cc2061-1c61-4ad5-b6e6-c7d28f86640f"
curl -s -X POST "http://127.0.0.1:8077/api/v1/admin/media/assets/$UUID/presign" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <JWT>" \
  -H "X-Tenant-UUID: 1" \
  -d '{
        "action":"upload",
        "filename":"logo-green.jpg",
        "content_type":"image/jpeg",
        "expires_in":900
      }'
```

**Local 驱动：成功响应（示例）**

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "method": "PUT",
    "url": "http://localhost:8077/media/f05fa64c-e25c-40fc-a08e-1401cd8b411f",
    "headers": {
      "Content-Type": "image/jpeg",
      "X-Corex-Upload-Expires": "1760188638",
      "X-Corex-Upload-Token": "961bbd1b...e52"
    },
    "storage_key": "f05fa64c-e25c-40fc-a08e-1401cd8b411f",
    "expiresAt": "2025-10-11T13:17:18Z"
  },
  "timestamp": 1760187738
}
```

**S3/MinIO 驱动：成功响应（示例）**

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "method": "PUT",
    "url": "https://s3.ap-northeast-1.amazonaws.com/powerx-media-dev/tenant-1/f05fa64c-e25c-40fc-a08e-1401cd8b411f",
    "headers": {
      "Content-Type": "image/jpeg",
      "x-amz-acl": "private",
      "x-amz-content-sha256": "UNSIGNED-PAYLOAD",
      "Authorization": "AWS4-HMAC-SHA256 Credential=..."
    },
    "storage_key": "tenant-1/f05fa64c-e25c-40fc-a08e-1401cd8b411f",
    "expiresAt": "2025-10-11T13:17:18Z"
  },
  "timestamp": 1760187738
}
```

---

### 步骤 3）上传文件字节（PUT 到 `data.url`）

> **严格带上 `presign` 返回的所有 headers**；**不要**携带平台 `Authorization`/`X-Tenant-UUID`。

**Local：**

```bash
URL="http://localhost:8077/media/f05fa64c-e25c-40fc-a08e-1401cd8b411f"
TOKEN="961bbd1b...e52"
EXP="1760188638"
FILE="/path/logo-green.jpg"

curl -i -X PUT "$URL" \
  -H "Content-Type: image/jpeg" \
  -H "X-Corex-Upload-Token: $TOKEN" \
  -H "X-Corex-Upload-Expires: $EXP" \
  --data-binary @"$FILE"
# 期望：HTTP/1.1 204 No Content（或 200）
```

**S3/MinIO：**

```bash
URL="https://s3.ap-northeast-1.amazonaws.com/powerx-media-dev/tenant-1/f05fa64c-e25c-40fc-a08e-1401cd8b411f"
curl -i -X PUT "$URL" \
  -H "Content-Type: image/jpeg" \
  -H "x-amz-acl: private" \
  -H "x-amz-content-sha256: UNSIGNED-PAYLOAD" \
  -H "Authorization: AWS4-HMAC-SHA256 Credential=...（按返回原样粘贴）" \
  --data-binary @"$FILE"
# 期望：HTTP/1.1 200 OK（或 204）
```

---

### 步骤 4）（可选）确认/激活

```bash
curl -s -X PATCH "http://127.0.0.1:8077/api/v1/admin/media/assets/$UUID" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <JWT>" \
  -H "X-Tenant-UUID: 1" \
  -d '{
        "status":"active",
        "size":123456,
        "storage_key":"f05fa64c-e25c-40fc-a08e-1401cd8b411f"
      }'
```

---

## 3. 下载流程（统一）

### 3.1 获取下载预签名（download）

```bash
curl -s -X POST "http://127.0.0.1:8077/api/v1/admin/media/assets/$UUID/presign" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <JWT>" \
  -H "X-Tenant-UUID: 1" \
  -d '{"action":"download","expires_in":600}'
```

**Local：成功响应（示例）**

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "method": "GET",
    "url": "http://localhost:8077/media/f05fa64c-e25c-40fc-a08e-1401cd8b411f",
    "expiresAt": "2025-10-11T13:30:00Z"
  },
  "timestamp": 1760188200
}
```

**S3/MinIO：成功响应（示例）**

```json
{
  "code": 200,
  "message": "success",
  "data": {
    "method": "GET",
    "url": "https://powerx-media-dev.s3.ap-northeast-1.amazonaws.com/tenant-1/f05f...b411f?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=...",
    "expiresAt": "2025-10-11T13:30:00Z"
  },
  "timestamp": 1760188200
}
```

### 3.2 发起下载

```bash
DL=$(curl -s -X POST "http://127.0.0.1:8077/api/v1/admin/media/assets/$UUID/presign" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <JWT>" \
  -H "X-Tenant-UUID: 1" \
  -d '{"action":"download","expires_in":600}' | jq -r '.data.url')

curl -I "$DL"   # 期望：HTTP/1.1 200 OK
```

---

## 4. 资源查询与维护

### 4.1 列表

```bash
curl -s "http://127.0.0.1:8077/api/v1/admin/media/assets?page=1&page_size=20&status=active&tag=channel:web&q=logo" \
  -H "Authorization: Bearer <JWT>" \
  -H "X-Tenant-UUID: 1"
```

### 4.2 详情

```bash
curl -s "http://127.0.0.1:8077/api/v1/admin/media/assets/$UUID" \
  -H "Authorization: Bearer <JWT>" \
  -H "X-Tenant-UUID: 1"
```

### 4.3 更新

```bash
curl -s -X PATCH "http://127.0.0.1:8077/api/v1/admin/media/assets/$UUID" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <JWT>" \
  -H "X-Tenant-UUID: 1" \
  -d '{
        "name":"logo-green-v2.jpg",
        "tags":["channel:web","campaign:q4"],
        "meta":{"alt":"Q4 JP banner"},
        "status":"active"
      }'
```

### 4.4 软删除

```bash
curl -s -X DELETE "http://127.0.0.1:8077/api/v1/admin/media/assets/$UUID" \
  -H "Authorization: Bearer <JWT>" \
  -H "X-Tenant-UUID: 1"
```

---

## 5. 错误码与排障

| 场景                     | 现象                                               | 原因 & 处理                                                               |
| ---------------------- | ------------------------------------------------ | --------------------------------------------------------------------- |
| presign(upload) 返回 400 | `media: unsupported operation`                   | 该驱动未实现该 action；确认 `storage.defaultDriver` 与对应 `drivers.*` 配置          |
| PUT 上传 403             | `X-Corex-Upload-Token` 或 `Expires` 缺失/过期；S3 签名错误 | 重新走 presign；校对系统时间；S3 需**完整带回**所有返回的 `x-amz-*`/`Authorization` 头      |
| PUT 上传 404（Local）      | `/media/:objectKey` 路由未注册或 BasePath 不一致          | 注册下载/写入路由；确保 BasePath 为**绝对路径**且与写入一致                                 |
| 下载 404（Local）          | 访问 `/media/<key>` 不存在                            | 路径规则包含租户/桶；以 **presign(download) 的 url** 为准；检查 `publicBaseURL`        |
| 下载 presign 返回 `null`   | 生成 URL 时缺少 `publicBaseURL` 或 `storage_key`       | 校对 `config.yaml` 的 `local.publicBaseURL`，并在 `PATCH` 时回填 `storage_key` |
| S3 301/307 重定向         | Region 或 path-style 不一致                          | `region` 与桶所在区域一致；MinIO 建议 `pathStyle: true`                          |
| S3 `NoSuchBucket`      | 桶不存在或区域不匹配                                       | 先创建桶；确认区域/endpoint                                                    |

---

## 6. 统一性约束（给调用方的承诺）

* 客户端永远只需这 4 步：**Create → Presign(upload) → PUT/POST → PATCH**；下载走 **Presign(download) → GET**。
* `presign` 响应的 **`method/url/headers`** 是**唯一真实来源**：**严格按返回执行**，不要自行组合。
* 无论 Local 还是 S3/MinIO，**客户端姿势完全一致**；差异封装在服务端驱动里。

---
