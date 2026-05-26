# Quickstart: IAM 用户与角色 RBAC 统一能力

## 1. 前置条件

- 当前分支：`026-iam`
- 已完成初始化并可访问 backend/web-admin
- 具备三类账号：`root`、`tenant admin`、`member`

账号矩阵（建议）：
- `root`: 平台超管，可跨租户管理
- `tenant admin`: 非 root，仅管理本租户
- `member`: 普通成员，无租户级用户管理权限

## 2. 启动与基础检查

1. 启动服务（`make dev` 或 systemd）。
2. 检查健康状态：

```bash
curl --noproxy '*' -sS http://127.0.0.1:8080/api/v1/health
curl --noproxy '*' -sS http://127.0.0.1:8080/api/v1/admin/setup/status
```

3. 检查身份上下文：

```bash
curl --noproxy '*' -sS http://127.0.0.1:8080/api/v1/admin/user/auth/me/context
```

## 3. 核心回归（US1 / US2 / US3 / US4）

1. root 登录 `/settings/users`：
- 展示 root 视图；
- 可切换并管理任意租户。

2. tenant admin 登录 `/settings/users`：
- 仅管理当前租户成员；
- 跨租户操作返回权限拒绝。

3. 用户管理动作语义：
- 点击租户行：仅查看/聚焦租户数据；
- 点击“切换并管理”：切换上下文；
- 点击“进入 Dashboard”：才做路由跳转。

4. 跨标签一致性：
- Tab A 切换租户后，Tab B 刷新 `/settings/users` 应收敛到最新上下文；
- 不应出现“tenant_uuid 缺失导致视图误分流”。

5. 新租户管理员路径：
- 新租户注册账号进入后应具备本租户 admin 能力；
- 对其他租户数据访问应被拒绝。
- 初始化管理员角色应包含 `role_admin`；`role_owner` 可选附加，不作为 admin 判定依据。

## 4. SaaS IAM 扩展回归（US5 / US6 / US7 / US8）

1. SaaS signup：
- 调用 `POST /api/v1/public/saas/signup` 创建新租户；
- 验证返回 token/context 指向新 `tenant_uuid + member_id/member_uuid`；
- 验证首个成员同时拥有 `role_owner`、`role_admin`、`role_user`；
- 重复 `tenant_key`、已有邮箱错误密码、初始化失败都不得留下半成品数据。

2. Root 平台身份：
- root 登录后默认进入 Platform Console；
- root 默认不展示 `/settings/ai` 和租户插件业务菜单；
- root 不应被前端 `isCurrentTenantAdmin` 判定为当前业务租户 admin。

3. Root Support Session：
- root 创建 support session 时必须填写 target tenant 和 reason；
- 默认只读；
- 写操作必须记录 root actor、target tenant、support session id。

4. 租户插件实例隔离：
- 租户 A 启用插件，租户 B 未启用；
- A 可看到菜单并访问 `/_p/<plugin>/admin` 与 `/_p/<plugin>/api`；
- B 看不到菜单，直接访问插件 admin/api 必须被拒绝；
- 停用 A 的插件不删除全局插件包，也不影响其他租户。
- A、B 都启用同一插件时，PowerX 节点内仍只应有一组 `plugin_id` / `plugin_id_admin` 运行进程；
- 插件进程处理请求时必须从当前请求 token/context 或事件 payload 读取 tenant/member，不得依赖进程启动时固定租户。
- 租户暂停/归档后，该租户插件业务入口和后台任务必须停止，但全局插件进程继续服务其他租户；
- 插件包全局停用/卸载只能由 Root/Platform 执行，并必须检查受影响的租户实例。

5. 历史数据迁移巡检：
- 执行 IAM migration report；
- 验证 root user、`system` tenant、root system member 状态可见；
- 缺 owner 但有 admin 的租户进入 auto-fix candidate；
- 缺 admin 的租户进入 manual-fix required；
- 自动补齐 owner 必须写审计。

## 5. 排障与日志

```bash
sudo journalctl -u powerx-backend -f --no-pager
sudo journalctl -u powerx-web-admin -f --no-pager
```

关键排查点：
- `me/context` 是否返回 `current_tenant_uuid` 与 `members[].is_admin`
- `switch-tenant` 失败是否为 `tenant_uuid` 非法格式或无权限
- 前端是否执行强制刷新：`fetchUserContext({ force: true })`
- WS 报错是否由无效 `tenant_uuid` 引发重连风暴

## 6. 已执行回归（2026-04-06）

```bash
cd backend && GOCACHE=$PWD/.gocache GOMODCACHE=$PWD/.gomodcache go test ./internal/service/auth ./internal/transport/http/admin/user/auth -count=1
cd backend && GOCACHE=$PWD/.gocache GOMODCACHE=$PWD/.gomodcache go test ./tests/contract/iam ./tests/integration/iam -count=1
cd web-admin && npm run test:unit -- tests/unit/settings-users/users-root-actions.spec.ts tests/unit/settings-users/tenant-context.spec.ts
cd web-admin && npm run test:unit -- tests/unit/settings-users/users-actions-semantics.spec.ts tests/unit/settings-users/user-store-context.spec.ts
cd web-admin && npm run build
```

结果：均通过（构建仅存在既有 chunk size warning，无阻断错误）。

E2E（按环境执行）：
```bash
cd web-admin && npm run test:e2e -- tests/e2e/iam/users-actions-semantics.spec.ts tests/e2e/iam/context-consistency.spec.ts
```
说明：若运行环境限制监听端口（例如 `listen EPERM 0.0.0.0:3300`），请在可绑定端口的 CI/宿主机执行。

补充回归（2026-04-06）：
```bash
cd web-admin && npm run test:unit -- tests/unit/settings-users
cd web-admin && npm run test:e2e -- --project=chromium tests/e2e/iam
```
结果：通过（`5 files / 9 tests` unit 全绿，chromium E2E `2 passed`）。

若你已手动启动可访问的前端地址，可跳过 Playwright 内置 webServer：
```bash
cd web-admin && PLAYWRIGHT_SKIP_WEBSERVER=1 PLAYWRIGHT_BASE_URL=http://127.0.0.1:3000 npm run test:e2e -- tests/e2e/iam/users-actions-semantics.spec.ts tests/e2e/iam/context-consistency.spec.ts
```

## 7. 预期结果

- 三类角色权限边界行为与 spec 一致；
- `/settings/users` 不再出现“行点击即跳 dashboard”的复合动作；
- 上下文异常可识别、可重试、可恢复。
- SaaS signup、root support、租户插件实例隔离、历史数据巡检均可独立验证。
