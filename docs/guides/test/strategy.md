# PowerX 全域测试策略

本策略覆盖 PowerX 核心平台（IAM、Tenant、Capability Registry、Integration Gateway 等）、
插件生态（`specs/009-install-plugin-pxp` 及其子阶段）、Web Admin、CLI 以及观测/审计系统。
不同域的测试方式可组合使用，以确保 PR、Release、Hotfix 都有一致的验证路径。

## 1. 域划分与主要测试类型

| 域 / 能力 | 代表目录 & 合同 | 主要测试类型 |
|-----------|----------------|--------------|
| **核心平台服务**（IAM、Tenant、Capability Registry、Integration Gateway、Event Fabric） | `backend/internal/service/{iam,tenant,capability_reg*}`、`api/grpc/contracts` | `go test ./internal/service/...`, Buf 合同测试、`make test-all` API curl 套件、MCP/Agent e2e 脚本 |
| **插件生态**（Init/Doctor/Import、Release、Host/Debug、Sandbox、Version Governance） | `specs/009-install-plugin-pxp`, `backend/internal/service/plugin_*`, CLI `cmd/px`, `docs/use_cases/_from_hub/SCN-DEV-PLUGIN-*` | 单测 + `scripts/ci/regression_pxp.sh`（Phase 9/10/11），`scripts/ci/run_quickstart.sh`、手工宿主/沙箱/治理操作 |
| **Admin / Web 前端** | `web-admin/`、Cypress 用例、`docs/standards/powerx-plugin/integration` | Unit（Vitest）、E2E（Cypress/Cucumber）、手动 UI checklist（菜单、权限、i18n） |
| **CLI 家族**（`px`、`px-admin`、`px-plugin`、`px-market`） | `cmd/px/...` 等 | Cobra unit tests、`go test` + shell-based smoke (`scripts/ci/cli_smoke.sh`)、安装/升级脚本验证 |
| **观测 & 审计** | `backend/internal/service/plugin_release/instrumentation`、`docs/guides/plugin_release/*` | 指标回归脚本（Grafana snapshot）、`go test` 针对 metrics exporter、审计表 spot check |

> 插件相关策略在本表第二行，仅是全域的一部分；其余域应按同样强度执行。

## 2. 回归套件与触发条件

| 套件 | 命令 / 位置 | 触发条件 |
|------|-------------|----------|
| CI 入口 | `make ci-all` | PR / main push 默认入口：串行执行 `go test ./...` + 插件回归 Phase 9–11 |
| 核心后端单测 | `go test ./...`（或 `make unit-test-all`） | 任意 backend 代码变更 |
| API/Agent curl 套件 | `make test-all`（`make_files/test.mk`） | 改动到 HTTP API、Intent Agent、配置 |
| 插件三阶段回归 | `make regression-pxp` | 触及 `backend/internal/service/plugin_*`、CLI `px*`、`specs/009-install-plugin-pxp` 相关内容 |
| Quickstart 干跑 | `scripts/ci/run_quickstart.sh` | 发布链路/对象存储/审批相关改动、发版前 |
| Web Admin E2E | `pnpm cypress run`（或对应 Make 目标） | Web Admin 页面/接口改动 |
| CLI 安装验证 | `docs/standards/_shared/cli-install-and-naming.md` 中脚本 | CLI 发布/rename/安装文档调整 |
| 观测 / 审计巡检 | `docs/guides/plugin_release/application_runbook.md` 清单 + Dashboards | 指标、告警、审计 schema 发生变化、合规审计前 |

## 3. 执行层级说明

1. **单元 / 包级测试**：所有 service、transport、CLI 必须拥有；`go test` 是最小准入门槛。  
2. **契约 / API 测试**：gRPC（Buf）、HTTP（`make test-all` curl）、CLI（smoke shell）验证接口兼容性。  
3. **场景回归**：通过脚本或 Cypress/E2E 复现用户旅程，如插件发布、宿主调试、版本治理看板。  
4. **手工见证**：对自动化无法覆盖的关键链路（热更新、沙箱回归、治理例外审批）进行人工复核并记录。  
5. **观测/审计复核**：修改指标、日志、审计结构后必须校验 Grafana/Alert、`audit_events` 等数据留存。

## 4. 插件生态专项（摘录）

- Phase 9（Init/Publish）：`scripts/ci/regression_pxp.sh` Phase 9 + Quickstart + CLI `px plugin` 冒烟。  
- Phase 10（Debug/Sandbox）：回归 Phase 10 + 宿主热更新手工 + 沙箱 `suiteId` 回归。  
- Phase 11（Governance/Compat）：回归 Phase 11 + `px version scan/board/compat` CLI + Admin `/internal/version/*`。

> 这类专项是全域策略的子集，依然需要遵循上文提到的通用准入/准出规则。

## 5. 准入 / 准出标准

- **变更准入**：相关模块单测必过；如涉及接口/流程，需运行对应回归套件或 E2E；文档更新需附执行结果。  
- **发版准出**：`make ci-all`（或等价 CI 入口）、Quickstart/手工见证/观测审计清单全部通过。  
- **阻断规则**：任意套件失败、告警/仪表不匹配、审计缺漏或 checklist 未签字视为阻断，必须修复并复测。  
- **证据留存**：测试日志、覆盖率报告、Grafana 截图等需回填到对应的 checklist（如 `specs/009-install-plugin-pxp/checklists/regression.md`）或发布记录。
