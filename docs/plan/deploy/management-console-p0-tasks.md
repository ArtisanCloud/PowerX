# 运维管理控制台 P0 实施任务清单

> 范围：`部署发布中心`、`插件生命周期中心`、`备份与恢复中心`  
> 目标：可直接进入研发排期与实现，不再做二次方案讨论。

## 1. 里程碑与交付物

## M1：后端能力可用（API + 任务执行 + 审计）

- 交付 admin API
- 交付备份任务执行器（可手动触发 + 定时触发）
- 交付统一审计事件

## M2：前端页面可用（列表 + 详情 + 操作）

- 交付 3 个 P0 页面
- 交付权限控制与操作确认
- 交付错误反馈与任务状态展示

## M3：联调与验收

- 通过 E2E 用例
- 通过回滚/恢复演练
- 输出上线验收报告

## 2. 后端任务拆解

## 2.1 部署发布中心 API

- [ ] `GET /api/v1/admin/deploy/releases`：发布历史列表（分页）
- [ ] `POST /api/v1/admin/deploy/releases`：触发发布（记录 release_id）
- [ ] `POST /api/v1/admin/deploy/rollback`：回滚到目标版本
- [ ] `GET /api/v1/admin/deploy/health`：聚合健康状态（backend/web-admin/nginx）
- [ ] 审计事件：`deploy.release.created`、`deploy.release.rollback`

数据结构建议：

- `deploy_release_records`
  - `id`, `environment`, `backend_version`, `web_admin_version`
  - `status`, `operator`, `started_at`, `ended_at`, `error_message`

## 2.2 插件生命周期中心 API（复用+增强）

- [ ] 复用现有：
  - `GET /api/v1/admin/plugins`
  - `GET /api/v1/admin/plugins/:id/status`
  - `POST /api/v1/admin/plugins/install/local`
  - `POST /api/v1/admin/plugins/:id/switch_version`
  - `POST /api/v1/admin/plugins/:id/uninstall`
- [ ] 新增 `GET /api/v1/admin/plugins/:id/audit`：插件生命周期审计轨迹
- [ ] 新增升级门禁结果字段：`gate_result`、`gate_reason`

数据结构建议：

- `plugin_lifecycle_audits`
  - `id`, `plugin_id`, `from_version`, `to_version`, `action`
  - `result`, `operator`, `created_at`, `detail_json`

## 2.3 备份与恢复中心 API

- [ ] `GET /api/v1/admin/backup/policies`
- [ ] `POST /api/v1/admin/backup/policies`
- [ ] `POST /api/v1/admin/backup/jobs/run`
- [ ] `GET /api/v1/admin/backup/jobs`
- [ ] `POST /api/v1/admin/backup/cleanup`
- [ ] `POST /api/v1/admin/backup/restore-drills/run`
- [ ] 统一状态机：`pending/running/success/failed`

数据结构建议：

- `backup_policies`
  - `id`, `name`, `backup_type`, `schedule`, `retention_days`, `enabled`
- `backup_jobs`
  - `id`, `policy_id`, `status`, `started_at`, `ended_at`, `error_message`
- `backup_artifacts`
  - `id`, `job_id`, `storage_uri`, `size_bytes`, `checksum`
- `restore_drills`
  - `id`, `job_id`, `status`, `report_uri`, `created_at`

## 2.4 执行器与调度

- [ ] 对接 `scripts/ops/backup-db.sh`、`cleanup-backups.sh`、`restore-drill.sh`
- [ ] Docker 场景：提供 `backup` worker 入口
- [ ] 非 Docker 场景：提供 `systemd timer` 触发与状态回写
- [ ] 失败重试策略：最多 3 次，指数退避

## 2.5 权限与安全

- [ ] 增加权限点：
  - `deploy.release.read/write`
  - `plugin.lifecycle.read/write`
  - `backup.read/write`
- [ ] 操作 API 必须写审计日志（operator + trace_id）
- [ ] 敏感信息脱敏（密钥、连接串）

## 3. 前端任务拆解（web-admin）

## 3.1 导航与路由

- [ ] 新增一级菜单：`运维中心`
- [ ] 新增子路由：
  - `/ops/deploy`
  - `/ops/plugins`
  - `/ops/backup`

## 3.2 部署发布中心页面

- [ ] 顶部状态卡片：当前版本、健康状态、最近发布结果
- [ ] 发布记录表格：支持按环境/状态筛选
- [ ] 回滚弹窗：目标版本选择 + 二次确认
- [ ] 失败详情抽屉：展示错误信息与 trace_id

## 3.3 插件生命周期中心页面

- [ ] 插件列表：当前版本、运行状态、已安装版本数
- [ ] 操作区：安装、切换版本、回滚、卸载
- [ ] 审计时间线：展示最近 N 次操作记录
- [ ] 升级门禁提示：阻断原因可视化

## 3.4 备份与恢复中心页面

- [ ] 策略页签：创建/编辑策略
- [ ] 任务页签：任务列表 + 手动触发 + 失败重试
- [ ] 演练页签：恢复演练历史 + 报告链接
- [ ] 清理动作：按 retention 执行并显示结果

## 3.5 交互规范

- [ ] 所有写操作统一二次确认
- [ ] 操作按钮幂等防抖
- [ ] 成功/失败 toast + 详情跳转
- [ ] API 错误按可读文案映射

## 4. 联调与测试任务

## 4.1 后端测试

- [ ] API contract tests（参数、权限、状态码）
- [ ] 任务执行器 tests（成功/失败/重试）
- [ ] 审计记录 tests（字段完整性）

## 4.2 前端测试

- [ ] 组件单测：列表渲染、弹窗交互、状态映射
- [ ] E2E 用例：
  - 发布后回滚
  - 插件切换后回滚
  - 备份触发后查询状态

## 4.3 集成验收场景

- [ ] 场景 1：执行一次应用发布并成功回滚
- [ ] 场景 2：安装插件新版本并切换，异常后回退
- [ ] 场景 3：触发备份、执行清理、执行恢复演练

## 5. 上线标准（P0 Exit Criteria）

- [ ] 3 个页面均可完成核心操作闭环
- [ ] 核心操作均有审计记录
- [ ] 至少 1 次备份恢复演练成功并可追踪
- [ ] 权限隔离有效（admin/system_admin）
- [ ] 发布后 7 天无 P0/P1 严重故障

## 6. 建议排期（两周迭代）

第 1 周：

- 后端 API + 数据表 + 执行器 + 审计
- 前端页面骨架 + 列表展示 + 基础操作

第 2 周：

- 前端完整交互 + 联调
- E2E + 演练 + 缺陷修复
- 上线验收与文档归档

