# 03. 启动后验收、排障与回滚（systemd）

## 1. 健康验收

```bash
curl -f http://127.0.0.1:8077/api/v1/health
bash backend/scripts/ops/deploy-check.sh
```

预期结果：HTTP 200，脚本输出 `healthy`。

## 2. 运维域 API 验收

```bash
curl -sS "http://127.0.0.1:8077/api/v1/admin/deploy/health" \
  -H "Authorization: Bearer <ADMIN_TOKEN>"
```

预期结果：返回 deploy 健康聚合信息。

## 3. 常见排障

### 3.1 服务未启动

```bash
sudo systemctl status powerx-backend powerx-runner powerx-web-admin --no-pager
journalctl -u powerx-backend -u powerx-runner -u powerx-web-admin -n 200 --no-pager
```

### 3.2 路径错误
- 现象：`ExecStart` 报文件不存在。
- 动作：核对 `/opt/powerx/{backend,runner,web-admin}` 软链与产物。

### 3.3 依赖不可达
- 现象：backend 报 DB/Redis 连接失败。
- 动作：核对 `/etc/powerx/powerx.env` 的 `DATABASE_DSN`、`REDIS_ADDR`。

## 4. 回滚方案

### 4.1 API 回滚记录（可选）

```bash
bash backend/scripts/ops/rollback-release.sh prod <TARGET_VERSION> systemd
```

### 4.2 制品回切 + 重启
1. 将软链回切到上一稳定版本：

```bash
sudo ln -sfn /opt/powerx/releases/<PREV_VERSION>/backend /opt/powerx/backend
sudo ln -sfn /opt/powerx/releases/<PREV_VERSION>/runner /opt/powerx/runner
sudo ln -sfn /opt/powerx/releases/<PREV_VERSION>/web-admin /opt/powerx/web-admin
```

2. 重启服务：

```bash
sudo systemctl restart powerx-backend powerx-runner powerx-web-admin
```

3. 再做健康验收。

## 5. 回滚成功判定
- `/api/v1/health` 返回 200。
- 管理端关键页面可访问。
- 新版本引入的问题消失。
