# Workflow Orchestration Release Notes

## Overview
本次交付完成了 CoreX Workflow Orchestration 的运行态控制、审计导出与运维观测能力，使设计者、运维人员、合规团队都可以在统一平台内完成工作流从定义、执行到治理的闭环。

## Highlights
- **运行态控制（User Story 2）**
  - 新增实例暂停/恢复/取消、步骤重试/补偿、Agent 待命重派逻辑。
  - gRPC/HTTP 控制接口全面一致，所有操作均会生成审计事件 `workflow.instance.*` / `workflow.step.*`。
  - 调度器（Redis 重试队列）补强，公开 `RetryQueueKey()` 便于监控。
- **审计导出（User Story 3）**
  - `ExportInstances` 服务提供 JSON/CSV 两种格式，支持步骤明细与 Tool Grant 版本追踪。
  - 新建 HTTP `/api/v1/admin/workflows/instances/export` 与 gRPC `ExportInstances`，覆盖多租户隔离。
  - Quickstart & runbook 补充了导出与控制端到端示例。
- **运维观测与文档（Polish）**
  - Grafana 模板 `deploy/observability/workflow_dashboard.json`：重试、补偿、控制操作集中展示。
  - Redis 重试队列运行手册 & Workflow Observability Runbook 新增。
  - `tests/unit/workflow_validator_test.go` 验证 StepGraph 校验及 RetryPolicy 解析默认值。

## Ops & Compatibility Notes
- 需要在数据库运行最新迁移（`go run cmd/database/main.go migrate`）并执行 `seed` 以生成 system/root 管理员。
- Redis / EventBus 需按照 `etc/config.yaml` 指定地址部署；事件骨干授权模块依赖 Redis 额外 DB 与 Cache 通道。
- Quickstart 脚本默认会尝试调用 `/api/v1/event-fabric/...`，请提供可用的 tenant/subject UUID（示例为 system + root）。

## Testing
| 测试项 | 命令 / 说明 | 结果 |
|--------|-------------|------|
| 单元测试 | `GOCACHE=$(pwd)/.gocache go test ./tests/unit/...` | ✅ |
| 合同测试（导出） | `GOCACHE=$(pwd)/.gocache go test ./tests/contract/workflow -run Export` | ✅ |
| API 套件 | `make test-all` | ✅ |
| Quickstart 脚本 | `scripts/demo/event_fabric_quickstart.sh`（system/root 凭证 + 实际租户/成员 UUID） | ✅ |

## Known Issues / Follow-up
- `scripts/demo/event_fabric_quickstart.sh` 在首次运行会注册能力；若已存在会提示 400 并继续执行。
- 如果目标环境的租户/成员 UUID 与默认值不同，需要通过 `EVENT_FABRIC_TENANT_UUID` / `EVENT_FABRIC_SUBJECT_UUID` 环境变量覆盖。
- 建议上线前在生产配置中启用 Gin `trust proxy` 与 `release` 模式以确保安全性。

## References
- Quickstart：`specs/006-workflow-and-agent/quickstart.md`
- Redis Runbook：`docs/runbooks/redis_workflow.md`
- Observability Runbook：`docs/runbooks/observability_workflow.md`
- Audit Log：`reports/event_fabric_authorization_quickstart.log`
