# 系统表容量监控配置接管

2026-11-05，SG-11 的首个资源接入。该策略仅控制单表容量、增长、采样与清理健康的告警判定，不授权删除。后续[文件日志保留](storage-file-lifecycle-control.md)已实现逐来源接管；归档目标、备份和快照仍待接管。正式进度仍见 [T166～T174](../../specs/027-monitor-center/tasks.md#us9-commercial-gates)。

## 配置流程

1. 迁移与服务准备：通过既有集中升级入口安装配置表，配置正式 Stream TaskBus，再由用户重启 Core。启动只注册固定绑定和消费者，不迁移、不导入、不启用。
2. 在“存储治理 → 系统表清单 → 监控与告警 → 告警阈值”查看旧阈值、旧版本和启用状态。旧 JSON 是只读来源。迁移／队列／worker 未准备时明确阻止写入。
3. 勾选接管确认后，“接管到数据库并暂停”校验旧版本和完整摘要，复制阈值并保留原启用值、版本、摘要作为不可变来源证据。**新配置默认暂停**，不改写旧文件。接管后应核对发布状态。
4. 单独编辑并保存告警阈值及启用值。页面分别展示期望版本、数据库确认版本、本地快照版本；保存成功不等于已生效。正式 TaskBus 完成发布后经 SSE 通知更新。
5. 409 保留草稿与原版本，不自动覆盖；“放弃草稿并载入最新版本”是显式只读刷新。网络结果不明时阻止重复提交，只能按原请求 UUID 核对回执。404 仍可能是原操作尚在途，不自动重放。

## 专用接口

统一前缀可配置，下列相对路径以 `/admin/ops/storage/database-tables/monitoring` 为根。全部要求真实平台 Root 用户 access JWT；拒绝 API Key、STS 和服务身份。HTTP handler 只绑定严格 DTO 并调用 service。

| 方法与路径 | 输入与结果 |
| --- | --- |
| `GET /` | 监控状态、实际运行策略、configuration；旧来源与期望／确认版本分别展示 |
| `POST /adoptions` | `request_id` UUID、`legacy_revision`、`legacy_digest`、`confirm=true`；首个数据库版本停用 |
| `PUT /` | `request_id` UUID、`expected_revision>=1`、`confirm=true`、六个可编辑 policy 字段；版本、身份、actor 等输入不接受 |
| `GET /requests/{request_id}` | 只读核对原提交 Root 的回执；原请求策略／版本与当前 configuration 分开返回 |

可编辑字段仅有 enabled、warning_bytes、growth_bytes、growth_window_minutes、stale_after_minutes、failure_limit。范围、单位及 required 条件以 [OpenAPI](../../specs/027-monitor-center/contracts/storage-governance.openapi.yaml) 为准。配置 CAS、请求回执、审计和正式任务登记复用同一个数据库事务。

## 运行端保护

权威是 `ops_storage_control_configurations` 与不可变 revisions。旧 `table-monitoring.json` 不再接受业务写入。执行快照为私有目录中的 `table-monitoring-control.json`，通过既有协调者的锁、0600 文件、fsync 与原子替换发布，不成为第二套配置编辑入口。

快照绑定协议版本、host／filesystem／installation、配置版本、正文摘要、原操作者与不可变版本时间。发布只接受最新期望版本，不能用旧数据库覆盖较新的本地快照。首次发布复核旧来源摘要，防止读取与接管之间发生旧写入。

监控的离线资格为 **不允许**：每次判定必须核对当前数据库权威与已确认快照。数据库不可读、schema 缺失、快照缺失／被改动、身份或版本不符时暂停该项监控并显示不可用／待核对；不能复活旧 JSON 的启用值。停用期望值在等待发布时也立即约束本节点旧启用快照；启用必须等待发布确认。

配置保存的进程事件与原子快照文件事件合并到现有 SSE。保活不查询业务状态，不增加页面周期查询。分区切换保留草稿；未知提交期间阻止重复保存。

## 升级与恢复边界

旧前端缺少请求 UUID、携带服务端元数据或尚未接管时明确失败；旧 service 写入口返回 428，不再回写 JSON。新前端遇到没有 configuration 的旧后端时只读展示，不能偷偷使用旧写法。

本阶段实现安装范围的表监控接管，不宣称完成跨实例所有权切换、旧执行器下线、全资源恢复或紧急停用设计。恢复了旧数据库、丢失快照或改变实例身份后，错误状态须先核对；不自动重建启用证明。完整升级回退与其他资源仍属于 T169／T171。日常数据库迁移、重启及正式站上线未在本轮执行。

本切片能阻止旧数据库覆盖较新的本地快照；数据库与运行目录同时回滚时的完整恢复时代次保护仍待 T171 实现，不能用本轮监控验收关闭该门槛。

验证结果见[表监控接入证据](evidence/storage-monitoring-control-validation-20261105.json)。
