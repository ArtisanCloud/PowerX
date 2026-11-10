# 租户用户管理与 Root 系统用户边界

租户管理员页面通过当前登录身份的成员管理接口创建用户。`/admin/system/users` 继续仅供 Root 使用；Root 在租户用户页中的原有操作保持系统接口语义。

## 授权和标识

- 租户成员管理入口为 `/api/v1/admin/iam/members`，需要用户 JWT 和有效的 `corex.iam.member_accounts:read/write/delete` 权限，分别对应读取、写入和删除。仅具有角色名称不绕过实时 RBAC。
- 创建默认 `role_user` 成员只需 `corex.iam.member_accounts:write`；创建时选择其他角色、或修改已有角色绑定时，另外检查 `iam.role:bind`。只能选择当前租户的 tenant-scope 角色，不能分配系统角色或其他租户角色。
- 租户由 JWT 上下文取得，前端不发送目标租户。请求中模型的 tenant/user 标识不能覆盖上下文。
- 页面操作使用 Member UUID。旧部门组件使用的数字 Member ID 仍兼容，但同样验证租户归属。User UUID 不能用于操作 Member。
- 非 Root 用户不能通过成员接口管理 Root 账号。API Key／STS 不用于此管理入口。

## 创建与编辑

创建请求 `POST /admin/iam/members`：

```json
{
  "member": {"username":"person","display_name":"Person","status":1},
  "user": {"email":"person@example.invalid","phone":""},
  "initial_password":"example-password",
  "dept_ids":[],
  "role_uuids":["11111111-1111-4111-8111-111111111111"]
}
```

User、Member、初始密码、部门及角色分配在同一事务中完成。角色无效或未获角色分配权限时全部回滚。创建已有全局账号的成员不会覆盖该账号原有密码；已在当前租户的账号不能重复加入。

编辑请求 `PATCH /admin/iam/members/{member_uuid}`：

```json
{
  "member":{"username":"person","display_name":"Person"},
  "contact":{"email":"person@example.invalid","phone":""},
  "role_uuids":["11111111-1111-4111-8111-111111111111"]
}
```

`role_uuids` 不传则不改角色，传入数组则在编辑事务中替换直接角色；空数组使用本租户 `role_user`。`contact` 区分未传字段与清空手机号，变更邮箱／手机号时同步密码登录标识。联系信息属于全局登录账号；成员姓名、状态和角色属于当前租户。

用户表单支持选择多个部门，回显列表响应中的 `DeptIDs`，保存使用 `dept_ids`。不传该字段则保留已有部门，`[]` 明确清空；只有修改部门选择时前端才提交它。创建和更新均验证部门属于当前租户且未删除，校验失败时整个事务回滚。Root 的系统用户接口遵循同样的字段语义。

部门筛选使用 `dept_id`，页面传 `recursive=true`，包含所选部门及其子部门。租户成员列表和 Root 系统用户列表都在服务端过滤后计数、分页，多部门成员只计一次。成员的 Meta 文本不作为正式部门关系。

部门负责人从当前租户的启用成员目录分页读取，支持搜索；候选加载失败时显示错误与重试操作。`PATCH /admin/iam/departments/{id}` 的 `leader_member_id` 不传时保留负责人，传成员 ID 时同步 ID 与 UUID，传 `null` 时清空两列。服务端拒绝其他租户、已删除或停用的成员。

## 关联操作

| 操作 | 请求 |
| --- | --- |
| 列表 | `GET /admin/iam/members?page=1&page_size=10&q=...` |
| 成员详情 | `GET /admin/iam/members/{member_uuid}` |
| 读取直接角色 | `GET /admin/iam/members/{member_uuid}/roles` |
| 替换直接角色 | `PUT /admin/iam/members/{member_uuid}/roles`，body 为 `{"role_uuids":[...]}` |
| 启停成员 | `PUT /admin/iam/members/{member_uuid}/status`，body 为 `{"status":1}` 或 `{"status":2}` |
| 删除成员 | `DELETE /admin/iam/members/{member_uuid}` |
| 恢复成员 | `PUT /admin/iam/members/{member_uuid}/restore` |
| 重置登录密码 | `PUT /admin/iam/members/{member_uuid}/password`，body 为 `{"new_password":"example-password"}` |

停用、删除只作用于当前租户成员，不全局停用或删除 User。重置密码作用于已验证归属的登录账号，并保护 Root。前端将历史视图里的禁用值 `0` 转为成员接口的禁用状态 `2`。

## 正式权限登记与本地授权

三项管理权限都归 Core 所有，来源为 `platform_capability`：

| 显示名称 | 权限码 | 用途 |
| --- | --- | --- |
| 查看租户成员 | `corex.iam.member_accounts:read` | 列表、详情与角色读取 |
| 管理租户成员账号 | `corex.iam.member_accounts:write` | 创建、编辑、启停、密码与角色写入 |
| 删除租户成员 | `corex.iam.member_accounts:delete` | 删除当前租户成员关系 |

先定向同步这三项正式能力和权限元数据，再由 Root 在“权限 → 角色配置 → 当前租户的 Tenant Admin → PowerX 底座 → IAM”明确勾选并保存。目录同步不改变已有租户角色 grants；不能凭角色名称自动扩权。只验证创建时，至少授予 read/write。

历史 `iam.user:read/write` 在升级实例中可能已经被插件占用或弃用，不能用它们代替上述权限。有效授权按 UUID 关系、权限来源、active/allow 状态和角色绑定有效期核验，不依赖陈旧数字关联列。

## 本地验收

1. 后端加载新代码后，使用非 Root 的 Tenant Admin 登录；角色应有用户管理权限；若要分配其他角色，还需角色绑定权限。
2. 进入“设置 → 用户管理 → 用户”，新增用户、选择 Tenant User 并保存。请求应为 `POST /api/v1/admin/iam/members`，成功后列表能找到新成员。
3. 检查编辑、角色、禁用／启用、密码重置及删除；所有租户操作应使用 Member UUID。
4. 使用新账号验证租户登录；现有 Root 专用系统用户接口仍拒绝该 Admin。
5. 编辑已有多部门用户，确认全部回显；仅改姓名不改变部门，取消全部选择并保存后再次编辑应为空。选择上级部门筛选时包含子部门成员，重置后恢复完整列表与总数。
6. 在部门编辑弹层搜索并选择负责人，保存后重新打开确认回显；选择“无”保存后确认已清空。候选接口失败时应能看到错误并手动重试，不能伪装成只有“无”。

自动化覆盖见 `member_accounts_test.go`、`users-pagination.spec.ts`、`tenant-user-service.spec.ts` 及浏览器用户创建用例。自动化使用隔离数据，不替代用户本地账号的人工验收。

底座成员管理权限由 `iam_member_accounts.yaml` 正式声明，来源必须为 `platform_capability` 且 active/allow，并按 UUID 角色关联及有效期实时检查。历史 `iam.user:read/write` 可能被插件占用或弃用，不作为底座管理的授权兜底。目录同步不自动修改租户角色授权；Root 在权限页对需要的角色明确勾选新成员管理权限后生效。
