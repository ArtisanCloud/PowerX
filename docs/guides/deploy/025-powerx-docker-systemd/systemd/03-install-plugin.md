# 03. Install Plugin（安装插件）

## 1. 前置条件
- 已完成 `01-deploy-config-start.md`
- 已完成 `/setup`
- 拥有管理员权限（Web Admin / Admin API）

## 2. 安装路径

### 路径 A：本地热更新（开发/联调）
参考：`docs/guides/plugin_release/application_runbook.md` 第 1 节

示例：
```bash
px-plugin build --target local
px-plugin dev --watch \
  --grpc-addr localhost:9090 \
  --tenant-uuid 101 --developer-id 2025 \
  --artifact ./dist/plugin-bundle.zip
```

### 路径 B：离线包导入（测试/生产推荐）
参考：`docs/guides/plugin_release/application_runbook.md` 第 4 节

包含：
- Web Admin UI 提交/审核
- CLI/API 导入（`px plugin import --offline ...` / `POST /api/tenant/offline-imports`）

## 3. 最小验证
```bash
curl -sS -X GET "http://127.0.0.1:8080/api/v1/admin/plugins/<plugin_id>/audit?page=1&page_size=20" \
  -H "Authorization: Bearer <ADMIN_TOKEN>"
```

## 4. 说明
`/setup` 第 5 步主要是引导，不等于完整插件发布安装流水线。
