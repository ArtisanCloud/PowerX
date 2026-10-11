# 插件请求身份解析实施记录

日期：2026-10-10；[正式合同](../../docs/contracts/plugin-request-identity.md)。

- [x] Core 用户网关票据签发、伪造头清除、独立 Redis 命名空间、60 秒 TTL。
- [x] 用户 access/refresh 会话关联、退出撤销复核、Root 无成员续期。
- [x] service typed binding / Tenant REST / Admin 自身检查三条入口，双授权平面正式能力。
- [x] live grant、精确 plugin owner、用户/成员/租户/插件状态、当前业务权限及原请求权限复核。
- [x] 用户/服务/票据/权限/依赖错误区分；Trace 与敏感字段脱敏。
- [x] Root / 普通成员、跨域与撤销单测；共享 business permission mapper。
- [x] 真实 PostgreSQL + Redis、正式 STS Exchange、API Key、Core 用户网关 HTTP、WS grant 和 WebSocket 101 握手测试。
- [x] 完整 capability-check / capability-seed / 当前开发 Host key grant-status=granted 及最终编译验收。
- [ ] Framework Go/.NET / skeleton / AI Craft 对齐与真实远程浏览器验收，由消费端完成。

证据：[Core 读取及握手验收](../../docs/contracts/evidence/plugin-request-identity-20261010.json)。本次使用隔离 PostgreSQL schema、真实 Redis ticket store、真实 gRPC Exchange 和本机临时 HTTP/WS upstream；不修改远程服务或业务数据。测试自身创建的监听器、fixture schema、临时凭据及票据必须清理。

可复验：设置 `POWERX_IDENTITY_LIVE_CONFIG=/absolute/private/config.yaml`、`POWERX_IDENTITY_EVIDENCE=/absolute/evidence.json`，在 backend 执行 `go test ./internal/bootstrap -run '^TestLivePluginRequestIdentity$' -count=1 -v`。

正式能力变化后仓库根目录执行 `make capability-check`、`make capability-seed`。不新增数据库表或迁移；新用户会话证明需要重新编译/重启及宿主续期/重新登录。当前工作区原有改动保持不变，提交/推送由用户处理。
