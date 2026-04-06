# 02. Verify & Rollback（验收与回滚）

## 1. 健康验收
```bash
export BACKEND_PORT=8080
curl -f http://127.0.0.1:${BACKEND_PORT}/api/v1/health
bash backend/scripts/ops/deploy-check.sh
```

## 2. 运维 API 验收
```bash
curl -sS "http://127.0.0.1:${BACKEND_PORT}/api/v1/admin/deploy/health" \
  -H "Authorization: Bearer <ADMIN_TOKEN>"
```

## 3. 常见排障

服务状态：
```bash
sudo systemctl status powerx-backend powerx-web-admin --no-pager
journalctl -u powerx-backend -u powerx-web-admin -n 200 --no-pager
```

runner（可选）：
```bash
sudo systemctl status powerx-runner --no-pager
journalctl -u powerx-runner -n 200 --no-pager
```
说明：无 runner 制品时，`powerx-runner.service` 可能显示 `ConditionPathExists ... was not met`，属于预期跳过（noop），不是故障。

实时跟日志（推荐）：
```bash
sudo journalctl -fu powerx-backend
sudo journalctl -fu powerx-web-admin
sudo journalctl -fu powerx-runner
```

定位最近一次启动失败原因（推荐）：
```bash
sudo systemctl status powerx-runner --no-pager -l
sudo journalctl -xeu powerx-runner --no-pager
```

`runner` 常见失败点：
- `/etc/powerx/powerx.env` 不存在（unit 使用了 `EnvironmentFile`）
- `/opt/powerx/runner/dist/main.js` 不存在（制品不完整）
- `User=powerx` 不存在或目录权限不足

`setup/status` 检查返回非 JSON（常见是代理页）：
```bash
env | grep -i proxy
curl -i --noproxy '*' http://127.0.0.1:8080/api/v1/admin/setup/status | sed -n '1,40p'
```
若命中了代理（例如出现 `Proxy-Connection`、HTML 错误页），先清理代理环境变量后重试：
```bash
unset http_proxy https_proxy HTTP_PROXY HTTPS_PROXY ALL_PROXY all_proxy
export NO_PROXY=127.0.0.1,localhost,::1
```

“设置 -> 用户与组织”点击租户后跳到 dashboard：
- 该现象通常是旧前端逻辑导致（切租户后强制跳转 `/dashboard`）。
- 处理方式：重新发布最新 `web-admin`，并强制刷新浏览器缓存后重试。

重点检查：
- `/opt/powerx/{backend,web-admin,runner}` 软链是否正确
- `/opt/powerx/backend/etc/config.yaml` 中 DB/Redis 配置是否可达
- `/etc/powerx/powerx.env` 文件是否存在（启用 runner 时）

## 4. 回滚

手工回切：
```bash
sudo ln -sfn /opt/powerx/releases/<PREV_VERSION>/backend /opt/powerx/backend
sudo ln -sfn /opt/powerx/releases/<PREV_VERSION>/web-admin /opt/powerx/web-admin
# runner 可选
sudo ln -sfn /opt/powerx/releases/<PREV_VERSION>/runner /opt/powerx/runner

sudo systemctl restart powerx-backend powerx-web-admin
# runner 可选
sudo systemctl restart powerx-runner
```

脚本回切（推荐）：
```bash
sudo bash backend/scripts/ops/switch-release-systemd.sh <PREV_VERSION> --with-runner
```

## 5. 回滚成功判定
- `/api/v1/health` 返回 200
- 管理端页面可访问
- 新版本问题消失
