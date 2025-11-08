# 插件安装与启用工作流（前后端接口与页面建议）

本文件概述 PowerX 中插件从“市场 -> 安装 -> 启用/停用 -> 租户启用 -> 凭证轮换”的端到端流程，给出后端接口与前端页面实现建议。

## 角色与范围
- 平台管理员（root/system）：系统级管理插件（安装/卸载、启用/停用、查看状态/日志）。
- 租户管理员（tenant admin）：在本租户维度启用/停用插件、查看与轮换租户凭证。

## 后端接口（HTTP）
所有路由位于受保护组 `/api/v1/admin/plugins` 下（已加 `AdminOnlyMiddleware`）。

- 市场列表
  - GET `/marketplace/plugins_v2`
  - 返回：已安装插件为主的“本地市场”视图（含 `systemStatus`、`isSystemInstalled`、`isSystemEnabled`、`icon` 等）。
- 系统级管理
  - GET `/`：插件列表（含 Admin URL、菜单、状态）
  - POST `/:id/enable`：系统启用（启动子进程、挂载反代、挂载 Admin 静态、落盘 state）
  - POST `/:id/disable`：系统停用（卸载路由、停子进程、落盘 state）
  - GET `/:id/status`：运行状态（pid/port/state/health 指标）
  - POST `/install/url`：从 URL 安装（可选启用）
  - POST `/install/local`：从本地目录安装
  - POST `/:id/uninstall`：卸载（可选 `purge` 清理磁盘）
  - POST `/:id/switch_version`：切换版本（可选立即启用）
  - GET  `/:id/logs`：最近运行日志
- 租户级管理
  - GET  `/:id/tenant_config`：查看是否已为本租户创建凭证、是否启用、client_id
  - POST `/:id/tenant_enable`：启用/停用（仅影响本租户）；首次启用会创建凭证并一次性返回明文 secret
  - GET  `/:id/credentials`：只读凭证信息（不返回明文 secret）
  - POST `/:id/credentials/rotate`：轮换密钥并一次性返回新明文 secret
  - DELETE `/:id/tenant_config`：删除本租户该插件的凭证配置（硬删）

说明：系统启用时若上下文携带了 `tenant_id`，会触发一次租户维度的 PostEnable 钩子（见 `internal/bootstrap/plugin.go`）。

## 宿主 → 插件 gRPC 推送（可选）
- Proto：`api/grpc/contracts/powerx/plugin/control/v1/control.proto`
- 方法：`UpsertTenantCredentials(tenant_id, plugin_id, client_id, client_secret)`
- 触发时机：
  - 租户首次启用成功（有一次性明文 secret）；
  - 租户轮换密钥成功。
- 编译开关：需以 `-tags plugin_control` 构建，启用 `internal/infra/plugin/manager/notify` 里的 gRPC 推送实现。

## 前端页面建议
1) 插件市场（系统级）
- 路由：`/admin/plugins/market`
- 列表卡片字段：名称、图标、版本、描述、标签、系统状态（未安装/已安装/启用/停用）。
- 操作：
  - 未安装：弹框“从 URL 安装”（输入包地址与可选 SHA256，是否安装后启用）。
  - 已安装：按钮“启用/停用”、“卸载”（二次确认）。
  - 详情抽屉：展示元信息（作者、标签、首页）、系统运行状态与日志入口。
- 数据源：GET `/marketplace/plugins_v2` + 系统级操作接口。

2) 插件详情（系统级）
- 路由：`/admin/plugins/:id`
- 区块：
  - 基本信息：版本、描述、作者、图标；
  - 运行状态：`GET /:id/status`；
  - 控制：启用/停用、切换版本、卸载、查看日志。

3) 租户启用（租户级）
- 路由：`/admin/plugins/:id/tenant`
- 区块：
  - 启用开关：`POST /:id/tenant_enable {enabled}`；
  - 凭证信息：`GET /:id/credentials`（显示 `client_id` 与是否存在/启用）；
  - 首次启用或轮换时弹窗显示“一次性明文 secret”，并提示妥善保存；
  - 轮换按钮：`POST /:id/credentials/rotate`；
  - 删除配置：`DELETE /:id/tenant_config`（危险操作，需要确认）。
- 辅助：如系统未启用，提示管理员先在“系统级”启用。

## 前端调用示例（curl）
- 获取市场列表：
```
curl -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/v1/admin/plugins/marketplace/plugins_v2
```
- 从 URL 安装并启用：
```
curl -X POST -H "Content-Type: application/json" -H "Authorization: Bearer <token>" \
  -d '{"url":"https://example.com/com.powerx.demo.hello_world-0.1.2.zip","sha256":"","enable":true}' \
  http://localhost:8080/api/v1/admin/plugins/install/url
```
- 系统启用：
```
curl -X POST -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/v1/admin/plugins/com.powerx.demo.hello_world/enable
```
- 租户启用（首次会返回一次性 secret）：
```
curl -X POST -H "Content-Type: application/json" -H "Authorization: Bearer <token>" \
  -d '{"enabled":true}' \
  http://localhost:8080/api/v1/admin/plugins/com.powerx.demo.hello_world/tenant_enable
```
- 轮换密钥（一次性返回新 secret）：
```
curl -X POST -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/v1/admin/plugins/com.powerx.demo.hello_world/credentials/rotate
```

## 实现细节与注意事项
- 系统启用会启动子进程、健康检查并挂载反向代理到 `/_p/:id/api`，Admin 静态到 `/_p/:id/admin`。
- 宿主在启用时为进程注入 `POWERX_INTERNAL_TOKEN`，用于本机 gRPC 通道鉴权。
- `tenant_enable` 在首次创建凭证时会尝试 gRPC 下发（best-effort，失败不阻断）。
- 明文 secret 仅在“首次创建”或“轮换”响应中出现一次；后续只可通过轮换获取新 secret。
- 构建时如需启用 gRPC 推送，请使用：`go build -tags plugin_control ./cmd/app`。

