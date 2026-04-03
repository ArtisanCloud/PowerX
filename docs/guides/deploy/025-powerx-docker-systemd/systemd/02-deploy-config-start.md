# 02. 安装 systemd、配置、启动

## 1. 目标
将 PowerX 以 systemd 托管方式启动，并设置开机自启：
- 基础双服务：`powerx-backend`、`powerx-web-admin`
- 可选：`powerx-runner`（仅当发布包包含 runner 时启用）

本地若仅做发布包自检（不启 systemd），请先按 `01-prepare-artifacts.md` 的“6.2 直接从 dist 制品启动”执行。

## 2. 安装 service 文件

先约定发布版本变量（示例）：

```bash
export VERSION=v2.0.1
```

从 dist 产物安装 service：

```bash
sudo cp dist/systemd/${VERSION}/systemd/*.service /etc/systemd/system/
sudo systemctl daemon-reload
```

预期结果：`systemctl list-unit-files | rg 'powerx-(backend|web-admin|runner)'` 可见 unit 文件。
失败处理：检查文件权限与语法。

## 3. 配置环境变量文件

service 文件默认读取：`/etc/powerx/powerx.env`。  
建议从 dist 产物模板复制：

```bash
sudo mkdir -p /etc/powerx
sudo cp dist/systemd/${VERSION}/config/powerx.env /etc/powerx/powerx.env
```

预期结果：`/etc/powerx/powerx.env` 存在且可读，且已按环境改成真实值。
失败处理：检查 DB/Redis 地址是否可达。

部署前务必先完成必须配置项核对：`../00-required-config.md`。

补充：
- `dist/systemd/${VERSION}/config/web-admin.env` 提供 web-admin 专项变量模板。
- 当前默认 service 统一读取 `/etc/powerx/powerx.env`；若你希望 web-admin 独立变量文件，可在 `powerx-web-admin.service` 增加或替换 `EnvironmentFile`。
- 兼容说明：`dist/.../config/*.env.example` 仍保留，供历史流程兼容。

端口策略说明：
- `POWERX_ENV=prod` 默认推荐 `web-admin=3000`、`backend=8080`
- `POWERX_ENV=dev` 默认口径为 `web-admin=3030`、`backend=8077`
- gRPC 端口通过 `POWERX_GRPC_PORT` 控制（prod 默认 `9010`，dev 默认 `9001`）
- 实际监听端口以 `/etc/powerx/powerx.env` 中 `POWERX_WEB_ADMIN_PORT`、`POWERX_BACKEND_PORT`、`POWERX_GRPC_PORT` 为准（有值即覆盖默认）

## 4. 校验 service 路径与制品一致
基础服务必查两项：
- `powerx-backend.service` 的 `ExecStart=/opt/powerx/backend/powerx`
- `powerx-web-admin.service` 的 `ExecStart=/usr/bin/node /opt/powerx/web-admin/.output/server/index.mjs`

可选（启用 runner 时）：
- `powerx-runner.service` 的 `ExecStart=/usr/bin/node /opt/powerx/runner/dist/main.js`

若你的目录不同，先改 service 文件再 reload。

## 5. 启动并设置自启

先启动基础双服务（backend + web-admin）：

```bash
sudo systemctl enable powerx-backend powerx-web-admin
sudo systemctl restart powerx-backend powerx-web-admin
sudo systemctl status powerx-backend powerx-web-admin --no-pager
```

预期结果：`powerx-backend`、`powerx-web-admin` 均为 `active (running)`。
失败处理：用 `journalctl` 看最近 200 行错误日志。

若你的发布包包含 `runner`，再启用 runner：

```bash
sudo systemctl enable powerx-runner
sudo systemctl restart powerx-runner
sudo systemctl status powerx-runner --no-pager
```

## 6. 首次安装引导配置页（推荐）

启动后访问 `http://<host>:<port>/setup` 完成首次安装引导。  
完整步骤、字段含义、接口说明与排障，请统一参考：`../setup.md`。

## 7. 引导页排障

排障条目已收敛到 `../setup.md`，本节仅保留索引，避免重复维护。
