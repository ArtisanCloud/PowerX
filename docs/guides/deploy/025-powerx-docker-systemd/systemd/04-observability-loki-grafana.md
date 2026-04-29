# 04. Observability：PowerX 直推 Loki + Grafana（Ubuntu）

## 1. 目标

本文只覆盖你当前使用的主链路：

`PowerX(应用直推) -> Loki -> Grafana`

说明：
- 不要求 Docker。
- 不依赖 Promtail。
- 不依赖 journald。
- 你已有 PowerX 直推能力，本篇只讲 Loki/Grafana 的安装与接入。

## 2. 部署模式约束

- 本文采用：**应用直推 Loki**。
- 同一份日志不要再被 Promtail 重采，避免重复写入。
- 如果你后续切到 Promtail 采集模式，请单独使用采集文档，不要与本文混用。

## 3. Ubuntu 快速安装（Loki + Grafana）

在服务器执行：

```bash
# 1) 安装依赖与 Grafana 源
sudo apt-get update
sudo apt-get install -y apt-transport-https software-properties-common wget gnupg
sudo mkdir -p /etc/apt/keyrings
wget -q -O - https://apt.grafana.com/gpg.key | sudo gpg --dearmor -o /etc/apt/keyrings/grafana.gpg
echo "deb [signed-by=/etc/apt/keyrings/grafana.gpg] https://apt.grafana.com stable main" | sudo tee /etc/apt/sources.list.d/grafana.list

# 2) 安装 loki + grafana
sudo apt-get update
sudo apt-get install -y loki grafana

# 3) 准备目录
sudo mkdir -p /etc/powerx/observability/loki /var/lib/loki /var/lib/grafana
sudo chown -R 472:472 /var/lib/grafana
```

## 4. Loki 配置与启动

1) 放置 Loki 配置：

```bash
sudo cp deploy/observability/loki/loki-config.yaml /etc/powerx/observability/loki/
sudo cp /etc/powerx/observability/loki/loki-config.yaml /etc/loki/config.yml
```

2) 启动 Loki：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now loki
sudo systemctl status loki --no-pager
```

3) 验证 Loki：

```bash
curl -sS http://127.0.0.1:3100/ready
```

返回 `ready` 即正常。

## 5. Grafana 配置与启动

1) 放置 provisioning（可选但推荐）：

```bash
sudo mkdir -p /etc/powerx/observability/grafana
sudo cp -R deploy/observability/grafana/provisioning /etc/powerx/observability/grafana/

# 仅拷贝 Loki 数据源与仪表盘（默认安全）
sudo mkdir -p /etc/grafana/provisioning/datasources /etc/grafana/provisioning/dashboards
sudo cp -R /etc/powerx/observability/grafana/provisioning/datasources/* /etc/grafana/provisioning/datasources/
sudo cp -R /etc/powerx/observability/grafana/provisioning/dashboards/* /etc/grafana/provisioning/dashboards/
```

说明：
- 默认不要直接拷贝 `provisioning/alerting`，不同 Grafana 版本可能因规则校验差异导致启动失败。
- 若你已拷贝 alerting 并启动失败，可先禁用：
```bash
sudo mkdir -p /etc/grafana/provisioning/_disabled_alerting
sudo mv /etc/grafana/provisioning/alerting/*.yaml /etc/grafana/provisioning/_disabled_alerting/ 2>/dev/null || true
```

2) 修正权限（Ubuntu deb 包必须）：
```bash
sudo chown -R grafana:grafana /var/lib/grafana /var/log/grafana /etc/grafana
sudo chmod -R u+rwX,g+rX /var/lib/grafana /var/log/grafana /etc/grafana
```

2) 启动 Grafana：

```bash
sudo systemctl enable --now grafana-server
sudo systemctl status grafana-server --no-pager
```

3) 验证 Grafana（先确认端口归属）：

```bash
sudo ss -lntp | grep :3000
curl -sS http://127.0.0.1:3000/api/health
```

说明：
- 若返回 JSON（包含 `database`、`version`），表示命中 Grafana。
- 若返回 HTML（如 Nuxt 页面），表示 `3000` 被其他服务占用（常见是 `powerx-web-admin`），此时需修改 Grafana 监听端口（如 `3001`）：

```bash
sudo sed -i 's/^;*http_port = .*/http_port = 3001/' /etc/grafana/grafana.ini
sudo systemctl restart grafana-server
curl -sS http://127.0.0.1:3001/api/health
```

访问：`http://<host>:3000`（或你改后的端口，如 `3001`）

4) 初始化登录与重置管理员密码：

- 推荐：先用默认账号 `admin / admin` 登录 Grafana，首次登录会强制要求在页面内修改密码。
- 仅在“忘记密码且无法登录 UI”时，再使用 CLI 重置：

```bash
sudo grafana cli --homepath /usr/share/grafana --config /etc/grafana/grafana.ini \
  admin reset-admin-password 'YourStrongPassword'
```

说明：
- 若提示找不到配置/默认值，通常是未传 `--homepath`。
- 重置后可直接重新登录，无需重装。

## 6. PowerX 直推 Loki 配置检查

确保 PowerX 采用直推模式（重点：修改运行时配置，而不是改编译产物模板）：

- 生效配置文件通常是：`/etc/powerx/config.yaml`
- 先用 service 确认真实配置路径：

```bash
systemctl cat powerx-backend | grep -E "POWERX_CONFIG|-config"
```

然后编辑该运行时配置文件（通常 `/etc/powerx/config.yaml`）：

```yaml
log:
  loki:
    enable: true
    url: http://127.0.0.1:3100
    labels:
      system: powerx
      service: powerx-backend
      env: prod
      instance: ${HOSTNAME}
      module: runtime
    batch_wait: 1
    batch_size: 100
```

改完后重启后端：

```bash
sudo systemctl restart powerx-backend
sudo systemctl status powerx-backend --no-pager
```

再验证能力是否生效：

```bash
curl -sS -H "Authorization: Bearer <ADMIN_TOKEN>" \
  "http://127.0.0.1:8077/api/v1/admin/monitor/logs/config"
```

- 应用日志配置中 `loki=true`。
- Loki 地址指向 `http://127.0.0.1:3100`（或实际 Loki 地址）。
- 不要再让 Promtail 采集同一日志源。

建议保留字段（便于检索）：

- `trace_id`
- `request_id`
- `tenant_uuid`
- `plugin_id`
- `level`
- `message`

## 7. 验证链路（PowerX -> Loki -> Grafana）

1) 触发一条业务请求（任意 API 或后台操作）。

2) 在 Grafana Explore 查询：

```logql
{service="powerx-backend"}
```

如你们标签不同，改为实际值。

3) 按 trace 检索：

```logql
{service="powerx-backend"} |= "trace_id"
```

## 8. 常见问题

1) Grafana 无数据
- 先确认 Loki ready：`curl http://127.0.0.1:3100/ready`
- 再看 Loki 日志：`sudo journalctl -u loki -n 200 --no-pager`
- 再确认 PowerX 端 `loki=true` 且地址可达。

2) 重复日志
- 原因：应用直推 + Promtail 重采同一源。
- 处理：保留一种，本文场景保留“应用直推”。

3) 能查到日志但字段不全
- 检查应用输出是否为结构化 JSON。
- 检查是否包含 `trace_id/tenant_uuid/plugin_id`。

## 9. 回滚

```bash
sudo systemctl stop loki grafana-server
sudo systemctl disable loki grafana-server
```

说明：
- 关闭 Loki/Grafana 不影响 PowerX 主业务。
