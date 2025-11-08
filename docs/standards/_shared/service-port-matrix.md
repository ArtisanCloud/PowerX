# PowerX Core & Plugin Stack 端口规范（Developer Default）

本表盘点 **PowerX Core**、**PowerX Plugin Skeleton** 与 **PowerX Plugin Marketplace** 三个仓库在本地开发环境下一键启动时的默认端口及其用途，涵盖 HTTP / gRPC / WebSocket / MCP / 辅助服务（MinIO、Redis 等）。除非另行说明，均可通过环境变量覆盖；生产环境请结合具体部署参数与安全策略调整。

| 仓库 | 组件/服务 | 协议 | 默认端口 | 关键路径/配置 | 说明 |
|------|-----------|------|----------|---------------|------|
| **PowerX Core** (`Core/PowerX`) | HTTP API / WebSocket | HTTP / WS | `8077` | `etc/config_example.yaml` → `server.port` | 提供 REST API、WebSocket（`/ws`）与 Admin 控制台代理。 |
| | gRPC 入口 | gRPC | `9001` | `server.grpc.port` | 暴露内部 gRPC 服务，默认启用反射、健康检查，可通过 `CORE_X_SERVER_GRPC_PORT` 覆盖。 |
| | MCP Server | HTTP (SSE/WebSocket) | `8086` | `mcp.server.port` | MCP (Model Context Protocol) 控制通道，默认启动 `/mcp/sse`、`/mcp/message` 端点。 |
| | Agent 网关 | HTTP (SSE/WebSocket) | `8082` | `agent.port` | 供 Agent 运行时收发消息；模式默认为 `ws_sse`。 |
| | Web Admin（前端仓 PowerXDocs 中运行） | Vite/Nuxt Dev | `3030` | `docs/guides/index.md` | 本地脚本 `npm run dev -- --port 3030`，若端口冲突可改写。 |
| | MinIO（若使用本地对象存储） | HTTP | `9000` | `storage.s3.endpoint` | 示例配置指向 `http://127.0.0.1:9000`，用于媒体、插件制品等对象存储。 |
| | Redis（事件总线/缓存） | TCP | `6379` | `cache.port` / `event_bus.redis_addr` | 作为缓存、事件去重与 License key 缓存用途。 |
| **PowerX Plugin Skeleton** (`Core/Plugins/PowerXPlugin`) | Backend HTTP | HTTP | `8078` | `config/config.yaml.example` → `server.listen` | 插件后端服务，监听健康检查 `/healthz` 与业务接口；默认与 Core MCP (`8086`) 分离，仍可通过 `PORT` 覆盖以适配本地环境。 |
| | Backend gRPC | gRPC | `8079` | `docs/guide/standalone-mode.md` | 插件服务暴露的 gRPC 端口，环境变量 `POWERX_GRPC_PORT` 可覆盖。 |
| | Nuxt 管理端 | Vite/Nuxi Dev | `3031` | `docs/guide/standalone-mode.md` | 插件独立运行时的 Web 控制台，冲突时自动寻找空闲端口；可通过 `npm run dev -- --port` 指定。 |
| | Test Preview 前端 | Node Preview | 动态（默认 `REGRESSION_FRONTEND_PORT`） | `docs/test/testing_usage.md` | E2E / 回归测试中可通过环境变量固定端口。 |
| **PowerX Plugin Marketplace** (`Core/PowerXPluginMarket`) | HTTP API | HTTP | `8080` | `backend/etc/config.yaml` → `http.addr` | 提供插件上架、审核、制品发放等接口。 |
| | Storage (MinIO 示例) | HTTP | `9001` | `storage.endpoint` | 默认指向 `http://localhost:9001`，用于 Marketplace 制品、许可证等对象。 |
| | Redis（License / 风险控制） | TCP | `6379` | `license.redis.address` | License 续期、撤销及风险策略依赖 Redis。 |

> **注意**：若本地同时启动 Core 与 Plugin Skeleton，请确保插件后端使用 `PORT=8078`（默认值）或其他未占用端口，确保与 Core MCP (`8086`) 互不干扰。

## 常见覆盖方式

| 变量 | 适用仓库 | 用于改写 | 示例 |
|------|-----------|-----------|------|
| `CORE_X_SERVER_PORT` | PowerX Core | HTTP/WS 端口 | `CORE_X_SERVER_PORT=9080 go run ./cmd/app` |
| `CORE_X_SERVER_GRPC_PORT` | PowerX Core | gRPC 端口 | `CORE_X_SERVER_GRPC_PORT=9101` |
| `CORE_X_MCP_PORT` | PowerX Core | MCP 端口 | `CORE_X_MCP_PORT=9081` |
| `PORT` | Plugin Skeleton | Backend HTTP | `PORT=8078 go run ./cmd/plugin` |
| `POWERX_GRPC_PORT` | Plugin Skeleton | Backend gRPC | `POWERX_GRPC_PORT=8090` |
| `NUXT_PORT` / `PORT` | Plugin Skeleton 前端 | Nuxt Dev 端口 | `npm run dev -- --port 3100 --hmr-port 43100` |
| `MARKETPLACE_HTTP_ADDR` | Plugin Marketplace | API HTTP | `MARKETPLACE_HTTP_ADDR=":8180"` |

若团队需要统一分配端口段，请更新 `.env.local` / `config.yaml` 并在 PR 描述中同步本表。
