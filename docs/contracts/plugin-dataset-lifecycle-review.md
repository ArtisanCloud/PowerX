# 插件数据集目录与只读预览合同评审

T175 平台合同。覆盖 T176～T177 的登记、观测和只读预览。Core 和 Framework 使用自有合成插件验收。业务插件独立接入，真实记录规则由对应插件维护。清理、归档和跨库写回执列明后续边界。运行验收见 [SG-12](../../specs/027-monitor-center/tasks.md#us9-sg-12)。

T176 已接通托管 PostgreSQL 来源的正式 STS 登记、Root 目录和只读计量，并用合成插件在隔离实例验证。T177 的 Core 端已接通正式 TaskBus 派发、领取授权、回执和管理视图。共用扫描执行器由 Framework 仓库交付。受控删除、源端回执与归档见 [执行合同](plugin-dataset-execution.md)。开发配置、启动和对接步骤见[开发联调文档](../guides/develop/storage-datasets.md)。

- [Root 管理 OpenAPI](../../specs/027-monitor-center/contracts/plugin-datasets.openapi.yaml) 提供目录、详情、创建预览、查询原预览。
- [Framework Host JSON Schema](../../specs/027-monitor-center/contracts/plugin-dataset-host.v1.schema.json) 定义包声明、登记、领取预览、上报及队列通知的类型。
- [总体设计](../plan/deploy/storage-governance-plugin-datasets.md) 说明产品范围、职责和实施顺序。

## 数据集与数据源

Core 按 `deployment_scope_id + owner_kind + owner_id + source_binding_uuid + dataset_key` 识别数据集。部署作用域使用 Core 的 `cfg.EnvironmentID`，实现前核对目标部署的持久性和唯一性。插件所有者用安装记录解析，保留正式插件 ID。应避免仅用环境名、Schema 名或插件数据库绑定 UUID 充当部署隔离标识。现行绑定 UUID 由部署环境和插件 ID 生成。 [Core 存储身份](../../backend/internal/bootstrap/storage_governance.go#L44) · [绑定 UUID 实现](../../backend/internal/infra/plugin/manager/host_config.go#L933)

- Core 内置目录按固定资源键投影，使用固定命名空间生成稳定 UUID。原系统表 ID 和控制域保持原义。一个现有策略覆盖多张表时，多个目录项可以引用同一原控制域。
- 插件数据集首次接纳时分配 UUID。插件升级、合同升级和同一逻辑绑定内的物理来源变化保留 UUID，后两项使旧预览失效。逻辑绑定变更产生新数据集，迁移与旧数据集退役由显式运维流程处理。
- 插件包内的 `plugin.yaml` 以 `data_lifecycle.<dataset_key>` 登记 `sha256` 和 `observation_tables`。Core 从安装根目录的 `contracts/storage-datasets/<dataset_key>.json` 读取声明并核验文件字节摘要。已接纳的合同另绑定观测表范围，修改范围需要新的合同版本。Framework 只提交声明键、版本、摘要和逻辑来源引用。包声明版本 `contract_version` 对应登记输入的 `definition_version`。服务信封的 `contract_version` 单独标识 wire 版本。
- 逻辑来源引用由 Host 解析。目标数据库、Schema 和执行器由可信绑定决定，绑定之外的物理对象一律拒绝。来源指纹由受控数据库身份、对象身份和来源代次生成。凭据轮换按授权变化重新验证连接，管理回包不含凭据和连接信息。
- 登记、来源和观测状态相互独立。绑定确实存在但尚无法核验 schema 或执行器时，可以登记为 `pending_validation`。绑定缺失返回 `422 DATASET_SOURCE_BINDING_UNRESOLVED`。物理来源不一致或恢复后缺少证明时置为 `needs_revalidation`，废止旧预览。退役记录保留原 UUID 和审计证据。

Core 首期复用本实例 PostgreSQL 连接池，通过只读事务读取元数据，并校验已安装插件角色对托管 schema 和关系的访问范围。独立来源通过可信部署绑定登记只读连接。缺少受控只读连接时，保留目录行并显示来源问题和未知容量。Framework 在数据源侧使用该插件受限连接执行预览。相同物理对象的容量在汇总按可靠的集群身份、数据库和 relation 身份去重，无法确认时报告覆盖不足；按租户预览计数不参与物理容量加总。 [现有只读采集边界](../../backend/internal/infra/storage_observer/system_tables.go)

## 权限与任务通道

| 界面 | 授权及资源范围 |
|---|---|
| Root 管理 | 复用人类 Root 会话和现有返回结构。列表只允许 OpenAPI 中的筛选与分页参数，其余管理接口拒绝查询参数。现有 `requireRootUser` 同时拒绝所有查询参数，实施时仅为新列表单独放行声明的键，旧路由保持原约束 |
| Framework 登记 | 通过正式 tenant invocation 使用固定 `core_internal`、`core://storage-governance/datasets`、`INVOKE` 和 `register` DTO。拟定能力 `com.corex.storage.datasets.register`，授权限有效 STS 插件服务及其已登记租户，必须核对该插件是当前包的安装所有者且租户启用 |
| Framework 预览执行 | 相同固定端点，拟定独立的 `com.corex.storage.datasets.preview_worker` 能力，仅允许 `claim_preview` 和 `report_preview`。校验插件、租户、当前运行绑定及实际 TaskBus 租约 |

两个服务能力在发布时写入正式平台目录并明确权限码、风险级别和 `agent_usable: false`。管理面保持 `admin_user` 与 Root 用户 JWT／RBAC；服务面保持 `service_actor` 与 STS。Root 路由不发布为 tenant invocation 的原始能力，服务权限不授予 Root 操作。首期服务合同仅支持托管环境 STS。若增加 Local Host API Key，应补显式授权元数据、同步 IAM 权限并验证 grant-status。

`register` 产生的是部署级插件声明，调用租户只证明服务具备该插件的合法入口权限。请求不含自选插件、租户、安装版本或物理库身份字段。注册后的目录不会扩大任一租户的运行范围。Root 每次预览仍要选择并核对一个真实启用租户。收到任意 endpoint、method、headers、SQL 或额外字段应返回 `400`。跨租户、跨插件或已撤销权限返回 `403`。

预览复用正式 TaskBus 和 Framework 的阻塞消费。Core 专用派发入口创建不可由普通入队设置的任务来源记录。记录绑定原请求 UUID、任务 ID、部署、租户、插件安装代次和目标订阅。预览回执和任务登记同事务提交，提交后沿用 TaskBus 恢复机制补投。通知只携带协议版本和原请求 UUID。Core 领取与首次上报时都要将活动租约的 `receipt_uuid`、`fence`、有效期及真实租约主体与这条权威任务记录比对。伪造通知即使取得普通租约，也因来源、任务或目标绑定不符而被拒绝。 [Host 入队与租约](../../backend/internal/service/event_fabric/task_queue/runtime_service.go) · [TaskBus 任务历史](../../backend/internal/service/event_fabric/task_queue/stream_history.go)

## 登记和预览过程

1. **登记。** Core 在权限核验后先核对原请求。相同请求 UUID、作用域、验证主体及请求摘要返回原登记回执。相同 UUID、不同请求返回 `409 DATASET_REQUEST_CONFLICT`。新 UUID 提交相同声明及相同来源时记录请求回执，并沿用当前接纳版本。当前安装版本不匹配、旧声明版本或同版本不同摘要返回 `409`。来源和结构核验后，目录公开当前接纳状态。保留范围必须满足最小值 ≤ 建议值 ≤ 最大值。
2. **创建。** Root 提交租户、期限、期望接纳版本和 `request_uuid`。Core 检查声明允许预览、核对租户和当前来源，固定 `cutoff_at = accepted_at - retention_days × 24h`，按 UTC 计算。Core 在同一事务保存回执和正式任务登记。新请求返回 `202`，同内容重放返回 `200` 及该请求当前状态。若提交情况未知，按原 UUID 查询。
3. **领取。** Framework 持签发给当前服务的 `receipt_uuid` 和 `fence` 调用 `claim_preview`。Core 验证活动租约及任务来源，复核版本、租户、安装和绑定，再返回固定范围、声明资产原始字节的 SHA-256、`schema_contract_version` 和适配器版本。Core 接纳修订同时绑定观测表范围。每次最多检查 2048 个根记录。默认五分钟内完成，执行 SQL 和事务另设更短限额。Framework 在事务前及每一扫描批次核对安装包摘要、期限、结构版本、适配器版本和实际来源指纹。托管 PostgreSQL 指纹按受管集群 ID、当前数据库 OID 与登记表 OID 计算。将每个成员编码为 `<cluster_id>:<database_oid>:<relation_oid>`，按字节排序后序列化为无空白 JSON 字符串数组，再计算 SHA-256。集群 ID 由部署提供，表成员只从已校验安装包读取。升级、恢复、绑定变化或当前授权失效时停止扫描，Core 拒绝旧结果覆盖当前状态。
4. **上报。** Core 仅接收当前活动租约的结果，将计数和状态落账后再允许 Framework 确认任务。报告已提交但响应丢失时，原身份、原租约及相同内容返回原结果，即使该租约随后到期。首个结果迟到、旧围栏、旧版本或不同报告内容返回 `409`；旧结果不能覆盖现态。数据源或租户状态变化后，原预览显示 `invalidated`。超时则显示 `expired`。
5. **展示。** 候选和保护数量以根记录计。每个已检查对象用至多一个主要保护原因。上报服务校验原因唯一，候选与保护数量之和不超过 `examined_count`，且 `examined_count` 不超过本次领取的 `max_examined`。登记和预览拒绝零 UUID。倒置的期限窗口由服务端拒绝。扫描未覆盖完整范围时标为 `bounded`，空结果仍保持对应覆盖口径。回执只返回计数、不返回主键清单或业务正文。既有 Root SSE 在回执落账后发布状态变更，页面仅在首载、事件、手动刷新和重连读取。

任何预览结果只描述观察时的候选条件。后续 T178 即使沿用该截止时间，也需重新核验策略版本、当前记录和业务引用并取得写入授权。

## 最小迁移与回滚

| 阶段 | 持久化改动 |
|---|---|
| T176 | Core 在现有集中迁移入口新增 `ops_storage_datasets`。内置目录从代码投影，现有业务源表、系统策略和历史均无需迁移 |
| T177 | 复用 `ops_storage_cleanup_runs` 中的新预览命名空间与正式 TaskBus 历史。利用其文档版本保存操作者、请求摘要、租户、版本、截止时间、状态和结果。只读预览不在插件库新建账本或执行 DDL |
| T178 | 插件源端需要同一事务内的执行幂等账本、执行围栏及待交付记录。已部署来源若没有满足原子性和租户范围的持久机制，需在插件源库做独立、集中管理的迁移；Core 继续汇总结果。业务变更与源端回执共同提交 |

Core 登记表按变更快照存储。每行具有独立行标识并保留稳定数据集 UUID，记录来源、安装版本、声明摘要、接纳版本、状态、时间和审计信息。当前头由 `is_current` 标记。相同声明的新请求可追加原请求回执而不推进接纳版本，不替换当前头。状态转换、当前头替换与审计快照在一个事务内提交。最低约束如下：

- 部署域内数据集 UUID 和逻辑唯一键各自仅有一个当前头。并发首次登记通过逻辑键唯一性和事务内重读取胜者。
- `(deployment_scope_id, request_uuid)` 对登记请求唯一，持久化认证主体标识、请求摘要和原响应。历史回执与当前头分开查询。来源失效和退役追加变更快照，保留来源绑定追溯信息。
- 声明版本单调推进；接纳版本在影响预览或执行的状态、来源、合同与适配器变化时推进。当前头与旧记录的唯一性检查不能因软删除失效。
- 插件预览回执和任务同事务登记，使用隔离命名空间。历史可按独立窗口清理，当前接纳快照与未确认回执保留。升级和回滚先停止新派发并核对已接纳批次，再撤下新版目录入口。登记表和回执可保留用于回读，已有系统表策略继续由原控制域管理。

独立 PostgreSQL 的写批次由源端事务同时保留数据变更、原批次 receipt 和待上报记录。Core 不使用网络响应判定是否提交。暂停时 Core 停止发新批次并核对源端代次，源端确认旧租约不能再提交且在途事务已排空后，页面才显示已停。T178 验证短租约、事务时限、旧实例恢复和提交成功后断线。

## 通用保护合同与验收样本

适配器按租户和稳定的根记录键判断。到期时间和终态只用于初筛。候选还需要确认记录完整、业务引用已核验、无需在线验证或重试、证据保留条件已满足。通用执行器提供只读事务、固定范围及聚合器。插件适配器返回有界的保护原因，通用执行器统一计数。业务规则无法核验时，适配器返回 `unknown_eligibility`。如果连任务或数据源范围也无法验证，预览应失败并显示原因。

| 条件 | 结果 |
|---|---|
| 已到期、结构完整、无业务引用、无恢复或留存要求 | 候选 |
| 仍由批次、用例、工单或其他对象引用 | `referenced` |
| 支持业务状态显示或续跑的最新有效记录 | `latest_effective` |
| 根与明细不完整、计数不一致 | `incomplete_record` |
| 仍在执行、有重试或有明确证据保留责任 | `active`、`retryable` 或 `retained_evidence` |
| 版本不识别或引用、证据来源、资格判断缺少证明 | `unknown_eligibility` |

每个产品只需声明源数据集以及规则启用范围。选择标准适配器需要证明数据符合相应的生命周期模型。复杂引用由接入插件实现适配器。真实报告规则不完整时，数据集仍可登记为仅观测。运营界面显示资格不可判定及其原因。

- **平台样本一。** Core 的 `capability_registry_health_probe_results` 投影至统一目录，操作入口仍指向原专项控制域。 [专项合同](system-table-retention-control.md)
- **平台样本二。** 使用 Core 和 Framework 自有的合成插件 `com.powerx.plugins.storage-fixture`，用虚构的报告、明细和引用构造正反例。T176 验收统一目录、租户和来源隔离、登记幂等和物理容量去重。T177 验收任务权限、保护结果、扫描计划及回执对账。
- **AI Craft 独立接入。** 已确认「单次回归报告＋断言明细」适合作为可选的目录样本。真实报告接入前，AI Craft 核对异常收尾完整性、最新有效报告、批次 JSON 中的引用、S08 最近 20 次续跑及验收证据归属，并确认实际保留窗口及根表扫描索引。这些规则只决定该数据集的候选资格。平台验收使用合成数据完成。

合成插件声明样例。适配器内部解析自身的测试表和关系，平台不接受请求指定表名。

```json
{
  "dataset_key": "fixture_report",
  "display_name_i18n": {"zh-CN": "合成执行报告", "en": "Synthetic reports"},
  "contract_version": 1,
  "schema_contract_version": 1,
  "source_binding_ref": "database",
  "tenant_scope": "tenant",
  "adapter_key": "fixture_report",
  "adapter_version": 1,
  "allowed_actions": ["observe", "preview"],
  "retention_bounds": {"minimum_days": 7, "maximum_days": 3650, "suggested_days": 30}
}
```
