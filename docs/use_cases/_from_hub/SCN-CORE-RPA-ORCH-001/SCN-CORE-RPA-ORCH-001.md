---
scn_id: SCN-CORE-RPA-ORCH-001
title: CoreX 智能体调度 RPA 插件执行跨系统流程
status: Draft
version: v0.1.0
owners:
  - name: Michael Hu
    role: Product Manager
    contact: matrix-x@artisan-cloud.com
domains: [core-platform]
layers: [service, plugin]
repos:
  - key: powerx
    scope: core-platform
    responsibility: Workflow Engine、Intent Parser、ACL、审计与遥测
  - key: powerx-plugin-rpa
    scope: plugin
    responsibility: RPA Flow Builder、Runner、录制器、桌面/浏览器沙箱
related_usecases:
  - doc_id: PX-RPA-ORCH-001
    layer: service
    domain: core-platform
last_reviewed_at: 2025-02-20

---

# Positioning & Goals

PowerX RPA 插件将 CoreX 智能体的「思考」扩展为「执行」，让自动化任务可以落地到浏览器、桌面、API 与文件处理等多 Runner。该主场景覆盖插件安装、Flow Builder、Flow JSON 契约、Runner 执行与 Workflow Engine 调度，保证智能体可以在统一权限与审计体系下复用 RPA 能力。成功标准：RPA Flow 建模 ≤ 5 分钟即可交付、跨系统执行成功率 ≥ 95%，所有步骤与变量均被追踪到统一审计与指标体系。

# Scope & Guardrails

- **In Scope**：RPA 插件安装与启用、Flow Builder/Recorder、Flow JSON 规范、Browser/Desktop/API/File Runner、CoreX Workflow 中的 RPA Step、审计/日志/告警、凭据与 Cookie 管理、插件计费策略挂钩。
- **Out of Scope**：行业专属插件开发、第三方机器人平台对接、自定义 CV 识别模型训练、Agent 自学习 Prompt 策略、Marketplace 分发流程。
- **Environment & Flags**：`corex-workflow-v3`、`rpa-plugin-enabled`、`telemetry-unified-sink`、`px-secrets-vault`；Runner 节点需要访问浏览器沙箱与桌面代理。

# Core Capabilities

1. **Plugin Enablement & Governance**：CoreX 安装/启用 RPA 插件并托管 Runner 套件，复用 ACL、Secrets Vault、计费策略与审计准则，确保浏览器/桌面/API/File Runner 的安全边界。
2. **Flow Modeling & Recorder Bridge**：Flow Builder/Recorder 录制 Click/Input/Loop/API/File 节点并输出标准 Flow JSON，支持参数绑定、变量回放、人工确认 Step 与多 Runner 下发。
3. **Workflow Embedding & Execution Feedback**：Workflow Engine 将 RPA Step 注入任务 DAG、调度 Runner 执行并实时采集日志/截图/变量，执行结果在 1 分钟内写入审计与遥测系统供 Agent 推理回放。

# Participants & Responsibilities

| Scope | Repository | Layer | 责任与交付物 | Owners |
|-------|------------|-------|--------------|--------|
| workflow-engine | powerx | service | 意图解析、Workflow DAG、RPA Step 编排、审计与 ACL 复用 | Michael Hu |
| rpa-plugin-backend | powerx-plugin-rpa | plugin | Flow Builder API、Flow JSON 解析、Runner 调度、凭据管理 | Michael Hu |
| flow-designer-ui | powerx-plugin-rpa | ui | 录制器、拖拽式节点、变量与参数面板、多 Runner 选择 | Michael Hu |
| observability-hub | powerx | service | RPA 执行日志、指标、告警路由、审批提醒 | Michael Hu |

# Validation Workflow

1. **Stage 1 – Plugin Enablement**：管理员在 CoreX 中安装 RPA 插件，加载 Runner 套件并在 ACL 中授予租户可用权限。
2. **Stage 2 – Flow Modeling**：业务专家使用 Recorder + Flow Builder 录制流程，补充 Click/Input/Loop/API 节点并生成标准 Flow JSON。
3. **Stage 3 – Workflow Embedding**：智能体在 Workflow DAG 中新增 RPA Step，绑定 Flow ID、输入参数、人工确认/定时策略。
4. **Stage 4 – Execution & Feedback**：Runner 在 Browser/桌面/API 沙箱里 replay Flow、采集变量、返回结果并写入审计/指标供 Agent 后续推理使用。

# Architecture Diagram

```mermaid
sequenceDiagram
  participant Actor as CoreX Agent
  participant WF as Workflow Engine
  participant Plugin as RPA Plugin
  participant Runner as Browser/Desktop/API Runner
  participant Audit as Telemetry & Audit

  Actor->>WF: 提交含 RPA Step 的任务
  WF->>Plugin: Request Flow (flow_id + params)
  Plugin-->>Runner: 下发步骤与凭据
  Runner-->>Runner: 执行 Click/Input/API/文件操作
  Runner->>Plugin: 回传变量、截图、日志
  Plugin->>WF: Execution status + outputs
  WF->>Audit: 写入审计与指标
  WF->>Actor: 返回执行结果或补充澄清
```

# Key Interactions & Contracts

- **APIs / Events**：`POST /plugins/rpa/flows`（Flow 创建）、`GET /plugins/rpa/flows/{id}`（Flow JSON 读取）、`POST /rpa/flow/run`（执行入口）、`EVENT rpa.flow.completed`/`rpa.flow.failed`、`POST /workflow/{taskId}/steps/rpa`（Workflow 注入）、`POST /secrets/vault/credentials`（凭据管理）。
- **Configs / Schemas**：`docs/meta/scenarios/powerx/core-platform/rpa/primary.md` 内的 Flow JSON Schema、Runner 配置（BrowserRunner/DesktopRunner/APIRunner）、`config/workflow/feature_flags.yaml#rpa-step`、`TODO_LINK_RPA_SCHEMA`。
- **Security / Compliance**：凭据加密存储、沙箱隔离、人工确认 Step、审计日志长留、Cookie 管理合规、敏感操作需双因子批准。

# Related Links

- `PX-RPA-ORCH-001` — CoreX Workflow 中 RPA Step 调度/审计的服务层用例。
- `docs/meta/scenarios/powerx/core-platform/rpa/primary.md` — RPA 设计稿与 Flow JSON 规范来源。
- `docs/scenarios/core-platform/SCN-CORE-RPA-BID-CAPTURE-001.md` 等子场景文档：招投标抓取、CRM↔ERP 对账、OA 巡检执行细节。

# Acceptance Criteria

1. Flow Builder 录制后可在 5 分钟内生成可执行 Flow JSON，并支持 Click/Input/Loop/API/File 节点组合。
2. Workflow Engine 调度 RPA Step 成功率 ≥95%，失败需自动触发补偿或人工确认提醒。
3. 所有执行日志、截图、变量需在 1 分钟内写入统一审计，且凭据/Session 不得泄露。

# Telemetry & Ops

- 指标：`rpa.flow.run_total`、`rpa.flow.success_rate`、`rpa.runner.latency_p95`、`rpa.runner.desktop_availability`、`workflow.rpa_step.retry_total`。
- 告警阈值：连续 3 次 Runner 超时 >60s、成功率 <95%、审计写入失败 5 次/小时、凭据加载失败事件。
- 观测来源：`scripts/qa/workflow-metrics.mjs`、Grafana RPA Dashboard、Datadog `rpa.*` 指标、审计系统。

# Open Issues & Follow-ups

| 风险/事项 | 影响范围 | 负责人 | ETA |
|-----------|----------|--------|-----|
| Desktop Runner 依赖可选扩展，需明确交付顺序 | 跨 Runner 覆盖率 | Michael Hu | 2025-03-15 |
| Flow JSON Schema 尚未与 docmap / seeds 同步，需生成引用 | 规范一致性 | Michael Hu | 2025-03-08 |
| docmap.yaml 中缺少该主场景与子场景映射 | 文档导航 | Michael Hu | TODO_DATE |

# Appendix

- docs/meta/scenarios/powerx/core-platform/rpa/primary.md
- docs/standards/scenarios/_template.md
- TODO_RPA_SCHEMA_LINK
