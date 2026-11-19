# 插件数据集删除和归档合同

Core 提供策略、正式任务、单次授权、回执和归档取回。Framework 使用同一合同在数据源端执行。普通业务插件接入时提供自己的资格与保护规则。Core 使用自有合成报告、明细及引用验证流程。

- [管理 OpenAPI](../../specs/027-monitor-center/contracts/plugin-dataset-execution.openapi.yaml)
- [Framework 执行 DTO](../../specs/027-monitor-center/contracts/plugin-dataset-execution.v1.schema.json)
- [目录、声明与只读预览](plugin-dataset-lifecycle-review.md)

## 权限和流程

| 步骤 | Core 的验收条件 |
|---|---|
| 启用 | 平台 Root 会话按当前修订保存一个数据集、一个租户的策略。安装声明允许对应动作，租户已启用，执行凭据具有 `com.corex.storage.datasets.execution_worker` 与 TaskQueue 消费能力。默认策略暂停 |
| 派发 | Root 明确提交 `request_uuid`、新鲜且已完成的预览和期望策略版本。同一操作者、租户、数据集、期限与接纳版本必须一致。一份预览最多派发一个批次。回执、预览消费状态、在途状态与正式任务同事务提交 |
| 领取 | 通过现有 Host TaskQueue 阻塞读取 `powerx.storage.dataset-execution.v1`。通知主题为 `storage.dataset.execution.requested`，仅含协议版本和原请求 UUID。用租约的 receipt UUID 和 fence 访问类型化 `claim_execution`，Core 校验专用任务来源、部署、插件安装代次、租户、策略和源指纹 |
| 授权 | Core 签发一次不可延期的原批次 UUID 与 grant UUID，写入授权最长 15 秒。授权同时绑定截止时间、结构版、适配器版、最多 2048 个检查对象和最多 128 个写入主记录。Core 在现有批次记录中原子保存授权快照。授权回复丢失后，原活动租约在准入仍开放时可取回同一授权。授权过期、租约更换或治理暂停时，领取只返回 `reconcile_only` |
| 对账 | `reconcile_execution` 按原批次 UUID 查取已经过期的授权快照，同一租户、插件及 STS 主体方可调用。全局暂停、策略暂停或原租约失效时仍可查取。Core 等待源端持久回执后才结束批次 |
| 上报 | `report_execution` 只接纳同一已授权 STS 主体、原授权 UUID 和源端不可变回执。Core 按 `source_sealed_at` 校验源端封存结果。已签发的批次需要回执才能释放互斥，即使队列租约离线或已过期也可按原批次重放上报。ACK 在 Core 落账之后 |
| 暂停 | 停止新派发和新授权。未发授权的队列批次可显式撤销。已有授权且结果未知时策略显示 `draining`，批次到期显示 `needs_reconciliation`。收到真实源端结果后才确认暂停。全局治理开关阻止新派发和新授权，原批次查取快照和上报回执仍可用 |

`claim_execution`、`reconcile_execution`、`report_execution`、`archive_execution` 均使用 `POST <prefix>/tenant/invocations`、固定 `core_internal`、`core://storage-governance/datasets` 和 `INVOKE`。协议为 `powerx.storage-dataset-execution/v1`。服务使用有效的 `powerx:api` STS，Root 运维 API 独立授权。

### 源端事务和故障恢复

Framework 在源 PostgreSQL 建受控账本，并通过正式升级迁移管理。账本按原批次 UUID 唯一，包含部署、插件安装代次、绑定、租户、策略修订、授权 UUID、截止时间和源回执 UUID。业务修改、不可变结果和待上报记录必须在同一事务提交。回执摘要按源端正式序列化合同固定，Core 验证同一请求重放时上报内容一致。

执行器先按原批次 UUID 获取唯一源账本记录并锁定，再检查终态。源事务失败时业务修改与该事务的结果一起回滚。源事务成功提交后，重试交付同一持久回执。业务引用创建、重试和实际删除共享事务保护约束。新引用、最新有效记录、不完整或无法确认的对象继续保留。

每批扫描、保护检查和变更使用固定租户与稳定键。源端在事务前和封存时核对原截止时间，事务和锁等待均使用有界超时。回执的 `source_sealed_at` 使用 PostgreSQL 时钟。对 `committed`，该值表示源端封存结果时最后一次授权检查，必须在 `[issued_at, deadline_at)` 内。WAL 刷盘和物理提交可能在这次检查之后完成。对 `aborted`，该值表示零变更结果的封存时间，可以晚于截止时间。Core 拒绝早于授权签发或晚于其当前时钟的上报。删除和归档共用该字段。

### 全局暂停与授权响应丢失

1. Framework 从原 TaskBus 通知或已有待对账记录保留 `request_uuid`；必要时可从 Root 批次详情取得。原租约仍有效时调用 `claim_execution`。获得 `reconcile_only` 时只走对账流程。
2. 即使没有可用租约，也可用该批次原 STS 主体调用 `reconcile_execution`，输入只包含 `request_uuid`。从未签发授权返回 409 `DATASET_EXECUTION_GRANT_STALE`，尚在授权期内返回 409 `DATASET_EXECUTION_GRANT_ACTIVE`。到期返回 `state: reconcile_only`、原请求及授权 UUID，以及 `expired_grant`。该快照仅用于定位并核对原数据源和批次。早期批次缺少已持久化的快照时省略 `expired_grant`，此时仍可重放已有真实回执。
3. Framework 校验快照的原绑定、源指纹、声明摘要、结构版本和适配器版本。若已有终态，重放原持久回执。若授权回复丢失时源库还未预约，先确认源端数据库时钟已到原截止时间，再在事务内通过原批次 UUID 的唯一约束创建或锁定同一账本记录。等并发事务结束后重查结果。确认没有终态时，在该锁保护下封存 `aborted`、`affected_count: 0` 的持久回执，随后上报。该路径仅写源账本，业务数据保持原状。仍待 Core 确认的源账本记录须保留。
4. 来源、版本、时钟或锁状态无法核验时保留待核对。单纯未查到预约或回执无法确认中止。Core 收到真实源端终态后释放该策略的在途批次。新派发仍需通过全局准入，暂停前签发的原回执继续归原策略版本。

### 归档与取回

归档策略选择现有归档控制面已验证的私有、版本化 S3 目标。Core 固定目标修订和指纹。不开放接收目标 URL、对象键、凭据或任意 SQL。Framework 使用只读快照导出有界 UTF-8 JSON Lines，每行一个对象，末行带换行，单批最多 128 行及 256 KiB。通过 `archive_execution` 上报原批次回执和数据。Core 先登记内容承诺，再在独立对象命名空间条件写入，固定对象版本并读回核对大小和 SHA-256。重复上传同一批次需携带相同内容。通用调用轨迹仅保留操作元数据，不记录归档内容或授权。

Root 可以按原批次取回已确认的归档，服务重查目标、对象版本和校验值后才发送附件。取回是验证及离线使用入口，业务恢复由插件按其结构合同办理。归档阶段保留源数据。声明要求先归档的业务数据集仅开放归档动作，待源端能验证同一成员集合的版本与引用证据后再制定删除合同。

## 独立 PostgreSQL

托管库复用 Core 的受管绑定。独立 PG 使用可信部署配置 `storage_governance.plugin_postgres_sources`，按安装插件 ID 精确指定逻辑绑定 UUID、集群 ID、schema、源执行角色和只读观察连接。部署声明的集群 ID 对同一物理集群保持稳定，由不同物理集群使用不同 ID。Framework 的执行连接使用该同一绑定，在数据源端使用其受限业务角色。租户请求不能指定或覆盖连接。 [隔离配置与联调](../guides/develop/storage-datasets.md)
