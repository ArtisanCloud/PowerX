# 媒体存储驱动使用指南（Local 与 S3/MinIO）
>
> 版本：v1  
> 本指南说明如何基于 `config.yaml` 配置并验证 PowerX 的媒体存储功能。  
> 无论使用 Local 还是 S3 驱动，上传与下载的客户端流程保持完全一致：  
> **Create → Presign(upload) → PUT 上传 → PATCH 激活 → Presign(download) → GET 下载**

---

## 一、配置说明（config.yaml）

```yaml
storage:
  defaultDriver: local   # 本地开发环境用 local；生产用 s3
  drivers:
    local:
      basePath: "/abs/path/to/your/repo/storage/media"  # 绝对路径
      publicBaseURL: "/media"                           # 与下载路由一致
      tokenSecret: "dev-hmac-secret"                    # 生成上传 HMAC
    s3:
      endpoint: "http://127.0.0.1:9000"  # MinIO；AWS 则为 https://s3.<region>.amazonaws.com
      region: "us-east-1"
      bucket: "powerx-media-dev"
      accessKey: "minioadmin"
      secretKey: "minioadmin123"
      pathStyle: true
      presignExpiration: 3600
````

---

## 二、Local 模式使用（开发环境）

### 1. 创建资源

```bash
curl -X POST "http://127.0.0.1:8077/api/v1/admin/media/assets" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <JWT>" \
  -d '{
        "name": "demo-banner.jpg",
        "mime_type": "image/jpeg",
        "tags": ["dev-test"],
        "source": { "kind": "upload" }
      }'
```

---

### 2. 生成上传预签名

```bash
UUID="f9cc2061-1c61-4ad5-b6e6-c7d28f86640f"
curl -s -X POST "http://127.0.0.1:8077/api/v1/admin/media/assets/$UUID/presign" \
  -H "Content-Type: application/json" \
  -d '{"action":"upload","filename":"demo-banner.jpg","content_type":"image/jpeg","expires_in":900}'
```

---

### 3. 上传文件

```bash
URL="http://localhost:8077/media/f05fa64c-e25c-40fc-a08e-1401cd8b411f"
TOKEN="961bbd1b...e52"
EXP="1760188638"
FILE="./demo-banner.jpg"

curl -i -X PUT "$URL" \
  -H "Content-Type: image/jpeg" \
  -H "X-Corex-Upload-Token: $TOKEN" \
  -H "X-Corex-Upload-Expires: $EXP" \
  --data-binary @"$FILE"
# 返回 204 No Content 表示上传成功
```

---

### 4. 激活文件

```bash
curl -X PATCH "http://127.0.0.1:8077/api/v1/admin/media/assets/$UUID" \
  -H "Content-Type: application/json" \
  -d '{"status":"active","size":123456,"storage_key":"f05fa64c-e25c-40fc-a08e-1401cd8b411f"}'
```

---

### 5. 下载验证

```bash
curl -s -X POST "http://127.0.0.1:8077/api/v1/admin/media/assets/$UUID/presign" \
  -H "Content-Type: application/json" \
  -d '{"action":"download","expires_in":600}'
```

---

## 三、S3/MinIO 模式使用（测试或生产）

### 1. 修改配置

```yaml
storage:
  defaultDriver: s3
  drivers:
    s3:
      endpoint: "http://127.0.0.1:9000"
      region: "us-east-1"
      bucket: "powerx-media-dev"
      accessKey: "minioadmin"
      secretKey: "minioadmin123"
      pathStyle: true
      presignExpiration: 3600
```

---

### 2. Presign（upload）

```bash
UUID="f9cc2061-1c61-4ad5-b6e6-c7d28f86640f"
curl -s -X POST "http://127.0.0.1:8077/api/v1/admin/media/assets/$UUID/presign" \
  -H "Content-Type: application/json" \
  -d '{"action":"upload","filename":"demo-banner.jpg","content_type":"image/jpeg"}'
```

---

### 3. 上传文件到 S3

```bash
URL="https://s3.ap-northeast-1.amazonaws.com/powerx-media-dev/f05f...b411f"
curl -i -X PUT "$URL" \
  -H "Content-Type: image/jpeg" \
  -H "x-amz-acl: private" \
  -H "x-amz-content-sha256: UNSIGNED-PAYLOAD" \
  -H "Authorization: AWS4-HMAC-SHA256 Credential=..." \
  --data-binary @"./demo-banner.jpg"
# 返回 200/204 即成功
```

---

### 4. 下载验证

```bash
curl -s -X POST "http://127.0.0.1:8077/api/v1/admin/media/assets/$UUID/presign" \
  -H "Content-Type: application/json" \
  -d '{"action":"download","expires_in":600}'
```

---

## 四、Insomnia 测试集合（导入模板）

可直接保存为 `PowerX_Media_Storage_Insomnia.json`，然后在 Insomnia → Import → From File 导入。

```json
{
  "_type": "export",
  "__export_format": 4,
  "__export_date": "2025-10-11T21:30:00.000Z",
  "__export_source": "PowerX Media Storage Guide",
  "resources": [
    {
      "_id": "wrk_powerx_media",
      "name": "PowerX Media Storage",
      "_type": "workspace"
    },
    {
      "_id": "req_create_asset",
      "parentId": "wrk_powerx_media",
      "name": "1️⃣ Create Asset",
      "url": "http://127.0.0.1:8077/api/v1/admin/media/assets",
      "method": "POST",
      "body": {
        "mimeType": "application/json",
        "text": "{\n  \"name\": \"demo-banner.jpg\",\n  \"mime_type\": \"image/jpeg\",\n  \"tags\": [\"dev-test\"],\n  \"source\": { \"kind\": \"upload\" }\n}"
      },
      "headers": [
        {"name": "Content-Type", "value": "application/json"},
        {"name": "Authorization", "value": "Bearer <JWT>"},
        {"name": "JWT claims（tid/tenant_uuid）", "value": "1"}
      ],
      "_type": "request"
    },
    {
      "_id": "req_presign_upload",
      "parentId": "wrk_powerx_media",
      "name": "2️⃣ Presign (Upload)",
      "url": "http://127.0.0.1:8077/api/v1/admin/media/assets/{{uuid}}/presign",
      "method": "POST",
      "body": {
        "mimeType": "application/json",
        "text": "{\n  \"action\": \"upload\",\n  \"filename\": \"demo-banner.jpg\",\n  \"content_type\": \"image/jpeg\",\n  \"expires_in\": 900\n}"
      },
      "headers": [
        {"name": "Content-Type", "value": "application/json"},
        {"name": "Authorization", "value": "Bearer <JWT>"},
        {"name": "JWT claims（tid/tenant_uuid）", "value": "1"}
      ],
      "_type": "request"
    },
    {
      "_id": "req_patch_activate",
      "parentId": "wrk_powerx_media",
      "name": "3️⃣ Activate Asset",
      "url": "http://127.0.0.1:8077/api/v1/admin/media/assets/{{uuid}}",
      "method": "PATCH",
      "body": {
        "mimeType": "application/json",
        "text": "{\n  \"status\": \"active\",\n  \"size\": 123456,\n  \"storage_key\": \"{{storage_key}}\"\n}"
      },
      "headers": [
        {"name": "Content-Type", "value": "application/json"},
        {"name": "Authorization", "value": "Bearer <JWT>"},
        {"name": "JWT claims（tid/tenant_uuid）", "value": "1"}
      ],
      "_type": "request"
    },
    {
      "_id": "req_presign_download",
      "parentId": "wrk_powerx_media",
      "name": "4️⃣ Presign (Download)",
      "url": "http://127.0.0.1:8077/api/v1/admin/media/assets/{{uuid}}/presign",
      "method": "POST",
      "body": {
        "mimeType": "application/json",
        "text": "{\n  \"action\": \"download\",\n  \"expires_in\": 600\n}"
      },
      "headers": [
        {"name": "Content-Type", "value": "application/json"},
        {"name": "Authorization", "value": "Bearer <JWT>"},
        {"name": "JWT claims（tid/tenant_uuid）", "value": "1"}
      ],
      "_type": "request"
    }
  ]
}
```

---

## 五、验证脚本（CLI 快速测试）

```bash
UUID=$(curl -s -X POST http://127.0.0.1:8077/api/v1/admin/media/assets -H "Content-Type: application/json" -d '{"name":"demo.png","mime_type":"image/png","source":{"kind":"upload"}}' | jq -r '.data.uuid')
PRE=$(curl -s -X POST http://127.0.0.1:8077/api/v1/admin/media/assets/$UUID/presign -H "Content-Type: application/json" -d '{"action":"upload","filename":"demo.png","content_type":"image/png"}')
URL=$(echo $PRE | jq -r '.data.url')
TOKEN=$(echo $PRE | jq -r '.data.headers["X-Corex-Upload-Token"]')
EXP=$(echo $PRE | jq -r '.data.headers["X-Corex-Upload-Expires"]')
curl -i -X PUT "$URL" -H "Content-Type: image/png" -H "X-Corex-Upload-Token: $TOKEN" -H "X-Corex-Upload-Expires: $EXP" --data-binary @"demo.png"
```

---

## 六、最佳实践

* 所有上传操作都使用 **presign url**，客户端永不直传 PowerX API。
* presign 响应的 `method/url/headers` 为唯一可信上传信息。
* Local 模式仅限开发；生产环境请使用 S3/MinIO 并关闭本地 PUT 路由。
* 上传成功后务必 PATCH 激活并保存 storage_key。
* 下载必须通过 presign(download)，不要拼 URL。

---
