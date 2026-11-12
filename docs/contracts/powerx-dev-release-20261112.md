# PowerX 开发版提交与升级检查（2026-11-12）

本批汇总 PowerX 当前工作区的客户、IAM、通知、插件启动、运维、品牌及构建调度改动。AI Craft 是独立仓库，其图片状态与删除策略补丁不在本批中。前一批 SSE 内容协商与断连释放已通过 PR #429 合入 develop。

## 改动范围

| 模块 | 本次行为 |
|---|---|
| 插件启动权限 | 每个插件只按明确选定的有效版本同步权限，避免历史版本反复覆盖当前声明；包缺失或多版本未选定时明确告警。 |
| 客户核验 | 新增最小只读服务能力，按当前租户、实时授权核验客户、成员关系和主联系人；不授予客户登录或交易权限。 |
| IAM 组织关系 | 类型化读取成员角色、部门及显式上级；管理员维护直属上级包含版本检查、循环防护与事务审计。 |
| 通知 | 按租户、实际服务 owner、幂等键保存原创建凭据；只读查回原结果，重试不重复创建或复活通知。 |
| 开发运维 | 独立 web-admin 启动脚本解析端口；可选 Runner 缺失时跳过；存储配置使用显式预览／应用流程，仅开启观测。 |
| 管理前端 | 更新品牌图形、图标和版本位置，补齐移动端侧栏开关及窄屏收起布局。 |
| 构建发布 | 固定源码 SHA、请求去重、verify/build/release 模式、可核对回执及不可覆盖的发布资产；双架构升级仍需真实 CI 验收。 |

## 开发环境升级

这批 Core 改动需要数据库迁移，区别于无需新增迁移的 AI Craft 删除补丁。新增结构为成员的 `direct_manager_member_uuid` 及通知创建凭据表 `system_notification_creation_receipts`；升级载荷只更新 `core` 和 `catalog` 校验和。

先核对目标为开发实例及其已有备份，按原流程编译产物，然后从 PowerX 仓库根目录用目标配置执行：

```bash
make migrate CONFIG=/etc/powerx-dev/config.yaml
make capability-seed CAPABILITY_SEED_CONFIG=/etc/powerx-dev/config.yaml CAPABILITY_SEED_FLAGS='-capability-ids com.corex.customer.memberships.service_read,com.corex.iam.organization.service_read,com.corex.iam.member_accounts.admin_write,com.corex.notifications.create,com.corex.notifications.creation_receipts.read'
```

能力同步默认保留安装状态，只更新选定目录与正式权限元数据。服务读取／创建能力仍需给实际插件或 Host 身份明确授予最小权限，再核对 grant-status；不能把 Admin 权限当服务授权。随后使用现有 release 切换流程更新 Core 后端和管理前端并重启开发实例。

此次提交不自动应用开发存储配置，也不自动开启清理、归档或旧脚本接管。存储观测脚本发现已有配置时要求先核对，不覆盖历史策略。

## 验收顺序

1. 检查开发站健康接口、Core 版本及管理前端均对应新产物。
2. 检查插件 current 版本与磁盘包一致，无启动权限同步失败／未选版本告警；按现有用户权限访问插件新接口。
3. 通过实际服务身份核对客户、组织和通知所需能力；撤权后下一次调用应拒绝，Admin 写能力不能因只读服务授权而可用。
4. 验证通知同键重放返回原通知、冲突明确拒绝、凭据只读查回不发送消息；持久化成功不等于真实浏览器已收到。
5. 在桌面及窄屏检查侧栏、主题、菜单、折叠、版本与图标；检查开发启动脚本无 awk 引号错误，可选 Runner 缺失不会反复重启。
6. PowerX 验收通过后，再单独升级 AI Craft 新包，验证图片状态与缺素材删单。这批 Core PR 本身不能证明插件补丁已部署。

## 已验证与边界

完整记录见[本批验证回执](evidence/powerx-dev-release-validation-20261112.json)。相关 14 个 Go 包、临时 PostgreSQL 客户／组织／通知测试、隔离 capability seed 和 fixture grant-status、141 项前端测试、61 项 Python 测试、vet、Linux amd64 编译及 Nuxt 构建通过。桌面 1440px、390px／320px 窄屏与明暗主题的导航检查使用合成只读 API；临时进程及数据库已清理。

整仓 `make unit-test` 仍引用缺失的 `backend/test-silent.sh`，本轮以相关包直接测试复验，没有把该入口标为通过。尚未执行真实 GitHub Actions dispatch、双架构容器升级或目标服务器部署。推送当前功能分支不触发 develop/main/master 的容器 push 发布入口；合入这些分支会沿用该入口。

回退源码前核对在途安装和通知任务，并保留数据库、已维护组织关系及通知幂等凭据。代码回退不代表数据回滚，不能删除新增表来替代恢复方案。
