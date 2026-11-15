# 成员组织关系依据

`com.corex.iam.organization.service_read` 通过固定 `core://iam/organization`、`INVOKE` 与 `read_member_authority` 操作提供事实读取。请求为 `powerx.iam.member-authority-query.v1`，包含1至200个唯一规范成员 UUID；租户从服务身份取得，调用者不能传租户、HTTP方法、地址或代理参数。

返回 `powerx.iam.member-authority.v1`，包含评估时间、每个请求成员的明确结果、有效直接成员角色、部门及所需祖先、直属上级和部门负责人 UUID。角色关联只按实际 UUID 连接；历史数字 ID 不作为替代。直属上级未配置、成员不存在、关系对象已停用分别显式表达。只读一致性事务限制角色关联4000条、部门2000个、层级64层；超限、循环、缺少组织字段或身份不一致明确失败，不截断为成功。

这些是组织事实，不是业务授权决定。调用方仍须验证来源范围、当前角色、任务分派和操作独立性。部门父子关系不能用于推测负责人，唯一候选也不能充当直属上级。

服务面需要显式授权 `_scope.iam.organization.service_read`、`read`、`capability`、`iam_organization`。每次读取检查当前授权；API Key 或 STS 服务身份不能通过此能力修改组织关系。管理员成员维护继续使用独立 `com.corex.iam.member_accounts.admin_write` 和用户身份。

## 调用失败的 HTTP 状态

`POST /api/v1/tenant/invocations` 保留组织服务 `IAM_ORGANIZATION_*` typed `dto.AppError` 的原始 HTTP 状态，包装错误同样按类型解包。参数不合法为 400，缺少服务身份为 401，租户不可用为 403，结构缺失、组织身份不完整、数量超限或服务不可用为 503；不把这些业务失败转换成通用 502。

错误响应保留独立的 `error_code`、`reason_code`、请求 `request_id` 和 `details.trace_id`。调用方应读取这些正式字段；`error` 是说明字符串，不能作为对象读取。仅字符串含相同前缀、未分类异常或不具备正式机器错误契约的 REST 错误，仍按既有通用失败处理。

2026-11-15 已完成路由回归与六个 Core Gin HTTP → .NET Framework 客户端场景，核对原状态及错误码、Request/Trace ID；详见[验证记录](evidence/iam-organization-http-status-20261115.json)。HTTP 场景注入业务错误，未连接数据库或验证实际授权，不代表日常实例部署或 CRM 业务验收。

## 明确维护直属上级

`PUT /api/v1/admin/iam/members/{member_uuid}/reporting-manager` 使用成员账户管理的用户权限。请求必须显式包含 `manager_member_uuid`（规范 UUID 或明确 null）、`expected_updated_at` 和真实 `reason`。遗漏目标字段不能被解释为清除。有效租户行串行化修改，检查目标同租户且有效、禁止自指与循环，按源时间条件写入并与审计同事务提交；失败整体回滚。

新增 `direct_manager_member_uuid` 是可空字段，普通迁移只增加结构，不猜测或回填历史上级。部门负责人读取现有 `leader_member_uuid`；不从旧数字负责人字段推断。

## 开发库限定结构同步

已有安装的 dev/test PostgreSQL 可在核对目标库并备份后，从仓库根目录执行：

`cd backend && go run ./cmd/database migrate -config etc/config.yaml -scope notification-reminder-dependencies`

该 scope 复用数据库迁移锁、事务和锁等待上限；要求既有 `iam_member` 与 `system_notifications` 表。只补 `iam_member.direct_manager_member_uuid` 可空 UUID 列及索引，以及 `system_notification_creation_receipts` 表、UUID 主键和 `(tenant_uuid, service_owner, key_hash)` 联合唯一索引，不对成员整表 AutoMigrate，不运行 seed、修改授权、发送通知或补造历史回执。上级关系值仍由管理员明确维护，普通通知不要求配置该关系。

重复执行保留已配置上级及已有回执；已有列类型、可空性或索引定义不兼容时明确失败，事务回滚本轮结构变更，不自动改列、删表或替换错误索引。staging/prod 继续使用正式完整升级计划。回退调用方源码时保留已新增结构及业务数据。

## 验证和启用边界

2026-11-12已在临时 PostgreSQL 验证实际 UUID 关联、关系有效性、角色撤销、服务面授权、缺失 schema、循环与并发反向报告线、版本冲突及审计失败回滚。能力检查、隔离能力同步及实际隔离 API Key grant-status 分别验证只读能力可用和管理员写能力未被授予。没有修改日常服务、数据库、组织关系或实际 API Key 授权。

部署时须先完成 schema 与能力同步，再按实际服务身份明确授予最小读取权限；撤销客户端/服务授权可停止读取。回退源码不应删除已维护关系；删除字段或历史修复须单独计划。DTO 详见 [OpenAPI 契约](iam-member-authority.openapi.yaml)。
