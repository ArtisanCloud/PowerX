# Phase 0 研究结论

## 模块边界与目录布局

- **Decision:** 将媒体能力定义为 CoreX 内核模块的一部分，核心逻辑放置在 `internal/infra/media` 下，包含驱动（driver）、管理器（manager）与服务封装。  
  数据模型与仓储接口定义在 `pkg/corex/db/persistence/model/media` 与 `pkg/corex/db/persistence/repository/media`。
- **Rationale:** Media 属于 CoreX 的基础能力，需被 Agent、Workflow、Knowledge 等模块长期复用。  
  内核实现统一的对象存储、权限校验与租户隔离，避免插件化导致的版本分裂与安全隔离问题。
- **Alternatives considered:** 将媒体能力以插件形式提供；但这会带来依赖混乱和重复 IAM 审计逻辑。

## 数据持久化与软删除策略

- **Decision:** 在 PostgreSQL 中创建 `media_assets` 表，使用 `gorm.Model` 扩展结构体，增加软删除标记与租户字段。
- **Rationale:** CoreX 采用 PostgreSQL 作为主存储（参考 `pkg/corex/db`），软删除符合 FR-006 要求，并支持后台清理任务。
- **Alternatives considered:** 使用对象存储元数据或 Redis 存档；不具备结构化查询与审计能力。

## 存储驱动抽象与默认驱动

- **Decision:** 使用 `MediaManager` 聚合驱动接口（`StorageDriver`），默认提供 `local` 与 `s3` 两种实现，配置文件指定默认驱动。
- **Rationale:** Manager 负责驱动生命周期、错误捕获与策略路由，契合“模块最小接口”与“可插拔驱动”原则。
- **Alternatives considered:** 在 Service 层硬编码驱动逻辑；不利于后续接入新云厂商或多租户隔离。

## 预签名链接与 TTL

- **Decision:** 默认生成 12 小时有效期的预签名链接（上传/下载），并在配置文件中支持自定义。
- **Rationale:** 统一 TTL 方便前后端签名同步，满足跨系统传输和中长流程审批场景。
- **Alternatives considered:** 固定短期（如 1 小时）；无法覆盖文档审阅或视频分发需求。

## 审计与租户隔离

- **Decision:** 复用 CoreX 的审计与租户机制：通过 `corex/audit` 写入日志，并在表结构中维护 `tenant_id`。
- **Rationale:** 宪法要求所有内核模块强制租户隔离与审计一致性，减少插件层重复实现。
- **Alternatives considered:** 仅在业务表记录操作者；缺乏跨模块审计与追踪能力。
