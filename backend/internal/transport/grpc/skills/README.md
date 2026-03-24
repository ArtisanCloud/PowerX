# Skills gRPC Transport

本目录实现 Skills gRPC 服务：
- SkillAdminService
- SkillInvokeService

约束：
- 服务注册由全局 gRPC server 统一装配。
- 通过拦截器链执行 auth/tenant/logging/recovery。
