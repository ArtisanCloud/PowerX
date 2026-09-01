# 02. Verify & Troubleshoot

## 1. 服务状态
```bash
sudo systemctl status powerx-dev-backend powerx-dev-web-admin --no-pager
sudo systemctl status powerx-dev-runner --no-pager
```

预期：
- `powerx-dev-backend` 为 `active (running)`
- `powerx-dev-web-admin` 为 `active (running)`
- 无 runner 制品时，`powerx-dev-runner` 可被 `ExecCondition` 跳过

## 2. 端口检查
```bash
ss -lntp | grep -E ':3001|:8081'
```

预期：
- `127.0.0.1:3001` 或 `0.0.0.0:3001` 存在
- `127.0.0.1:8081` 或 `0.0.0.0:8081` 存在

## 3. 本机 HTTP 验证
```bash
curl -I http://127.0.0.1:3001
curl -f http://127.0.0.1:8081/api/v1/health
```

预期：
- web-admin 返回 HTTP 响应
- backend health 返回 200

## 4. 公网域名验证
```bash
export POWERX_DEV_DOMAIN=dev.powerx.example.com
curl -I https://${POWERX_DEV_DOMAIN}
curl -f https://${POWERX_DEV_DOMAIN}/api/v1/health
```

预期：
- HTTPS 证书有效
- `/api/v1/health` 返回 200

## 5. 日志查看
```bash
sudo journalctl -fu powerx-dev-backend --no-pager
sudo journalctl -fu powerx-dev-web-admin --no-pager
sudo journalctl -fu powerx-dev-runner --no-pager
```

最近日志：
```bash
sudo journalctl -u powerx-dev-backend -n 200 --no-pager
sudo journalctl -u powerx-dev-web-admin -n 200 --no-pager
```

## 6. 常见失败处理

### 6.1 `3001/8081` 没监听
检查 service env：
```bash
sudo systemctl cat powerx-dev-backend
sudo systemctl cat powerx-dev-web-admin
sudo cat /etc/powerx-dev/powerx.env
sudo grep -n "port\\|web_admin_port\\|bind_addr" /etc/powerx-dev/config.yaml
```

处理：
- `POWERX_CONFIG` 必须指向 `/etc/powerx-dev/config.yaml`
- `server.port` 必须是 `8081`
- `server.web_admin_port` 必须是 `3001`

### 6.2 Nginx 502
检查 dev service 是否运行：
```bash
ss -lntp | grep -E ':3001|:8081'
sudo journalctl -u powerx-dev-backend -n 100 --no-pager
sudo journalctl -u powerx-dev-web-admin -n 100 --no-pager
```

处理：
- 后端未启动，先修 `powerx-dev-backend`
- 前端未启动，先修 `powerx-dev-web-admin`
- Nginx 配置必须把 `/api/` 转发到 `8081`，把 `/` 转发到 `3001`

### 6.3 Dev 连接到生产 API
检查前端启动日志和 env：
```bash
sudo journalctl -u powerx-dev-web-admin -n 100 --no-pager
sudo cat /etc/powerx-dev/powerx.env | grep -E 'POWERX|NUXT|WS'
```

处理：
- `POWERX_HTTP_PROXY_BASE` 必须是内部 backend 地址，例如 `http://127.0.0.1:8081`
- `POWERX_PUBLIC_BASE_URL` 必须是 dev 域名，例如 `https://${POWERX_DEV_DOMAIN}`
- `POWERX_PUBLIC_WS_ORIGIN` 必须是 dev WS 域名，例如 `wss://${POWERX_DEV_DOMAIN}`
- `POWERX_GATEWAY_BASE_URL` / `POWERX_GATEWAY_WS_BASE_URL` 是兼容旧变量；新配置优先使用 `POWERX_PUBLIC_*`

### 6.4 Dev 与生产插件互相影响
检查 config 中插件运行路径：
```bash
grep -n "deployment:\\|env:\\|installed_dir\\|registry_file" /etc/powerx-dev/config.yaml
```

处理：
- `deployment.env` 必须明确为 `dev`；不能用 `version`、`dev_mode` 或目录名推导
- `installed_dir` 必须在 `/opt/powerx-dev/plugins/installed`
- `registry_file` 必须在 `/opt/powerx-dev/plugins/registry.json`
- dev 插件 ID 建议使用 `.dev` 后缀，避免 taskqueue、scheduler、ws topic 与生产插件共享状态
- 插件 Schema/Database 沿用 `px_<plugin_slug>`；Role/User 必须带 `pxu_dev_` 前缀。如果 Role/User 仍为无环境段的旧名称，停止自动恢复并先执行迁移 dry-run

### 6.5 数据写入生产库
检查数据库配置：
```bash
grep -n "dsn\\|schema" /etc/powerx-dev/config.yaml
```

处理：
- 使用独立 database，或至少使用独立 schema
- 执行 dev migration 前必须确认 `POWERX_CONFIG=/etc/powerx-dev/config.yaml`

## 7. Dev migration
如果 dev 使用独立 schema/database，发布后执行：

```bash
cd /opt/powerx-dev/backend
sudo -u powerx POWERX_CONFIG=/etc/powerx-dev/config.yaml ./database migrate
```

补 seed：
```bash
cd /opt/powerx-dev/backend
sudo -u powerx POWERX_CONFIG=/etc/powerx-dev/config.yaml ./database seed
sudo -u powerx POWERX_CONFIG=/etc/powerx-dev/config.yaml ./platform_capability_seed
```

如果 `/opt/powerx-dev/backend/platform_capability_seed` 不存在，说明当前 dev release 是旧产物；重新执行 `make dist` 并通过 `switch-develop-systemd.sh` 切换到新 release 后再补 seed。

## 8. 回滚 dev release
```bash
ln -sfn /opt/powerx-dev/releases/<PREV_VERSION>/backend /opt/powerx-dev/backend
ln -sfn /opt/powerx-dev/releases/<PREV_VERSION>/web-admin /opt/powerx-dev/web-admin
ln -sfn /opt/powerx-dev/releases/<PREV_VERSION>/runner /opt/powerx-dev/runner

sudo systemctl restart powerx-dev-backend powerx-dev-web-admin powerx-dev-runner
```

验收：
```bash
curl -f http://127.0.0.1:8081/api/v1/health
curl -I https://${POWERX_DEV_DOMAIN}
```
