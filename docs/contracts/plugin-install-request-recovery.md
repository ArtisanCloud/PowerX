# 插件安装任务、请求取消与收尾恢复

日期：2026-11-12。范围是 Core 本地／URL 安装、显式启用及安装弹窗；不修改插件权限清单，不自动修复现场授权或租户映射。

管理页面现在使用持久安装任务，下述同步请求取消规则继续用于兼容入口和显式启用。浏览器断开只停止上传／进度连接，不取消已入队安装。

## 状态和写入边界

安装产物、全局运行状态和当前租户配置是三个不同阶段。`registry.state=enabled` 不能单独证明当前租户已就绪。

- 产物提交前失败：继续清理首次安装半成品；替换失败继续恢复原版本和登记。
- 首次安装产物、迁移及登记已提交后激活失败：保留已提交产物，返回 `PLUGIN_INSTALL_FINALIZATION_REQUIRED`，供管理员显式继续启用，避免再次上传及迁移。替换安装仍采用原有回滚语义。
- 启用过程中请求取消：停止本次新启动的后端／管理前端，卸载本次路由，不继续登记 `enabled`。开始前已经健康启用的运行实例不因另一个取消请求被停止。
- 进程型管理前端探针失败：只有存在实际 `index.html` 的目录可以降级为静态管理页；SSR 的 public 资源目录不能充当可用管理页。请求取消直接失败，不通过静态降级伪报成功。
- 运行就绪核对只接受已经生效的静态路由：当前插件目录匹配、入口文件存在，并且没有优先使用的管理代理。管理进程退出但仍保留旧代理时，即使磁盘上有备用入口也报告未就绪；显式启用使用同一判断，不能因备用文件返回 `already_enabled`。只读查询不卸载代理、不切换路由。
- 全局启动成功、当前租户启用或 Topic 注册失败：返回明确的待完成状态，保留已提交资源及现有 ACL。普通 GET、重启和查询不补授权。

错误 details 只包含 `code`、插件 ID、已安装版本／修订／全局状态、失败阶段及 `tenant_ready=false`，不包含 HostConfig、凭据或原始内部错误。阶段包括 `runtime_activation`、`tenant_activation`、`topic_registration`。

## 正式恢复入口

沿用 Root 管理入口 `POST /api/v1/admin/plugins/{id}/enable`。恢复请求携带 `expected_version` 和 `expected_revision`；Handler 预检后，Manager 在现有安装变更锁内再次核对版本，版本变化明确拒绝。已有客户端空 body 启用合同保持。

运行实例已经启用时，`already_enabled` 不再阻断当前租户收尾；其他运行错误仍正常拒绝。显式恢复继续调用正式 TenantPluginInstanceService / Event Fabric 准备事务，按当前声明及归属核对并保存映射，不绕过能力授权或恢复被撤销的 ACL。

现有 `GET /api/v1/admin/plugins/{id}/tenant_config` 增补 `installation`：版本、修订、全局状态、`runtime_ready`、`tenant_ready`、`ready` 和原因码。租户就绪通过已持久化配置及 `pluginbinding.Prepare(provision=false)` 核对实时 ACL；读取不创建主题、授权、凭据或映射。运行就绪同时核对后端与进程型管理前端。

## 持久安装任务合同

Root 用户 JWT、当前租户和发起用户 UUID 共同限定任务访问；普通管理员、插件 STS 和 API Key 不使用这些接口。任务归属固定在当前环境身份与暂存根目录，不能移交到另一个实例执行。

| 接口 | 行为 |
|---|---|
| `POST /api/v1/admin/plugins/installations` | 先保存安装意图，返回 `task_id`；`request_id` 是用户生成的 UUID，同一用户／租户同一输入幂等，输入改变返回冲突 |
| `PUT /api/v1/admin/plugins/installations/{task_id}/package` | 仅本地任务，上传文件夹或 tar.gz；持久化完整来源后，在同一数据库事务登记正式 TaskBus；重复上传核对同一任务 |
| `GET /api/v1/admin/plugins/installations` | 最近 50 条本人当前租户和环境的任务；不触发执行 |
| `GET /api/v1/admin/plugins/installations/{task_id}` | 只读任务回执；不补权限、映射或重跑安装 |
| `GET /api/v1/admin/plugins/installations/{task_id}/events` | SSE 当前快照与变更快照；鉴权在 Header，不在 URL；每分钟重新连接鉴权 |
| `DELETE /api/v1/admin/plugins/installations/{task_id}/source` | 取消尚未入队的上传，或清理终态私有来源；禁止删除排队／运行任务；保留回执，不卸载插件 |

新增 `plugin_installations` 只保存安装意图、上传摘要和执行阶段。租约、续租、fencing、重试和投递恢复全部使用正式 `event_task_histories` / Stream TaskBus，不建立私有队列。任务中只传递租户和任务 UUID；URL、元数据等私有输入使用当前 Core 主密钥派生的 AES-GCM 密钥加密保存，租户和任务 UUID 绑定为 AAD，不返回 HostConfig 或凭据。主密钥、环境身份和来源目录应在升级时保持稳定。

Worker 生命周期属于 Core，不属于创建／上传 HTTP 请求；单次执行上限 15 分钟。后台 worker 被禁用、未运行，或任务结构未迁移时拒绝接受新执行，不伪装成磁盘已满。原同步 `/install/local`、`/install/url` 继续兼容旧客户端，其请求生命周期合同不变。

执行保留原 Manager 的安装锁、版本 CAS、发布者确认、drain、迁移、替换事务和启用回滚。后台任务请求启用时仍在原安装事务内启动，替换启动失败恢复原安装；首次提交后的启动失败保留产物，进入 `needs_attention`。随后按同一修订完成当前租户凭据和已启用租户 Event Fabric 配置，不扩大权限或忽略被撤销的 ACL。

本地来源拒绝链接、特殊文件、越界路径、重复归档项和校验过程中变化的文件；完整来源上限 1 GiB、10000 个文件。上传、暂存、复制和 URL 下载／解压使用现有空间准入及受约束 Writer；空间准入明确开启时必须成功预留才写入，续租失败停止。URL 沿用原下载／展开大小限制，并受 6 GiB 累计写入预算约束；替换暂存与安装复制共享预算。不同文件系统无法核对到现有受管磁盘时保持拒绝，不能绕过保护。安装前应为临时来源及替换副本预留额外空间。

暂存位置为配置 `plugin.installed_dir` 的同级 `install-tasks/{task_id}`，权限 0700，不接受客户端路径字段。终态清理私有来源；未完成上传可显式取消。进程异常退出留下的 `staging-{UUID}` 在同一任务上传锁内重试或取消时清理，不删除未知目录、已安装产物或业务数据。回执属于执行／恢复状态，已加入存储治理的系统表只读观测目录，不进入普通日志删除白名单。

### Core 中断后的处理

安装修订固定为任务 UUID，并在复制、迁移、登记和启用阶段保存受 TaskBus 租约约束的检查点。

- 尚未执行的队列任务：通过正式 TaskBus 恢复执行。
- 已进入执行且登记版本／修订与本任务一致：仅继续启用和租户收尾，不再次复制或迁移。
- 已进入执行但无法证实登记提交，或被其他安装替换：进入 `needs_attention`，人工核对日志、迁移结果与安装登记；不自动重跑迁移。
- 回执已经终态：不自动再执行；来源清理不改变安装回执。

## 页面行为

页面先读取本地包字面量 ID／版本，创建持久任务，再上传完整安装包。上传中断允许在同一任务重新选择相同来源、重新上传整个包；这不是分块断点续传。来源持久化成功后显示排队、校验、复制、迁移、登记、启动检查、租户收尾等真实阶段，不用固定等待时间宣告成功。

关闭弹窗取消当前上传／读取连接；已经入队的任务继续。重新打开或刷新后从正式列表找回任务，无需再次创建安装。SSE 发送初始快照和变更，心跳只保活；无变更时不周期查询详情／列表。手动核对、重新进入、断线重连可以同步一次状态。连接异常明确显示并按有上限退避重连；身份／租户变化立即释放旧连接、拒绝迟到结果，表单与进行中的任务按目标隔离。URL 或凭据不写入浏览器持久存储。

失败回执显示阶段和安全原因码；替换确认与 drain 失败允许返回表单调整，再创建新意图。`needs_attention` 且有已安装修订时进入原只读安装核对与显式启用流程，绑定目标版本和修订，不重新上传。没有可核实修订时要求管理员先核对，不提供盲目重跑迁移按钮。人工恢复后原任务仍保留“需要核对”的历史回执，当前运行就绪以正式只读安装核对结果为准。

## 验证与部署

回归覆盖请求取消后的进程退出与监听端口释放、已提交产物保留、健康旧实例保护、修订变化拒绝、已启用实例继续收尾、错误回包凭据裁剪、缺映射只读检查及撤权保持。前端验证表单／文件保留、无自动重传、显式恢复和身份切换；浏览器使用受控 API 检查桌面及窄屏实际布局。

历史 PR #427 的同步取消与收尾保护继续由原 Manager／Handler 测试覆盖。当前任务化增加真实隔离 PostgreSQL／Redis、HTTP／SSE、请求断开、迁移前后中断、撤权、来源篡改、替换回滚、上传目录／归档边界及桌面／390px 窄屏回归。浏览器使用受控回环 API，验证页面刷新后找回同一任务、仅一次创建／上传、实时完成、无轮询、关闭连接与状态保留；这与真实服务器安装验收分别记录。

发布前另从 HEAD 与暂存 blob 导出待提交源码复核，排除工作区的通知、IAM、客户和品牌等改动。核对新增迁移只改变 `core` 载荷校验和，并重跑隔离后端、前端和浏览器验收。三个旧 Bootstrap 测试会修改 Gin 全局模式，改为不与其他测试并行，不修改业务断言。仓库 `make unit-test` 仍引用缺失的 `backend/test-silent.sh`，不能据此宣称整仓测试通过；相关包使用下列明确命令验收。

页面证据：[桌面](evidence/plugin-install-tasks-20261112/desktop.png)、[390px 窄屏](evidence/plugin-install-tasks-20261112/mobile.png)，均为合成数据的回环 fixture。

可复现检查（数据库名称必须以 `powerx_install_test_` 开头；Redis 应是专用隔离实例，不可指向日常或服务器资源）：

```sh
POWERX_INSTALL_TEST_DSN='<隔离 PostgreSQL DSN>' POWERX_INSTALL_TEST_REDIS='127.0.0.1:<专用端口>' GOWORK=off go test -C backend -race ./internal/service/plugin_install ./internal/transport/http/admin/plugin ./internal/infra/plugin/manager ./internal/infra/storage_budget
# web-admin：构建与日常 Vite 产物隔离；无需额外 fixture 页面。
POWERX_NUXT_BUILD_DIR=../tmp/plugin-install-tasks-20261112/nuxt-product-build POWERX_NITRO_OUTPUT_DIR=../tmp/plugin-install-tasks-20261112/nuxt-product-output npm run build
npm exec -- playwright test --config playwright.plugin-install-tasks.config.ts
```

本地验证记录见 [任务化验收回执](evidence/plugin-install-tasks-20261112.json)。浏览器 fixture 固定占用 127.0.0.1:39581／39582，不复用已运行服务，记录 PID 和命令并关闭自己启动的进程。实际部署仍待用户操作；源码、隔离测试和构建不等于 200 服务器验收。

### 升级与服务器验收

1. 按现有发布流程备份数据库／运行配置并准备新 Core 前后端；保留主密钥、环境身份、插件数据和安装目录。该补丁只新增结构，不自动补写历史授权／映射。
2. 在新源码仓库根目录明确开发环境配置执行 `make migrate CONFIG=/etc/powerx-dev/config.yaml`；不要执行全量 seed、reset 或未经审核的 rollback。迁移包含仓库当前其它模块，发布者应按完整版本范围审阅。
3. 通过现有 systemd 发布切换新 Core／管理端，确认常驻 worker 已启用、TaskBus PostgreSQL／Redis 及任务结构就绪；不要同时启动第二个 Core 执行器。Agent 不擅自重启用户服务。
4. Root 在开发站按正常流程安装**准备升级的包**。记录 task UUID，看到任务进度后关闭弹窗，再打开或刷新；应找回同一任务，无重复安装／迁移，最后成功完成所需阶段。
5. 核对包内与回执的版本／修订、进程健康、当前租户就绪，以及该包实际声明的事件映射和 SDK 权限；业务只读验证与插件真实队列验证由相应现场验收继续。不为验收卸载已正常运行的插件或重跑业务。

排障：`PLUGIN_INSTALL_SCHEMA_NOT_READY` 检查本环境迁移；`PLUGIN_INSTALL_WORKER_DISABLED` 检查运行模式与 worker 生命周期；`PLUGIN_INSTALL_ACTOR_REVOKED` 检查任务发起 Root／租户仍有效；`PLUGIN_INSTALL_INTERRUPTED_REVIEW_REQUIRED` 先核对迁移和登记，不重传；启动／租户收尾失败走绑定修订的正式核对与恢复。SSE 连接失败检查 `/installations/{task_id}/events` 代理流、Header 会话和 Redis，不能改成定时状态轮询。

回滚：先通过既有维护流程停止接纳新安装并确认无在途任务，再切回旧应用产物；保留新增任务表、暂存来源、安装备份和现有业务数据，不自动反向迁移或删除安装目录。旧 UI 恢复为同步请求合同，不能据此宣称历史任务已经完成；需要核对的任务必须由管理员单独处理。已经执行的插件数据库迁移不由宿主文件回滚代替。

代码入口：`service/plugin_install`、`bootstrap/plugin_installations.go`、`model/plugin_install`、`repository/plugin_install`、`database/migration.go`、`manager/install*.go`、`admin/plugin/installation*.go`、`InstallDialog.vue`、`PluginInstallTaskProgress.vue`、`PluginInstallRecovery.vue`、`server/middleware/01-proxy-api.ts`。
