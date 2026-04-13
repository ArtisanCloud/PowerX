# Research: 监控中心闭环（Backup + Logs）

## Decision 1: 自动备份默认频率采用每 6 小时
- Decision: 默认调度频率为每 6 小时执行一次。
- Rationale: 在恢复点目标与资源成本间平衡，满足“日内可恢复”需求且不会显著放大备份窗口冲突风险。
- Alternatives considered:
  - 每小时：恢复点更细，但 IO/存储与任务争用成本偏高。
  - 每天：成本低，但数据回退窗口过大。

## Decision 2: 默认保留最近 14 份备份
- Decision: 默认仅保留最近 14 份备份产物。
- Rationale: 支持短中期误操作回滚，同时避免快速堆积导致存储告警常态化。
- Alternatives considered:
  - 7 份：窗口偏短，难覆盖跨周问题。
  - 30 份：成本与清理压力明显增大。

## Decision 3: 恢复任务默认每周一次
- Decision: 对启用自动备份的策略默认每周触发一次恢复任务。
- Rationale: “可恢复性”是闭环核心，每周频率可及时发现产物损坏、脚本漂移或权限变更问题。
- Alternatives considered:
  - 每两周/每月：演练成本低，但风险暴露滞后。
  - 仅手动：无法形成稳定运营基线。

## Decision 4: 连续 2 次失败升级为高优先级告警
- Decision: 备份作业连续两次失败后自动升级为高优先级告警。
- Rationale: 单次失败可能为瞬时抖动，连续失败更能表征系统性风险。
- Alternatives considered:
  - 单次即升级：噪声高，容易告警疲劳。
  - 连续 3 次升级：发现偏慢，存在数据保护窗口风险。

## Decision 5: 默认时区固定 Asia/Shanghai
- Decision: 自动调度默认时区使用 Asia/Shanghai。
- Rationale: 与当前生产排障语境一致，减少“日志时间/调度时间”对齐成本。
- Alternatives considered:
  - UTC：统一但对本地运维不直观。
  - 跟随服务器时区：多环境不一致风险。

## Decision 6: 监控入口与备份中心形成双入口闭环
- Decision: 在“监控中心”展示任务状态和日志汇总，在“备份中心”展示策略、作业、恢复任务操作。
- Rationale: 监控页承担态势感知，备份页承担运维操作，职责清晰且减少单页复杂度。
- Alternatives considered:
  - 全部聚合到单页：信息密度过高，操作路径混乱。

## Decision 7: 日志能力采用多驱动适配（loki/file/stdio）
- Decision: 统一日志 API，后端按驱动适配，前端能力感知降级。
- Rationale: 不同部署形态（本地/单机/prod）驱动不同，必须保证功能可用而不是“只在 loki 可用”。
- Alternatives considered:
  - 只支持 loki：会让 file/stdio 部署失效。
  - 每种驱动独立页面：维护成本高，用户心智割裂。

## Decision 8: Logs/Trace 采用“应用内查询 + Grafana 深链”双层模式
- Decision: 在 PowerX 内提供统一检索与基础分析，在 `loki` 驱动下提供 Grafana 深链用于深度排障。
- Rationale: 兼顾平台内闭环体验与专业日志平台能力，不强迫用户离开控制台即可做一级排障。
- Alternatives considered:
  - 仅 Grafana：平台内无法形成闭环，权限与地址配置复杂。
  - 仅应用内：高级聚合与可视化能力不足。

## Decision 9: 安全边界放在后端代理层
- Decision: 前端不直接携带 Loki 凭据，所有日志查询通过 Admin API 代理。
- Rationale: 降低密钥泄露风险，便于审计和限流，并统一错误语义。
- Alternatives considered:
  - 前端直连 Loki：凭据暴露风险高，跨环境配置复杂。
