# PowerX Project Constitution (v1.1.0)

## 0. 目的与范围
- 本宪法约束 PowerX（核心与插件生态）的“规范 → 计划 → 任务 → 实现”全流程，确保平台安全、可观测、可回滚。
- 宪法优先于所有子规范（spec/plan/tasks）。任何实现与本宪法冲突，必须回滚或修正后方可上线。

## 1. 规范驱动（Spec-Driven）
- 一切变更先改 `spec.md`，再依次执行 `/clarify → /plan → /tasks → /implement`，严禁“代码先行”。
- 每个 Feature 位于 `specs/<domain>/<name>/`，三件套同目录：`spec.md` / `plan.md` / `tasks.md`，缺一不可。
- 模板分型：`plugin/`、`agent-server/`、`mcp-server/`、`crud/`，使用 **语义化版本**（例：`crud@1.3.0`）治理并记录于 Front-matter。
- 文档 Front-matter 必含：`template_type`、`template_version`、`last_synced_template_version`、`owner`、`spec_version`。
- 文档采用 **AUTOGEN 可覆盖段** 标记：`<!-- BEGIN:AUTOGEN:{id} --> ... <!-- END:AUTOGEN:{id} -->`。升级模板时仅替换 AUTOGEN 段；人工段禁止被自动覆盖，若需手工维护请在段内添加 `@manual`。

## 2. 命名与契约
- 命名约定：HTTP 路由使用 `kebab-case`；gRPC Service/Message 使用 `PascalCase`；数据库列使用 `snake_case`；Go 包名小写无下划线。
- API 契约为唯一真理源：HTTP(OpenAPI)/gRPC(proto)/MCP(tools schema) 必须在 `spec.md` 的 **Contracts** 段定义，并与实现保持 1:1 对齐。
- 数据模型（DTO/Entity）与配置结构体需在契约中声明字段类型、校验规则、默认值。
- 版本策略优先向后兼容；破坏性变更必须提供兼容期与迁移说明，写入 `plan.md`，并在 `CHANGELOG.md` 记录影响面。
- 代码仓库层面，提交前运行 `make format vet unit-test`，确保 `go fmt` / `go vet` / `golangci-lint`（如可用）通过；禁止吞异常、禁止未记录的全局变量。

## 3. 安全、身份与租户
- 认证：统一采用 JWT/OIDC（RS256/JWKS）；配置 aud、scope，禁止对称密钥落盘；敏感配置通过密钥管理服务分发。
- 授权：RBAC 权限点命名为 `<domain.action>`，在 Spec 的 **RBAC** 段列出并同步到实现中间件。所有入口需校验权限并记录拒绝原因。
- 多租户：HTTP Header/Token 注入 `tenant_id`；数据库使用 schema 隔离或行级安全（RLS）；缓存与队列订阅需携带租户维度。
- 安全基线：输入校验、输出编码、速率限制、CSRF/SQL 注入防护必须落实；依赖漏洞扫描（`make deps-audit` 或等价脚本）定期执行，高危漏洞 24 小时内响应。

## 4. 审计与可观测
- 审计事件在 Spec 的 **Audit** 段定义，字段至少包含 `actor`、`tenant_id`、`action`、`resource`、`result`、`latency`、`request_id`。
- 日志统一结构化 JSON，必含 `trace_id`、`request_id`、`tenant_id`、`user_id`，写入集中管道并保留 ≥90 天。
- 指标：每个接口至少暴露 `qps`、`error_rate`、`p95_latency`；为关键服务设置自动告警阈值并记录到 Spec 的 **Observability** 段。
- Tracing：OpenTelemetry 必开，所有服务间调用及外部依赖生成 span，并通过 W3C Trace Context 传递。

## 5. 质量与测试
- 代码风格：Go 使用 `gofmt`、`golangci-lint`；前端/Node 组件使用 `eslint`、`prettier`，并在 Spec 中声明 lint 套件。
- 覆盖要求：核心模块单测覆盖率 ≥80%；接口回归用例需覆盖 Happy Path + 主要异常路径；对多租户、RBAC、审计路径提供集成测试。
- 性能基线：新增接口必须在 Spec 中标注目标 `p95` 延迟与吞吐阈值，上线后通过基线监控验证，不达标视为回归。
- 所有生成的脚本/迁移/配置，提交前必须通过 linters 与最小回归用例。

## 6. 发布与回滚
- 分支规范：`feat/<slug>`，PR 必附对应 `spec.md` diff 链接与发布/回滚计划。
- 灰度策略：先在内核沙箱验证，再进行租户级灰度，满足指标后再全量发布。灰度过程需记录在 Spec 的 **Release Plan** 段。
- 回滚：预置 Feature Flag 与 DB 迁移 `down` 脚本；重大故障 30 分钟内可完成回滚。回滚后 1 小时内完成根因分析初稿并更新 Spec。
- 变更日志：破坏性变更与兼容期必须同步到 `CHANGELOG.md` 及 `specs/.../plan.md`。

## 7. 模板升级与批量同步
- 模板发布需产出《迁移说明》，并更新 `template_version`。各 Spec 更新后需记录 `last_synced_template_version`。
- 批量升级优先使用脚本/CI Action 机械替换 AUTOGEN 段；需要理解性改写的段落由 AI 生成差异文本，再经人工审查后落盘。
- AUTOGEN 段的上下文、变量说明需在模板仓库维护；业务仓库仅填充差异内容。

## 8. AI 使用规范
- 与 AI 交互时指令须以“按 Constitution 执行（.specify/memory/constitution.md）”开头。
- AI 不得擅自调整 API 契约、RBAC 策略、安全配置；涉及安全或数据迁移的建议必须写入 `plan.md` 并经过人工审批。
- 若 AI 生成脚本/迁移/配置，提交前必须执行 `make format vet unit-test` 及相关 lint/test，确保最小回归用例通过。

## 9. 验收清单（PR 必勾）
- [ ] `spec/plan/tasks` 三件套齐全且一致，Front-matter 字段完整。
- [ ] 契约与实现一致，RBAC/租户隔离/审计链路已接入并通过测试。
- [ ] 指标/日志/Trace 正常，性能基线与测试用例全部通过。
- [ ] 使用新模板版本时：AUTOGEN 段已同步，`last_synced_template_version` 已更新，迁移项已记录。

## 工作流与审查
- PR 审查需确认契约更新、测试结果、监控告警及回滚计划均满足本宪章；审查记录需附在 PR 描述。
- Spec 审批通过后方可进入开发；实施过程中若偏离方案，必须提交修订版并重新审批。
- 发布前需召开 Go/No-Go 评审，包含安全、运维、业务代表签字；上线后 1 个工作日内完成审计复核。

## 治理与修订
- 本宪章高于其他流程文档，开发、运营、合规流程必须遵循。
- 修订需提交 RFC，包含变更动机、影响面、迁移计划；经技术委员会审批后发布新版本并更新本文档的版本信息。

**Version**: 1.1.0 | **Ratified**: 2024-04-01 | **Last Amended**: 2024-06-01
