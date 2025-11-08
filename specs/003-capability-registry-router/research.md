## Research Findings

### Decision: Registry 快照按能力 ID × 租户存储
- **Rationale**: 满足多租户隔离要求，允许不同租户独立配置适配器与策略，同时维持环境级策略作为扩展字段，避免重复记录。
- **Alternatives considered**:
  - 全局单快照：无法覆盖租户差异化策略，违背 Constitution 第 III 条多租户要求。
  - 能力 × 环境快照：仍需额外区分租户，导致策略叠加复杂。

### Decision: 客户端缓存 TTL 默认 2 分钟
- **Rationale**: 在缓存命中率与变更传播延迟之间取得平衡，与 Success Criteria 要求的 3 分钟同步窗口兼容。
- **Alternatives considered**:
  - 30 秒：刷新频率过高，增加 Registry 压力。
  - 5 分钟：变更传播延迟过大，影响降级与策略修复。

### Decision: Router 失败冷却时间设为 60 秒
- **Rationale**: 给予适配器恢复时间，降低抖动，同时在 SLA 要求下保持较快复原，符合健康检查策略。
- **Alternatives considered**:
  - 30 秒：易造成频繁切换及误判。
  - 5 分钟：降级恢复过慢，违反 SC-002 500ms 自动降级目标。

### Decision: Registry 写入采用乐观并发控制（ETag/版本号）
- **Rationale**: Registry 高并发背景下需支持并行操作，版本冲突可被检测并提示用户重试，配合审计记录。
- **Alternatives considered**:
  - 后写覆盖：存在配置丢失风险，不符合审计可追溯性。
  - 全局锁串行化：放大延迟，影响高频策略更新。

### Decision: 使用 Postgres + Redis + EventBus 作为基础设施
- **Rationale**: Spec 假设底层存储与缓存已就绪；Postgres 适合版本化快照与审计，Redis 支撑 discovery 缓存与权重热更新，EventBus 提供实时推送。
- **Alternatives considered**:
  - 纯内存存储：不满足持久化与历史回溯要求。
  - 仅依赖 Postgres：推送延迟高，无法满足 30 秒内广播目标。
