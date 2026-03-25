# 02. 安装 systemd、配置、启动

## 1. 目标
将 PowerX 三服务以 systemd 托管方式启动，并设置开机自启。

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

预期结果：`systemctl list-unit-files | rg 'powerx-(backend|runner|web-admin)'` 可见三个服务。
失败处理：检查文件权限与语法。

## 3. 配置环境变量文件

service 文件默认读取：`/etc/powerx/powerx.env`。  
建议从 dist 产物模板复制：

```bash
sudo mkdir -p /etc/powerx
sudo cp dist/systemd/${VERSION}/config/powerx.env.example /etc/powerx/powerx.env
```

预期结果：`/etc/powerx/powerx.env` 存在且可读，且已按环境改成真实值。
失败处理：检查 DB/Redis 地址是否可达。

## 4. 校验 service 路径与制品一致
重点检查三项：
- `powerx-backend.service` 的 `ExecStart=/opt/powerx/backend/powerx`
- `powerx-runner.service` 的 `ExecStart=/usr/bin/node /opt/powerx/runner/dist/main.js`
- `powerx-web-admin.service` 的 `ExecStart=/usr/bin/node /opt/powerx/web-admin/.output/server/index.mjs`

若你的目录不同，先改 service 文件再 reload。

## 5. 启动并设置自启

```bash
sudo systemctl enable powerx-backend powerx-runner powerx-web-admin
sudo systemctl restart powerx-backend powerx-runner powerx-web-admin
sudo systemctl status powerx-backend powerx-runner powerx-web-admin --no-pager
```

预期结果：全部为 `active (running)`。
失败处理：用 `journalctl` 看最近 200 行错误日志。
