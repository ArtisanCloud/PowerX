# Implementation Plan: 监控中心闭环（Backup + Logs）

**Branch**: `027-monitor-center` | **Date**: 2026-04-13 | **Spec**: [/private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX/specs/027-monitor-center/spec.md](/private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX/specs/027-monitor-center/spec.md)
**Plan Status**: In Progress（Backup 已实现，Logs 进入实施）
**Input**: Feature specification from `/specs/027-monitor-center/spec.md`

## Summary

围绕 Root 管理员构建“监控中心闭环”：
- 已落地：自动备份策略、作业历史、告警升级、恢复任务、监控入口联动。
- 本轮补齐：日志与链路追踪能力（`loki/file/stdio` 三驱动能力感知 UI、统一查询接口、Grafana 深链）。

## Technical Context

**Language/Version**: Go 1.24（backend）, TypeScript + Nuxt 4（web-admin）  
**Primary Dependencies**: Gin HTTP, GORM, PostgreSQL, Redis, systemd/ops scripts, Nuxt UI  
**Storage**: PostgreSQL（策略/作业/恢复/告警元数据）+ 文件或对象存储（备份产物）+ 日志后端（loki/file/stdio）  
**Testing**: Go test（service/handler）, Nuxt build + E2E smoke（ops/backup, monitor/logs）  
**Target Platform**: Linux server (systemd, nginx)  
**Project Type**: Web application（backend + web-admin）  
**Performance Goals**: 监控页面查询 p95 < 1s；日志查询首屏 < 10s；告警可见性 ≤ 1 分钟  
**Constraints**: 仅 Root 可操作；默认时区 Asia/Shanghai；不可影响生产在线业务；日志能力必须按驱动降级  
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
