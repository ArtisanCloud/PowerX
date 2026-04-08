# Phase 0 Research: PowerX 部署与运维治理基线

## Decision 1: 首版生产拓扑采用“单节点上线 + 多节点兼容标准前置”

- Decision: 生产首发采用单节点应用拓扑，配置与部署资产按未来 K8s/多节点兼容设计。
- Rationale: 在交付速度、运行复杂度、可维护性之间平衡最佳，同时避免后续架构返工。
- Alternatives considered:
  - 双节点高可用首发：上线风险与实施复杂度显著增加。
  - 直接多节点体系一次到位：对当前阶段不经济，且验收链路过长。

## Decision 2: 数据库恢复目标采用 RTO 1 小时 + RPO 15 分钟

- Decision: 设定恢复目标为 RTO<=1h、RPO<=15m。
- Rationale: 与业务连续性要求匹配，且可通过“定期全量备份 + WAL 归档 + 演练”落地。
- Alternatives considered:
  - RTO 4~8 小时：恢复能力偏弱，不利于生产故障窗口控制。
  - RPO 1 小时/24 小时：数据丢失窗口过大。
  - RPO 5 分钟：首版实施成本与复杂度偏高。

## Decision 3: 日志栈采用 Promtail + Loki + Grafana，默认保留 30 天

- Decision: 统一日志汇聚方案采用 Promtail 采集，Loki 存储，Grafana 查询与告警，首版默认保留 30 天。
- Rationale: 可覆盖部署排障、插件升级窗口、审计复盘等核心场景，成本与可追溯性平衡。
- Alternatives considered:
  - 7/15 天保留：历史排障窗口不足。
  - 90 天保留：首版成本压力大，建议后续按合规需求升级。

## Decision 4: 插件升级流程采用“安装不启用 -> 验证 -> 切换 -> 回滚”

- Decision: 固化版本切换流程，禁止覆盖式直接升级。
- Rationale: 最大限度降低插件变更风险，满足“可回退、可审计、可追踪”。
- Alternatives considered:
  - 直接覆盖升级：故障风险高，回滚窗口窄。
  - 仅灰度管线首发：当前无完整市场能力，落地门槛高。

## Decision 5: 高风险操作审批策略按环境可配置

- Decision: 在管理控制台支持“无需审批”与“双人审批”按环境切换。
- Rationale: 兼顾不同阶段治理强度（开发/预发/生产）与落地效率。
- Alternatives considered:
  - 全量强制双人审批：首版实施与流程成本过高。
  - 永不审批：生产风险不可控。

## Decision 6: P0 页面范围限定为 Deploy/Plugin/Backup 三域

- Decision: P0 只交付部署发布中心、插件生命周期中心、备份恢复中心。
- Rationale: 形成最小可闭环运维能力；日志与配置密钥中心作为 P1 扩展。
- Alternatives considered:
  - 首版全量交付 6 个中心：范围过大，延期风险高。

## Decision 7: 迁移策略采用“全量迁移 + 切换窗口 + 回滚预案”

- Decision: A->B 实例迁移采用标准化 runbook，明确“数据库迁移完成”不等于“实例完整迁移完成”。
- Rationale: 降低迁移误判风险，保障切换与回滚确定性。
- Alternatives considered:
  - 仅迁库：会遗漏配置/插件目录/对象存储，导致运行不一致。

