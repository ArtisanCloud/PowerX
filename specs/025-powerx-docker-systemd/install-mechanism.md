# Install Mechanism: Welcome Install（YAML 首真相）

## 1. 目标

在“数据库尚未可用”的场景下，PowerX 仍可正常启动，并通过 `/setup` 完成首次安装。
安装完成后再切换到正式运行态，避免“先要 DB 才能保存安装状态”的死锁。

## 2. 核心设计决策

- 安装状态首真相：`config.yaml.install.status`
- 数据库存储：仅作兼容与审计，不作为首判定来源
- 未安装访问策略：全局硬拦截
- 运行配置落点：`/etc/powerx`（`config.yaml` + `powerx.env`）

## 3. 配置模型

运行时配置文件（`/etc/powerx/config.yaml`）包含：

```yaml
install:
  status: uninstalled      # uninstalled | configuring | installed
  lock_mode: strict        # 当前仅 strict
  allow_without_db: true   # 首装允许无 DB 启动
```

说明：

- `status=uninstalled/configuring`：系统进入安装模式。
- `status=installed`：系统进入正常模式。

## 4. 启动与访问控制

### 4.1 启动阶段

1. 启动时先读取 `/etc/powerx/config.yaml` 中的 `install`。
2. 当 `status != installed`：
   - 允许服务正常启动。
   - 不依赖 DB 中的 setup 标记判断系统是否可用。

### 4.2 未安装硬拦截白名单

未安装模式仅允许：

- `/api/v1/admin/setup/*`
- `/api/v1/health`
- `/healthz`
- Web 静态资源与 setup 页面必要资源

其他 API 统一返回 `SYSTEM_NOT_INSTALLED`（建议 HTTP 503）。

## 5. Setup 两阶段闭环

### 阶段 A：采集与验证（不落业务态）

- `/setup` 页面填写域名、端口、DB、Redis、存储等。
- 后端仅做参数合法性与连通性校验。

### 阶段 B：完成安装（原子切换）

`POST /api/v1/admin/setup/complete` 执行：

1. 写入部署生效配置（`/etc/powerx/config.yaml`）。
2. 使用新配置执行 migrate/seed。
3. 成功后将 `install.status` 切换为 `installed`。
4. 可选写入 DB 键 `platform.setup.completed=true`（兼容旧逻辑/审计）。

失败处理：

- 任一步失败都保持非 installed。
- 返回结构化错误，允许修正后重试。

## 6. 前端行为约束

- 路由守卫基于 `/api/v1/admin/setup/status`：
  - 未安装：强制跳转 `/setup`
  - 已安装：访问 `/setup` 跳回 `/home`
- 已安装后禁止 setup 重入写操作：
  - `PUT /api/v1/admin/setup/config`
  - `POST /api/v1/admin/setup/provision`
  - `POST /api/v1/admin/setup/complete`
  - 默认返回 409（可通过运维开关 `POWERX_ALLOW_SETUP_REENTRY` 临时放开）
- AI onboarding 与 install onboarding 解耦：
  - 未安装时不得展示 AI 欢迎引导
  - 仅 installed 后允许显示 AI 引导

## 7. 兼容策略

- 兼容读取历史键：`platform.setup.completed`、`platform.installed`、`system.installed`。
- 但安装态首判定仍以 `config.install.status` 为准。

## 8. 验收标准

1. DB 不可用时仍能访问 `/setup` 与健康接口。
2. 未安装时业务 API 被统一拦截。
3. 完成 setup 后自动进入正常模式。
4. `make dist` 产物默认可进入安装模式并完成闭环。
5. 已安装环境误访问 `/setup` 不会触发重装或重跑 migrate/seed。

## 9. 端口真源与生效语义（US8）

端口矩阵：

- dev：`web-admin=3030`，`backend=8077`，`grpc=9001`
- prod：`web-admin=3000`，`backend=8080`，`grpc=9010`

优先级（高 -> 低）：

1. 进程环境变量：`POWERX_WEB_ADMIN_PORT` / `POWERX_BACKEND_PORT`
2. setup 目标配置（草稿/完成态写入）
3. 按 `POWERX_ENV` 的默认值

`GET /api/v1/admin/setup/status` 约定：

- `desired_ports`：目标值（配置期望）
- `effective_ports`：当前进程生效值
- `restart_required=true`：`desired != effective`，需重启服务后生效
- `config_source`：标识 desired/effective 的来源链路（便于排障）

## 10. 升级路径（已安装实例）

- 代码升级（无 DB 变更）：`make dist + switch-release`，不走 `/setup`。
- 结构升级（有 DB 变更）：发布后显式执行 `database migrate`。
- 数据初始化/补数：仅在需要时显式执行 `database seed`，不与常规发布绑定。
