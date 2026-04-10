# Implementation Plan: 自动备份闭环（Backup Center）

**Branch**: `027-monitor-center` | **Date**: 2026-04-10 | **Spec**: [/private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX/specs/027-monitor-center/spec.md](/private/var/www/html/ArtisanCloud/X/PowerX/Core/PowerX/specs/027-monitor-center/spec.md)
**Plan Status**: Ready for Implementation (Phase 1 Frozen)
**Input**: Feature specification from `/specs/027-monitor-center/spec.md`

## Summary

围绕 Root 管理员构建“自动备份闭环”能力：支持备份策略管理（默认每 6 小时、保留 14 份、Asia/Shanghai 时区）、自动执行与告警升级（连续 2 次失败升级）、恢复演练（默认每周）以及监控入口下的任务与日志可观测。

## Technical Context

**Language/Version**: Go 1.24（backend）, TypeScript + Nuxt 4（web-admin）  
**Primary Dependencies**: Gin HTTP, GORM, PostgreSQL, Redis, systemd/ops scripts, Nuxt UI  
**Storage**: PostgreSQL（策略/作业/演练/告警元数据）+ 文件或对象存储（备份产物）  
**Testing**: Go test（service/integration）, Nuxt build + E2E smoke（ops/backup, monitor）  
**Target Platform**: Linux server (systemd, nginx)  
**Project Type**: Web application（backend + web-admin）  
**Performance Goals**: 监控页面查询 p95 < 1s；告警可见性 ≤ 1 分钟；备份触发率 ≥ 99%（7 天）  
**Constraints**: 仅 Root 可操作；默认时区 Asia/Shanghai；不可影响生产在线业务；保留策略清理需幂等  
**Scale/Scope**: 单租户优先验证，支持扩展到多租户；单实例每日备份作业量 < 500

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **Domain Ownership**: 该需求属于 CoreX 运维与系统能力，按 CoreX Module 处理（非插件）。
- **COREX_DECLARED**: PASS（本计划明确 CoreX 归属，路径落在 `internal/service`、`internal/transport/http/admin`、`web-admin/app/pages`）。
- **NO_PLUGIN_REGISTRY**: PASS（不引入 `plugins/registry.json` 及插件目录实现）。
- **COREX_LAYOUT_MATCH**: PASS（采用 backend/web-admin 既有结构）。
- **COREX_DUAL_TRANSPORT**: CONDITIONAL（本期按运维闭环仅交付 Admin HTTP；gRPC 扩展明确延期到后续里程碑，并在任务中记录豁免说明与补齐计划）。
- **COREX_BUF_CONFIG**: PASS（不新增 proto 合同，现有 Buf 配置保持不变）。
- **COREX_SERVER_WIRING**: PASS（复用现有 HTTP 路由聚合与系统服务装配）。
- **COREX_MIGRATION_WIRING**: PASS（新增模型按 CoreX migration 统一挂载）。
- **X.3 Blocking Gates**:
  - `HTTP_PRESENT`: PASS
  - `GRPC_PRESENT`: CONDITIONAL（本期按 spec 豁免仅交付 Admin HTTP，gRPC 延期到后续里程碑）
  - `PROTOBUF_DEFINED`: PASS（无新增 proto）
  - `SERVER_DEFINED`: PASS（无新增 gRPC server）
  - `MAKE_TARGETS`: PASS（沿用现有）

## Project Structure

### Documentation (this feature)

```
specs/027-monitor-center/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   └── backup-center.openapi.yaml
└── tasks.md
```

### Source Code (repository root)

```
backend/
├── internal/
│   ├── service/backup_ops/
│   ├── transport/http/admin/backup/
│   ├── transport/http/admin/menu/
│   └── transport/http/admin/event_fabric/
├── pkg/corex/db/persistence/model/
└── pkg/corex/db/database/migration.go

web-admin/
├── app/pages/ops/backup.vue
├── app/pages/monitor/*.vue
├── app/components/ops/backup/
└── app/components/monitor/
```

**Structure Decision**: 采用现有 backend + web-admin 双端结构；备份策略/作业/演练/告警走 backend CoreX 服务，监控与备份中心页面在 web-admin 落地。

## Phase 0: Research Output

- 产出文件：`research.md`
- 目标：冻结调度基线、保留策略、告警升级、演练节奏、时区策略与可靠性方案。

## Phase 1: Design & Contracts Output

- 产出文件：`data-model.md`
- 合同：`contracts/backup-center.openapi.yaml`
- 验证手册：`quickstart.md`
- Agent 上下文更新：运行 `.specify/scripts/bash/update-agent-context.sh codex`

## Post-Design Constitution Check

- 仍保持 CoreX Module 实现路径，无插件注册耦合。
- 合同为 Admin HTTP，满足当前闭环交付；gRPC 扩展留待后续迭代（非本次阻断项）。
- 迁移与模型统一走 CoreX migration 汇聚流程，不新增分散迁移入口。

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| 本阶段不新增 gRPC 合同 | 需求聚焦运维管理端闭环，HTTP 已满足 | 同步新增 gRPC 会放大范围并延后交付，不影响当前可验收目标 |
