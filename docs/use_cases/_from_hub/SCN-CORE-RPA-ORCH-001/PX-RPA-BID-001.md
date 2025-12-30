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

> 场景摘要：CoreX Workflow 通过 RPA 插件把智能体计划转化为可执行的 Flow，覆盖插件启用、Flow Builder、Runner 执行与跨系统反馈。本 Seed 聚焦“PX-RPA-BID-001”——在政府采购门户上自动抓取标讯并交付给 CoreX Agent。

本文档面向 PowerX Core 服务仓的 Stewards，用于把 RPA 招投标抓取能力拆分为可实施的仓内用例 Seed，并指导 docmap 与分发脚本的维护。

## Seed 的定位

- 描述 PowerX Core 在 “RPA 自动化投标信息抓取与推送” 子场景下需要交付的 watcher、Scheduler、通知与审计能力。
- 与 `docs/_data/docmap.yaml` 中 `scn_id: SCN-CORE-RPA-ORCH-001` 的 `doc_id: PX-RPA-BID-001` 节点保持字段一致，是 `npm run publish:usecases` 推送到下游仓的唯一数据源。
- 支撑 `_from_hub/SCN-CORE-RPA-ORCH-001/PX-RPA-BID-001.md` 收敛的合规内容，确保 RPA Flow、Agent、审计三方可追溯。

## 前提条件

- 场景文档 `docs/scenarios/core-platform/SCN-CORE-RPA-BID-CAPTURE-001.md` 已定义跨仓流程、指标与参与者。
- `docs/_data/docmap.yaml` 中已存在对应子节点，且 repo 映射为 `powerx`。
- `docs/_data/repos.yaml#powerx` 记录了 `usecase_seed_root`、默认分支 `dev/docs` 与分发配置。
- **TODO**：补充需要的门户账号白名单、代理出口与 Captcha 识别策略，便于落地时一次性集齐依赖。

## 生成流程

1. **登记/更新 docmap 子节点**

   ```yaml
   # docs/_data/docmap.yaml
   - scn_id: SCN-CORE-RPA-ORCH-001
     title: CoreX 智能体调度 RPA 插件执行跨系统流程
     children:
       - doc_id: PX-RPA-BID-001
         scope: powerx
         layer: service
         domain: core-platform
         optional: false
         repo: powerx
         path: docs/use_cases/_from_hub/SCN-CORE-RPA-ORCH-001/PX-RPA-BID-001.md
   ```

   - `doc_id`、`scope/layer/domain`、`repo` 与 `path` 必须与 Seed frontmatter 完整对齐。
   - 若未来新增可选门户或多租户差异，可另建 `optional: true` 的扩展子节点，并在 docmap 中与本 Seed 并列。

2. **复制模板并放置到对应目录**

   ```bash
   mkdir -p docs/usecases-seeds/SCN-CORE-RPA-ORCH-001
   cp docs/usecases-seeds/_template.md \
     docs/usecases-seeds/SCN-CORE-RPA-ORCH-001/PX-RPA-BID-001.md
   ```

   - 由于 PowerX Core 只在 service 层交付，Seed 目录直接放在场景 ID 下；下游 `_from_hub/` 目录仍保持 `scope/layer/domain` 结构。
   - 文件名需保持与 `doc_id` 相同，否则脚本无法匹配。

3. **填充 Frontmatter 区域**

   - `doc_id: PX-RPA-BID-001`、`scn_id: SCN-CORE-RPA-ORCH-001`、`scope/layer/domain: powerx/service/core-platform`、`repo_key: powerx` 必须逐字匹配 docmap。
   - 建议 `owners` 与场景文档保持一致（默认 Michael Hu），`linked_requirements` 可指向 CRM/AI 平台的需求或 PXIP ID。
   - `feature_flags` 可写入 `rpa-plugin-enabled`、`browser-runner-headful`、`telemetry-bid-feed`，保持 Feature Gate 透明。
   - **TODO**：补上 `code_refs`（例如 `apps/workflow/rpa_bid_watcher/**`、`scripts/cron/rpa_bid_sync.mjs`），供跨仓协作定位入口。

4. **完善正文章节**

   - `Usecase Overview`：说明 watcher 创建、调度策略、指标（成功率≥97%、每日≥2次、5分钟内完成）。
   - `Context & Assumptions`：列出门户账户托管在 `px-secrets-vault`、代理/白名单限制、Captcha 处理方式、智能体通知渠道。
   - `Solution Blueprint`：细化 Scheduler → Plugin → Runner → Agent 的时序，可附带 mermaid（参考场景文档中的图）。
   - `Contracts & Interfaces`：落地 `POST /internal/rpa/bid-watchers`、`EVENT rpa.bidfeed.created`、`POST /agent/notifications` 等接口契约。
   - `Testing Strategy`：包含 Mock 门户 + Runner Sandboxing、自建招投标数据集与 e2e replay。
   - `Observability & Ops`：指标 `rpa.bid.run_total`、`rpa.bid.success_rate`、`rpa.bid.notify_latency`，另注明截图归档与审计保留策略。
   - `Rollback & Failure Handling`：解释 Flow 回滚、Watcher 暂停、凭据吊销、Portal DOM 变更的紧急预案。
   - `Follow-ups & Risks`：如验证码升级、DOM 变更监控脚本、人工确认 Step 设计等。

5. **与场景文档互相链接**

   - 在 `SCN-CORE-RPA-BID-CAPTURE-001` 的 “Usecase Links” 表中引用 `_from_hub/SCN-CORE-RPA-ORCH-001/PX-RPA-BID-001.md`。
   - 若 Seed 中定义了新的 Flow JSON Schema 或 config，请同步在 `docs/standards/rpa/**` 增补，避免各仓自定义。

## 自检清单

- docmap ↔ Seed Frontmatter 无字段差异，`slug`、`scenario_name` 与主场景一致。
- Seed 中描述的 API/Event 均在 PowerX Core 仓可实现，并配有鉴权与审计说明。
- `feature_flags`、`telemetry`、`告警` 字段覆盖 Portal 登录失败、抓取为空、推送延迟等关键指标。
- 已在 `scripts/qa/workflow-metrics.mjs` 或专用 QA 脚本中增加 `--target rpa-bid` 的校验路径。
- **TODO**：运行 `npm run publish:usecases -- --scn-id SCN-CORE-RPA-ORCH-001` 前，确认 Seed Markdown 通过 lint 与 docs build。

## 常见问题

| 问题 | 处理方式 |
|------|----------|
| 门户验证码或 MFA 改版导致 Runner 阻塞 | 在 Seed 的 `Rollback & Failure Handling` 中提供人工确认 Step、二次凭据注入流程，并在 `feature_flags` 中加入 `rpa-bid-mfa-manual`。 |
| 多门户配置差异 | 建议在同一 Seed 内以配置表列出各门户的 URL、字段与调度窗口，必要时结合 `optional: true` 的扩展 doc_id。 |
| Run 截图或数据未同步到审计 | 在 `Contracts & Interfaces` 中明确 `POST /audit/rpa/artifacts`，并在 Observability 章节加入截图缺失告警。 |
| Captcha/OTP 依赖人工协助 | 在 `Solution Blueprint` 中定义 Agent 澄清节点，确保出现异常时能暂停 Flow 并推送审批人。 |

完成上述步骤后，即可调用 `npm run publish:usecases -- --scn-id SCN-CORE-RPA-ORCH-001 --dry-run` 验证配置，并通过 `_from_hub` 脚本分发给 powerx 下游仓库。若需要回写 `_collected` 视图，请同步执行 `npm run publish:collected`.
