# 恢复副本合同

## 本轮交付

管理员在备份中心选择已登记备份，或上传 PostgreSQL `.dump`，确认当前 PostgreSQL 服务器后创建独立恢复库。后台异步执行、记录进度并保留结果；页面提供核验报告、副本专用只读连接和受保护的删除操作。

当前业务库 A 继续被当前 Core 使用；恢复只向新库 B 导入表结构及数据，不修改主实例 DSN、不启动副本上的插件或定时任务。普通 HTTP/权限审计仍沿用 Core 当前的运维记录。副本任务清单独立保存在稳定运维目录，避免依赖 release 或恢复库本身。

正式切换 Core / 插件连接、完整应用隔离预览不属于本轮接口。

## 安全和执行要求

- 仅平台 Root 用户 JWT；API Key、STS、普通租户管理员不能调用整库恢复。
- 后端自动生成 UUID 对应的新库名，禁止客户端指定数据库名、DSN、SQL 或服务器路径。
- 校验文件大小、SHA256、归档格式、工具版本、目标版本及所需扩展。
- 已登记归档读取与清理使用同一策略锁；上传归档进入私有运维目录。
- 新库由本次任务独占的低权限角色所有，恢复账号无超级用户、CREATEDB、CREATEROLE 权限；不使用 `--create` 或 `--clean`。
- 默认要求目标 PostgreSQL 不低于归档版本；旧服务器兼容尝试必须由管理员在界面明确确认，失败保留新库供检查，不能修改原库来规避错误。
- 连接凭据使用副本专用只读角色，24 小时到期；Core 运行时一分钟内撤销新登录，已有连接不强制结束；不返回源库密码，凭据响应禁止缓存。
- 删除只操作任务实际创建且 OID 匹配的新库；当前库、身份不匹配的库、仍有连接的库拒绝删除，不强制终止连接。
- 恢复后展示结构和关键业务数量；“恢复完成”不代表当前版本 PowerX 的所有业务已验收。不自动运行 seed 或迁移。

## 验收

使用本机 PostgreSQL，新建测试源库并保留哨兵数据；经真实 HTTP 上传备份、创建副本、轮询结果、验证新库内容、测试只读凭据和防误删。原库业务数据、当前实例配置需保持一致。禁止使用 Docker；测试进程和本次自建的测试数据库按归属清理，用户日常进程由用户管理。

## 客户端连接入口（v1.1）

`POST /api/v1/admin/ops/backup/restore-copies/{copy_id}/connection` 仍仅接受 Root 用户 JWT 与空 JSON。运维在 Core 进程环境中配置访问入口；客户端不能提交地址或 DSN。设置仅影响返回的 TablePlus 连接信息，恢复、身份核验和凭据管理仍连接当前 Core 使用的 PostgreSQL 服务器。必须确保客户端入口转发到同一服务器，Core 不会主动探测管理员电脑的回环端口。

| 环境变量 | 默认与支持范围 |
|---|---|
| `POWERX_OPS_RESTORE_ACCESS_MODE` | 空值等同 `direct`；仅支持 `direct`、`ssh_tunnel` |
| `POWERX_OPS_RESTORE_CLIENT_HOST` | 裸主机名或 IP，不接受 URL、Unix socket、主机加端口、多主机或通配地址。与 CLIENT_PORT 成对填写 |
| `POWERX_OPS_RESTORE_CLIENT_PORT` | 1–65535。`direct` 为客户端可访问的数据库端口；`ssh_tunnel` 为管理员电脑的本地转发端口 |
| `POWERX_OPS_RESTORE_CLIENT_SSL_MODE` | 空值沿用源连接 SSL 模式；支持 disable、allow、prefer、require、verify-ca、verify-full。需要的 CA/客户端证书由管理员在 TablePlus 配置，不能沿用服务器文件路径 |
| `POWERX_OPS_RESTORE_TUNNEL_HOST` | 仅 `ssh_tunnel` 使用；SSH 服务器能访问的 PostgreSQL 主机或回环端口映射地址 |
| `POWERX_OPS_RESTORE_TUNNEL_PORT` | 仅 `ssh_tunnel` 使用；与 TUNNEL_HOST 成对填写，1–65535 |

- systemctl/原生部署没有 CLIENT_HOST/PORT 时，默认保留现有数据库 TCP 地址。远程回环地址仍需要管理员建立 SSH 隧道。
- `POWERX_MODE=docker` 必须显式配置 CLIENT_HOST/PORT；不会回退到 `postgres:5432` 等内部服务名。Unix socket 也不能生成桌面 DSN，需配置 TCP 入口。
- `ssh_tunnel` 必须填写客户端回环地址、客户端端口及完整转发目标；SSH 登录主机、用户名与凭据由管理员提供。复制 DSN 不会自动创建 SSH 隧道。
- 未配置入口返回 HTTP **409**、`RESTORE_CONNECTION_NOT_CONFIGURED`；非法配置返回 HTTP **503**、`RESTORE_CONNECTION_CONFIG_INVALID`。错误保留标准 Trace/request ID，配置值不回显。两者均在连接数据库、轮换密码和修改到期时间前返回。
- 环境配置在协调器创建时读取；修改后重启 Core。基础 Compose 默认不发布 PostgreSQL 端口。可选回环映射及隧道步骤见 [容器部署指南](../../deploy/powerx/container/README.md#恢复副本客户端连接)。

隧道响应样例（所有凭据均为占位示例）：

```json
{
  "code": 200,
  "data": {
    "connection": {
      "access_mode": "ssh_tunnel",
      "host": "127.0.0.1",
      "port": 15433,
      "tunnel": { "host": "127.0.0.1", "port": 25432 },
      "database": "powerx_restore_copy_a3aaa535afec4a9c88bcc3ab0eac2cc9",
      "username": "pxrc_read_a3aaa535afec4a9c88bcc3ab0eac2cc9",
      "password": "<临时只读密码>",
      "ssl_mode": "disable",
      "read_only": true,
      "expires_at": "2026-10-12T00:00:00Z"
    }
  }
}
```

`host/port` 是电脑本地端点；`tunnel.host/port` 是 SSH 服务器端目标。管理员建立 `15433 → 服务器 127.0.0.1:25432` 的 SSH 转发后，再用本地 DSN 连接。`direct` 返回相同凭据字段和客户端端点，不返回 `tunnel`。前端对没有完整访问方式/端点的响应禁止复制，配置错误在连接弹窗中显示；关闭弹窗后到达的异步响应不会重新打开或保存凭据。

## 使用与存储

操作手册：[US5 保留恢复副本](../guides/deploy/027-monitor-center/usecase-us5-retained-copy.md)。副本保留至手动删除，备份文件保留策略不会删除数据库副本；无任务的上传暂存 24 小时，任务引用的上传在最后一个引用副本删除后清理。
