# Implementation Plan: 监控中心闭环（Backup + Logs）

**Branch**: `027-monitor-center`（功能标识，不要求切换当前分支） | **Date**: 2026-10-28 | **Spec**: [spec.md](spec.md)
**Plan Status**: In Progress（US8 既有交付与待验收边界见下文；US9 首批只读切片已本地实现，完整治理与部署验收待完成）
**Input**: Feature specification from `/specs/027-monitor-center/spec.md`

## Summary

围绕 Root 管理员构建“监控中心闭环”：
- 已落地：自动备份策略、作业历史、告警升级、恢复任务、监控入口联动。
- 本轮补齐：日志与链路追踪能力（`loki/file/stdio` 三驱动能力感知 UI、统一查询接口、Grafana 深链）。
- 本轮新增：插件 framework logger 与宿主采集链路对齐（stdout+json 默认链路、policy/probe 编排、低基数标签约束）。
- 2026-10-15 新增计划：系统快照统一数据库、媒体、必要配置和程序版本，支持完整包跨环境恢复及受控正式切换；本地已实现统一包、Root 页面／API／离线工具、原生新集群与应用验证、独立正式切换及中断补偿；Linux／生产插件和真实定时第二轮尚待验收。
- 2026-10-28 新增 US9：120 GiB 主机正式／开发双环境的容量、历史窗口与生命周期治理。详细设计位于 docs，任务状态统一维护在 [tasks.md 的 US9](tasks.md#us9-storage-governance)，首批只读切片已本地实现，完整治理与运行验收尚未完成。

## Technical Context

**Language/Version**: Go 1.26.7（backend）, TypeScript + Nuxt 4（web-admin）
**Primary Dependencies**: Gin HTTP, GORM, PostgreSQL, Redis, systemd/ops scripts, Nuxt UI  
**Storage**: PostgreSQL（策略/作业/恢复/告警元数据）+ 文件或对象存储（备份产物）+ 日志后端（loki/file/stdio）  
**Testing**: Go test（service/handler）, Nuxt build + E2E smoke（ops/backup, monitor/logs）  
**Target Platform**: Linux server (systemd, nginx)  
**Project Type**: Web application（backend + web-admin）  
**Performance Goals**: 监控页面查询 p95 < 1s；日志查询首屏 < 10s；告警可见性 ≤ 1 分钟  
**Constraints**: 整站快照与恢复仅 Root 用户会话；默认时区 Asia/Shanghai；隔离演练不改正式业务，采集／正式切换按受控停写窗口协调；日志能力按驱动降级
**Scale/Scope**: 单租户优先验证，支持扩展到多租户；监控页默认最近 15 分钟窗口

## Constitution Check

- **Domain Ownership**: CoreX 运维与系统能力，按 CoreX Module 实施。
- **COREX_DECLARED**: PASS
- **NO_PLUGIN_REGISTRY**: PASS
- **COREX_LAYOUT_MATCH**: PASS
- **COREX_DUAL_TRANSPORT**: CONDITIONAL（本期仅 Admin HTTP；gRPC 后续补齐）
- **COREX_BUF_CONFIG**: PASS（本轮不新增 proto）
- **COREX_SERVER_WIRING**: PASS
- **COREX_MIGRATION_WIRING**: PASS

## Project Structure

### Documentation (this feature)

```
specs/027-monitor-center/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── backup-center.openapi.yaml
│   └── monitor-logs.openapi.yaml
└── tasks.md
```

### Source Code (repository root)

```
backend/
├── internal/
│   ├── service/backup_ops/
│   ├── service/monitor_logs/
│   ├── transport/http/admin/backup/
│   ├── transport/http/admin/monitor/
│   └── transport/http/admin/menu/

web-admin/
├── app/pages/ops/backup.vue
├── app/pages/monitor/index.vue
├── app/components/ops/backup/
└── app/components/monitor/
```

**Structure Decision**: 继续沿用现有 backend + web-admin 结构，日志能力以 monitor 模块扩展，不侵入备份核心服务。

## Logs Architecture (新增)

### Driver Capability Matrix

- `loki`
  - 支持：标签过滤、trace/job/policy 检索、范围查询、Grafana 深链。
  - 不支持：无。
- `file`
  - 支持：按时间窗口与关键字检索、关联 trace/job/policy 文本过滤。
  - 限制：无标签聚合、无原生 Grafana 深链。
- `stdio`
  - 支持：最近窗口 ring buffer 检索、关键字过滤。
  - 限制：历史范围受 ring buffer 大小限制，默认不保证跨重启保留。

### Query Model

- 统一入口：`GET /api/v1/admin/monitor/logs/config` + `GET /api/v1/admin/monitor/logs/query`
- 后端按 driver dispatch 到 `loki/file/stdio` 适配器。
- UI 依据 `capabilities` 字段动态展示按钮和提示文案。

### Security Boundary

- 前端不直连 Loki 凭据，不暴露后端日志访问密钥。
- Grafana 跳转仅生成受控链接参数，不拼接敏感 token。
- 所有查询写审计日志（操作者、时间、过滤条件摘要、结果状态）。

## Log Retention Architecture（新增）

- 统一入口：`log.retention`（不新增平级配置对象），在同一日志域下治理文件日志与 DB 日志表清理。
- 执行模型：后端定时任务按 cron 执行，分批删除并限制单次最大删除量，避免高峰期 IO/DB 抖动。
- 覆盖范围：应用日志文件目录、审计/运行日志表（含历史兼容表）、驱动映射策略（如 Loki retention 提示）。
- 可观测性：每次清理写结构化运行日志与审计记录，监控中心可查询最近执行结果与失败原因。

## Plugin Logger Host Alignment（新增）

### Scope

- 宿主模式插件日志默认采集链路与 027 监控中心日志能力对齐。
- 插件 `policy/probe` 管理能力纳入平台运维流程与审计。
- 对齐低基数标签策略：`plugin_id/tenant_uuid/component/level`。

### Current Gaps (as of 2026-04-24)

- 插件进程 stdout 默认可能仅落进程 ring buffer（未必统一透传宿主 stdout）。
- Promtail 配置未对插件 JSON 做稳定标签抽取与约束。
- 平台侧缺少统一的插件日志 `policy/probe` 编排流程。

### Implementation Strategy

- Host Runtime:
  - 确保插件子进程 stdout/stderr 默认进入宿主采集链路（systemd/journald 或等效管道）。
- Collector:
  - 在 Promtail pipeline 中对插件 JSON 提取低基数标签；高基数字段仅保留内容体。
- Control Plane:
  - 在插件 enable/变更流程中编排 `GET/PUT /admin/runtime/logging/policy` 与 `POST /admin/runtime/logging/probe`。
  - 记录策略变更与探测审计（operator/plugin_id/tenant_uuid/result）。

### Risks & Mitigations

- 风险：开启 stdout 透传后日志量上涨。  
  缓解：先灰度插件范围并设置采样/保留策略阈值。
- 风险：标签膨胀导致 Loki 查询性能下降。  
  缓解：严格固定低基数标签白名单，审计变更。
- 风险：插件侧策略不一致。  
  缓解：以平台下发策略为准，探测失败阻断变更发布。

## Phase Outputs

## Phase 0: Research Output

- 产出文件：`research.md`
- 目标：冻结备份策略与日志驱动能力矩阵决策。

## Phase 1: Design & Contracts Output

- 产出文件：`data-model.md`
- 合同：`contracts/backup-center.openapi.yaml`、`contracts/monitor-logs.openapi.yaml`
- 验证手册：`quickstart.md`

## Post-Design Constitution Check

- 仍保持 CoreX Module 实现路径，无插件注册耦合。
- 合同以 Admin HTTP 为主，满足当前监控中心闭环交付。
- 迁移与模型统一走 CoreX migration 汇聚流程。

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| 本阶段不新增 gRPC 合同 | 需求聚焦管理端可观测闭环 | 同步新增 gRPC 会放大范围并延后交付 |
| 日志多驱动适配层 | 生产环境存在 loki/file/stdio 混部 | 仅支持 loki 会导致非 loki 环境不可用 |

## System Snapshot & Restore（2026-10-15，本地实现完成，待部署验收）

完整设计：[系统快照与统一恢复计划](../../docs/plan/deploy/system-snapshot-and-restore.md)；验收：[US8 两轮验收](../../docs/guides/deploy/027-monitor-center/usecase-us8-system-snapshot-acceptance.md)。

### 复用边界

- `backup_ops` 继续提供 dump、独立副本和验证；`archive_ops` 继续提供 CAS、同 MVCC 媒体引用与 dump、SSH 归档。二者保持组件边界；新增 `system_recovery` 负责整站恢复协调。
- 新增父快照／恢复任务、封存 manifest、配置／版本采集、独立完整包、目标映射和正式切换账本；持久化位于被恢复业务库之外。
- 首版一致性以托管实例停写及采集前后配置／程序生命周期指纹为基线，不把 MVCC 误认为文件原子快照；独立写入者不在支持范围。
- 先完成跨快照对象引用保护，再复用旧 CAS；旧清理器不能删除新系统快照仍引用的对象。
- 恢复测试隔离必须在 bootstrap 启动插件、scheduler 和 worker 前生效。正式切换由独立协调器执行，不能依赖将被停止的 Core 请求。
- Root typed API、错误码、权限及异步合同已冻结于 [system-snapshots.openapi.yaml](../../docs/contracts/system-snapshots.openapi.yaml)；支持路径和部署接入见[托管恢复手册](../../docs/guides/deploy/027-monitor-center/usecase-us8-managed-recovery.md)。

### 交付顺序

1. 定义快照组件、密钥与版本依赖、状态机及隔离账本。
2. 统一策略调度与一致性采集，整套保留、告警及旧产物兼容。
3. 完整包导出／导入，离开源实例仍可校验、解密并取得匹配平台程序。
4. 隔离恢复、目标环境／插件绑定映射、联合业务验证。
5. 维护确认、当前回退点、统一切换、补偿与中断恢复。
6. 系统快照主入口和分层详情；组件级操作保留于高级维护。

### 发布门槛

先通过本地状态 A → 快照 → 状态 B → 空目标恢复 A，以及自建测试实例的切换失败回退；再由运维部署，观察至少一个真实调度周期，将 scheduled 完整包带到独立本地／开发实例验收。可按一天覆盖真实定时链路，不要求手动恢复必须等一天。

不得以测试通过、HTTP 202、dump 可解析、文件复制成功或媒体子任务 success 替代整站业务验收。现有策略和线上周期不因本计划自动改变。

## US9 Storage Governance（2026-10-28，只读切片已本地实现，其余待完成）

### 规格、设计与任务权威

- 需求：`spec.md` 的 US9、FR-049～FR-068、SC-017～SC-024。
- 详细参数与规则：[存储治理方案](../../docs/plan/deploy/storage-governance.md)；产品：[UI 设计](../../docs/plan/deploy/storage-governance-ui.md)；操作：[US9 手册](../../docs/guides/deploy/027-monitor-center/usecase-us9-storage-governance.md)。
- **任务状态唯一来源**：[tasks.md / Phase 11 / US9](tasks.md#us9-storage-governance)，T101～T165；docs 的任务页面改为索引，不维护第二套完成勾选。
- 当前 v0.3 开放概览、按环境／实例及资源绑定的草稿和测算、占用历史；三张平台元数据表集中挂载于 migration.go。运行期建议目录来自可信部署来源，按天／份数／引用保护独立语义；主机预算和当前资源分别保存。此次仅扩展 JSON，无新迁移。启用、清理、覆盖与接管合同继续由未完成任务冻结，保持现有策略和流程。

### 复用与新增边界

| 范围 | 当前复用落点 | US9 拟新增落点／要求 |
|---|---|---|
| 日志保留 | `backend/internal/service/monitor_logs/` | 类型化业务保护；新旧清理所有权协调，不能直接扩展按时间删表 |
| 归档 | `backend/internal/service/archive_ops/`、`backend/cmd/archive_worker/main.go` | 压缩编码、journal、源文件握手、远端覆盖与独立到期任务 |
| 备份与快照 | `backup_ops/`、`snapshot_ops/` | 字节额度、最低验证点、统一对象引用和全局预留 |
| 统一治理 | 已有观测／纯测算切片 | 后续扩展 `backend/internal/service/storage_governance/`，包含资源、计量、估算、预览、预算／租约、执行和审计；最终文件结构在合同阶段冻结 |
| 持久化与安全 | 集中 migration、部署来源及既有 Root 用户鉴权 | 主机账本不依赖业务 DB；策略带 revision；固定来源，无任意 SQL／路径／shell |
| 页面 | 监控导航、`/ops/backup`、纵向分区组件与事件接入 | 拟新增 `web-admin/app/pages/monitor/storage.vue` 及配套 API client／store／事件；复用权威策略与详情 |
| 外部协作 | 插件自有附件；XDocker 实现尚未核对 | 仅定义登记／租约及主机预留接管合同，当前仓库不替外仓清理或改写宿主机 |

### 预算、采集与运行方式

- 一级池为数据库 30、日志 8、备份快照 14、临时暂存 4、可重建缓存 2、系统及其他 32 GiB，合计 90 GiB；数据库中的追踪／审计子额度不重复求和。
- 普通可用目标 20 GiB、紧急阈值 10 GiB，资源预算 80%／90% 告警；后台采集初始 5 分钟、生命周期调度 15 分钟，页面不轮询。
- 估算以 14 天每日实际新增物理量的 P95、1.3 倍余量及额外保护占用计算；无采样不显示可行，不把预算当实占。
- 每文件系统只允许一个跨实例／进程预留权威；原子记录未写预留与已写实占，初始高占用并发 1，Core／worker 重启可接管，业务 DB 不可用仍能检查。
- 模式按 observe → preview → 受控 execute → enforce 推进，默认 observe；每类来源单独通过保护、归档和回退验收后启用。

### 阶段、任务与依赖

| 阶段 | 任务 | 依赖与交付门禁 |
|---|---|---|
| 文档与范围确认 | T101～T103 | 文档登记；运维／关键审计／来源／远端目标待确认 |
| P0 合同与只读观测 | T104～T116 | 身份、DTO／OpenAPI、兼容、总账／覆盖／估算；不自动删除 |
| P1 安全基础与归档 | T117～T130 | 全局预留／租约／预览、压缩、PG／journal、远端取回、业务非阻塞 |
| P2 分类生命周期 | T131～T146 | P1 完成后接入临时文件、终态 DB、备份快照与 CAS；逐类核验 |
| P3 UI 与双环境验收 | T147～T160 | UI 可先做标记 fixtures，启用依赖真实后端；WS／SSE、权限、7 天自动周期与 14 天复核 |
| P4 XDocker 接管 | T161～T165 | 独立协作合同；主机／卷计量、唯一预留权威、服务授权与接管／退回验收 |

任务正文、每阶段验收与证据矩阵以 [tasks.md](tasks.md#us9-storage-governance) 为准；上述映射便于排期，不是另一套进度清单。T099／M8.8 的 US8 Linux、真实插件和真实定时快照验收继续保留，不被 US9 文档覆盖。

### 新增交付门槛

1. 真实路由授权、跨环境／进程预算竞争、活动／引用／终态保护、旧对象兼容和预览过期／重复执行通过。
2. 真实临时文件系统／PostgreSQL／回环归档集成，故障注入覆盖断网、目标满、DB 不可用、重启、采集断档和空间不足；不得接生产数据跑测试。
3. 桌面和 390／320 px 浏览器流程、无周期状态请求、事件去重／重连及身份连接释放通过。
4. 正式与开发至少 7 天自动周期、远端校验／取回与独立恢复及业务检查；14 天有效样本后复核预算。不能把 7 天观察写成已有 90 天完整历史。
5. 程序版本、正式合同、配置迁移、回退办法和未完成项可追溯；collector 启用、DB 结构变更及生产部署另有维护窗口与授权。

### 2026-10-28 只读切片交付边界

当前本地已实现目录观测、按环境／实例及资源绑定的人工测算与版本化草稿、可选 PostgreSQL 计量与占用采样。正式合同为 [v0.3](contracts/storage-governance.openapi.yaml)。新增字段仅扩展 JSON 草稿，无新迁移；旧草稿保留原值，建议目录不写入审计。采样记录不是清理前新增量，也不是文件／远端历史覆盖；真实 R95、产物清单、归档覆盖、预算协调、旧清理器接管、执行、通知投递及 XDocker 尚未完成。已按用户授权定向迁移本地开发库的三张治理元数据表；本轮环境策略修改未操作用户数据库、私有配置、远程或用户服务，自动清理始终关闭。

### 2026-10-29 v0.4 普通应用日志交付

当前合同升级为 [v0.4](contracts/storage-governance.openapi.yaml)：实际配置发现日志／本地媒体／已启用的 Agent 调试与审计目录，覆盖清单明确未知类型；五分钟摘要追加各来源占用／文件数／状态／错误与归因指纹。新增 Root 用户的预览、同步有限批次和逐项审计回读，自动治理仍关闭。普通日志沿用原生保留配置并共享目录锁，旧通用清理器已启用或正式／预发需验证归档时不执行；不会自动移交现有策略。每批最多 32 项，未知结果不重删，文件大小不冒充物理释放。

新清理记录模型集中注册，并沿限定入口迁移；本地只新增 `ops_storage_cleanup_runs`，没有完整迁移／seed 或远程操作。UI 优先实际日志流程，其他资源高级草稿不会执行清理。当前同步批次没有后台轮询／私有队列；未来异步／自动任务仍必须遵循 TaskBus 与 WS/SSE 规则。真实新增量／R95、全面归档和源生命周期、全局预算、其他资源执行和运行验收仍为待办，不因此完成整个 US9。细节及证据见[应用日志验证](../../docs/contracts/evidence/storage-governance-logs-validation-20261029.json)。


### 2026-11-05：SG-11 商用配置治理发布门槛

用户明确要求按商用产品交付，UI 可用不再作为该能力收尾标准。新增 FR-069～FR-075、SC-025～SC-028 及 [T166～T174](tasks.md#us9-commercial-gates)，全部待实现。设计见[商用交付约束](../../docs/plan/deploy/storage-governance-commercial.md)。

实施顺序为权威与合同 → Root 边界 → 数据库事务／版本／幂等 → 发布与执行确认 → 密钥治理 → 旧配置接管及恢复 → UI 状态 → 隔离故障与真实流程 → 发布复核。平台数据库是普通配置唯一权威；本地发布快照与独立运维账本有明确职责，不能成为另一写入口。恢复能力继续独立于业务 DB。

集中迁移、权限兼容、旧值保留、唯一执行所有权、未知结果及回退均需单独验收；安装／升级不能自行启动归档或删除。日常与生产迁移、服务重启及真实策略启用保持既有授权边界。原 SG-09 的长期运行／取回门槛继续阻塞商用关闭，既有局部测试不替代新增门槛。
