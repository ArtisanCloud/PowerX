# 02. 安装 systemd、配置、启动

## 1. 目标
将 PowerX 以 systemd 托管方式启动，并设置开机自启：
- 基础双服务：`powerx-backend`、`powerx-web-admin`
- 可选：`powerx-runner`（仅当发布包包含 runner 时启用）

本地若仅做发布包自检（不启 systemd），请先按 `01-prepare-artifacts.md` 的“6.2 直接从 dist 制品启动”执行。

## 1.1 分支与 Tag 策略（建议）
- `develop`：仅用于预发布验证环境（staging/UAT），允许快速迭代。
- 生产发布：使用不可变 Git tag（例如 `v2.0.2`），不要直接用分支名作为生产版本。
- 推荐流程：
  - 先在 `develop` 构建并验证；
  - 验证通过后从对应提交打 tag；
  - 从该 tag 重新构建生产包并发布；
  - 通过软链切换（`/opt/powerx/{backend,web-admin,runner}`）完成上线与回滚。

## 1.2 从 Git 拉代码并切换到 tag（推荐）
示例（服务器本地构建）：

```bash
git clone https://github.com/ArtisanCloud/PowerX.git
cd PowerX
git fetch --tags --prune
git checkout v2.0.2
```

说明：
- `git checkout <tag>` 会进入 detached HEAD，这是预期行为。
- 若需要临时调试，可基于 tag 建分支：`git checkout -b hotfix/v2.0.2-fix v2.0.2`。

## 2. 从 GitHub 下载发布包（可选）
若你不在服务器本地构建，可直接下载 CI/Release 产物。示例：

```bash
export VERSION=v2.0.2
export REPO=ArtisanCloud/PowerX
export PKG=powerx-systemd-${VERSION}.tar.gz

# 需先在 GitHub Release 中上传该文件名的打包产物
curl -fL -o "${PKG}" "https://github.com/${REPO}/releases/download/${VERSION}/${PKG}"

sudo mkdir -p /opt/powerx/releases/${VERSION}
sudo tar -xzf "${PKG}" -C /opt/powerx/releases/${VERSION}
```

预期结果：`/opt/powerx/releases/${VERSION}` 下包含 `backend/`、`web-admin/`、`systemd/` 等目录。

## 3. 安装运行时依赖（目标机）
最小依赖：
- `systemd`（服务托管）
- `bash`、`curl`（运维脚本/健康检查）
- `node`（web-admin/runner 运行时）

Ubuntu/Debian 示例：

```bash
sudo apt-get update
sudo apt-get install -y systemd curl ca-certificates bash

# Node.js 20（若系统未安装）
curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
sudo apt-get install -y nodejs
node -v
```

CentOS/RHEL 示例（按企业源调整）：

```bash
sudo yum install -y systemd curl ca-certificates bash
# Node.js 20 建议使用企业镜像或 NodeSource/RPM 源安装
```

## 4. 安装 service 文件
先约定发布版本变量（示例）：

```bash
export VERSION=v2.0.2
```

从 dist 产物安装 service：

```bash
sudo cp dist/systemd/${VERSION}/systemd/*.service /etc/systemd/system/
sudo systemctl daemon-reload
```

预期结果：`systemctl list-unit-files | rg 'powerx-(backend|web-admin|runner)'` 可见 unit 文件。
失败处理：检查文件权限与语法。

## 5. 配置环境变量文件（可选）
`make dist` 不产出 `.env`；运行期如需覆盖配置，可使用 `/etc/powerx/powerx.env`。
若不需要环境覆盖，可跳过本节，仅使用 `config.yaml`。

示例（手工创建）：

```bash
sudo mkdir -p /etc/powerx
sudo tee /etc/powerx/powerx.env >/dev/null <<'EOF_ENV'
POWERX_BACKEND_PORT=8080
POWERX_WEB_ADMIN_PORT=3000
EOF_ENV
```

预期结果：`/etc/powerx/powerx.env` 存在且可读，且已按环境改成真实值。
失败处理：检查 DB/Redis 地址是否可达。

部署前务必先完成必须配置项核对：`../00-required-config.md`。

补充：
- 当前默认 service 统一读取 `/etc/powerx/powerx.env`；若你希望 web-admin 独立变量文件，可在 `powerx-web-admin.service` 增加或替换 `EnvironmentFile`。
- 建议仅保留少量“实例级”覆盖项到 env 文件（端口、上游地址等），业务配置放在 `config.yaml`。

端口策略说明：
- `POWERX_ENV=prod` 默认推荐 `web-admin=3000`、`backend=8080`
- `POWERX_ENV=dev` 默认口径为 `web-admin=3030`、`backend=8077`
- gRPC 端口通过 `POWERX_GRPC_PORT` 控制（prod 默认 `9010`，dev 默认 `9001`）
- 实际监听端口以 `/etc/powerx/powerx.env` 中 `POWERX_WEB_ADMIN_PORT`、`POWERX_BACKEND_PORT`、`POWERX_GRPC_PORT` 为准（有值即覆盖默认）

## 5.1 配置优先级与追溯
- 优先级：`EnvironmentFile(/etc/powerx/powerx.env)` > `backend/etc/config.yaml`
- 发布追溯最小集合：
  - `dist/systemd/${VERSION}/manifest.txt`
  - `/opt/powerx/backend/etc/config.yaml`（或软链目标）
  - `/etc/powerx/powerx.env`（若启用）

## 6. 校验 service 路径与制品一致
基础服务必查两项：
- `powerx-backend.service` 的 `ExecStart=/opt/powerx/backend/powerx`
- `powerx-web-admin.service` 的 `ExecStart=/usr/bin/node /opt/powerx/web-admin/.output/server/index.mjs`

可选（启用 runner 时）：
- `powerx-runner.service` 的 `ExecStart=/usr/bin/node /opt/powerx/runner/dist/main.js`

若你的目录不同，先改 service 文件再 reload。

## 7. 启动并设置自启
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

## 7.1 使用脚本按 tag 切换版本（推荐）
当 `/opt/powerx/releases/<tag>` 已存在时，使用脚本切换：

```bash
sudo bash backend/scripts/ops/switch-release-systemd.sh v2.0.2 --with-runner
```

说明：
- 脚本会自动切换软链、重启服务、执行健康检查。
- 健康检查失败会自动回滚到切换前的软链目标。

## 8. 首次安装引导配置页（推荐）
启动后访问 `http://<host>:<port>/setup` 完成首次安装引导。
完整步骤、字段含义、接口说明与排障，请统一参考：`../setup.md`。

## 9. 引导页排障
排障条目已收敛到 `../setup.md`，本节仅保留索引，避免重复维护。
