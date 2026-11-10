# LLM 流式服务合同 v1

Framework 使用 `POST /api/v1/tenant/invocations/stream` 调用 `com.corex.ai.llm.stream`，明确指定 `preferred_protocol=core_internal`。正式绑定为 `core://ai/llm/stream`、方法 `INVOKE`、`service_actor`、租户范围。DTO 使用 `LLMStreamRequest` 的 `model_key`、`inputs`、`params` 和 `stream_options`；不接收任意 URL、HTTP 方法、鉴权头、租户字段或额外 context。query 仅允许 env，且不能覆盖凭据中明确绑定的环境。

## 授权与登记

正式服务授权为 `_scope.ai.llm.stream / stream / capability / ai_llm_stream`。平台权限生成器必须生成 `type=capability_service`、`allow_api_key=true`、`api_key_explicit=true` 和固定内部 endpoint。保留 capability 发布、租户最新 registration、当前凭据及实时精确 grant、模型授权和版本锁检查。

现有 `POST:/api/v1/ai/llm/stream` 路由授权不是该服务 grant，不据此推断或自动扩大任何 Key 的权限。直接 REST 接口继续使用自身正式授权；Framework 不回退到它。同步 capability 不会自动把新权限授予任何 Key，也不扩大共享 Profile。

## 事件与错误

Core SSE 事件为 `start`、`delta`、可选 `usage`、成功终止 `done`，或失败 `error`。成功的 `done` 包含 `ok=true`、最终 text、finish_reason、traceId。Framework 把 `done` 适配为统一 LLM 事件 `final` 后 `end`；EOF、旧 end/completed 帧或 ok 缺失不能代表本合同成功结束。关闭消费者必须取消模型调用并关闭生产者，原始供应商错误正文不向流透传。

HTTP 非 200 错误使用现有结构化错误信封，调用方保留安全的 error_code、reason_code、request_id、trace_id 和已发布 details，不保存凭据、原始响应体或任意上游字段。

## 部署步骤

1. 运行 `make capability-check`，构建新 Core；Core 由服务所有者重新启动以加载 LLMStreamInvoker。
2. 从 Core 根目录执行 `make capability-seed CAPABILITY_SEED_FLAGS="-capability-ids=com.corex.ai.llm.stream"`。该命令只同步指定能力及其正式权限元数据，保留已安装状态；不会执行业务数据修复。若实际版本锁要求升级批准，使用正式能力升级流程，不强制跳过。
3. 在 API Key 管理分区找到当前 AI Craft 使用的开发 Key，点击该行“追加权限”，只预选这个 Key；选择标记为“服务能力调用”的 LLM Stream，预览核对后提交。权限配置和追加弹层共用调用类型展示，HTTP 接口权限展示其 HTTP 入口，服务权限展示固定 Core 内部入口；同名标题不能作为相同授权的依据。该操作不修改共享 Profile，也不清除现有权限。通过正式显式授权接口或 UI 执行，不直接 UPDATE/INSERT 权限表。
4. 对当前服务身份检查 grant-status 为 granted。撤销该条服务 grant 后实际调用必须再次被拒绝，旧 REST 路由权限不能补足它。
5. AI Craft 通过 replace 使用本地 Framework，重新编译/启动后再验收 Shopify 原问题和实际进度查询。已保存失败消息不自动重发。

受控验收覆盖：正式 YAML 到权限生成器、真实 API Key HTTP 中间件与 InvocationService 到 LLMStreamInvoker、缺 grant/撤权拒绝、跨租户/环境与代理参数拒绝、事件结束及消费者关闭。模型替身不代表真实模型、真实 Shopify、已运行 Core 或其数据库已完成升级。
