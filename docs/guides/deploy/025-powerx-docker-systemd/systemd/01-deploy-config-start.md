# 01. Deploy Config & Start（Linux: 下载 -> 构建 -> systemd 启动）

## 1. 目标
在 Linux 上完成：
1. 获取代码并切版本（tag/develop）
2. 构建 `dist/systemd/<version>`
3. 部署到 `/opt/powerx/releases/<version>`
4. systemd 启动并进入 `/setup`

## 2. 前置文档（先看）
- `00-runtime-deps-versions.md`（PowerX / PostgreSQL / Redis 版本与安装）
- `00-nginx-install-config.md`（可选，生产建议）

## 3. 统一变量
```bash
export POWERX_VERSION=v1.0.0
# 预发可用 develop：
# export POWERX_VERSION=develop
```

## 4. 获取代码

机器无仓库：
```bash
git clone https://github.com/ArtisanCloud/PowerX.git
cd PowerX
git fetch --tags --prune
```

机器已有仓库：
```bash
cd ~/workspace/PowerX
git fetch --tags --prune
```

## 5. 切到目标版本

生产推荐 tag：
```bash
git checkout ${POWERX_VERSION}
```

预发用 develop：
```bash
git checkout develop
git pull --ff-only origin develop
export POWERX_VERSION=develop-$(date +%Y%m%d)
```

可选：切到最新 tag：
```bash
export POWERX_VERSION=$(git tag -l 'v*' --sort=-v:refname | head -n 1)
git checkout ${POWERX_VERSION}
```

## 6. 构建 dist
先执行构建前依赖准备（首次构建或依赖变化时必须）：

```bash
# 固定 Go 工具链版本（推荐）
go env -w GOTOOLCHAIN=go1.24.12
go version
go env GOVERSION GOTOOLCHAIN

# Go 依赖（建议在 backend 目录）
cd backend
go mod tidy
cd ..

# Node 依赖
cd web-admin
npm ci
cd ..

# runner 存在时再执行
if [ -d backend/runner ]; then
  cd backend/runner
  npm ci
  cd ../..
fi
```

如果你是国内网络，建议先设置 Go 代理：
```bash
go env -w GOPROXY=https://goproxy.cn,direct
```

然后再执行：
```bash
make dist DIST_VERSION=${POWERX_VERSION}
```

可选（依赖已安装）：
```bash
make dist DIST_VERSION=${POWERX_VERSION} NPM_INSTALL=0
```

## 7. 校验并打包
```bash
ls -la dist/systemd/${POWERX_VERSION}
ls -la dist/systemd/${POWERX_VERSION}/backend
ls -la dist/systemd/${POWERX_VERSION}/web-admin/.output/server

tar -czf powerx-systemd-${POWERX_VERSION}.tar.gz -C dist/systemd/${POWERX_VERSION} .
```

## 8. 部署到服务器目录
```bash
sudo mkdir -p /opt/powerx/releases/${POWERX_VERSION}
sudo tar -xzf powerx-systemd-${POWERX_VERSION}.tar.gz -C /opt/powerx/releases/${POWERX_VERSION}
```

## 9. 安装并启动 systemd
推荐直接使用切换脚本（自动创建 `powerx` 用户/组并赋权）：
```bash
sudo bash backend/scripts/ops/switch-release-systemd.sh ${POWERX_VERSION} --with-runner
```
说明：
- 当前版本下，`runner` 是可选组件。
- 传入 `--with-runner` 且发布目录不存在 `runner/dist/main.js` 时，脚本会给出 warning，并进入 noop-runner 模式；`powerx-runner.service` 会因 `ConditionPathExists` 被跳过，不影响 backend/web-admin 启动。
- 若不需要 runner，可直接不带参数执行：
```bash
sudo bash backend/scripts/ops/switch-release-systemd.sh ${POWERX_VERSION}
```
若需临时开启 setup 状态诊断日志（`/admin/setup/status` 判定链路），可直接走脚本参数：
```bash
sudo bash backend/scripts/ops/switch-release-systemd.sh ${POWERX_VERSION} --with-setup-trace
```
关闭诊断日志：
```bash
sudo bash backend/scripts/ops/switch-release-systemd.sh ${POWERX_VERSION} --without-setup-trace
```
说明：该脚本会自动从 `/opt/powerx/releases/${POWERX_VERSION}/systemd/` 同步 `.service` 到 `/etc/systemd/system/`，并执行 `daemon-reload + enable + restart`。
另外会自动创建 `backend/logs` 与 `backend/logs/audit` 并修正 service 运行用户权限。
启用 `--with-runner` 时，也会自动创建 `/etc/powerx/powerx.env`（优先复制 `systemd/powerx.env.example`），并自动写入可用的 `NODE_BIN` 路径。
若找不到对 service 运行用户可执行的 `NODE_BIN`，脚本会直接失败退出（不再“假成功”）。

运行用户策略（默认）：
- 默认使用当前登录用户（`whoami`，sudo 场景等价于 `SUDO_USER`）作为 service 用户。
- 脚本会校验 systemd 实际生效的 `User/Group`，若和目标用户不一致会直接失败退出。
- 可通过环境变量覆盖：
```bash
export POWERX_SERVICE_USER=ubuntu
export POWERX_SERVICE_GROUP=ubuntu
sudo bash backend/scripts/ops/switch-release-systemd.sh ${POWERX_VERSION} --with-runner
```

手工方式（仅在排障时使用）：
```bash
sudo ln -sfn /opt/powerx/releases/${POWERX_VERSION}/backend /opt/powerx/backend
sudo ln -sfn /opt/powerx/releases/${POWERX_VERSION}/web-admin /opt/powerx/web-admin
sudo ln -sfn /opt/powerx/releases/${POWERX_VERSION}/runner /opt/powerx/runner

sudo systemctl daemon-reload
sudo systemctl enable powerx-backend powerx-web-admin powerx-runner
sudo systemctl restart powerx-backend powerx-web-admin powerx-runner
sudo systemctl status powerx-backend powerx-web-admin powerx-runner --no-pager
```

## 10. 配置说明
- 必改：`/opt/powerx/backend/etc/config.yaml`
- 配置清单：`../00-required-config.md`

当前 unit 行为：
- `powerx-backend.service`：无 `EnvironmentFile`
- `powerx-web-admin.service`：从 `backend/etc/config.yaml` 读取端口
- `powerx-runner.service`：读取 `/etc/powerx/powerx.env`

## 11. 首次安装
访问：`http://<host>:<web-admin-port>/setup`

若页面未进入 `/setup`，请先核对 setup 状态（避免代理干扰）：
```bash
# 直连本机 backend
curl --noproxy '*' -sS http://127.0.0.1:8080/api/v1/admin/setup/status

# 若机器配置了 http(s)_proxy，先临时禁用再测
unset http_proxy https_proxy HTTP_PROXY HTTPS_PROXY ALL_PROXY all_proxy
export NO_PROXY=127.0.0.1,localhost,::1
```

下一步：
- `02-verify-and-rollback.md`
- `03-install-plugin.md`
