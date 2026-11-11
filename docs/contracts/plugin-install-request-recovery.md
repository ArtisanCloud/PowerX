# 插件安装请求取消与收尾恢复

日期：2026-11-11。范围是 Core 本地／URL 安装、显式启用及安装弹窗；不修改插件权限清单，不自动修复现场授权或租户映射。

## 状态和写入边界

安装产物、全局运行状态和当前租户配置是三个不同阶段。`registry.state=enabled` 不能单独证明当前租户已就绪。

- 产物提交前失败：继续清理首次安装半成品；替换失败继续恢复原版本和登记。
- 首次安装产物、迁移及登记已提交后激活失败：保留已提交产物，返回 `PLUGIN_INSTALL_FINALIZATION_REQUIRED`，供管理员显式继续启用，避免再次上传及迁移。替换安装仍采用原有回滚语义。
- 启用过程中请求取消：停止本次新启动的后端／管理前端，卸载本次路由，不继续登记 `enabled`。开始前已经健康启用的运行实例不因另一个取消请求被停止。
- 进程型管理前端探针失败：只有存在实际 `index.html` 的目录可以降级为静态管理页；SSR 的 public 资源目录不能充当可用管理页。请求取消直接失败，不通过静态降级伪报成功。
- 全局启动成功、当前租户启用或 Topic 注册失败：返回明确的待完成状态，保留已提交资源及现有 ACL。普通 GET、重启和查询不补授权。

错误 details 只包含 `code`、插件 ID、已安装版本／修订／全局状态、失败阶段及 `tenant_ready=false`，不包含 HostConfig、凭据或原始内部错误。阶段包括 `runtime_activation`、`tenant_activation`、`topic_registration`。

## 正式恢复入口

沿用 Root 管理入口 `POST /api/v1/admin/plugins/{id}/enable`。恢复请求携带 `expected_version` 和 `expected_revision`；Handler 预检后，Manager 在现有安装变更锁内再次核对版本，版本变化明确拒绝。已有客户端空 body 启用合同保持。

运行实例已经启用时，`already_enabled` 不再阻断当前租户收尾；其他运行错误仍正常拒绝。显式恢复继续调用正式 TenantPluginInstanceService / Event Fabric 准备事务，按当前声明及归属核对并保存映射，不绕过能力授权或恢复被撤销的 ACL。

现有 `GET /api/v1/admin/plugins/{id}/tenant_config` 增补 `installation`：版本、修订、全局状态、`runtime_ready`、`tenant_ready`、`ready` 和原因码。租户就绪通过已持久化配置及 `pluginbinding.Prepare(provision=false)` 核对实时 ACL；读取不创建主题、授权、凭据或映射。运行就绪同时核对后端与进程型管理前端。

## 页面行为

本地目录先只读取最外层清单的字面量 ID 和版本；清单身份只是查询目标，正式安装仍由 Core 验包。已知目标在提交前读取当前修订，避免将同版本旧安装误认成本次替换成功。

请求超时、网络结果未知或服务返回待完成错误时，弹窗保留文件与表单，切换到结果核对视图。读取一次状态后提供手动核对、显式继续启用及已安装页面入口，不周期查询。暂未发现记录不证明原请求已经结束，因此不提供盲目重传按钮。

仅在版本、修订及请求所需阶段都得到确认后显示完成。恢复操作只调用启用接口，不再上传原包。登录身份／租户变化后禁止继续原操作；关闭弹窗或离开组件释放旧的核对请求。

## 验证与部署

回归覆盖请求取消后的进程退出与监听端口释放、已提交产物保留、健康旧实例保护、修订变化拒绝、已启用实例继续收尾、错误回包凭据裁剪、缺映射只读检查及撤权保持。前端验证表单／文件保留、无自动重传、显式恢复和身份切换；浏览器使用受控 API 检查桌面及窄屏实际布局。

源码及隔离验证不等于 200 服务器验收。更新 Core 后应通过正式管理流程恢复现场已有版本，验收当前租户 13 项映射、真实插件身份调用和管理页面；本补丁不在迁移或启动时自动补写现场数据。

此补丁保持同步安装合同，不引入分离于请求的后台安装或私有队列。上传分阶段、正式 TaskBus 安装任务、WebSocket／SSE 进度及崩溃恢复是独立后续范围，不能把当前有界收尾恢复宣称为完整异步安装任务化。

代码入口：`manager/install.go`、`manager/lifecycle.go`、`service/plugin/installation_readiness.go`、`admin/plugin/install_handler.go`、`admin/plugin/plugin_handler.go`、`admin/plugin/tenant_handler.go`、`InstallDialog.vue`、`PluginInstallRecovery.vue`。
