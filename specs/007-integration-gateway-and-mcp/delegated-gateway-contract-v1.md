# Delegated Gateway Contract v1（精准版，不做兼容）

> Token 边界以 `docs/guides/auth/plugin_auth_token_model.md` 为准。

## 1. 目标

在宿主模式（delegated）下，插件访问 PowerX Capability Gateway 的认证链路只保留一套契约，避免多变量、多策略并存导致排障困难。

业务调用主凭证统一为 STS access token；`PX_PLUGIN_TOOL_TOKEN` 仅用于 bootstrap/过渡探活。

## 2. 强约束（MUST）

1. 认证方案固定为 `bearer`。
2. `PX_PLUGIN_TOOL_TOKEN` 仅可用于启动期契约探活（bootstrap），不得作为业务调用长期凭证。
3. 插件仅接受 `PX_GATEWAY_BASE_URL` 作为 Gateway 入口地址。
4. delegated 模式下，任一关键变量缺失都必须启动失败（fail-fast），禁止运行中软降级成 503。
5. delegated 模式下禁止使用 `PX_GATEWAY_API_KEY`。
6. delegated 模式下禁止读取 `PX_TOOL_TOKEN`（不保留别名兼容）。

## 3. PowerX（宿主）责任边界

1. 在插件进程启动前注入：
   - `PX_GATEWAY_BASE_URL`
   - `PX_GATEWAY_AUTH_SCHEME=bearer`
   - （可选）`PX_PLUGIN_TOOL_TOKEN`，仅用于 bootstrap 探活
2. 注入后执行启动前检查，缺失即拒绝启用插件，并返回结构化错误码。
3. 插件启用成功后执行凭证探活（health + dry-run），探活目标按接口元数据（`auth_required`、`tenant_scoped`）选择，失败则将插件状态标记为启用失败并附原因。
4. 不允许默认走凭证推送 stub 路径；若使用凭证下发通道，必须保证真实链路可用且可观测。

## 4. PowerXPlugin（插件框架/运行时）责任边界

1. delegated 模式初始化 Gateway Client 时必须强校验：
   - `PX_GATEWAY_BASE_URL` 非空
   - `PX_GATEWAY_AUTH_SCHEME == bearer`
   - 若执行 bootstrap probe 且 `auth_required=true`，则 `PX_PLUGIN_TOOL_TOKEN` 非空
2. 删除 delegated 分支中的以下读取逻辑：
   - `PX_TOOL_TOKEN`
   - `PX_GATEWAY_API_KEY`
3. 所有 capability 调用入口统一复用同一个 Gateway Guard，输出统一错误结构与错误码；业务请求需按当前请求上下文执行 STS exchange，禁止按 URL 前缀硬编码鉴权策略。
4. 启动日志必须输出（脱敏）：
   - `iam_mode`
   - `gateway_auth_scheme`
   - `gateway_base_url_present`
   - `plugin_tool_token_present`

## 5. 统一错误码（建议）

- `GW_CFG_MISSING_BASE_URL`
- `GW_CFG_INVALID_AUTH_SCHEME`
- `GW_CFG_MISSING_PLUGIN_TOOL_TOKEN`
- `GW_CFG_APIKEY_FORBIDDEN_IN_DELEGATED`
- `GW_BOOTSTRAP_CONTRACT_BROKEN`

## 6. 版本策略

本方案为 breaking change，不提供兼容期。实施顺序：

1. 先落地 PowerX 注入与启用前检查。
2. 再落地 PowerXPlugin 的严格校验与旧变量删除。
3. 最后切换文档和 CI 检查规则，阻止旧变量回流。
