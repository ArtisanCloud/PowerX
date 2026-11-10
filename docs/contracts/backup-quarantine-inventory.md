# 隔离产物台账与只读复查

2026-11-09，T172 接续[人工隔离](backup-cleanup-quarantine.md)。当前安装的 Root 管理会话可以定位已经退出待核对清单的处置，查询原 UUID、token、原因、分类与时间，并显式复查当前文件。台账和复查没有写入／删除／解除隔离接口。

## 历史、当前与覆盖

历史来源是当前安装 Scope 的独立维护凭据与同 Scope 的 `quarantine_cleanup` SQL 审计。独立记录优先；SQL 存在而独立记录缺失时仍保留可见条目并标记未完整覆盖，不补建独立凭据。原回执查询／显式导入流程的授权和执行资格不因台账而改变。

每条历史请求单独占一行：

| 字段 | 含义 |
|---|---|
| request_id / artifact_id / job_id / policy_id / token | 历史固定身份，ID 均为字符串；不输出变化后的新 token |
| result / diagnosis / reason / actor_id / created_at / completed_at | 历史处置结果及当时分类；不是当前文件情况 |
| current | quarantined、not_quarantined、missing、unavailable、identity_changed |
| cleanup_state / size_bytes | 当前数据库标记和登记容量；不是本次目录扫描或磁盘实占 |
| history_status | covered、sql_replayed、sql_missing、checkpoint_required、independent_missing、conflict |
| can_inspect / snapshot | 是否能固定当前来源进行只读复查、该观察的摘要；无写入或恢复授权 |
| valid_backup_proof | 恒 false |

独立终态与恢复后 SQL checking 并存显示 sql_replayed；不把独立历史重放到当前库。SQL 回执缺失保持 sql_missing，输入／结论冲突为 conflict。当前记录缺失、软删／孤立、Job/Policy/token 改变分别呈现，不能把它们都算成“当前已隔离”。冲突不开放文件复查。即使文件后来变完整，历史分类仍是当时分类，隔离与下载／恢复保护保持不变。

本库中存在、但不能归属到当前 Scope 精确历史身份的隔离标记另报 `unattributed_quarantined_artifacts`。它们不混入有归属的历史条目，也不宣称全部覆盖；页面给出覆盖告警并引导核对执行历史。跨 Scope SQL 记录不借此暴露详情。坏的 SQL-only 请求会令来源核对失败；缺表、缺列或独立凭据不可用返回专用错误，不能伪装成空清单。

## 查询一致性与界限

独立记录有既有 4096 身份／16 MiB 上限。当前 Scope SQL 来源、合并请求及本库隔离标记各最多 4096；超过返回 423，不截断后假装完整。一个三秒、有界只读 repeatable-read SQL 事务批量读取审计、当前产物与关联 Job、隔离标记，无 N+1。SQL 查询不读取 dump、sidecar 或凭据秘密。合并前后检查独立记录摘要及恢复代次，变化返回 409。没有 SQL／文件系统原子快照或长期锁定分页的承诺。

在有界来源合并后执行服务端过滤、排序和分页。current、result、diagnosis 是固定枚举；q 仅精确匹配历史请求 UUID、原 token、artifact/job ID。sort 为 time（终态时间，checking 用创建时间）、size（当前登记容量，未知按 -1）、artifact（数值）；direction 为 asc/desc，UUID 作稳定次序。page 1–100000、page_size 1–100，默认 1/20。动态状态变化后总数可变化，采集时间随每次观察更新。

本功能不新增数据库迁移、维护协议或普通能力准入依赖。使用已实现的 v7／凭据 v4 及 protected 审计模型。未就绪只影响台账／复查；不进入 ControlService.Ready 或插件容量门禁。

## 显式复查

GET SSE 必须绑定原 request UUID 与 expected_snapshot。摘要包含历史身份、来源核对状态、当前元数据及独立恢复代次；仅用于只读复查，不是清理、解除隔离或恢复的授权票据。

最长五分钟、请求取消即停止。取得既有 policy 会话锁后重新查询精确快照，在长 SQL 事务外复用 no-follow、单链接、固定父目录／文件描述符与 ctime 的完整诊断，再核对当前 SQL 身份与代次。单连接池、忙策略、缺失来源、旧快照、元数据或恢复变化均不能报告稳定成功。不会创建 TaskBus 任务、私有队列或脱离请求的扫描进程。

progress 为 running 的 `CleanupVerification`；complete 包含 request_id、snapshot、verification、currently_quarantined、read_only=true、quarantine_unchanged=true、valid_backup_proof=false。verification.content_verified=true 仅说明本次文件内容校验通过，不代表完整业务可恢复，也不解除隔离。中断、incomplete／EOF、结果身份不匹配不能当完成。外部非协作进程写文件不属于策略锁排空证明，结果仅为当次观察。

## API 与页面

相对可配置 API 前缀，默认 `/api/v1`；全部为 Root 人类 GET，无插件服务能力：

- GET `/admin/ops/backup/cleanup-quarantine-inventory`：固定过滤、排序和分页。
- GET `/admin/ops/backup/cleanup-quarantine-inventory/:request_id`：原 UUID 的历史与当前状态，不接受 query。
- GET `/admin/ops/backup/cleanup-quarantine-inventory/:request_id/inspection?expected_snapshot=...`：一次按需 SSE，只接受该精确 query。

[OpenAPI](backup-quarantine-inventory.openapi.yaml)。未知／重复 query、非法枚举与自由路径拒绝；API Key、STS、service_actor 和普通用户拒绝。404 不证明此前未执行。

Web Admin：备份中心 → 数据库备份 → **隔离产物台账**。筛选／固定排序 → 详情 → 处置历史／当前文件复查，桌面纵向任务导航，窄屏任务选择器。详情按 UUID 固定，历史结果与当前标记分开；草稿筛选和进行中复查在分区切换时保留。元数据变化或断库使复查失效，旧列表保留并提示未知；已发现的读取／覆盖告警在备份中心顶部仍可发现。共享 backup_cleanup SSE 只同步元数据；不自动扫文件、POST 或周期轮询。离开页面／换身份／关闭详情释放旧请求，重连可做一次状态同步。

## 验收与交付边界

[操作指南](../guides/deploy/027-monitor-center/usecase-us9-quarantine-inventory.md)、[证据](evidence/backup-quarantine-inventory-validation-20261109.json)区分源码、原生隔离 SQL 回放、受控浏览器故障和现场部署。台账不修复未覆盖历史，不重新隔离数据库回放后的产物，不允许新 UUID 绕过未知原请求，不开放解除隔离或删除。

本切片不关闭整个 T172／SG-11。下一现场优先项按任务清单仍为 **T155 长期私有 S3／OSS 目标验收**：通过正式 UI 准备 endpoint、bucket、region、服务器凭据引用后，以独立前缀完成归档、全量校验、源消失后精确取回及收尾；同机或依赖 Mac 在线的 MinIO 不替代长期目标。目标和维护授权就绪后再进入旧执行器现场交接。整站业务恢复、外部密钥、双环境、7/14 天与完整发布审查仍待办。
