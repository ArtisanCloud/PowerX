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

### Decision: Gateway 鉴权采用 API Key / JWT 单凭证分流
- **Rationale**: 插件 Host/Standalone、系统联调与生产集成需要统一行为；Host 模式固定 JWT，Standalone/外部调用固定 API Key，避免“失败回退”导致行为不确定。
- **Alternatives considered**:
  - JWT 优先：与分场景凭证模型不一致，且会弱化 API Key 的治理价值。
  - API Key 失败回退 JWT：会引入隐式提权与排障歧义。
  - 仅 internal/ws-bus 支持 API Key：会形成入口分裂，导致策略和审计不可统一。

### Decision: JWT 必须执行“签名校验 + 主体状态校验”双阶段鉴权
- **Rationale**: 仅验证 JWT 签名、过期时间无法覆盖 `db-refresh`、租户迁移、成员禁用等场景；旧 token 可能继续通过第一层中间件，造成“表面有效、业务无效”的异常状态。
- **Alternatives considered**:
  - 仅签名校验：性能高但安全与一致性不足，无法满足租户隔离与失效收敛要求。
  - 每次都直查 DB：一致性好但高并发下 DB 压力过高，不适合作为默认路径。

### Decision: 主体校验采用 `cache-first + DB-fallback`，并配合事件失效
- **Rationale**: 认证属于高频路径，必须降低 DB 抖动风险。先查 Redis 快照（user/member/tenant），未命中再回源 DB 并回填缓存，可兼顾吞吐与正确性。
- **Alternatives considered**:
  - 长 TTL 纯缓存：失效收敛慢，风险窗口过大。
  - 无缓存仅 DB：在 Gateway 高频调用场景下会放大数据库压力。

### Decision: 引入 `session_version`（或 `token_epoch`）实现强制失效
- **Rationale**: 需要一个可显式推进的“会话代际”来立即失效既有 token；适用于密码重置、角色重大变更、租户重建（含 `db-refresh`）等运维场景。
- **Alternatives considered**:
  - 仅依赖 JWT 短 TTL：窗口期仍可能较长，且无法应对立即失效需求。
  - 全局改 JWT 密钥：可行但过于粗暴，会影响全租户在线会话。

### Decision: `me/context` 发现主体漂移时返回失败，不做静默“自动修正租户”
- **Rationale**: 自动 fallback 到某个 membership 虽可减轻前端报错，但会掩盖真实失效态，导致“token 已不一致但页面仍显示已登录”，排障成本高。
- **Alternatives considered**:
  - 保留自动修正：短期体验平滑，但长期会导致权限错觉和多租户行为不透明。
