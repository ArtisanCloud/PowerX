## Research Findings

### Decision: CapabilityRegistry 归属 CoreX Agent，采用 Postgres + Redis 双层形态
- **Rationale**: Agent Hub、Workflow Builder、Integration Gateway 均是 CoreX 核心模块，Registry 若落插件会破坏统一升级路径；Postgres 提供版本、审计、事务，Redis 负责 3 分钟内的同步展示，契合 spec 的“单一事实来源 + 缓存刷新”目标。
- **Alternatives considered**:
  - 插件级 Registry：需要每个插件部署数据库，治理成本高且无法跨插件聚合。
  - 仅 Redis：缺少持久化审计，重启后需重新从 `.pxp` 恢复，延迟 >3 分钟。

### Decision: Selector 采用 `capability.policy.prefer` → MCP/REST 并发读、gRPC 写兜底，并记录 fallback 事件
- **Rationale**: 读能力强调低延迟与流式反馈，MCP + REST 并发命中可提高 90% 成功率；写能力必须保持幂等与强一致，强制走 gRPC；Selector fallback 事件与指标是治理闭环的关键。
- **Alternatives considered**:
  - 单通道（全部 gRPC）：读响应慢且失去 MCP 工具体验。
  - 调用方自行切换协议：Selector 无法做统一审计，违背 spec “选择器优先” 原则。

### Decision: Workflow/Agent 模板升级必须人工确认（锁定旧 `capabilities_hash`）
- **Rationale**: 插件频繁更新 Workflow 模板，若自动升级会导致租户流程未经验证即变更，Spec Clarification 也要求默认锁定；提供 Admin/Builder 批量升级动作即可控制节奏。
- **Alternatives considered**:
  - 自动升级：高风险，回滚成本大。
  - 全量禁止重复使用旧模板：阻塞租户复用既有流程。

### Decision: 观测与事件统一走 `integration.gateway.*` + W3C TraceContext
- **Rationale**: 成功/失败事件需 1 分钟内到达，下游告警也依赖可预测 topic；W3C Trace ID 在 HTTP/Gin、gRPC 和 MCP 中均可注入，方便串联 InvocationTrace + EventPublication。
- **Alternatives considered**:
  - 每通道独立事件：难以做跨协议汇总，也无法满足 95% Trace 完整性指标。
  - 自定义追踪实现：重复已有 OTel 能力，增加维护成本。

### Decision: gRPC 合同命名 `powerx.integration_gateway.v1`，Buf 输出到 `api/grpc/gen/go/powerx/integration_gateway/v1`
- **Rationale**: 与现有命名规范一致，后续在多模块共享 `CapabilityInvoke` DTO；Buf 管理 go_package 与 source_relative 路径，避免自定义脚本。
- **Alternatives considered**:
  - 复用旧 `integration_route` 协议：字段与语义无法覆盖 `protocols`、`workflow_template_ref` 等新增信息。
  - 独立仓库维护 proto：打破单体流程，增加 CI 开销。
