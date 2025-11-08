# Phase 0 Research — Plugin Release & Marketplace Publishing Foundation

## 1. Observability & Alerting Stack
- **Decision**: 继续沿用 PowerX 现有 Prometheus + Grafana 栈，新增 plugin_release-* 指标、灰度 KPI 仪表、以及 5 分钟内回滚 SLA 的专属告警规则。
- **Rationale**: 与现有 CoreX 监控体系完全兼容，可直接复用 scraping/alertmanager pipeline，减少多套栈带来的权限、费用与集成复杂度，同时满足 Constitution 对统一观测面的要求。
- **Alternatives considered**: (a) 切换至 Datadog/SaaS（成本高且需要跨境数据合规评估）；(b) 自研轻量指标管道（维护成本高且无法提供跨租户 dashboard、告警联动）。

## 2. Offline Distribution Storage
- **Decision**: 离线分发库复用现有多区域对象存储集群（MinIO/S3 兼容），针对 plugin_release 建立加密分区并写入签名、指纹与元数据索引。
- **Rationale**: 现有对象存储已通过备份、加密与容量验证，直接复用可简化部署，同时满足 FR-005/NFR-INF-001 要求的 180 天可追溯性。
- **Alternatives considered**: (a) 独立离线镜像站（缺乏自动化同步且维护成本高）；(b) 第三方 CDN（审计与补件流程难以与租户隔离）。

## 3. Marketplace 补件治理
- **Decision**: 当同一版本补件次数达到 2 次时自动升级至合规负责人，并通过调度中心暂停后续发布窗口直至补件完成。
- **Rationale**: 快速暴露高风险版本、与现有 SLA 监控联动，且与 Edge Case 要求一致，可防止补件超时导致的批量上线延迟。
- **Alternatives considered**: (a) 仅记录补件率（无法实时遏制高风险版本）；(b) 使用评分模型动态 SLA（实现成本高且缺乏基础数据）。

## 4. Pipeline Orchestration Backbone
- **Decision**: 测试租户流水线与审批链基于现有 Workflow Engine + Event Fabric，service 层以 `pipeline.Service` 调用 workflow DAG 节点（构建、扫描、审批、计划生成），CLI 和 Web Admin 通过 gRPC/HTTP 触发。
- **Rationale**: Workflow Engine 已具备租户/审计能力，可用 DAG 节点表达 24 小时 SLA 与回滚步骤，避免重复造轮子；Event Fabric 负责指标与告警广播。
- **Alternatives considered**: (a) 另建 Argo/Temporal 集群（引入外部依赖、权限复杂）；(b) 单体 Cron Job（缺失 DAG/补偿语义，无法满足多阶段审批）。

## 5. CLI ↔ Backend Transport
- **Decision**: `px-plugin` 与 `px publish` 均通过 gRPC `PluginReleaseService` 交互（含本地构建产物上传、计划查询、灰度触发），Web Admin 使用 HTTP Admin/OpenAPI 层，所有入口复用同一 service/repository。
- **Rationale**: CLI 需要流式上传与实时日志，gRPC 支持双向流与统一 AuthN；Admin API 仍以 Gin/HTTP 便于与现有控制台整合。共用 service 可执行一致的 RBAC/审计。
- **Alternatives considered**: (a) CLI 全走 REST（流式性能与错误反馈不佳）；(b) 为 CLI 单独实现直连数据库/缓存（破坏多租户隔离与审计要求）。
