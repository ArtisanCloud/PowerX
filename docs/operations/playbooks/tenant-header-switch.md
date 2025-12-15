# Tenant Header 灰度与回退 Playbook

## 目的
在租户上下文逐步切换至 UUID-only 模式时，提供可重复的操作步骤与监控指引，确保在发现兼容性问题时可以一分钟内回退。

## 核心指标与告警
| 指标 | 来源 | 说明 |
| --- | --- | --- |
| `tenant_header_reject_total` | `backend/pkg/auth/middleware/metrics.go`（expvar） | 任何仍携带 `X-Tenant-ID`/旧 header 的请求都会被拒绝，并触发该计数器；值>0 即代表仍有未升级的客户端。 |

Prometheus 建议：
```promql
increase(tenant_header_reject_total[1m]) > 0
```
定义为 P1 告警，并在告警信息中附带最近触发的 `trace_id` 或 `request_id` 方便回溯。指标来自 expvar，可通过 sidecar 抓取 `/_expvar`。

> 注意：fallback 逻辑与相关环境变量 (`PX_HEADER_UUID_ONLY`、`PX_ALLOW_TENANT_ID_HEADER`) 已移除，任何旧 header 都会被拒绝，且无法通过配置临时放开。

## 灰度流程
1. **基线观察**：在开启 UUID-only 前（现已默认开启）至少观察 24 小时，确认 `tenant_header_reject_total` 为 0。
2. **Stage/Canary**：发布只读变更或新版本时，滚动重启服务即可，无需额外开关。密切关注指标与租户端报错。
3. **Production**：每次大版本发布后，持续 1h 观察指标与日志，若仍为 0 即视为成功。

## 回退流程
1. 触发 P1 告警后，立即联系相应客户端 owner，要求升级至 UUID-only。
2. 若确需临时恢复，可**回滚**至带有 fallback 的旧版本镜像，并在事故报告中记录影响范围。回滚步骤：触发 `kubectl rollout undo` 或恢复上一版本 Helm release。
3. 记录阻断请求的 `request_id`、`tenant_uuid`，并在问题解决后重新发布最新版本。

## 验证清单
- `GET /debug/vars | jq '.tenant_header_reject_total'` 与 Prometheus 数据一致。
- Web Admin/CLI 端的租户切换不再依赖 `X-Tenant-ID`（参考 T5 验证用例）。
- Playwright/Contract tests（`backend/tests/contract/*tenant*`) 在 UUID-only 模式下全部通过。
