# PowerX 存储治理开发任务索引

> 更新：2026-11-08。完整任务已登记到 `specs/027-monitor-center/tasks.md` 的 US9。**任务勾选与进度只在 specs 维护**；本文保留阶段入口与设计文档链接，不再维护第二套清单。

> 本文下方日期段是历史实施记录，不重复生成待办。全局紧急停用、受控恢复代次、跨资源版本重建及 S3 凭据／目标重新核对已在后续实现；当前现场剩余项以正式 tasks 的 SG-11 最新记录为准。

## 1. 正式规格与任务入口

| 文档 | 用途 |
|---|---|
| [spec.md / US9](../../../specs/027-monitor-center/spec.md) | 需求与验收：FR-049～FR-068、SC-017～SC-024 |
| [plan.md / US9](../../../specs/027-monitor-center/plan.md) | 实施边界、代码落点、阶段与依赖 |
| [tasks.md / US9](../../../specs/027-monitor-center/tasks.md#us9-storage-governance) | **完整开发任务与唯一进度来源：T101～T174** |
| [总体方案](storage-governance.md) | 容量总账、保留参数、生命周期与 XDocker 边界 |
| [UI 设计](storage-governance-ui.md) | 分区、字段、草稿、预览与事件推送 |
| [实施与验收手册](../../guides/deploy/027-monitor-center/usecase-us9-storage-governance.md) | 操作、联调、排障与回滚 |

## 2. 阶段任务导航

| 阶段 | 正式任务 | 设计编号 | 主要内容 |
|---|---|---|---|
| SG-00 | [T101～T103](../../../specs/027-monitor-center/tasks.md#us9-sg-00) | SG-00.x | 文档、规格登记与范围确认 |
| SG-01 | [T104～T109](../../../specs/027-monitor-center/tasks.md#us9-sg-01) | SG-01.x | 身份、计量合同、DTO／OpenAPI、权限与兼容 |
| SG-02 | [T110～T116](../../../specs/027-monitor-center/tasks.md#us9-sg-02) | SG-02.x | 只读采集、总账、估算、历史覆盖与告警 |
| SG-03 | [T117～T122](../../../specs/027-monitor-center/tasks.md#us9-sg-03) | SG-03.x | 跨环境预留、租约、准入、预览与审计 |
| SG-04 | [T123～T130](../../../specs/027-monitor-center/tasks.md#us9-sg-04) | SG-04.x | PG／journal、压缩、异地覆盖及日志交付 |
| SG-05 | [T131～T135](../../../specs/027-monitor-center/tasks.md#us9-sg-05) | SG-05.x | 临时上传、导出、附件与可重建缓存 |
| SG-06 | [T136～T141](../../../specs/027-monitor-center/tasks.md#us9-sg-06) | SG-06.x | 终态追踪、事件、审计与数据库空间 |
| SG-07 | [T142～T146](../../../specs/027-monitor-center/tasks.md#us9-sg-07) | SG-07.x | 备份、已验证恢复点、CAS 引用及回收 |
| SG-08 | [T147～T153](../../../specs/027-monitor-center/tasks.md#us9-sg-08) | SG-08.x | 真实 UI／API、纵向分区与 WS／SSE |
| SG-09 | [T154～T160](../../../specs/027-monitor-center/tasks.md#us9-sg-09) | SG-09.x | 双环境自动周期、故障、取回与运行交付 |
| SG-10 | [T161～T165](../../../specs/027-monitor-center/tasks.md#us9-sg-10) | SG-10.x | XDocker 合同、唯一预留权威与接管验收 |
| SG-11 | [T166～T174](../../../specs/027-monitor-center/tasks.md#us9-commercial-gates) | SG-11.x | 商用发布阻塞：配置唯一权威、原子版本、Root、发布确认、密钥及恢复 |

## 3. 实施与验收原则

商用配置治理按[正式约束](storage-governance-commercial.md)执行；SG-11 总门槛尚未关闭，已完成配置域与剩余项以正式 tasks.md 的续进记录为准；普通配置的数据库权威、独立运维状态和密钥职责必须明确。UI 可用和局部测试不构成商用交付完成，进度不在本索引另行勾选。

先只读观测，再建立全局预留／保护及归档，逐类预览、受控执行和启用自动治理，最后做 XDocker 接管。页面可用明确标记的 fixtures 先验布局，不能替代真实 API、权限、推送和运行验收。

正式任务保留阶段交付物、文件落点、保护条件、验收与证据矩阵。文件落点中“拟新增”的合同／代码是待冻结设计，不能当作现有 API／配置使用。

历史本地 v0.3 已有只读观测、登记目录计量、按环境／实例及资源编辑草稿、绑定来源的人工测算与占用历史；按份数真实产物估算和引用保护执行仍待接入，具体完成状态只看 specs。既有服务、线上策略和数据库不因本轮环境策略修改或任务登记而被 Agent 改写／重启，US6／US8 原有未完成项继续保留。


## 2026-10-30 v0.5 实施记录

本轮完成文件日志自动治理、同盘预算账本、受控写入准入、归档删除前握手、Loki 异步和 SSE 页面，数据库事件按用户要求只监控。代码和部署步骤见[场景 F](../../guides/deploy/027-monitor-center/usecase-us9-storage-governance.md#场景-f显式接管自动文件日志治理v05)。任务完整状态仍以 specs 为准：T117／T120／T130 的本地代码切片有新证据；外部程序／全部恢复路径、CAS 编码、journal 游标覆盖、真实新增量、备份字节淘汰、XDocker 和长周期现场验收未完成，不由本轮更新自动勾选。

追加本机真实实例验收，使用独立 Core／PG／Redis 和真实 Nuxt 浏览器，不模拟存储 API、不重启日常服务。验收发现并修正两处遗漏：五分钟观察里的时间差判断可能错过十五分钟清理边界，现改为独立定时器；旧 HTTP 媒体上传绕过驱动预算，现复用共享准入和原子暂存，并流式处理 multipart。结果和收尾见[真实实例证据](../../contracts/evidence/storage-governance-live-validation-20261030.json)，复验步骤见场景 F 第 6 节。单次隔离周期不替代 SG-09 的双环境／七天／十四天要求。

v0.5.2 审计加固：临时守护停止直接删除 audit／PG 文件，原生 logrotate 关闭过期删除；新增固定 SSH→私有版本化 S3 桥接和完整读回后回收。修正审计构造器重新启用原生保留的漏洞，开发环境也强制归档。本地 `mod` 已生效，数据库原生采集器重启、新 Core 部署、常在线目标和完整业务审计覆盖继续待验收。见[专用手册](../../guides/deploy/027-monitor-center/usecase-us9-storage-audit-custody.md)。

2026-11-01 开发收尾详见正式 tasks 的同日记录与[操作手册](../../guides/deploy/027-monitor-center/usecase-us9-storage-archive-controls.md)：新增调用审计独立增量范围、观察模式就绪状态和未确认批次上限；运行验收在开发交付后安排，不依赖操作者何时启动服务。

2026-11-07 两张调用日志表期限的数据库权威、预览／批次／回执和共享状态读取已完成隔离切片，见[合同](../../contracts/database-retention-control.md)。系统表专用期限规则、恢复交接和长期证据仍保持待办；不以此关闭 SG-11。

2026-11-07 专项系统表两适配器独立配置域已完成源码与隔离验收，见[合同](../../contracts/system-table-retention-control.md)。同一统一周期不再将未接管／暂停策略记成失败；此处是该切片完成时的边界；后续紧急停用和受控恢复重建见 storage-execution-control 合同。旧二进制现场交接与长期证据仍未关闭。


2026-11-09 追加 T172 备份清理待核对的只读分区、双层 Root 检查、元数据快照与共享 SSE；源码及隔离桌面／窄屏验收完成，未解锁或处理任何现场未知记录。[合同与证据](../../contracts/database-backup-policy-control.md#清理待核对2026-11-09)记录范围；自动完整校验／处理 UI、长期在线归档目标、实际执行器交接及运行门槛继续待办，以正式 tasks 最新记录为准。


2026-11-09 续进 T172 按需完整性诊断：清理弹窗按文件现状／完整校验分区，Root 一次 SSE 完整读取与记录复核已实现；它是可取消的只读诊断，不是持久证明或自动清理。合同／证据见[数据库备份安全边界](../../contracts/database-backup-policy-control.md#按需完整性诊断2026-11-09)。下一独立开发项为人工确认保留的 UUID／CAS／持久回执合同和实现；长期目标、现场接管与长期运行仍以正式 tasks 为准。


2026-11-09 T172 人工确认保留并保护已完成源码和隔离闭环；不清空原 token、不删除文件。新增审计回执表、dev/test 精确迁移、版本／围栏及原 UUID 核对见[合同](../../contracts/backup-cleanup-retention.md)。下一项 T171 为新增回执与受控恢复／升级的联合验收；本地启用必须先迁移再重启，未报告日常／生产生效或关闭 SG-11。


2026-11-09 T171 保留请求的恢复／升级联合切片完成源码与隔离验证：独立代次绑定核对版本、v1/v2→v3 锚点接续升级、旧执行器隔离，以及真实测试 schema 的 pg_dump/pg_restore、正式 Core 升级回调的幂等／事务故障。现存 UUID／保护／token 可保留，但 SQL 快照之后的审计仍需独立恢复凭据，作为下一工程项；不关闭 T171／SG-11，不报告日常或线上生效。[合同](../../contracts/backup-cleanup-retention.md#2026-11-09-t171恢复代次与正式升级联合验收)与正式 tasks 的同日续进是进度依据。


2026-11-09 T171 独立 UUID 凭据切片完成源码与隔离验收：v4 维护根先登记身份、SQL 确认后保存历史；恢复后读原请求不重放保护。仅保护 v4 起登记的请求，旧 SQL 回执的显式盘点／导入／覆盖证明是下一项，未关闭 T171／SG-11。见[合同](../../contracts/backup-retention-recovery-ledger.md)及正式 tasks 同日记录。
