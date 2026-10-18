# Implementation Plan: 监控中心闭环（Backup + Logs）

**Branch**: `027-monitor-center`（功能标识，不要求切换当前分支） | **Date**: 2026-10-15 | **Spec**: [spec.md](spec.md)
**Plan Status**: In Progress（快照包及隔离数据副本已实现；完整恢复与正式切换待完成）
**Input**: Feature specification from `/specs/027-monitor-center/spec.md`

## Summary

围绕 Root 管理员构建“监控中心闭环”：
- 已落地：自动备份策略、作业历史、告警升级、恢复任务、监控入口联动。
- 本轮补齐：日志与链路追踪能力（`loki/file/stdio` 三驱动能力感知 UI、统一查询接口、Grafana 深链）。
- 本轮新增：插件 framework logger 与宿主采集链路对齐（stdout+json 默认链路、policy/probe 编排、低基数标签约束）。
- 2026-10-15 新增计划：系统快照统一数据库、媒体、必要配置和程序版本，支持完整包跨环境恢复及受控正式切换；本地已实现统一包、Root 页面／API／离线工具、原生新集群与应用验证、独立正式切换及中断补偿；Linux／生产插件和真实定时第二轮尚待验收。

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
