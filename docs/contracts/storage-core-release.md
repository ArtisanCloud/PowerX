# 2026-11-10 存储核心范围发布合同

用户明确接受先发布已验收核心功能，未验收的 SSH／OSS 远端归档与现场接管保持关闭。本合同界定候选源码与发布范围，不宣称已部署或完成全部商用验收。

## 范围

保留当前磁盘／系统表观测、按资源安全保留规则、数据库非业务表期限、Root 诊断、原请求核对、紧急暂停，以及具备本地完整对象的导出／已验收备份路径。原有 Root、配置权威、正式 TaskBus、版本／预览／内容／引用、容量和恢复代次检查继续生效。既有文件日志、审计及 PostgreSQL SQL 日志配置不由发布门槛修改。

未开放远端 SSH／S3 目标探测、远端写入／重传、远端取回和清理，以及旧执行器开始／完成接管。历史记录仍能查看；已经在本地的完整对象无需远端就能导出，缺失对象不会绕过门槛取回。尚未完成的现场、Linux 原生、独立密钥／墓碑、完整恢复和 7／14 天证据独立保留。

## 部署门槛与策略权威

`storage_governance.advanced_operations_enabled` 为受信任部署配置，默认为 false。它仅表示部署是否允许进入高级验收范围，不能代替 UI 策略、目标验证、现场所有权或删除许可。未来开放必须另外完成相关验收并批准部署；本次保持 false。

Core 在载入既有归档配置后重新设置 `remote_execution_blocked=true` 并发布到既有运行配置。SSH／S3 固定网络入口及策略启用／新任务／调度均检查该门槛；独立归档 worker 同样读取该字段，不依赖只在内存传递的 Options。旧 KnownFields 解析器无法理解新增字段时停止加载，不允许忽略未知字段继续执行。

旧执行器交接在 Root Service 入口及正式 SQL 事务提交前检查阻塞；恢复后的旧发布消息不能完成接管。已有不可变 UUID 回执优先只读返回，不能把历史结果当作新的现场资格。当前数据库策略、凭据与验证历史不被自动改写。

## API 增量

不新增路由，不扩大授权：

- 归档 overview 新增 `remote_execution: {enabled: false, problem: "ARCHIVE_RELEASE_NOT_ACCEPTED"}`；关联远端策略的 execution_ready=false、execution_problem 同码。Root 请求远端操作返回 423，部分旧重试桥接保留既有错误形态，实际网络入口仍拒绝。
- execution-control 配置新增 `legacy_handoff_available: false`；begin_handoff／complete_handoff 返回 423 `STORAGE_HANDOFF_RELEASE_NOT_ACCEPTED`。紧急暂停、恢复保护和只读诊断不因该字段关闭。
- UI 缺少字段时保持旧版本兼容；新 Core 关闭时展示未就绪提示，并禁止交接提交。后端是最终门槛，不能依赖前端禁用按钮。
- 核心业务调用存储准入及配置未就绪／真实容量不足的错误区分保持原合同，不能为发布清零错误或返回假正常。

## 系统快照兼容修复

独立媒体／数据库采集入口此前直接调用受控 pg_dump，缺少 execution 与初始命令记录，采集失败。现在先使用既有执行注册，再调用同 MVCC 采集；移除临时组件目录前核对原命令排空。不新造目标探测、不绕过命令保护、不把锁可取得当作进程退出。

原生测试在 pg_dump 已开始、媒体已读取时继续提交业务 INSERT／UPDATE，恢复包仍还原 A 时刻数据库及媒体，源 B 保留；手动和定时都覆盖。此为独立包恢复，不替代整站现场维护恢复。

## 发布判定

本轮当前源代码通过 Linux 后端与 Nuxt 生产构建、8 个包完整 race 回归及另外 4 个包的存储／准入／schema 定向回归、374 项前端、原生升级／SQL／Root HTTP、权限／错误分类及真实桌面窄屏检查。另一个 repository 包没有测试文件，不计为有测试通过。过程失败保留并由明确纠正后的专项结果覆盖。

结论为**本地核心范围源码、隔离和用户重启后的只读运行验收通过；共同总结提交与正式部署尚未执行**。候选使用 uncommitted 标记；当前分支与大量既有改动必须明确入版范围，不能仅根据 HEAD 值生成正式 release。

[操作指南](../guides/deploy/027-monitor-center/usecase-us9-core-release.md)、[本轮证据](evidence/storage-core-release-validation-20261110.json)、[冻结任务](../../tmp/storage-release-20261110/PLAN.md)。SG-09／SG-11 总门槛仍未关闭。

## 本地重启与最新 SDK 复验

用户按原方式重启本地 Core 后，Root 观测／runtime／期限／执行控制／归档／系统表接口均返回 200。schema 和控制依赖 ready；当前预算、期限仍待 UI 接管，数据库自动清理保持未启用，不将 ADOPTION_REQUIRED 误报为已执行清理。

原专用身份通过官方 Framework v0.0.28 和最新正式 v0.0.29 的两项 GrantStatus 与 typed Lookup；结果均 granted／found=false，未出现先前配置型或容量型 503。v0.0.29 的构建信息确认无 replace。仅验证只读客户能力，不宣称所有 SDK 新功能、WorkspaceX 客户关联、订单或支付已验收，不修改已安装插件的 SDK 依赖和运行配置。

SDK 最新版本查询不是要求提前给 PowerX 定版本；按用户安排，在一起总结后上传、打版本。临时转发／远端探针已清理；正式站仍未部署，完整商用门槛仍未关闭。
