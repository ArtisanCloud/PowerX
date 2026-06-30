# Quickstart: IAM 用户与角色 RBAC 统一能力

## 1. 前置条件

- 当前分支：`026-iam`
- 已完成初始化并可访问 backend/web-admin
- 具备三类账号：`root`、`tenant admin`、`member`

账号矩阵（建议）：
- `root`: 平台超管，可跨租户管理
- `tenant admin`: 非 root，仅管理本租户
- `member`: 普通成员，无租户级用户管理权限
- `vendor`: 供应商成员，绑定 `role_vendor`，默认只开放供应商工作台相关入口

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
- Tab A 切换租户后必须保存服务端返回的新 token/context，Tab B 刷新 `/settings/users` 应收敛到最新上下文；
- 不应出现“tenant_uuid 缺失导致视图误分流”。

5. 新租户管理员路径：
- 新租户注册账号进入后应具备本租户 admin 能力；
- 对其他租户数据访问应被拒绝。
- 初始化管理员角色应包含 `role_admin`；`role_owner` 可选附加，不作为 admin 判定依据。

## 3A. 菜单 RBAC 回归

菜单权限是角色级权限，不做用户级单独授权。权限模型固定为：

```text
module=menu
resource=<menu resource>
action=read
```

常用菜单权限示例：

```text
menu:agent:read
menu:agent.chat:read
menu:skills:read
menu:knowledge:read
menu:workflow:read
menu:dashboard:read
menu:settings.users:read
menu:plugin.com.powerx.plugins.base.templates:read
```

插件/App 菜单权限不通过人工创建。插件 manifest 声明 `frontend.admin.menus` 后，插件安装/启用/权限同步会自动生成：

```text
module=menu
resource=plugin.<plugin_id>.<menu_id>
action=read
source=plugin:<plugin_id>
```

角色权限页面会把这类权限展示在“已安装 App / <插件名称>”分组；管理员只负责勾选授权，不维护菜单资源本身。

验证步骤：

1. 执行 seed，确保内置权限和角色写入：

```bash
cd backend
make seed
```

2. 检查菜单权限是否存在：

```sql
SELECT module, resource, action, meta
FROM public.iam_permission
WHERE module = 'menu'
ORDER BY resource;
```

检查插件/App 菜单权限：

```sql
SELECT module, resource, action, source, meta
FROM public.iam_permission
WHERE module = 'menu'
  AND resource LIKE 'plugin.%'
ORDER BY resource;
```

3. 检查供应商角色是否存在：

```sql
SELECT tenant_uuid, code, name, builtin
FROM public.iam_role
WHERE code = 'role_vendor';
```

4. 检查角色菜单授权：

```sql
SELECT r.code, p.module, p.resource, p.action
FROM public.iam_role r
JOIN public.iam_role_permission rp ON rp.role_id = r.id
JOIN public.iam_permission p ON p.id = rp.permission_id
WHERE p.module = 'menu'
ORDER BY r.code, p.resource;
```

5. 登录不同角色后请求菜单：

```bash
curl --noproxy '*' -sS "$BASE/admin/menus" \
  -H "Authorization: Bearer $TOKEN"
```

预期：
- `role_admin` 默认拥有租户可用菜单；
- `role_user`/`role_readonly` 只拥有白名单菜单；
- `role_vendor` 默认只拥有 Dashboard、Agent、Agent Chat、Plugin Chat 等供应商工作台入口；
- 插件/App 菜单只有在插件 manifest 同步出 `menu:plugin.<plugin_id>.<menu_id>:read`，且当前角色拥有该权限时才显示；
- 没有对应 `menu:*:read` 的菜单不应出现在响应里。

页面设计：
- 第一版复用 `/settings/users` 的角色权限管理，不新增系统菜单 CRUD。
- 权限列表中 `meta.type=menu` 的记录归入“菜单权限”分组；插件菜单按 `meta.plugin_name` 显示为“已安装 App / <插件名称>”。
- 后续如需独立页面，新增 `/settings/menus`，只做菜单树预览、角色授权查看和跳转到角色权限编辑，不允许租户自由修改系统菜单结构。

## 4. SaaS IAM 扩展回归（US5 / US6 / US7 / US8）

1. SaaS signup：
- 调用 `POST /api/v1/public/saas/signup` 创建新租户；
- 验证返回 token/context 指向新 `tenant_uuid + member_id/member_uuid`；
- 验证首个成员同时拥有 `role_owner`、`role_admin`、`role_user`；
- 页面要求用户填写租户名称、组织标识、邮箱或手机号、密码；验证码仅在开关开启时展示；
- `tenant_key` 默认由系统根据租户名称生成，用户可以手动修改；显式填写的 `tenant_key` 必须全局唯一，冲突时注册失败且不得留下半成品数据；
- 验证码由 `feature_gate.enable_saas_signup_verification_code` 控制，默认关闭；关闭时页面不展示验证码字段，注册接口也不要求 `verification_code`；
- 未显式填写 `tenant_key` 时，重复租户名称允许存在，系统应生成 `acme-inc`、`acme-inc-2` 这类唯一 key；
- 已有邮箱/手机号错误密码、显式 key 冲突、初始化失败都不得留下半成品数据。

示例请求：

```bash
curl --noproxy '*' -sS http://127.0.0.1:8080/api/v1/public/saas/signup \
  -H 'Content-Type: application/json' \
  -d '{
    "tenant_name": "Acme Inc",
    "tenant_key": "acme-inc",
    "plan": "free",
    "owner_email": "owner@example.com",
    "owner_password": "secret123",
    "owner_display_name": "Owner"
  }'
```

启用验证码后：

```yaml
feature_gate:
  enable_saas_signup_verification_code: true
```

```bash
curl --noproxy '*' -sS http://127.0.0.1:8080/api/v1/public/saas/signup/verification-code \
  -H 'Content-Type: application/json' \
  -d '{"contact":"owner@example.com"}'
```

当前默认驱动是本地开发驱动，会把验证码写入后端日志；生产接入 SMTP/短信服务时替换 `SignupVerificationDriver`。

登录默认租户回归：
- 使用同一邮箱或手机号创建两个租户；
- 直接登录，不传组织，系统应进入 `iam_user.last_tenant_uuid` 指向的最近租户；
- 删除或禁用最近租户 member 后再次登录，系统应回退到该 user 的第一个 active member；
- 手机号注册账号的顶部用户信息和侧边栏不得显示伪造邮箱。

租户切换回归：
- 调用 `/api/v1/admin/user/auth/me/switch-tenant` 后，响应必须包含 `access_token`、`refresh_token` 和 `context`；
- 前端必须保存新 token，再刷新 `me/context`；
- 新 token 的 claims 必须包含目标 `tenant_uuid + member_id/member_uuid`。

2. Root 平台身份：
- root 登录后默认进入 Platform Console；
- root token 可以带 `system` 特殊租户的 `tenant_uuid + member_id/member_uuid`，这是平台身份锚点，不代表业务租户 admin；
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
- 停用 A 的插件不删除全局插件包，也不影响其他租户；
- A、B 都启用同一插件时，PowerX 节点内仍只应有一组 `plugin_id` / `plugin_id_admin` 运行进程；
- 插件进程处理请求时必须从当前请求 token/context 或事件 payload 读取 tenant/member，不得依赖进程启动时固定租户。
- 租户暂停/归档后，该租户插件业务入口和后台任务必须停止，但全局插件进程继续服务其他租户；
- 插件包全局停用/卸载只能由 Root/Platform 执行，并必须检查受影响的租户实例。
- 插件卸载、下架、drain、replace 只能作用于目标 `plugin_id` 或 `plugin_id + version`，不得影响其他插件和同租户其他业务；
- 同版本 replace 只替换目标版本物理包和运行时，不得删除租户实例、订阅、权限、配置或业务数据。

验证命令示例：

```bash
BASE=http://127.0.0.1:8080/api/v1
PLUGIN_ID=com.powerx.plugins.base

# 租户 A token 下启用插件实例。该接口只创建/启用 TenantPluginInstance，不启动新的租户级插件进程。
curl --noproxy '*' -sS -X POST "$BASE/admin/plugins/tenant-instances/$PLUGIN_ID/enable" \
  -H "Authorization: Bearer $TENANT_A_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"config":{}}'

# 租户 A 可见租户实例；租户 B 未启用时不应出现在列表中。
curl --noproxy '*' -sS "$BASE/admin/plugins/tenant-instances" \
  -H "Authorization: Bearer $TENANT_A_TOKEN"
curl --noproxy '*' -sS "$BASE/admin/plugins/tenant-instances" \
  -H "Authorization: Bearer $TENANT_B_TOKEN"

# 租户 B 直接访问插件 admin/api 必须被拒绝。
curl --noproxy '*' -i "http://127.0.0.1:8080/_p/$PLUGIN_ID/admin/" \
  -H "Authorization: Bearer $TENANT_B_TOKEN"
curl --noproxy '*' -i "http://127.0.0.1:8080/_p/$PLUGIN_ID/api/v1/healthz" \
  -H "Authorization: Bearer $TENANT_B_TOKEN"

# 状态接口必须明确展示全局共享进程，而不是租户进程。
curl --noproxy '*' -sS "$BASE/admin/plugins/$PLUGIN_ID/status" \
  -H "Authorization: Bearer $ROOT_TOKEN"
```

`status` 返回中应包含：

```json
{
  "runtime_scope": {
    "scope": "global_plugin_process",
    "tenant_isolated": false,
    "shared_by_tenants": true
  },
  "tenant_instances": {
    "managed_by": "TenantPluginInstance"
  }
}
```

卸载影响检查：

```bash
curl --noproxy '*' -i -X POST "$BASE/admin/plugins/$PLUGIN_ID/uninstall" \
  -H "Authorization: Bearer $ROOT_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"purge":true}'
```

如果仍存在任意租户实例，必须返回 `409`，并在响应 details 中给出 `requires_tenant_instance_cleanup=true`。清理路径是逐租户停用/删除租户实例；不得用卸载全局包来隐式删除租户业务状态。

drain / final uninstall 验收口径：

1. Root 发起普通 uninstall 时，如果存在任意 `TenantPluginInstance`，接口必须拒绝同步卸载并返回 `DRAIN_REQUIRED` 或等价错误码。
2. Root 创建 drain plan 后，目标插件必须停止新增订阅、启用、业务写入、scheduler job、queue task、workflow run、webhook/event delivery。
3. 每个租户实例必须独立进入 `draining_requested`，并在 active session/request/task/job/event 清零且插件 `DrainStatus=ready` 后进入 `drained`。
4. `idle` 不等于 `drained`。只要目标插件入口仍开放，用户仍可新增任务，就不能 final uninstall。
5. 所有目标租户实例 `drained` 后，Root 才能执行 final uninstall，停止目标插件运行时、卸载目标插件动态路由、删除目标版本 registry，并在 `purge=true` 时删除目标版本物理目录。
6. final uninstall 不得重启 PowerX backend、web-admin、数据库、Redis、Event Fabric、Scheduler、STS 或 Gateway，也不得影响其他插件运行时、路由、租户实例、订阅、配置和业务数据。
7. 前端全局 loading 只能表示卸载请求正在执行或等待响应，不得表达为 PowerX 底座正在重启。
8. `emergency disable` 只阻断目标插件继续使用，必须保留租户实例、订阅、配置、凭证引用和历史业务数据。

同版本 replace 验收口径：

1. replace 只能由 Root/Platform 发起。
2. replace 允许停止目标 `plugin_id + version` 的运行时、删除目标版本目录并复制新的同版本 dist。
3. replace 完成后，租户实例列表、订阅状态、插件配置、菜单授权和历史业务数据必须保持不变。
4. 生产常规升级不得依赖同版本 replace，应安装新版本、healthcheck、切 current version，并具备失败回滚。

租户暂停/归档验证：

```bash
# 将租户置为非 active 后，该租户的 TenantPluginInstance.enabled 应被批量置为 false。
curl --noproxy '*' -sS "$BASE/admin/plugins/tenant-instances" \
  -H "Authorization: Bearer $SUSPENDED_TENANT_TOKEN"
```

预期：该租户插件入口不可见，`/_p/<plugin>/admin` 和 `/_p/<plugin>/api` 拒绝访问；同一全局插件进程仍可服务其他 active 租户。

5. 历史数据迁移巡检：
- 执行 IAM migration report；
- 验证 root user、`system` tenant、root system member 状态可见；
- 缺 owner 但有 admin 的租户进入 auto-fix candidate；
- 缺 admin 的租户进入 manual-fix required；
- 自动补齐 owner 必须写审计。

只读巡检：

```bash
make iam-migration-report

# 或直接执行
cd backend && go run ./cmd/database iam-report
```

预期 JSON：

```json
{
  "root_users": [],
  "system_tenant_status": "ok",
  "root_system_member_status": "ok",
  "tenant_owner_missing": [],
  "tenant_admin_missing": [],
  "auto_fix_candidates": [],
  "manual_fix_required": []
}
```

受控 owner 补齐：

```bash
make iam-migration-fix-owner

# 或直接执行
cd backend && go run ./cmd/database iam-fix-owner
```

补齐规则：
- 只处理 `auto_fix_candidates` 中“缺 owner 但已有 active admin”的租户；
- 给最早的 active admin member 追加 `role_owner`；
- 写入 `audit_event.operation=IAM_MIGRATION_FIX_OWNER`；
- 对 `manual_fix_required` 中缺 active admin 的租户只报告，不自动猜测 owner。

生产发布顺序：
1. 备份数据库。
2. 执行 `make db-migrate`。
3. 确认迁移包含 tenant domain 回填、member username 租户内唯一索引、API key profile 租户内唯一索引、`iam_user.last_tenant_uuid`。
4. 执行 `make db-seed`，保留 root user、`system` tenant member 和 setup 完成记录。
5. 执行 `make iam-migration-report`。
6. 若存在 `auto_fix_candidates`，执行 `make iam-migration-fix-owner`。
7. 若存在 `manual_fix_required`，由 root 在 Platform Console 人工指定管理员后再次巡检。
8. 巡检通过后再开放 SaaS signup 与租户插件实例隔离能力。

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

Final SaaS IAM 回归（Phase 12）：

```bash
cd backend && GOCACHE=$PWD/.gocache GOMODCACHE=$PWD/.gomodcache go test ./tests/contract/iam ./tests/integration/iam ./tests/contract/plugin ./tests/integration/plugin -count=1
cd web-admin && npm run test:unit -- tests/unit/iam tests/unit/plugins
```

前端操作验收截图建议覆盖：
- root 登录后的 Platform Console 入口，确认不展示租户 AI Settings 和租户插件业务菜单；
- tenant admin 登录后的 `/settings/users`，确认只显示当前租户成员管理；
- SaaS signup 成功后的 dashboard/context，确认当前 `tenant_uuid + member_id/member_uuid` 指向新租户；
- 插件市场或已安装插件页，确认展示“全局插件包”和“本租户启用状态”的区别；
- 租户未启用插件时直接访问 `/_p/<plugin>/admin` 的拒绝状态；
- IAM migration report 输出，确认 `auto_fix_candidates` 与 `manual_fix_required` 可见。

若你已手动启动可访问的前端地址，可跳过 Playwright 内置 webServer：
```bash
cd web-admin && PLAYWRIGHT_SKIP_WEBSERVER=1 PLAYWRIGHT_BASE_URL=http://127.0.0.1:3000 npm run test:e2e -- tests/e2e/iam/users-actions-semantics.spec.ts tests/e2e/iam/context-consistency.spec.ts
```

## 7. 预期结果

- 三类角色权限边界行为与 spec 一致；
- `/settings/users` 不再出现“行点击即跳 dashboard”的复合动作；
- 上下文异常可识别、可重试、可恢复。
- SaaS signup、root support、租户插件实例隔离、历史数据巡检均可独立验证。
