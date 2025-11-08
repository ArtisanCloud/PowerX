# Data Model — Plugin Release & Marketplace Publishing Foundation

> 所有实体存放于 Postgres，表名采用 `px_plugin_release_*` 前缀。GORM 模型位于 `pkg/corex/db/persistence/model/plugin_release`，并通过 `pkg/corex/db/database/migration.go` 注册 AutoMigrate。所有记录均包含 `TenantID`, `CreatedAt`, `UpdatedAt`, `CreatedBy`, `UpdatedBy`, `TraceID`.

## 1. PluginReleaseCandidate
- **Purpose**: 描述每次提交的构建、扫描与审批状态，是发布流程的单一真相源。
- **Key Fields**  
  | Field | Type | Notes |
  |-------|------|-------|
  | `ID` | `uint64` | 主键，自增 |
  | `TenantID` | `uint64` | 多租户隔离列，复合索引 (`TenantID`, `Version`) |
  | `PluginID` | `uuid` | 关联 plugins registry |
  | `Version` | `varchar(64)` | 语义化版本，唯一键 (`TenantID`, `PluginID`, `Version`) |
  | `BuildArtifactURI` | `text` | 指向构建结果（S3/对象存储） |
  | `CommitHash` | `char(40)` | Git commit |
  | `ReleaseNotes` | `text` | Markdown/JSON |
  | `ScanScore` | `jsonb` | 覆盖率、安全/许可证扫描结果 |
  | `GateStatus` | `enum(pending,failed,passed)` | 质量门禁状态 |
  | `ApprovalStatus` | `enum(draft,submitted,approved,rejected)` | 审批链结果 |
  | `Labels` | `jsonb` | Feature flag/渠道标签 |
  | `RollbackPlanID` | `uint64` | 外键→ReleasePlan |
  | `AuditRef` | `uuid` | 对应 `AuditEvent` |
- **Lifecycle / State Machine**: `draft → submitted → gate_failed/passed → approved/rejected`. 质量门禁失败或版本信息不一致会退回 `draft` 并记录 `FailureReason`.

## 2. ReleasePlan
- **Purpose**: 生产部署蓝图（上线窗口、灰度批次、回滚联系人）。
- **Key Fields**  
  | Field | Type | Notes |
  |-------|------|-------|
  | `ID` | `uint64` | 主键 |
  | `ReleaseCandidateID` | `uint64` | 外键 |
  | `WindowStart/WindowEnd` | `timestamptz` | 上线窗 |
  | `CanaryBatches` | `jsonb` | 每批租户/流量百分比、指标阈值 |
  | `RollbackScripts` | `jsonb` | shell/ansible refs |
  | `DependencyList` | `jsonb` | 依赖服务/插件 |
  | `NotificationTargets` | `jsonb` | PagerDuty/Email/Webhook |
  | `ApprovalTrail` | `jsonb` | 角色 + 签署时间 |
  | `Status` | `enum(draft,scheduled,executing,completed,rolled_back)` |
- **Relationships**: 1:N 到 `CanaryDeploymentRecord`; 1:1 with `ReleaseCandidate`.

## 3. CanaryDeploymentRecord
- **Purpose**: 记录灰度批次执行细节与指标。
- **Key Fields**  
  | Field | Type | Notes |
  |-------|------|-------|
  | `ID` | `uint64` | 主键 |
  | `ReleasePlanID` | `uint64` | 外键 |
  | `BatchName` | `varchar(64)` | 例如 `tenant_batch_1` |
  | `TenantScope` | `jsonb` | 租户/百分比列表 |
  | `MetricSnapshot` | `jsonb` | QPS、错误率、p95 |
  | `ThresholdBreached` | `bool` | 是否越界 |
  | `ActionTaken` | `enum(continue,hold,rollback)` |
  | `CompletedAt` | `timestamptz` | 用于 5 分钟 SLA 监控 |
- **Constraints**: (`ReleasePlanID`, `BatchName`) 唯一；提供 GIN 索引以支持指标查询。

## 4. OfflineDistributionPackage
- **Purpose**: 离线 `.pxp` 包与校验文件的编目。
- **Fields**  
  | Field | Type | Notes |
  |-------|------|-------|
  | `ID` | `uint64` | 主键 |
  | `ReleaseCandidateID` | `uint64` | 外键 |
  | `PackageURI` | `text` | 对象存储路径 |
  | `Checksum` | `char(128)` | sha512/sha256 |
  | `SignatureFingerprint` | `char(64)` | 统一签名策略 |
  | `Dependencies` | `jsonb` | 插件/服务依赖 |
  | `LicenseReport` | `jsonb` | 第三方库清单 |
  | `HealthCheckLog` | `text` | 导入演练日志 |
  | `Status` | `enum(draft,submitted,approved,rejected,superseded)` |
  | `SLADeadline` | `timestamptz` | 48h 审核限期 |
- **Relationships**: 1:N 到 `MarketplaceListing`（在线/离线双通道共用相同包）。

## 5. MarketplaceListing
- **Purpose**: 上架审核与渠道同步。
- **Fields**  
  | Field | Type | Notes |
  |-------|------|-------|
  | `ID` | `uint64` | 主键 |
  | `OfflinePackageID` | `uint64` | 外键 |
  | `Channel` | `enum(online,offline,hybrid)` |
  | `Pricing` | `jsonb` | 定价/货币 |
  | `SupportPolicy` | `jsonb` | SLA/联系人 |
  | `SubmissionForm` | `jsonb` | Web Admin 表单快照 |
  | `ReviewStatus` | `enum(pending,need_fix,approved,rejected,published)` |
  | `ReviewCount` | `int` | 累计补件次数 |
  | `EscalatedAt` | `timestamptz` | >=2 次补件自动赋值 |
  | `NotificationTicket` | `uuid` | 指向通知/报表任务 |
- **Rules**: `ReviewCount >= 2` 自动设置 `EscalatedAt` 并触发暂停窗口；上架成功时写入 `PublishedAt`.

## 6. LocalInstallSession
- **Purpose**: 支撑 FR-001/002 的本地构建+安装闭环。
- **Fields**  
  | Field | Type | Notes |
  |-------|------|-------|
  | `ID` | `uuid` | 主键 |
  | `TenantID` | `uint64` | 对应本地测试租户 |
  | `DeveloperID` | `uint64` | 操作者 |
  | `ArtifactURI` | `text` | CLI 推送的产物 |
  | `Status` | `enum(in_progress,success,failed)` |
  | `LogPointers` | `jsonb` | 日志/trace 链接 |
  | `FeatureFlags` | `jsonb` | 启用的实验特性 |
  | `ExpiredAt` | `timestamptz` | 清理缓存 |
- **Relationships**: 与 `PluginReleaseCandidate` loosely coupled（成功导入后可引用 CandidateID）。

## 7. Audit Linking
- **Purpose**: 180 天审计要求。
- **Implementation**: `AuditTrailRef` 列统一引用 `pkg/corex/db/persistence/model/audit.AuditEvent` 表。所有服务层方法必须调用 `audit.Service.RecordReleaseEvent(...)`，包含 `tenant_id`, `actor`, `action`, `resource`.

## Derived Views & Indexes
- Materialized view `mv_plugin_release_status`（按租户/渠道聚合当前状态）供仪表盘使用。
- Timescale/Partitioning：`plugin_release_candidates` 按月分区，支撑 180 天留存并便于清理。
- Redis key scheme：`plugin_release:pipeline:{tenant}:{candidate}`，保存流水线状态与幂等 token。

## Validation & Integrity Rules
1. 所有版本必须通过统一签名策略（证书提前 30 天预警）—在模型层校验 `SignatureFingerprint` 与吊销名单。
2. `ReleasePlan` 必须引用 `RollbackScripts`、`NotificationTargets`，否则审批无法进入 `approved`。
3. `OfflineDistributionPackage` 状态不为 `approved` 时禁止生成 `MarketplaceListing`.
4. Local Install Session 15 分钟 TTL，超时自动回滚并通知开发者。
5. 每个 `CanaryDeploymentRecord` 在记录 `ThresholdBreached = true` 时必须关联 `ActionTaken = rollback` 并在 5 分钟内落日志，用于自动化回滚 KPI。
