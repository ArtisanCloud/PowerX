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

> 场景摘要：CoreX Workflow 将智能体意图解析为含 RPA Step 的 DAG，通过插件驱动 Browser/Desktop/API/File Runner 执行跨系统任务。本 Seed `PX-RPA-ORCH-001` 聚焦 service 层主职责：Workflow→RPA 插件的编排接口、权限/审计集成、指标统一与失败补偿。

本文面向 PowerX Core（service 层）Stewards，确保 docmap、脚本与实现职责一致，便于将 RPA Orchestration 能力分发给下游仓库。

## Seed 的定位

- 描述 CoreX Workflow Engine 在 RPA 主场景下的核心责任：插件启用、Flow Builder 接入、RPA Step 执行调度、凭据/ACL 复用、审计/Telemetry 接入。
- 与 `docs/_data/docmap.yaml` 中 `doc_id: PX-RPA-ORCH-001` 节点一一对应，是 `_from_hub/SCN-CORE-RPA-ORCH-001/PX-RPA-ORCH-001.md` 的内容来源。
- 作为 `npm run publish:usecases`、`npm run publish:collected` 脚本生成下游 Seed 与管理视图的唯一输入。

## 前提条件

- 场景文档 `docs/scenarios/core-platform/SCN-CORE-RPA-ORCH-001.md` 已定义业务背景、流程与指标。
- docmap 条目已创建，repo 映射到 `powerx`（参考下节 snippet）。
- `docs/_data/repos.yaml#powerx` 提供默认分支 `dev/docs`、usecase seed 目录与维护人信息。
- **TODO**：补全 RPA 插件版本兼容矩阵、Runner 节点部署策略、Secrets Vault 与 ACL 同步流程，确保 Seed 能引用统一来源。

## 生成流程

1. **登记/更新 docmap 子节点**

   ```yaml
   # docs/_data/docmap.yaml
   - scn_id: SCN-CORE-RPA-ORCH-001
     title: CoreX 智能体调度 RPA 插件执行跨系统流程
     children:
       - doc_id: PX-RPA-ORCH-001
         scope: powerx
         layer: service
         domain: core-platform
         optional: false
         repo: powerx
         path: docs/use_cases/_from_hub/SCN-CORE-RPA-ORCH-001/PX-RPA-ORCH-001.md
   ```

   - `doc_id/scope/layer/domain/repo/path` 必须与 Seed frontmatter 完全一致。
   - 若未来拆分 CLI/Workflow Engine 等不同责任，可新增额外 doc_id 并在 docmap 中与本节点并列。

2. **复制模板并放置到对应目录**

   ```bash
   mkdir -p docs/usecases-seeds/SCN-CORE-RPA-ORCH-001
   cp docs/usecases-seeds/_template.md \
     docs/usecases-seeds/SCN-CORE-RPA-ORCH-001/PX-RPA-ORCH-001.md
   ```

   - 所有 powerx service Seed 均以场景 ID 作为目录，保持与 `_from_hub` 脚本的查找逻辑一致。
   - 文件名需与 `doc_id` 完全一致，方便脚本匹配。

3. **填充 Frontmatter 区域**

   - `doc_id: PX-RPA-ORCH-001`、`scn_id: SCN-CORE-RPA-ORCH-001`、`scope/layer/domain: powerx/service/core-platform`、`repo_key: powerx` 与 docmap 保持一致。
   - `owners` 默认沿用 Michael Hu，可追加 Workflow/Observability Guild 代表。
   - `feature_flags` 建议列出 `rpa-plugin-enabled`、`corex-workflow-v3`、`telemetry-unified-sink`、`px-secrets-vault`。
   - `code_refs` 需指向 CoreX Workflow Engine、RPA Step Handler、审计/指标写入模块等。**TODO**：补充具体包路径（如 `apps/workflow/engine/rpa_step.ts`、`pkg/audit/rpa_flow_writer.go`）。

4. **完善正文章节**

   - `Usecase Overview`：概述 RPA 插件启用、Flow Builder 接入、DAG 中 RPA Step 的价值，量化成功指标（建模 ≤5 分钟、执行成功率 ≥95%、日志 1 分钟内入库）。
   - `Context & Assumptions`：说明插件安装前置条件、Runner 注册、ACL/Secrets 同步、Feature Flag、与 docmap 子用例的关系。
   - `Solution Blueprint`：拆分为 Workflow Engine、Plugin Gateway、Runner Registry、Audit Sink 四层，定义 Stage 1-4（Plugin Enablement、Flow Modeling、Workflow Embedding、Execution & Feedback）。可引用场景文档 mermaid，并说明 service 层代码入口。
   - `Contracts & Interfaces`：列出 `POST /plugins/rpa/flows`、`GET /plugins/rpa/flows/{id}`、`POST /workflow/{taskId}/steps/rpa`、`POST /rpa/flow/run`、`EVENT rpa.flow.completed` 等接口，包含鉴权、参数、重试策略。
   - `Implementation Checklist`：强调 Workflow DAG 扩展、审计、Telemetry、Secrets/ACL、桌面 Runner 扩展等任务。
   - `Testing Strategy`：覆盖 Planner→Workflow→Plugin→Runner 的 e2e、故障注入（Runner 掉线、凭据失效）、多租户隔离、审计写入校验。
   - `Observability & Ops`：指标 `rpa.flow.run_total`、`rpa.flow.success_rate`、`workflow.rpa_step.retry_total`、`rpa.runner.latency_p95`；日志与告警需要区分 Runner 类型；Dashboards 对应 `scripts/qa/workflow-metrics.mjs` 输出。
   - `Rollback & Failure Handling`：描述 Feature Flag 关闭、Flow ID 禁用、Runner 下线、凭据失效回滚、审计补写脚本。

5. **与场景文档互相链接**

   - 在 `SCN-CORE-RPA-ORCH-001` 的 Usecase Links 段落指向 `_from_hub/SCN-CORE-RPA-ORCH-001/PX-RPA-ORCH-001.md`。
   - 若 Seed 衍生新的标准/Schema，需同步更新 `docs/standards/rpa/**` 和 `scripts/qa/workflow-metrics.mjs`。

## 自检清单

- docmap ↔ Seed frontmatter 完全一致，`slug/scenario_name` 与主场景匹配。
- `feature_flags`、`code_refs`、`owners` 已填充真实值，避免脚本警告。
- Seed 中的 API/Event/Config 与场景文档保持一致，且引用 `docs/meta/scenarios/powerx/core-platform/rpa/primary.md` 的 Flow JSON Schema。
- Mermaid/流程描述清楚主场景 Stage 1-4 的责任划分。
- `npm run publish:usecases -- --scn-id SCN-CORE-RPA-ORCH-001 --dry-run` 可通过（需先确保 Seed 无 TODO）。**TODO**：在提交前运行该命令并记录结果。

## 常见问题

| 问题 | 处理方式 |
|------|----------|
| Plugin Gateway 与 Workflow Engine 版本不匹配 | 在 `feature_flags` 中增加版本守卫，Seed 中要求通过 `px plugins:list` 校验；若冲突则回滚至兼容版本或触发蓝绿部署。 |
| Runner 扩缩容导致调度失败 | 在 `Observability & Ops` 中加入 Runner 心跳监控与告警，`Rollback` 章节说明如何暂停 Flow 或切换 Runner。 |
| 审计/Telemetry 延迟 | 定义 `POST /audit/rpa/flows`、`POST /metrics/rpa` 的重试策略，并在 Seed 中要求脚本 `scripts/qa/workflow-metrics.mjs` 监测。 |
| Secrets/ACL 与插件不同步 | 在 `Implementation Checklist` 中增加 Secrets Vault 同步与 ACL 继承任务，必要时加入 `px-secrets-vault sync` 命令。 |

完成 Seed 后，请刷新 `_from_hub/SCN-CORE-RPA-ORCH-001/PX-RPA-ORCH-001.md`，并执行 `npm run publish:usecases -- --scn-id SCN-CORE-RPA-ORCH-001 --dry-run` 验证，再视需要运行 `npm run publish:collected`。
