# 受控导入 Skill（开发者/第三方）

本文说明如何通过 `POST /admin/skills/import` 导入 Skill。

## 规则摘要

1. 仅支持 `upload` 模式（接口内部固定，不能远程在线拉取）。
2. `bundle_uri` 必须是上传后的地址（例如 `s3://...`）。
3. `checksum` 必填，且需为 `sha256:` 或 `sha256-` 前缀。
4. `signature` 是否必填由策略控制（环境变量 `POWERX_SKILL_REQUIRE_SIGNATURE`）。

## 步骤 1：准备导入请求

```json
{
  "skill_id": "skill.thirdparty.demo",
  "version": "1.0.0",
  "source": "third_party",
  "bundle_uri": "s3://skills/skill.thirdparty.demo-1.0.0.tgz",
  "checksum": "sha256:123abc",
  "signature": "optional-signature",
  "source_url": "https://github.com/example/skills",
  "source_ref": "refs/tags/v1.0.0"
}
```

## 步骤 2：执行导入

```bash
curl -sS -X POST "$POWERX_HTTP_BASE/admin/skills/import" \
  -H "Authorization: Bearer $ROOT_TOKEN" \
  -H "Content-Type: application/json" \
  -d @import.json
```

预期：HTTP `202`，返回状态 `draft`。

## 步骤 3：验证导入结果

```bash
curl -sS "$POWERX_HTTP_BASE/admin/skills?skill_id=skill.thirdparty.demo" \
  -H "Authorization: Bearer $ROOT_TOKEN"
```

预期：能看到 `source_url/source_ref/import_type` 已落库。

## 常见失败与处理

1. `checksum is required`  
   原因：未提供 checksum。  
   处理：补充 `checksum` 字段。

2. `checksum mismatch`  
   原因：格式不符合策略（非 `sha256:` 或 `sha256-` 前缀）。  
   处理：改为合规 SHA256 摘要格式。

3. `remote repository online pull is disabled`  
   原因：`bundle_uri` 使用了 `http(s)` 在线地址。  
   处理：先上传包，再使用内部存储 URI（如 `s3://`）。

## Web Admin 操作路径

1. `设置 -> AI -> Skills`。
2. 在“导入 Skill（仅 upload）”表单填入字段。
3. 点击“导入”，成功后自动刷新 Registry。
