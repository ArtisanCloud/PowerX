# systemd 生产部署方案（非 Docker）

## 1. 适用场景

- 需要通过 `systemctl` 管控进程生命周期
- 服务器资源固定、部署链路强调简单和可观察
- 与容器方案并行维护，作为兜底/兼容路径

## 2. 推荐单元文件

示例：`/etc/systemd/system/powerx-backend.service`

```ini
[Unit]
Description=PowerX Backend Service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=powerx
Group=powerx
WorkingDirectory=/opt/powerx/current/backend
EnvironmentFile=/opt/powerx/shared/config/powerx-backend.env
ExecStart=/opt/powerx/current/backend/powerx-backend -config /opt/powerx/shared/config/config.yaml
Restart=always
RestartSec=3
TimeoutStopSec=30
LimitNOFILE=65535
StandardOutput=append:/opt/powerx/shared/logs/backend/stdout.log
StandardError=append:/opt/powerx/shared/logs/backend/stderr.log

[Install]
WantedBy=multi-user.target
```

示例：`/etc/systemd/system/powerx-web-admin.service`

```ini
[Unit]
Description=PowerX Web Admin Service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=powerx
Group=powerx
WorkingDirectory=/opt/powerx/current/web-admin
Environment=NODE_ENV=production
ExecStart=/usr/bin/node /opt/powerx/current/web-admin/.output/server/index.mjs
Restart=always
RestartSec=3
TimeoutStopSec=30
LimitNOFILE=65535
StandardOutput=append:/opt/powerx/shared/logs/web-admin/stdout.log
StandardError=append:/opt/powerx/shared/logs/web-admin/stderr.log

[Install]
WantedBy=multi-user.target
```

## 3. 启停与开机自启

```bash
sudo systemctl daemon-reload
sudo systemctl enable powerx-backend powerx-web-admin
sudo systemctl start powerx-backend powerx-web-admin
sudo systemctl status powerx-backend powerx-web-admin
```

## 4. 发布流程（应用）

1. 发布新版本到：
   - `/opt/powerx/releases/backend-<version>`
   - `/opt/powerx/releases/web-admin-<version>`
2. 切换软链：
   - `/opt/powerx/current/backend`
   - `/opt/powerx/current/web-admin`
3. 重启服务：
   ```bash
   sudo systemctl restart powerx-backend powerx-web-admin
   ```
4. 健康检查：
   - `curl -f http://127.0.0.1:8077/api/v1/health`
   - `curl -f http://127.0.0.1:3000/healthz`

## 5. 回滚流程（应用）

1. 软链切回旧版本目录。
2. 执行：
   ```bash
   sudo systemctl restart powerx-backend powerx-web-admin
   ```
3. 验证关键链路并观察 10~30 分钟。

## 6. 插件目录与配置（必须）

- `config.yaml` 必须显式配置 `deployment.env: prod`。它是该 PowerX 实例的稳定身份，也是插件 Role/User 命名的唯一环境来源；Schema/Database 名称保持不变。
- `config.yaml` 中插件路径需固定到持久目录：
  - `/opt/powerx/plugins/registry.json`
  - `/opt/powerx/plugins/installed`
  - `/opt/powerx/plugins/market_cache`
- systemd 重启后，CoreX 会根据 registry 自动恢复已启用插件。
- 若 `deployment.env` 与 registry 中记录的插件环境不一致，自动恢复必须阻断；不得按新环境静默创建第二套对象或复用旧对象。处理流程见 [插件数据库部署环境隔离计划](./plugin-database-isolation.md)。
