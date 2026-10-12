# 恢复副本客户端入口修复验收（2026-10-11）

## 问题与修改

PR 审查指出连接接口直接返回 Core 自用的 `postgres:5432`，默认 Compose 未发布数据库端口，桌面 TablePlus 无法连接。

- 增加受控的客户端地址、端口、SSL 和 `direct` / `ssh_tunnel` 配置；不会修改 Core 的数据库 DSN 或恢复任务目标。
- systemctl/原生部署默认保留现有 TCP 地址；Docker 必须显式配置客户端入口。缺失入口返回 409 / `RESTORE_CONNECTION_NOT_CONFIGURED`，非法配置返回 503 / `RESTORE_CONNECTION_CONFIG_INVALID`，均在轮换旧凭据前拒绝。
- 连接弹窗显示状态、错误码、Trace 和隧道说明；不完整响应禁止复制 DSN，关闭弹窗后到达的响应不重新打开或保存凭据。
- 可选 `compose.restore-access.yml` 只发布服务器回环端口，随发布包交付。管理员电脑通过 SSH 转发后使用本地 DSN。
- 正式合同与 OpenAPI 已更新为 v1.1；使用与回退步骤见容器指南及 US5 指南。

## 本机验证

| 验证 | 结果 |
|---|---|
| `go test ./internal/service/backup_ops ./internal/transport/http/admin/backup` | 两个包通过；默认不开启真实数据库测试 |
| `go vet` 同上两个包 | 通过 |
| 前端 `npm run test:unit -- --run tests/unit/backup` | 4 个文件、26 项通过；最终弹窗错误码/SSL 校验调整后，恢复副本 11 项再次通过 |
| 原生 PostgreSQL 14.15，race 测试 | 5 组通过：默认地址、显式客户端/IPv6、不同电脑与服务器端口、非法配置、环境解析、缺失入口无副作用、真实恢复与只读校验 |
| Compose 原生合并检查 | 默认无 PostgreSQL 端口；覆盖后仅 `127.0.0.1:15432 → 5432`，客户端及隧道参数一致 |
| JSON/OpenAPI/文档链接与 `git diff --check` | 通过 |

原生测试使用 `POWERX_RESTORE_TEST_ADMIN_DSN` 指向本机已有 PostgreSQL，管理员连接仅传给测试环境，不记录在报告中。测试源库 `powerx_restore_test_6571b0a1c629`，恢复副本 `powerx_restore_copy_486b730a52234ad3a80234fdefa21211`，哨兵表 1 张。使用返回的客户端 Host/Port 与副本凭据建立真实连接，SELECT 成功；INSERT、CREATE、写入型 SECURITY DEFINER 函数失败。原库哨兵保持不变，到期撤销和防误删验证通过。测试结束后查询确认源库、副本和对应专用角色均已删除。

配置上述测试环境变量后，在 `backend` 中复现：

```sh
go test -race ./internal/service/backup_ops -run 'TestRestoreCopy(NativePostgres|Client|MissingClient)' -count=1 -v
```

## 验收范围

本轮未启动后台、前端或 Docker 容器，未操作远程服务；使用者自己的 PostgreSQL 和日常服务保持运行。Compose 仅解析配置，没有启动数据库端口映射。真实远程 SSH 转发与 TablePlus/浏览器连接仍需在部署新版本并配置访问入口后验收；本机结果不作为其通过证据。本轮没有执行全仓库回归或发布 CI。
