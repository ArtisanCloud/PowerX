# MCP 工具编目与命名规范

本文聚焦在“PowerX 自身 MCP 工具”与“第三方/插件 MCP 工具”如何共存，目标是让配置与运营层面一眼区分来源，同时保证 Agent 调用体验一致。

## 1. Tool Spec 目录与加载顺序

PowerX 在 `etc/config_example.yaml` 中演示了如何通过 `mcp.tool_spec` 同时加载多个目录：

```yaml
mcp:
  tool_spec:
    core_dir: "PowerX/mcp/tool_specs"
    app_dirs:
      - "MediaX/mcp/tool_specs"
      - "extensions/mcp/tool_specs"
```

启动 MCP Server 时会遍历 `core_dir` 及 `app_dirs` 中的所有 spec，并写入统一注册表。【F:etc/config_example.yaml†L45-L68】【F:internal/server/mcp/server.go†L16-L104】

**建议目录划分**：

| 目录示例 | 存放内容 | 适用 HandlerType |
| --- | --- | --- |
| `PowerX/mcp/tool_specs/core/` | 官方内建工具（账号、知识库等） | `native` / `script` |
| `PowerX/mcp/tool_specs/plugins/` | 插件通过代理暴露的能力 | `remote` |
| `PowerX/mcp/tool_specs/external/` | SaaS、第三方 MCP 平台 | `remote` |
| `tenant/<id>/tool_specs/` | 租户自定义工具 | `remote` / `script` |

## 2. 命名与元数据标注

### 2.1 Tool ID 前缀

约定工具 ID 前缀便于 Agent、UI 与日志区分来源：

| 前缀 | 含义 | 示例 |
| --- | --- | --- |
| `px.` | PowerX 官方 | `px.account.lookup` |
| `plg.` | 插件托管 | `plg.crm.sync` |
| `ext.` | 外部第三方 | `ext.salesforce.query` |
| `biz.` | 业务自研 | `biz.media.publish` |

### 2.2 metadata 扩展字段

`ToolSpec` 支持附加 `metadata` 字段，可配置来源、可见性、鉴权策略等：

```json
{
  "id": "ext.salesforce.query",
  "handler_type": "remote",
  "metadata": {
    "provider": "salesforce",
    "category": "crm",
    "visibility": "tenant-only"
  }
}
```

注册流程会把 metadata 透传给 MCP Server，方便管理端过滤与展示。【F:internal/server/mcp/register/factory/remote.go†L18-L43】

## 3. 配置与运行时隔离

| 场景 | 建议做法 | 代码/配置 |
| --- | --- | --- |
| 多租户隔离 | 按租户维护独立的 spec 目录，通过配置载入 | `etc/config_example.yaml` 中的 `app_dirs` 列表可按租户生成 |
| 环境区分（dev/stage/prod） | 通过环境变量覆盖 Tool Spec 路径 | `config/config.go` 读取 `MCP_TOOL_SPEC_*` 环境变量（如已扩展） |
| 插件灰度 | 在插件管理器中控制插件启停，并与 Tool Spec 同步 | 可在插件发布流程中自动生成/移除 spec 文件 |

## 4. 管理端展示建议

- **工具市场/配置页面**：读取 MCP 注册表，按 `provider`、`category`、`visibility` 分组展示，官方工具默认启用，第三方工具提供启用开关。  
- **审计日志**：将调用日志中记录的工具 ID、provider、来源目录等信息落库，方便追踪。  
- **自动化校验**：在 CI 中对 Tool Spec 目录运行校验脚本，确保 ID 前缀、metadata 字段符合约定。

## 5. 与 Agent 执行的衔接

无论工具来源如何，Agent 调用时只需要工具 ID。通过上述命名与 metadata，编排层可以：

- 根据工具 ID 决定是否允许在当前 Flow/租户使用；
- 对第三方工具追加鉴权（例如读取 metadata 中的 `auth_type`）；
- 在日志或监控里区分工具来源，避免混淆。

如需在 Flow 节点中直接区分来源，可在节点参数里增加 `source` 字段，并由执行器结合 Tool Spec metadata 做校验。
