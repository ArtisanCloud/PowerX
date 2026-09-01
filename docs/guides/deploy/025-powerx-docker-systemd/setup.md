# PowerX 首次安装引导（/setup）

本文档描述 `PowerX Admin` 首次安装向导（`/setup`）的实际使用流程，适用于 Docker 与 systemd 两种部署方式。

## 1. 进入向导前提

1. 后端与 web-admin 已启动。
2. 访问入口（示例）：
   - `http://127.0.0.1:3000/setup`（本地生产端口示例）
   - `http://<your-domain>/setup`（反向代理后）
3. 系统处于未安装状态（`install.status=uninstalled|configuring`）时，会优先进入安装向导。

## 2. 向导步骤总览

目标页面仍为 6 步，但“部署环境”必须在首次安装开始阶段明确选择：

1. `系统检查 & 许可`
2. `域名与 HTTPS`
3. `数据库 & 基础配置`
4. `超级管理员 & 租户初始化`
5. `插件与智能体安装`
6. `LLM 模型配置`

说明：第 4、5 步当前主要是引导信息与前端表单，后端持久化以步骤 2/3/6 提交的数据为主。

> 实现状态：setup 页面与接口已支持 `deployment.env`。首次安装必须在页面明确选择 `dev/test/staging/prod`；后端会严格校验、写入实际 `config.yaml`，并在再次读取 setup 配置时回显。已安装实例不能通过普通 setup 写接口修改该字段。

## 3. 各步骤填写建议（本地生产验证）

## 3.1 系统检查 & 许可

1. 勾选许可协议、服务条款。
2. 选择“部署环境”：开发 `dev`、测试 `test`、预发布 `staging`、生产 `prod`。本地页面可以预选 `dev`，但必须由用户确认。
3. 确认检查项可访问（数据库/缓存/AI 检测状态会显示）。

后端必须把选择作为结构化字段保存：

```json
{
  "deployment": {
    "env": "prod"
  }
}
```

字段缺失或不是允许枚举时，保存与完成安装都必须失败并显示可操作的校验错误，不得根据端口、域名或 `POWERX_ENV` 推导。

## 3.2 域名与 HTTPS

1. `系统域名`：生产/本地生产验证建议填 `127.0.0.1:3000`（或你的正式域名，不带协议）；仅开发口径通常是 `127.0.0.1:3030`。
2. `API 子域名`：可留空。
3. `后端端口`：建议 `8080`（或你的生产端口）。
4. `管理端端口`：建议 `3000`（或你的生产端口）。
5. `HTTPS`：
   - 本地调试可选 `disable`。
   - 生产按证书模式填写 `cert_email` 或手动证书内容。

## 3.3 数据库 & 基础配置

1. `缓存`：若使用 Redis，需填写 `redis_host/redis_port`。
2. `对象存储`：
   - `local`：填本地目录。
   - `s3/minio/oss/cos`：需填写 `access_key/secret_key/bucket`（及 endpoint/region）。
3. `邮件`：可选；开启时需填 `smtp_host/smtp_port`。

说明：页面里展示的数据库字段目前不作为后端 `/setup` 保存入参；数据库连接仍由 `config.yaml` / 环境变量（如 `DATABASE_DSN`）决定。

## 3.4 超级管理员 & 租户初始化

1. 按页面填写管理员与租户信息用于安装流程确认。
2. 当前后端 `setup` 接口不直接落库这些字段，实际账号初始化仍以后端安装流程与后续用户管理为准。

## 3.5 插件与智能体安装

1. 按需勾选默认插件项。
2. 当前主要是引导展示，不会直接触发完整插件安装流水线。

## 3.6 LLM 模型配置

1. 可跳过（`跳过 LLM 配置`）。
2. 若配置，至少填写 `provider / model / apiKey`。
3. 完成后会调用连接测试逻辑并进入完成安装。

## 4. 提交后发生了什么

安装向导核心接口：

1. `GET /api/v1/admin/setup/status`：安装态、检查项、版本号、`desired_ports/effective_ports/restart_required/config_source`。
2. `GET /api/v1/admin/setup/config`：读取向导草稿。
3. `PUT /api/v1/admin/setup/config`：保存向导草稿（deployment/domain/https/storage/cache/email/ports）。
4. `POST /api/v1/admin/setup/complete`：完成安装，写回运行配置状态。

完成后通常会跳转 `/home`。

完成安装时，最终 `/etc/powerx/config.yaml` 必须包含：

```yaml
deployment:
  env: prod
```

已安装实例继续受 setup 重入保护，不能通过普通 setup 写接口修改部署身份。首次插件安装后如需变更，只能走显式数据库迁移/repair 流程。

## 5. 常见问题

1. 访问 `/setup` 报 503 或被拦截：
   - 确认后端可访问，且当前实例允许安装态接口（`install.status` 不是 `installed` 时会放行 setup 相关接口）。
2. 点击完成后失败：
   - 看后端日志中 `setup/complete` 错误信息。
   - 检查缓存、对象存储、邮件字段是否满足校验规则。
3. 页面端口与实际监听不一致：
   - 以环境变量优先（如 `POWERX_BACKEND_PORT`、`POWERX_WEB_ADMIN_PORT`）。
   - 当 `setup/status.restart_required=true` 时，表示“目标值已变更但当前进程未生效”，必须重启 backend/web-admin 后再验证。
4. 插件安装提示部署环境缺失或不一致：
   - 检查实际启动日志中的配置路径，不要只修改 release 目录内的示例文件。
   - 核对 `/etc/powerx/config.yaml` 的 `deployment.env` 与插件 Registry 记录。
   - 已存在无环境段的 `px_*` / `pxu_*` 对象时，使用独立 repair 工具 dry-run；不得通过修改安装请求或目录名绕过。
