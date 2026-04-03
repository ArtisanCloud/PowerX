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

sudo ln -sfn /opt/powerx/releases/${POWERX_VERSION}/backend /opt/powerx/backend
sudo ln -sfn /opt/powerx/releases/${POWERX_VERSION}/web-admin /opt/powerx/web-admin
# runner 存在时再执行
sudo ln -sfn /opt/powerx/releases/${POWERX_VERSION}/runner /opt/powerx/runner
```

## 9. 安装并启动 systemd
```bash
sudo cp /opt/powerx/releases/${POWERX_VERSION}/systemd/*.service /etc/systemd/system/
sudo systemctl daemon-reload

sudo systemctl enable powerx-backend powerx-web-admin
sudo systemctl restart powerx-backend powerx-web-admin
sudo systemctl status powerx-backend powerx-web-admin --no-pager

# runner 可选
sudo systemctl enable powerx-runner
sudo systemctl restart powerx-runner
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

下一步：
- `02-verify-and-rollback.md`
- `03-install-plugin.md`
