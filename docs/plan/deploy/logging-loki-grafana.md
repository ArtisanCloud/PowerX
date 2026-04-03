# 生产日志方案：Loki + Grafana（含 Docker / 非 Docker）

## 1. 目标

- 将 PowerX 的 backend / web-admin / nginx / 插件运行日志统一汇聚到 Loki
- 通过 Grafana 做检索、聚合、告警
- 支持插件升级窗口与租户维度的快速排障

## 2. 架构建议

- 日志采集：`promtail`
- 日志存储：`loki`
- 可视化与告警：`grafana`

生产建议：

- Loki 与 Grafana 可以集中部署（运维平台）
- 每台业务节点部署 1 个 promtail（agent）

## 3. 标签规范（必须统一）

建议所有日志带以下 label：

- `env`：`prod/staging`
- `service`：`powerx-backend/powerx-web-admin/nginx/plugin-runtime`
- `instance`：主机名或节点 ID
- `level`：`debug/info/warn/error`
- `plugin_id`：插件日志可选
- `tenant`：租户日志可选

## 4. Docker 模式采集方案

### 4.1 采集来源

- 优先采集容器 stdout/stderr（Docker json log）
- 对于文件日志（如审计/自定义日志）可额外挂载目录采集

### 4.2 promtail 示例（简化）

```yaml
server:
  http_listen_port: 9080
  grpc_listen_port: 0

positions:
  filename: /tmp/positions.yaml

clients:
  - url: http://loki.monitoring.svc:3100/loki/api/v1/push

scrape_configs:
  - job_name: docker
    static_configs:
      - targets: [localhost]
        labels:
          job: docker
          env: prod
          __path__: /var/lib/docker/containers/*/*-json.log
    pipeline_stages:
      - docker: {}
      - labels:
          service:
          level:
```

## 5. 非 Docker（systemd）模式采集方案

### 5.1 采集来源

- 优先采集 `journald`（systemd 服务日志）
- 补充采集应用文件日志目录

### 5.2 promtail 示例（journald）

```yaml
scrape_configs:
  - job_name: systemd-journal
    journal:
      path: /var/log/journal
      labels:
        job: systemd-journal
        env: prod
    relabel_configs:
      - source_labels: ['__journal__systemd_unit']
        target_label: 'unit'
      - source_labels: ['__journal__hostname']
        target_label: 'instance'
```

## 6. Grafana 面板（首批）

建议先做 3 个固定面板：

1. 错误日志速率（按 service）
2. 插件升级窗口日志（按 `plugin_id`）
3. 租户异常日志（按 `tenant`）

## 7. 告警建议

- 某服务 `error` 日志 5 分钟突增
- 插件切换窗口内出现连续 `healthcheck failed`
- 某租户在短时间内出现高频失败日志

## 8. 与 PowerX 部署联动

- Docker：`promtail` 加入 compose，挂载 docker logs 与 app logs
- systemd：`promtail.service` 常驻运行，随主机启动
- 升级 SOP 期间临时按 `plugin_id` 过滤日志，配合 `switch_version` 快速回滚

