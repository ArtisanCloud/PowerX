# 封存文件日志策略的数据库接管

2026-11-06，SG-11 的逐来源资源接入。进度以 [T166～T174](../../specs/027-monitor-center/tasks.md#us9-commercial-gates) 为准；本合同不表示归档目标、备份、快照、数据库期限或主机预算已全部接管。

## 页面与操作顺序

日志来源由可信部署登记，不接受页面输入目录、表名或脚本。每一路支持治理的日志有自己的配置版本；应用日志与审计文件日志不会因为保存其中一路而互相覆盖。

1. 通过既有集中升级入口准备配置表和正式 Stream TaskBus，再由用户重启 Core。启动注册固定来源的发布绑定与消费者，不迁移、不导入、不自动启用。
2. “存储治理 → 资源保留策略 → 当前日志资源 → 编辑配置”或“自动治理 → 配置策略”打开相同弹层。默认“保留策略”，次级“预览与清理”；窄屏使用分区选择器。切换保留表单和预览，错误、未知提交及版本状态始终可见。
3. 旧策略只读展示。明确勾选“接管并先暂停”，核对旧版本和完整摘要后创建数据库 v1，启用值固定为 false。保留旧来源的正文、摘要、版本及原启用状态，原 JSON 不回写。
4. 核对天数、预算与删除方式，再单独确认保存。每次请求有 UUID，保存使用该来源的期望版本。页面区分期望、数据库确认、本地快照版本；保存成功不代表已生效。
5. 发布成功由 SSE 通知状态同步。需要手动清理时，切到“预览与清理”，核对候选与保护项；只有表单与已生效策略一致、重新预览且没有未知提交时，才允许启动一次有界清理。日常自动清理由原有随 Core 启动的治理 worker 执行，不新增守护脚本。
6. 版本冲突保留原版本和草稿，用户可明确“放弃草稿并载入最新版本”。超时、断网或响应格式不明，只能核对原 UUID 回执；404 不等于原请求没有发生，不能重发。清理批次继续使用原有独立请求编号和运行账本，不重放未知删除。

“直接到期删除”与“归档后删除”是显式不同选择。关闭归档本身不会自动切换删除方式。归档方式仍需原生目标可用及逐文件校验；本切片不改变归档推送目标、凭据或取回合同。容量超限不缩短保留时间，活动文件、最新轮转、未知文件、链接、来源及所有者不匹配等保护继续有效。

## 专用接口

前缀跟随部署配置，以下以 `/admin/ops/storage/runtime/policies/{source_id}` 为根。全部要求真实平台 Root 用户 access JWT；拒绝 API Key、STS 和服务身份。不提供通用 JSON 配置接口。

| 方法与路径 | 输入及结果 |
| --- | --- |
| `GET /` | 单来源权威、旧来源证据、期望策略、期望／确认／快照版本、准备情况 |
| `POST /adoptions` | UUID `request_id`、`legacy_revision`、64 位 `legacy_digest`、`confirm=true`；首次接管默认暂停 |
| `PUT /` | UUID `request_id`、`expected_revision>=1`、enabled、cleanup_mode、retention_days、max_bytes、与模式一致的 require_archive、`confirm=true` |
| `GET /requests/{request_id}` | 原操作者的不可变操作策略／版本，加当前 configuration；只读，不重放写入 |

可编辑策略仅四项：enabled、cleanup_mode、retention_days（1～3650 整天）、max_bytes（1～2^50 整数字节）。disabled 模式不能启用。未知字段、目录、安装身份、操作者等覆盖被严格拒绝。范围以 [OpenAPI 0.8.0](../../specs/027-monitor-center/contracts/storage-governance.openapi.yaml) 为准。旧客户端缺 UUID 或 expected_revision=0 明确失败；没有数据库权威的新接口不会退回旧 JSON 写法。旧内部 SaveRuntimePolicy 在配置权威启用时返回 428，Engine.Save 在权威绑定后也拒绝写入。

## 唯一权威、发布与最终删除

配置复用 `ops_storage_control_configurations` 和不可变 `ops_storage_control_revisions`，tenant_id=0。安装范围继续使用既有 scope_id，不改历史哈希；来源域为 `file-lifecycle.` 加 source_id 的 SHA256 前 32 个十六进制字符，正文保留完整 source_id 和可信来源指纹。未登记、仅观察和其他实例来源不能绑定任意执行器。

CAS、请求回执／审计及正式 TaskBus 登记同事务提交。相同 UUID／操作者／规范化内容返回原操作版本；同键异载荷或异操作者冲突。每来源独立版本。普通配置保存不删除文件。

私有 `lifecycle.json` 的旧 policies 保留只读证据；published 保存经数据库批准的执行快照，runs 继续记录意图及结果。published 不是可由页面独立修改的配置。发布复用主机锁、文件权限、fsync 和原子替换，绑定协议、host、filesystem、installation、来源指纹、版本、摘要、原操作者及不可变版本时间。首次发布再次校验旧来源摘要；旧消息不能覆盖新版本。

离线破坏性操作资格为 **不允许**。策略读取核对数据库当前权威、不可变版本和本地批准快照。断库、缺表、未知身份、缺失或篡改快照会阻止清理；pending 的任何变更暂停使用旧快照。接管前只有数据库确认该来源尚未接管时，才允许原有有效旧策略继续运行；数据库状态未知时不能借旧启用值继续删除。

最终删除逐文件采用固定顺序：**数据库资源锁 → 当前配置行锁 → 主机账本锁 → 原生文件保护与 unlink → 有界结果提交**。配置保存使用相同 PostgreSQL 事务级 advisory lock，覆盖首行尚不存在的接管窗口，防止旧批次穿过“旧配置未接管”的空隙。发布保持“配置行锁 → 主机锁”，不存在主机锁内回调数据库的反向取锁。

暂停成功返回之前，已进入最终文件提交的一个有界操作可能完成；暂停不能伪装撤销已发生删除。保存成功之后，后续旧版本文件提交被拒绝。若文件提交后数据库事务结果不明，运行标为 unconfirmed，保留证据并禁止自动重放。读写操作仅在尚未进入回调时，为取得短暂主机锁有界等待最多 250ms；持久化／unlink 回调最多调用一次，不重试已进入的操作。这不是固定页面 loading 或周期状态查询。

## 升级、故障与交付边界

运行端必须先具备集中配置迁移、TaskBus 和当前来源登记。无 configuration 的旧后端在新前端只能查看；未迁移或发布器退出时明确阻止新启用。停止期望值可先保存以阻止旧批次，即使发布确认尚未完成。客户端旧草稿不自动换成新版本重试。

| 状况 | 处理 |
| --- | --- |
| 旧来源版本或摘要变动 | 409；重新读取并人工核对，不静默导入 |
| 保存冲突 | 保留草稿和基准版本；明确载入最新再编辑 |
| 保存响应丢失 | 查询同一来源／原 UUID；回执不存在仍保持待核对 |
| pending／failed | 不用旧启用快照执行；先排查正式 TaskBus 与发布错误 |
| DB 不可用、快照或来源不符 | 停止新删除；不修改来源文件来绕过校验 |
| 数据库或目录恢复 | 核对版本、身份、不可变证据；不能自动重建启用证明 |
| 回退代码 | 先暂停并核对在途运行；不得把旧 JSON 重新作为可写启用权威 |

本轮不执行日常数据库迁移、用户服务重启、正式站变更或真实客户日志删除。数据库和运行目录同时回滚的恢复时代次保护、紧急停用的跨实例所有权、归档密钥与目标、其他资源的数据库接管，以及 SG-09 远端取回和 7／14 天运行证据仍待完成。不能据此关闭整个 SG-11。

源码映射：`internal/infra/storage_lifecycle/authority.go`、`engine.go`，`internal/service/storage_governance/lifecycle_control.go`，`ControlRepository.WithCurrent`，HTTP `lifecycle_control_handler.go` 与 `runtime_handler.go`，Web `StorageRuntimePanel.vue` 与 `storageRuntimeService.ts`。隔离验证见 [证据](evidence/storage-lifecycle-control-validation-20261106.json)。
