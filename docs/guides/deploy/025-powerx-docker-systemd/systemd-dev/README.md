# systemd Dev 环境部署索引（025）

## 1. 目标
在同一台 Linux 机器上新增一套 PowerX dev 环境，与已有生产 systemd 环境并行运行。

## 2. 推荐端口与路径
- 生产前端：`3000`
- 生产后端：`8080`
- Dev 前端：`3001`
- Dev 后端：`8081`
- 生产运行根：`/opt/powerx`
- Dev 运行根：`/opt/powerx-dev`
- 生产运行配置：`/etc/powerx/config.yaml`
- Dev 运行配置：`/etc/powerx-dev/config.yaml`
- 生产 env：`/etc/powerx/powerx.env`
- Dev env：`/etc/powerx-dev/powerx.env`

## 3. 执行顺序
1. `00-nginx-dev-config.md`
2. `01-deploy-dev-config-start.md`
3. `02-verify-and-troubleshoot.md`

## 4. 关键约束
- Dev 不复用 `/opt/powerx`。
- Dev 不复用 `/etc/powerx/config.yaml`。
- Dev systemd service 使用独立名称，例如 `powerx-dev-backend.service`。
- Dev 数据库必须使用独立 database 或 schema。
- Dev 插件目录、storage 目录、gateway public base 必须和生产隔离。
- Dev unit 默认使用 `powerx` 作为 systemd service 用户；如果机器尚未部署过生产 PowerX，先创建 `powerx` 用户再执行 ACL。
- 安装 unit 与切换 release 时显式设置 `POWERX_SERVICE_USER=powerx` / `POWERX_SERVICE_GROUP=powerx`，并使用 `sudo -E`，避免底层切换脚本在 sudo 场景回退到 `SUDO_USER`。
- 日常 dev release 切换优先使用 `backend/scripts/ops/switch-develop-systemd.sh`。
- `backend/scripts/ops/switch-release-systemd.sh` 默认操作生产 root 与生产 service。dev 直接调用底层脚本时必须覆盖 root、service name、health url，并设置 `POWERX_SYNC_SYSTEMD_UNITS=0`。
- Dev 目录权限使用 ACL：`ubuntu` 可写，`powerx` 可读执行；`storage/plugins` 额外给 `powerx` 写权限。

## 5. 相关生产文档
- `../systemd/README.md`
- `../systemd/01-deploy-config-start.md`
- `../systemd/02-verify-and-rollback.md`
