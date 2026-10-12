# 编辑客户主资料

## 页面操作和权限

1. 管理员进入用户管理 → 权限 → 角色配置，选择目标角色。
2. 在「能力/API权限」中选择 PowerX 底座 / customer，搜索 `customer.admin_customers_accounts_customer_uuid:update`。
3. 勾选 **PATCH `/api/v1/admin/customers/accounts/{customer_uuid}`** 后保存。创建、联系人编辑和状态修改有各自的权限项。
4. 该角色用户刷新客户管理页面；列表每行的「编辑客户资料」和客户详情中的同名按钮都可以打开表单。
5. 修改姓名、昵称、主邮箱、主电话，点击保存。列表保留当前搜索和页码，详情原地回显。

没有编辑权限时隐藏编辑入口；直接调用接口仍返回 403。只授予创建或菜单权限不授予编辑权限。表单只提交四个资料字段和读取时的版本，姓名必填，可选字段留空表示清空。

## 实际修改的数据

写入 `customer_accounts` 的 `display_name / nickname / primary_email / primary_phone` 和更新时间，并新增一条 `audit_event`。认证身份、登录密码、客户类型、状态、联系人和租户成员关系使用各自合同，不在该表单更新。

Customer UUID 是全局主记录：若它关联多个租户，相关租户会读到修改后的主资料。权限检查要求当前租户拥有该客户的成员关系；每个租户的 Contact 资料仍独立维护。

## 并发、失败和恢复

- GET 和列表返回 `profile_version`，它由客户 UUID 和四个可编辑字段计算。保存时锁定成员关系和客户主记录，比较原版本。
- 其他人先保存时返回 `409 CUSTOMER_PROFILE_CHANGED`。保留当前表单，点击「重新读取（替换当前填写内容）」才载入最新资料，然后重新编辑保存。
- 权限撤销、deny、deprecated 或角色绑定到期后，写请求即时返回 403。
- 审计和资料写入在同一事务；审计失败返回 503 并回滚。
- 切换租户、切换客户、关闭弹窗或组件卸载后，中止等待中的请求并忽略迟到的结果。
- 业务回退：有权限的用户重新读取当前版本，再把四个字段改回原值保存。失败请求无需恢复，它没有提交资料变更。

## 部署与验证

无新增表或列，本轮无需 migrate 或全量 seed。发布新后台和前端后执行 `make capability-seed`，增量同步正式权限和能力目录；命令默认保留既有权限 effect/status、角色和 API Key grants。然后重启 Core，按上面的页面流程授予目标角色编辑权限。

本机验收使用独立 PostgreSQL schema 和临时测试记录，结束删除测试 schema；既有业务客户保持原样。HTTP 验收可通过给 `POWERX_CUSTOMER_ADMIN_TEST_DSN` 配置测试数据库连接，运行 `go test ./internal/transport/http/admin/customer -run TestCustomerProfile -count=1`。不要把 DSN 或密码提交到仓库。

对应实现：`account_profile_handler.go`、`account_admin_profile.go`（Service/Repository）、`CustomerEditModal.vue`。接口合同：[customer-admin-profile.openapi.yaml](../../../contracts/customer-admin-profile.openapi.yaml)。
