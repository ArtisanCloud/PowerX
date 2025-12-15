# Tenant UUID-only 示例服务

> 该目录收录了在 PowerX 内开发新服务/脚本时应遵循的最小实践，帮助团队验证“只暴露 `tenant_uuid`” 的行为。本 README 可作为脚手架或 code review checklist。

## 目录结构

- `README.md`（本文件）
- `http-handler.md`（可选：记录示例 handler 伪代码）
- `repo.sql`（可选：示例表结构，默认仅展示 `tenant_uuid` 列）

> 当前仅提供文档示例；如需实际代码，可参考 `backend/internal/service/agent_lifecycle`、`backend/tests/contract/agent_lifecycle` 已完成的 UUID-only 改造。

## 实践要点

1. **入口**：HTTP/gRPC handler 必须从 `reqctx.RequireTenantUUIDFromGin` 或上下文解析 UUID，并拒绝 body/query/header 中的 `tenant_id` 字段。
2. **Service**：函数签名以 `tenantUUID string` 作为租户标识；如确需数值 ID，仅能在 Repository 层通过 `tenants` 表查出，并不得向上返回。
3. **Repository**：表结构只保留 `tenant_uuid` 列，可与 `UNIQUE (tenant_uuid, ...)` 组合索引；禁止新增/保留 `tenant_id`。
4. **测试**：使用 `pkg/testing/tenantuuid` 或测试常量（`tests/fixtures.TenantUUID`）注入合法 UUID，并断言响应 JSON/Proto 中不存在 `tenant_id`。
5. **监控/日志**：在日志、metrics、OpenTelemetry attribute 中统一使用 `tenant_uuid` 字段，示例：`log.With("tenant_uuid", tenantUUID)`.

## 代码片段（伪代码）

```go
func (h *ExampleHandler) Create(ctx *gin.Context) {
    tenantUUID := reqctx.RequireTenantUUIDFromGin(ctx)
    input := parseRequest(ctx) // 不包含 tenant 字段

    if err := h.service.Create(ctx, tenantUUID, input); err != nil {
        render.Error(ctx, err)
        return
    }
    render.Success(ctx)
}

func (s *ExampleService) Create(ctx context.Context, tenantUUID string, input CreateInput) error {
    return s.repo.Insert(ctx, repository.CreateArgs{
        TenantUUID: tenantUUID,
        Payload:    input.Payload,
    })
}
```

## 参考
- `backend/tests/contract/agent_lifecycle/http_helpers_test.go`：演示如何在测试中断言响应不包含 `tenant_id`。
- `scripts/migrations/tenant-uuid/README.md`：了解 schema 迁移要求。
- `docs/operations/tenant-uuid-upgrade.md`：面向外部的行为准则。
