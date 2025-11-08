# Feature Specification: Plugin Release & Marketplace Publishing Foundation

**Domain Ownership**: CoreX (`corex.plugin_release`)  
**Feature Branch**: `[001-install-plugin-pxp]`  
**Created**: 2025-11-05  
**Status**: Draft  
**Input**: User description: "请根据docs/use_cases/_from_hub/SCN-DEV-PLUGIN-PUBLISH-001下所有的需求文档，实现相关的spec文档"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - End-to-end Release Guardrail (Priority: P1)

Plugin开发团队需要在 24 小时内把一个待上线版本从本地调试推进到获批的发布计划，包含质量门禁、审批链和回滚预案，确保未经验证的制品无法进入生产。

**Why this priority**: 这是所有后续渠道（生产灰度、Marketplace、离线导入）的前置条件，直接关系到发布质量与合规性。

**Independent Test**: 执行一次从本地构建到测试租户审批的发布申请，并核对输出的发布计划、回滚联系人和审计记录是否完整且在 SLA 内生成。

**Acceptance Scenarios**:

1. **Given** 开发者已在本地完成构建并生成版本说明，**When** 触发测试租户发布流水线，**Then** 系统在 24 小时内产出测试、扫描、审批结论及完整的生产发布计划。
2. **Given** 质量门禁检测到覆盖率或安全扫描不达标，**When** 发布经理尝试审批上线窗口，**Then** 系统阻断审批、通知责任人并保留审计链接，版本标签不会被锁定。

---

### User Story 2 - Controlled Production Rollout (Priority: P2)

Release Manager 需要按照灰度策略把已批准的版本发布到生产租户，实时观测指标并在异常时 5 分钟内回滚，确保租户业务稳定。

**Why this priority**: 生产租户的稳定性直接影响收入与 SLA，灰度与自动回滚能力是上线可信任的关键。

**Independent Test**: 基于既有发布计划执行一次灰度->全量的上线演练，验证指标采集、阈值判定、通知与回滚执行是否按策略运行。

**Acceptance Scenarios**:

1. **Given** 发布计划包含灰度批次、指标阈值与回滚脚本，**When** Release Manager 触发灰度部署，**Then** 系统在 30 分钟内完成扩容并生成观测报告与通知。
2. **Given** 灰度阶段指标偏差超过设定阈值，**When** 系统检测到异常，**Then** 自动在 5 分钟内回滚至上一版本并记录租户级别的恢复状态。

---

### User Story 3 - Multi-channel Distribution & Marketplace Visibility (Priority: P3)

Marketplace 运营与企业租户管理员需要在 2 个工作日内分别完成离线包送审入库和在线即时上架，同时保障签名一致、订阅通知与运营报表同步。

**Why this priority**: 多渠道分发确保不同网络环境的租户都能获取最新版插件，并维持 Marketplace 的商业运作。

**Independent Test**: 对同一版本分别完成离线包上传审核与在线发布流程，验证签名、许可证、通知和指纹校验在 SLA 内落地。

**Acceptance Scenarios**:

1. **Given** 离线包附带签名、依赖和许可证说明，**When** 运营在 Marketplace 提交离线入库申请，**Then** 审核系统在 48 小时内完成校验、返回结果并把包体同步到离线分发库。
2. **Given** 在线发布经过自动合规校验，**When** 审核员批准版本，**Then** Marketplace 即时上架并在 5 分钟内向订阅租户发送通知与初始运营报表链接。

---

### Edge Cases

- 测试租户资源不足或 Feature Flag 未开启导致流水线无法部署时，系统必须提供可复现的失败报告并允许重新排队。
- 本地调试产物与审批提交流水线的版本号不一致时，应拒绝锁定标签并提示开发者重新同步。
- 灰度阶段遇到第三方监控指标缺失，系统需要进入安全等待状态并提示人工决策。
- 离线导入过程中校验指纹与证书吊销列表不匹配时必须立即终止并保持旧版本运行。
- Marketplace 审核超出 SLA 或补件次数超阈值时，需要自动升级通知至合规负责人并暂停后续发布窗口。

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: 系统必须提供从 `px-plugin build/dev` 到 Web Admin 本地安装的闭环体验，使开发者单轮构建+安装迭代在 15 分钟内完成并输出调试日志。
- **FR-002**: 平台必须在本地安装与热更新过程中校验插件签名、最小权限模板与操作者凭证，所有操作写入可检索的审计记录。
- **FR-003**: 测试租户发布流水线必须执行自动化测试、覆盖率统计、安全与许可证扫描，未达标时阻断审批并通知提交者、QA 与发布经理。
- **FR-004**: 审批系统必须在 24 小时内完成多角色审批，生成包含上线窗口、灰度参数、回滚联系人和依赖列表的生产发布计划。
- **FR-005**: 平台必须支持 `powerx publish package --offline` 生成含制品、依赖、校验文件和签名指纹的离线包，并保留至离线分发库。
- **FR-006**: `powerx plugin import --offline` 导入流程必须校验签名与许可证、执行健康检查并在失败时自动回滚到上一稳定版本。
- **FR-007**: 灰度发布必须依据发布计划执行批次扩容、实时采集指标与日志，并在异常时于 5 分钟内完成自动回滚和责任人通知。
- **FR-008**: 系统必须将灰度与全量发布的指标、错误率、耗时等数据汇总到统一仪表盘，并针对偏差、缺失或超时触发告警。
- **FR-009**: Marketplace 在线发布必须要求填报版本元数据、定价与支持策略，自动校验签名、兼容矩阵和合规项后才能提交人工审核。
- **FR-010**: Marketplace 离线上传入口必须验签 `.pxp` 包、检查依赖及许可证信息，并在补件率超过 5% 时给出分级提醒与模版化指引。
- **FR-011**: 审核通过的版本必须在 Marketplace 上架后 5 分钟内向订阅租户发送通知，并生成初始运营报表与发布回执。
- **FR-012**: 所有发布相关的审计日志、签名指纹、审批结论与通知记录必须保留至少 180 天，支持按版本、租户或渠道查询。
- **FR-013**: Marketplace 审核流程中同一版本补件次数达到两次时，系统必须自动升级通知至合规负责人并暂缓后续发布窗口，直至补件完成或被驳回。

### Non-Functional Requirements

- **NFR-OBS-001**: 灰度与全量发布的指标、日志与告警应沿用现有 PowerX Prometheus + Grafana 栈，并针对发布场景补充所需的观测指标、告警规则与回滚触发通知，确保能在 5 分钟内完成异常检测与响应。
- **NFR-INF-001**: 离线分发库应复用现有 PowerX 多区域对象存储集群，通过加密分区托管离线包、校验文件与指纹数据，避免额外运维成本并保证与发布流水线的自动化衔接。

### Key Entities *(include if feature involves data)*

- **Plugin Release Candidate**: 描述每次提交的版本号、构建指纹、测试与扫描结果、审批状态，用于决定能否进入生产。
- **Release Plan**: 记录上线窗口、灰度批次、指标阈值、回滚联系人和通知模板，是生产部署与回滚的执行蓝图。
- **Offline Distribution Package**: 包含签名制品、依赖清单、校验文件和许可证状态的离线包体，关联离线分发库与租户导入记录。
- **Canary Deployment Record**: 捕捉每个灰度批次的租户范围、指标快照、告警、回滚动作与扩容结果，支撑复盘与合规。
- **Marketplace Listing**: 聚合版本元数据、审核状态、上架渠道、通知对象与运营报表链接，支撑线上与离线双通道发布。
- **Audit Trail**: 汇总本地调试、审批、导入、回滚与 Marketplace 操作的责任人、时间、指纹和结论，满足合规追踪需求。

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 90% 的发布申请在 24 小时内完成从提交到生成生产发布计划的闭环。
- **SC-002**: 本地构建+安装循环的中位数耗时 ≤ 15 分钟，热更新成功率 ≥ 95%。
- **SC-003**: 生产灰度发布的异常检测触发后，99% 的回滚在 5 分钟内完成且租户业务无新增 P1 告警。
- **SC-004**: 离线导入成功率 ≥ 98%，导入总耗时（含健康检查）平均 < 10 分钟。
- **SC-005**: Marketplace 审核（在线+离线）在 2 个工作日内完成率 ≥ 95%，补件率 < 5%，通知延迟 ≤ 5 分钟。
- **SC-006**: 发布相关审计与指标数据 100% 可在单一仪表盘内追溯到具体版本与渠道，并支持 180 天留存审计抽查。

## Assumptions

- 全量上线前的所有构建均使用统一的签名策略与证书轮换机制，证书有效期提前 30 天预警。
- 监控与告警系统能够在分钟级提供指标与事件推送，发布团队具备接收多渠道通知的权限。
- 企业租户具备访问离线分发库或镜像站的能力，且允许临时开启导入窗口以满足 10 分钟导入 SLA。
- Marketplace 审核团队跨区域覆盖工作日，能够在补件时于 1 个工作日内给出反馈。

## Admin Web UI Extension

- 为方便 root/system_admin 直接在 PowerX Web Admin 中调试 plug-in release 流程，需要新增“插件发布”菜单。
- UI 至少覆盖以下功能：
  1. 离线包入库：表单提交 `POST /api/admin/plugin-release/offline-packages`，展示包体校验结果与审计 ID。
  2. Marketplace 列表：分页列出 `GET /api/admin/plugin-release/marketplace/listings`（可先以本地 mock 实现），提供补件次数与 SLA 倒计时。
  3. 审核详情/操作：在详情页调用 `POST /api/admin/plugin-release/marketplace/listings/:id/reviews`，支持 need_fix / approved / rejected。
- UI 必须沿用 AdminOnly 中间件，失败时展示 API 返回的错误信息、审计 reference，方便回溯 CLI/后端日志。
- UX 要求：表单必填校验、上传进度条、提交按钮 loading；列表需支持搜索/排序/分页，加载态采用 skeleton + Spin，所有失败使用 toast 显示错误与审计 reference；仅 root/system_admin 可见编辑操作，普通管理员只读。
