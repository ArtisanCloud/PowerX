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

> 场景摘要：CoreX Workflow 借助 RPA 插件完成跨系统执行。本 Seed `PX-RPA-OA-001` 聚焦“企业 OA 待办巡检与自动填报”子场景，负责在 PowerX Core 中交付 watcher、智能体摘要、审批推送与审计闭环。

本文档指导 PowerX Core 工程团队如何交付 OA 巡检用例 Seed，确保 docmap、脚本分发与实现责任对齐。

## Seed 的定位

- 定义 CoreX 服务在 OA Flow 中的职责：晨间调度、凭据/Session 交互、Agent 汇总、日报推送、审计回放。
- 与 `docs/_data/docmap.yaml` 中 `doc_id: PX-RPA-OA-001` 完全一致，是 `_from_hub/SCN-CORE-RPA-ORCH-001/PX-RPA-OA-001.md` 的内容来源。
- 为 `npm run publish:usecases` 生成的下游 Seed、`npm run publish:collected` 领导力视图提供权威字段。

## 前提条件

- 场景文档 `docs/scenarios/core-platform/SCN-CORE-RPA-OA-AUTOFLOW-001.md` 已描述流程、指标与参与仓。
- docmap 条目已创建（见下文 snippet），且 repo 映射为 `powerx`。
- `docs/_data/repos.yaml#powerx` 中包含默认分支、种子目录与维护人配置。
- **TODO**：补充 OA 系统所需的 VPN / Intranet 访问、MFA 供应商、附件解析（OCR 引擎、PDF 解析器）等依赖，确保 Seed 中有明确交付入口。

## 生成流程

1. **登记/更新 docmap 子节点**

   ```yaml
   # docs/_data/docmap.yaml
   - scn_id: SCN-CORE-RPA-ORCH-001
     title: CoreX 智能体调度 RPA 插件执行跨系统流程
     children:
       - doc_id: PX-RPA-OA-001
         scope: powerx
         layer: service
         domain: core-platform
         optional: false
         repo: powerx
         path: docs/use_cases/_from_hub/SCN-CORE-RPA-ORCH-001/PX-RPA-OA-001.md
   ```

   - `doc_id/scope/layer/domain/repo/path` 必须与 Seed frontmatter 相同。
   - 若未来拆分移动端/桌面端不同流程，可增设 `optional: true` 子节点。

2. **复制模板并放置到对应目录**

   ```bash
   mkdir -p docs/usecases-seeds/SCN-CORE-RPA-ORCH-001
   cp docs/usecases-seeds/_template.md \
     docs/usecases-seeds/SCN-CORE-RPA-ORCH-001/PX-RPA-OA-001.md
   ```

   - RPA 主场景下所有 powerx service Seed 均放在 `SCN-CORE-RPA-ORCH-001/` 目录中，保持与脚本路径一致。
   - 文件名需使用 `doc_id`，否则 `_from_hub` 生成脚本无法匹配。

3. **填充 Frontmatter 区域**

   - `doc_id`, `scn_id`, `scope`, `layer`, `domain`, `repo_key` 全部与 docmap 保持一致。
   - 默认 owner：`Michael Hu`，如需多方协作可增加 OA 平台或安全团队联系人。
   - `feature_flags` 建议包含 `browser-runner-headless`、`file-runner-enabled`、`workflow-morning-check`、`telemetry-oa`。
   - `code_refs` 需列出 watcher Cron、Agent 汇总模块、审批通知服务、审计落盘脚本等入口。**TODO**：补充这些路径（例如 `apps/workflow/oa_morning_check/**`、`pkg/agents/reporting/oa_daily.ts`）。

4. **完善正文章节**

   - `Usecase Overview`：说明晨间巡检 5 分钟内完成、登录成功率 ≥98%、人工干预率 <10% 的目标。
   - `Context & Assumptions`：列出 OA 凭据托管 (`px-secrets-vault`)、MFA/OTP 流程、附件解析的存储权限、审批人列表。
   - `Solution Blueprint`：细化 Scheduler → Runner → Agent → OA → Audit 的流程，可直接引用场景文档 mermaid 图并针对 CoreX 模块转写。
   - `Contracts & Interfaces`：明确 `POST /rpa/flow/run?trigger=morning`、`GET /rpa/attachments/{run}`、`POST /agent/reports/daily`、`EVENT rpa.oa.todo.synced` 等接口的字段、鉴权、重试策略。
   - `Implementation Checklist`：新增 OA 模块特有的配置（MFA 通知、OCR 引擎、审计截图归档）。
   - `Testing Strategy`：包含 Mock OA Server、MFA 回放脚本、附件解析准确率测试、晨间调度 e2e 验证。
   - `Observability & Ops`：指标 `rpa.oa.run_total`、`rpa.oa.login_success_rate`、`agent.oa.daily_report_latency`；日志需记录登录步骤、MFA 状态、附件解析摘要、填报结果。
   - `Rollback & Failure Handling`：说明当 MFA 失败、附件解析异常、自动填报失败时的暂停、人工确认、数据回滚步骤。

5. **与场景文档互相链接**

   - 在 `SCN-CORE-RPA-OA-AUTOFLOW-001` 的 Usecase Links 表中引用 `_from_hub/SCN-CORE-RPA-ORCH-001/PX-RPA-OA-001.md`。
   - 若 Seed 中新增了标准（如附件解析字段、审批步骤），须同步到 `docs/standards/rpa/**` 或相应的 OA 规范。

## 自检清单

- Seed Frontmatter ↔ docmap 字段一致，无大小写或路径偏差。
- `feature_flags`、`config`、`telemetry` 字段列明 OA Flow 的关键依赖与指标。
- 场景中要求的 KPI（登录成功率、解析准确度、填报反馈时长）在 Seed 中有明确实现/监控方案。
- `scripts/qa/workflow-metrics.mjs --target oa` 或等效脚本已定义执行步骤。**TODO**：若无此 target，请补充 QA 脚本示例。
- 运行 `npm run publish:usecases -- --scn-id SCN-CORE-RPA-ORCH-001 --dry-run` 确认 Seed 字段通过校验。

## 常见问题

| 问题 | 处理方式 |
|------|----------|
| OA 系统频繁更换 MFA 机制 | 在 Seed 的 `Rollback & Failure` 段落中定义人工确认 Step、MFA 配置表和超时策略，并新增 `rpa-oa-mfa-manual` Feature Flag。 |
| 附件解析准确率不足 | 在 `Testing Strategy` 中加入样本数据集与 OCR/Tesseract/LLM 比对流程，并配置 `agent.oa.attachment_parse_errors` 告警。 |
| 自动填报影响审批链 | 在 `Contracts & Interfaces` 中定义审批确认 API 与审计日志字段，必要时增加双人审批 Feature Flag。 |
| 截图/附件未落审计 | 配置 `POST /audit/rpa/attachments` 接口、审计存储路径和失败重试策略，并在 Observability 中加入缺失告警。 |

完成本 Seed 后，请更新 `_from_hub/SCN-CORE-RPA-ORCH-001/PX-RPA-OA-001.md`、运行 `npm run publish:usecases -- --scn-id SCN-CORE-RPA-ORCH-001 --dry-run` 验证，再按需要执行 `npm run publish:collected` 刷新领导力视图。
