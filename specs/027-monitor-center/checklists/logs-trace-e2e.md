# Logs / Trace E2E 验收清单（027-monitor-center）

## 0. 前置条件

- 已启动 backend + web-admin。
- 已拿到 Root `TOKEN`。
- 监控页面：`/monitor/logs-trace`。
- API 基础地址：`http://127.0.0.1:8080/api/v1`。

可复用变量：

```bash
export BASE="http://127.0.0.1:8080/api/v1"
export AUTH="Authorization: Bearer $TOKEN"
```

## 1. 通用验收（任何驱动都要过）

### 1.1 查询配置

```bash
curl -sS -H "$AUTH" "$BASE/admin/monitor/logs/config" | jq
```

通过标准：
- `code=200`
- `data.driver` 为 `loki|file|stdio` 之一
- `data.capabilities` 字段完整

### 1.2 查询日志

```bash
curl -sS -H "$AUTH" \
  "$BASE/admin/monitor/logs/query?page=1&page_size=20&keyword=backup" | jq
```

通过标准：
- `code=200`
- `data.items` 为数组
- `data.query_meta.driver` 与 config 的 `driver` 一致

### 1.3 页面对照

在 `/monitor/logs-trace` 验证：
- 顶部显示当前 `driver`
- 能力徽标与接口返回一致
- 输入筛选后点击“查询日志”，表格更新

## 2. Loki 驱动验收

### 2.1 配置要求

`config.yaml`：
- `log.loki.enable=true`
- `log.loki.url` 已配置可访问地址

重启 backend 后执行：

```bash
curl -sS -H "$AUTH" "$BASE/admin/monitor/logs/config" | jq '.data'
```

通过标准：
- `driver = "loki"`
- `capabilities.supports_grafana_link = true`

### 2.2 深链验证

```bash
curl -sS -H "$AUTH" \
  "$BASE/admin/monitor/logs/query?page=1&page_size=20&trace_id=<TRACE_ID>" | jq '.data.query_meta'
```

通过标准：
- `query_meta.grafana_url` 非空
- 页面“打开 Grafana”按钮可点击并新开页面

## 3. File 驱动验收

### 3.1 配置要求

`config.yaml`：
- `log.loki.enable=false`
- `log.file.enable=true`
- `log.file.info_file_path/error_file_path` 路径存在且可读

重启 backend 后执行：

```bash
curl -sS -H "$AUTH" "$BASE/admin/monitor/logs/config" | jq '.data'
```

通过标准：
- `driver = "file"`
- `capabilities.supports_grafana_link = false`

### 3.2 文件查询验证

```bash
curl -sS -H "$AUTH" \
  "$BASE/admin/monitor/logs/query?page=1&page_size=20&keyword=monitor.logs.query" | jq '.data'
```

通过标准：
- 能返回日志行（`items`）或明确 hint（路径无数据时）
- 页面显示“不可用能力”提示，不出现可点击的 Grafana 入口

## 4. Stdio 驱动验收

### 4.1 配置要求

`config.yaml`：
- `log.loki.enable=false`
- `log.file.enable=false`

重启 backend 后，先制造几条日志（打开一次 Logs 页面并点查询）。

### 4.2 最近窗口验证

```bash
curl -sS -H "$AUTH" \
  "$BASE/admin/monitor/logs/query?page=1&page_size=20&keyword=monitor.logs" | jq '.data.query_meta'
```

通过标准：
- `driver = "stdio"`
- `degraded = true`
- `hint` 提示“最近窗口/ring buffer”

## 5. 审计日志验收（T057）

执行一次配置查询和日志查询后，检查 backend 日志：

```bash
journalctl -u powerx-backend -n 200 --no-pager | grep -E "monitor.logs.config|monitor.logs.query"
```

通过标准：
- 有 `monitor.logs.config`
- 有 `monitor.logs.query`
- 字段包含：`operator`、`trace_id`、`status`；查询日志还应包含过滤摘要字段

## 6. 故障定位快速表

- `driver` 不符合预期：检查 `config.yaml` 中 `log.loki.enable / log.file.enable`，并确认已重启 backend。
- `loki` 无结果：检查 `log.loki.url` 可达性与 Loki 数据源内容。
- `file` 无结果：检查日志路径、权限、是否有实际写入。
- `stdio` 无结果：先触发几次请求产生日志；stdio 仅保留最近窗口，不保证历史。
