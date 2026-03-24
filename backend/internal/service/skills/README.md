# Skills Service Layer

本目录承载 Skills 领域服务：导入校验、生命周期状态机、调用路由、审计追踪。

约束：
- Service 层负责业务规则，不直接拼接 SQL。
- Repository 依赖通过构造函数注入。
- 所有调用逻辑必须显式传递 `tenant_uuid` 并记录 `trace_id`。
