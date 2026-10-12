# 客户管理入口修复验收（2026-10-11）

## 交付

- 系统菜单已有 `menu.customer:view`，补入首次安装权限种子，并通过集中迁移增量登记。权限页增加“业务运营 → 客户管理”。增量更新保留 UUID、effect/status、用户描述、墓碑和已有角色授权。
- 客户页增加个人/企业创建表单、操作权限检查、20 项分页与租户切换清理。Admin 创建使用用户会话和实时平台 API 授权；菜单授权、STS/API Key 不授予创建能力。复用 Customer/membership/primary Contact 原子事务，企业自然人主要联系人必须明确提供。
- 新增 UTF-8 CSV 导入：1 MiB/200 行，固定模板、持久化输入快照、版本/checksum/24 小时确认期限、逐行校验、当前租户及文件重复检查、明确确认、并发幂等提交、结果回读/导出和 Trace。失败行不留下半份客户；整批结果与成功创建共同提交。完成后的失败行通过修正后的新预览重试。
- 新 Admin 导入能力的三项权限均 `allow_api_key=false`，运行时明确拒绝 API Key/STS；原 Framework typed Customer/Contact service grants 未改变。
- 模型挂载在 `pkg/corex/db/database/migration.go`，没有启动时自动迁移。升级 registry 的 core 载荷增加 IAM permission helper 依赖，重新生成内容寻址 checksum。

## 验证

| 项目 | 结果 |
|---|---|
| Go customer service / transport / database / IAM repository / API Key permissions / HTTP auth / database CLI / seed | 8 个相关包通过 |
| Race：customer service、transport、database、IAM repository | 4 个包通过 |
| go vet：customer service、transport、database、customer/IAM repositories、API Key permissions、shared DI | 通过 |
| Nuxt customers / IAM / backup | 8 个文件、49 项通过；其中新客户管理 11 项 |
| `make capability-check` | 通过：declared=1101、referenced=64、rest_routes=1028、candidates=1044、ignored_route_rules=26 |
| `make capability-seed` | 在独立 PostgreSQL 测试数据库成功；Registry 有正式导入能力，三项 API 权限均不可由 API Key 授权 |
| 完整 `database migrate` 与再次执行 | 在独立 PostgreSQL 数据库均退出 0；表和菜单存在，旧权限及业务哨兵、配置字节保持一致 |
| OpenAPI、locale、文档链接、upgrade registry --check、diff --check | 通过 |

### 原生 PostgreSQL HTTP 处理链

`POWERX_CUSTOMER_ADMIN_TEST_DSN` 指向本机 PostgreSQL 14.15，在 `backend` 执行：

```sh
go test -race ./internal/transport/http/admin/customer -run 'TestCustomer(AdminImportHTTPPostgres|ImportRejectsUnknown)' -count=1 -v
```

使用真实 Gin Handler 和 PostgreSQL，用户上下文由隔离测试中间件注入；没有替代生产 JWT 登录/会话验证。测试 schema `customer_admin_test_7a82f284e3f04e7787f4e1a6c18951e4`，导入 UUID `c3a49a50-2a0b-4999-b9e2-c6bbda5bc09c`，Trace `customer-admin-http-40254f5d-c622-4b7e-97d0-cb5421c629d9`。

结果：企业手动创建 HTTP 201，缺主要联系人 HTTP 400；API Key/STS 导入 HTTP 403；跨租户回读 HTTP 404；错误 checksum HTTP 409；两个并发确认均 HTTP 200，但同一邮箱只新增一位客户，得到非空主要联系人 UUID；登录身份数量保持 0；再次回读为 completed。未知请求字段、数据覆盖及多 JSON 对象明确拒绝。测试结束后 schema 已清理并查询确认不存在。

### 迁移与目录保留

独立数据库 `powerx_customer_admin_test_690b9bc991c4`，任务私有配置和合成密钥，不使用生产配置。首次 `database migrate` 与 `make capability-seed` 成功。主动写入一条保留角色授权、deny/deprecated 菜单状态、自定义描述和业务哨兵后，再跑 migrate，UUID、状态、描述、角色授权、哨兵及配置 SHA256 均保持。

测试数据库在确认无活动连接后删除；临时配置、日志、二进制和目录全部清理。没有启动 HTTP/Web/Docker 服务，没有重启或关闭使用者进程。本轮发现的默认 API Key 目录派生问题已修正并通过针对性测试。

## 部署与验收边界

当前代码在 `024-ai-engineering-skills` 分支，未提交或推送。前次恢复副本连接修复和使用者的 AGENTS 改动保持；无关临时工具文件删除保持现状。

发布前后端更新后，使用者须按正式升级合同或开发迁移命令执行迁移并重启后台，然后显式授予创建/导入权限。生产浏览器应复验权限页客户菜单、个人/企业创建、预览无写入、确认/回读、错误与分页，并保留生产 request/Trace。生产浏览器尚未验收，本机结果不作为生产通过证据。

正式材料：[操作手册](../../guides/features/030-customer-contact/usecase-core-admin-ingress.md)、[OpenAPI](../customer-admin-import.openapi.yaml)、spec/plan/tasks 的 Core admin ingress 补充。
