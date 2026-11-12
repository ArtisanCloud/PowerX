# 成员组织关系依据

`com.corex.iam.organization.service_read` 通过固定 `core://iam/organization`、`INVOKE` 与 `read_member_authority` 操作提供事实读取。请求为 `powerx.iam.member-authority-query.v1`，包含1至200个唯一规范成员 UUID；租户从服务身份取得，调用者不能传租户、HTTP方法、地址或代理参数。

返回 `powerx.iam.member-authority.v1`，包含评估时间、每个请求成员的明确结果、有效直接成员角色、部门及所需祖先、直属上级和部门负责人 UUID。角色关联只按实际 UUID 连接；历史数字 ID 不作为替代。直属上级未配置、成员不存在、关系对象已停用分别显式表达。只读一致性事务限制角色关联4000条、部门2000个、层级64层；超限、循环、缺少组织字段或身份不一致明确失败，不截断为成功。

这些是组织事实，不是业务授权决定。调用方仍须验证来源范围、当前角色、任务分派和操作独立性。部门父子关系不能用于推测负责人，唯一候选也不能充当直属上级。

服务面需要显式授权 `_scope.iam.organization.service_read`、`read`、`capability`、`iam_organization`。每次读取检查当前授权；API Key 或 STS 服务身份不能通过此能力修改组织关系。管理员成员维护继续使用独立 `com.corex.iam.member_accounts.admin_write` 和用户身份。

## 明确维护直属上级

`PUT /api/v1/admin/iam/members/{member_uuid}/reporting-manager` 使用成员账户管理的用户权限。请求必须显式包含 `manager_member_uuid`（规范 UUID 或明确 null）、`expected_updated_at` 和真实 `reason`。遗漏目标字段不能被解释为清除。有效租户行串行化修改，检查目标同租户且有效、禁止自指与循环，按源时间条件写入并与审计同事务提交；失败整体回滚。

新增 `direct_manager_member_uuid` 是可空字段，普通迁移只增加结构，不猜测或回填历史上级。部门负责人读取现有 `leader_member_uuid`；不从旧数字负责人字段推断。

## 验证和启用边界

2026-11-12已在临时 PostgreSQL 验证实际 UUID 关联、关系有效性、角色撤销、服务面授权、缺失 schema、循环与并发反向报告线、版本冲突及审计失败回滚。能力检查、隔离能力同步及实际隔离 API Key grant-status 分别验证只读能力可用和管理员写能力未被授予。没有修改日常服务、数据库、组织关系或实际 API Key 授权。

部署时须先完成 schema 与能力同步，再按实际服务身份明确授予最小读取权限；撤销客户端/服务授权可停止读取。回退源码不应删除已维护关系；删除字段或历史修复须单独计划。DTO 详见 [OpenAPI 契约](iam-member-authority.openapi.yaml)。
