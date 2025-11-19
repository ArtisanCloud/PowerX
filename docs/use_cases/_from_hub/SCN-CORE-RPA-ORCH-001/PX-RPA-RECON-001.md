scn_id: "SCN-CORE-RPA-ORCH-001"
scenario_name: "CoreX 智能体调度 RPA 插件执行跨系统流程"
slug: "scn-core-rpa-orch-001"
primary_scope: "powerx"
primary_layer: "service"
primary_domain: "core-platform"
primary_repo: "powerx"
doc_owner: "Michael Hu <matrix-x@artisan-cloud.com>"
last_generated_at: "2025-11-19"
---

# CoreX 智能体调度 RPA 插件执行跨系统流程 Usecase Seed 生成指南

> 场景摘要：RPA 插件帮助 CoreX Agent 在 ERP ↔ CRM 间自动同步对账文件。`PX-RPA-RECON-001` 聚焦 PowerX Core 的 service 层责任：调度对账 Flow、管理凭据、监控 Runner、处理异常订单、推送对账报告与审计日志。

本文档指导 CoreX 团队如何生成并分发该 Seed，确保 docmap → Seed → `_from_hub` → 下游仓路径一致。

## Seed 的定位

- 描述 PowerX Core 在“CRM ↔ ERP 对账文件自动同步”子场景中的职责（Scheduler、文件管道编排、异常审核、报告推送、审计与告警）。
- 与 `docs/_data/docmap.yaml` 中 `doc_id: PX-RPA-RECON-001` 节点严格对齐，是 `_from_hub/SCN-CORE-RPA-ORCH-001/PX-RPA-RECON-001.md` 的源数据。
- 为 `npm run publish:usecases` / `publish:collected` 分发提供结构化字段，供多仓协同。

## 前提条件

- 场景文档 `docs/scenarios/core-platform/SCN-CORE-RPA-CRM-ERP-SYNC-001.md` 已定义流程、指标与参与者。
- docmap 已登记该子节点（见下节 snippet），repo 映射为 `powerx`。
- `docs/_data/repos.yaml#powerx` 记录了默认分支与 seed 目录（`docs/usecases-seeds/**`）。
- **TODO**：补充 ERP Desktop Runner 安装要求、VPN/专线访问信息、对账文件加密策略，便于 Seed 后续引用。

## 生成流程

1. **登记/更新 docmap 子节点**

   ```yaml
   # docs/_data/docmap.yaml
   - scn_id: SCN-CORE-RPA-ORCH-001
     title: CoreX 智能体调度 RPA 插件执行跨系统流程
     children:
       - doc_id: PX-RPA-RECON-001
         scope: powerx
         layer: service
         domain: core-platform
         optional: false
         repo: powerx
         path: docs/use_cases/_from_hub/SCN-CORE-RPA-ORCH-001/PX-RPA-RECON-001.md
   ```

   - 若 docmap 中 `optional`、`scope/layer/domain`、`path` 不一致，脚本将无法生成 Seed，需保持字段同步。

2. **复制模板并放置到对应目录**

   ```bash
   mkdir -p docs/usecases-seeds/SCN-CORE-RPA-ORCH-001
   cp docs/usecases-seeds/_template.md \
     docs/usecases-seeds/SCN-CORE-RPA-ORCH-001/PX-RPA-RECON-001.md
   ```

   - 场景目录 (`SCN-CORE-RPA-ORCH-001/`) 统一存放该场景下所有 powerx Seed，便于脚本聚合。
   - 文件名必须与 `doc_id` 完全一致。

3. **填充 Frontmatter 区域**

   - `doc_id`, `scn_id`, `scope`, `layer`, `domain`, `repo_key` 与 docmap 完全一致（`powerx / service / core-platform`）。
   - `owners` 默认使用 Michael Hu，可根据实际加上财务/数据团队联系人。
   - `feature_flags` 建议包含：`desktop-runner-enabled`、`api-runner-enabled`、`rpa-file-vault`、`workflow-batch-window`。
   - `code_refs` 需指向对账 Scheduler、文件管道、异常审核 Agent、通知/审计模块。**TODO**：补充具体路径（例如 `apps/workflow/reconciliation_scheduler.ts`、`pkg/file_pipeline/**`、`pkg/agents/reconciliation/**`）。

4. **完善正文章节**

   - `Usecase Overview`：强调凌晨批处理窗口、Flow 成功率 ≥98%、报告 5 分钟内生成、敏感数据加密等目标。
   - `Context & Assumptions`：列出 ERP/CRM 访问方式（VPN/桌面代理/API）、凭据托管、文件加密、调度窗口、异常工单流程。
   - `Solution Blueprint`：分解 Stage 1-4（ERP Export、Data Shaping、CRM Import、Agent Review），对应模块/接口/数据流，可引用场景文档 mermaid 图并标注 service 层责任。
   - `Contracts & Interfaces`：明确 `POST /rpa/flow/run`、`GET /rpa/artifacts/{run_id}`、`EVENT rpa.reconciliation.completed`、`POST /agent/reports/reconciliation`、`POST /audit/rpa/artifacts` 等接口契约与权限控制。
   - `Implementation Checklist`：覆盖调度器、文件清洗、异常推送、审计、Secrets/VPN 配置。
   - `Testing Strategy`：包含 ERP/CRM sandbox、文件模拟（Excel/CSV）、异常注入、时序回放、性能测试（批量 10k 行），并引用 `scripts/qa/workflow-metrics.mjs --target reconciliation`。
   - `Observability & Ops`：指标 `rpa.reconciliation.run_total`、`rpa.reconciliation.latency_p95`、`rpa.reconciliation.error_ratio`、`agent.reconciliation.alert_total`，日志/告警涵盖文件 export/import 失败、异常未确认、VPN 断连等情况。
   - `Rollback & Failure Handling`：说明 Flow 暂停、凭据吊销、文件回滚、异常工单处理、人工回填流程。

5. **与场景文档互相链接**

   - 在 `SCN-CORE-RPA-CRM-ERP-SYNC-001` 的 Usecase Links 中引用 `_from_hub/SCN-CORE-RPA-ORCH-001/PX-RPA-RECON-001.md`。
   - Seed 中若提及新 Schema/配置，应同步 `docs/standards/rpa/**` 或相关财务对账规范。

## 自检清单

- Seed Frontmatter 与 docmap 字段一致，`slug/scenario_name` 与主场景匹配。
- `feature_flags`、`code_refs`、`owners`、`linked_requirements` 已填入实际值，避免 TODO。
- Seed 描述的 KPI 对应 Acceptance Criteria（对账 ≤60 分钟、导入成功率 ≥98%、报告 5 分钟内输出）。
- `scripts/qa/workflow-metrics.mjs --target reconciliation` 或等效脚本可验证流程（**TODO**：若仍缺 target，请在 QA 脚本中新增）。
- 运行 `npm run publish:usecases -- --scn-id SCN-CORE-RPA-ORCH-001 --dry-run` 前完成 lint/docs build，确保 Seed 可被脚本识别。

## 常见问题

| 问题 | 处理方式 |
|------|----------|
| ERP 桌面控件识别失败 | 在 `Rollback & Failure` 中加入视觉定位兜底与手动导出步骤，Seed 中要求记录异常并自动触发人工通知。 |
| 文件格式变更导致导入失败 | 在 `Context & Assumptions` 中列出字段映射表，配置 `config/rpa/file_mapping.yaml`，并在 `Testing Strategy` 中加入 schema diff 检测。 |
| VPN/专线不稳定 | Observability 中增加连接状态指标，提供 CLI 脚本切换备用通道，并在 `Rollback` 中说明如何暂停 Flow。 |
| 异常订单未及时处理 | `Contracts & Interfaces` 中定义异常推送 API，`Implementation Checklist` 添加人工确认任务，并在 `Follow-ups & Risks` 中追踪处理 SLA。 |

完成 Seed 后，更新 `_from_hub/SCN-CORE-RPA-ORCH-001/PX-RPA-RECON-001.md`，运行 `npm run publish:usecases -- --scn-id SCN-CORE-RPA-ORCH-001 --dry-run` 验证，并视需要执行 `npm run publish:collected`。
