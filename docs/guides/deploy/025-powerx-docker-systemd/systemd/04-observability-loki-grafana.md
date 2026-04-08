# 04. Observability（可选）：接入 Loki + Grafana + Promtail

## 1. 目标与策略
你当前采用两阶段是正确的：

1. 阶段 1（默认）：先完成 PowerX 安装与启动，不接入 Promtail。
2. 阶段 2（可选）：再接入 Loki + Grafana + Promtail，把日志统一打入 Loki。

结论：
- Promtail 未接入不会阻塞 PowerX 启动。
- Promtail 只影响“日志观测”，不影响“业务服务可用性”。

## 2. 日志驱动建议（生产）
- backend：`console=false`、`file=true`、`loki=false`
- web-admin/runner/插件：优先输出到 stdout/stderr（systemd journal）
- 统一采集：Promtail 采集 journald -> Loki -> Grafana

说明：
- 不建议 backend 同时直推 loki + promtail 采集，避免重复写入。

## 3. 准备目录与配置
```bash
sudo mkdir -p /etc/powerx/observability/{loki,promtail,grafana}
sudo mkdir -p /var/lib/loki /var/lib/promtail /var/lib/grafana
sudo chown -R 472:472 /var/lib/grafana
```

拷贝仓库配置：
```bash
sudo cp deploy/observability/loki/loki-config.yaml /etc/powerx/observability/loki/
sudo cp deploy/observability/promtail/promtail-config.yaml /etc/powerx/observability/promtail/
sudo cp -R deploy/observability/grafana/provisioning /etc/powerx/observability/grafana/
```

## 4. 安装与启动（Docker 方式，推荐）
创建 compose 文件：
```bash
cat >/tmp/powerx-observability.compose.yaml <<'YAML'
version: "3.9"

services:
  loki:
    image: grafana/loki:2.9.8
    command: ["-config.file=/etc/loki/loki-config.yaml"]
    volumes:
      - /etc/powerx/observability/loki/loki-config.yaml:/etc/loki/loki-config.yaml:ro
      - /var/lib/loki:/var/loki
    ports:
      - "3100:3100"
    restart: unless-stopped

  promtail:
    image: grafana/promtail:2.9.8
    command: ["-config.file=/etc/promtail/promtail-config.yaml"]
    volumes:
      - /etc/powerx/observability/promtail/promtail-config.yaml:/etc/promtail/promtail-config.yaml:ro
      - /var/lib/promtail:/var/lib/promtail
      - /var/log:/var/log:ro
      - /run/log/journal:/run/log/journal:ro
      - /etc/machine-id:/etc/machine-id:ro
    restart: unless-stopped
    depends_on:
      - loki

  grafana:
    image: grafana/grafana:11.1.4
    environment:
      GF_SECURITY_ADMIN_USER: admin
      GF_SECURITY_ADMIN_PASSWORD: admin123
    volumes:
      - /var/lib/grafana:/var/lib/grafana
      - /etc/powerx/observability/grafana/provisioning:/etc/grafana/provisioning:ro
    ports:
      - "3001:3000"
    restart: unless-stopped
    depends_on:
      - loki
YAML
```

启动：
```bash
sudo docker compose -f /tmp/powerx-observability.compose.yaml up -d
sudo docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'
```

## 5. Promtail 读 journald 的关键点
`deploy/observability/promtail/promtail-config.yaml` 已按 systemd 方式配置：
- 采集 `powerx-backend.service`
- 采集 `powerx-web-admin.service`
- 采集 `powerx-runner.service`
- 可选采集插件文件日志：`/opt/powerx/backend/logs/plugins/**/*.log`

若 promtail 容器日志提示 journal 权限问题，可改宿主机 systemd 方式部署 promtail，或额外检查宿主机 journal 权限策略。

## 6. 验证
### 6.1 服务健康
```bash
curl -sS http://127.0.0.1:3100/ready
curl -sS http://127.0.0.1:9080/ready
curl -sS http://127.0.0.1:3001/api/health
```

### 6.2 PowerX 侧生成日志
```bash
sudo systemctl restart powerx-backend powerx-web-admin powerx-runner
sudo journalctl -u powerx-backend -n 30 --no-pager
```

### 6.3 Grafana 检索
访问 `http://<host>:3001`（默认 `admin/admin123`），Loki 数据源已预置。  
示例查询：
```logql
{job="powerx",app="backend"}
{job="powerx",app="web-admin"}
{job="powerx",app="runner"}
```

## 7. 回滚（关闭观测，不影响业务）
```bash
sudo docker compose -f /tmp/powerx-observability.compose.yaml down
```

说明：
- 关闭 Loki/Grafana/Promtail 不会影响 `powerx-backend/web-admin/runner`。
- 你可随时再次 `up -d` 恢复观测。

## 8. 常见问题
1. Grafana 无数据
- 先看 promtail 日志：`sudo docker logs --tail=200 <promtail-container-name>`
- 再看 Loki ready：`curl http://127.0.0.1:3100/ready`

2. 只有 backend 有日志，runner/web-admin 没有
- 检查对应 service 是否启动：
  - `sudo systemctl status powerx-web-admin powerx-runner --no-pager`

3. 插件日志没采到
- 插件必须输出到 journald 或 `/opt/powerx/backend/logs/plugins/**/*.log`。
