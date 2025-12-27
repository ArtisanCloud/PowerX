# Capability Registry OpenAPI Handlers

该目录对应 specs/007-integration-gateway-and-mcp 中租户侧 `capability_registry` HTTP API（`/tenant/capabilities`、`/tenant/invocations*` 等）。
后续任务将于此实现 Gin Handler、DTO 绑定与路由装配，并复用统一错误结构与 Selector。
