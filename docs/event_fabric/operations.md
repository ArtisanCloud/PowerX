# Event Fabric 运维与操作指引

## 主题目录（Topic Directory）

### 创建主题
```bash
curl -X POST https://$ADMIN_HOST/event-fabric/topics \
  -H 'Authorization: Bearer <token>' \
  -H 'Content-Type: application/json' \
  -d '{
    "tenant_id": "tenant-corex",
    "namespace": "corex.workflow",
    "name": "approved",
    "payload_format": "json",
    "max_retry": 5,
    "ack_timeout_sec": 30,
    "versioning_mode": "strict",
    "retention_policy": "{\"type\":\"time\",\"value\":\"7d\"}"
  }'
```

### 查询主题
```bash
curl "https://$ADMIN_HOST/event-fabric/topics?tenant_id=tenant-corex&namespace=corex.workflow"
```

### 更新生命周期
```bash
curl -X PATCH https://$ADMIN_HOST/event-fabric/topics/<topicUUID>/lifecycle \
  -H 'Authorization: Bearer <token>' \
  -H 'Content-Type: application/json' \
  -d '{
    "target_state": "deprecated",
    "change_reason": "migrate to v2"
  }'
```

## 常见巡检
- `GET /event-fabric/topics`：按租户/命名空间筛选，确认生命周期、重试、Ack 超时设定。
- 关注返回的 `full_topic` 字段，确保命名遵循 `<tenant>.<namespace>.<name>`。
- 当生命周期进入 `deprecated` 时，`deprecated_at` 会为非空。

## ACL 授权管理

### 授权主体
```bash
curl -X POST https://$ADMIN_HOST/event-fabric/acl \
  -H 'Authorization: Bearer <token>' \
  -H 'Content-Type: application/json' \
  -d '{
    "tenant_id": "tenant-corex",
    "topic_full_name": "tenant-corex.corex.workflow.approved",
    "grants": [
      {"principal_type":"service","principal_id":"svc-workflow","action":"publish"}
    ]
  }'
```

### 查看权限
```bash
curl "https://$ADMIN_HOST/event-fabric/acl?tenant_id=tenant-corex&topic_uuid=<topicUUID>"
```

### 撤销权限
```bash
curl -X POST https://$ADMIN_HOST/event-fabric/acl \
  -H 'Authorization: Bearer <token>' \
  -H 'Content-Type: application/json' \
  -d '{
    "tenant_id": "tenant-corex",
    "topic_full_name": "tenant-corex.corex.workflow.approved",
    "revokes": [
      {"principal_type":"service","principal_id":"svc-workflow","action":"publish"}
    ]
  }'
```

## 告警建议
- 主题数量异常增长：定期对比 `topics` 列表与变更操作员。
- Ack 超时频繁：观察 `ack_timeout_sec` 与消费服务健康状况。

## 高可用与部署建议
- **多副本部署**：管理平面（Admin HTTP + gRPC）建议最少部署 2 个副本，并启用 HPA，确保重试调度器与订阅处理协程不会成为单点。
- **数据层复制**：PostgreSQL 建议开启流复制/主备，Redis 至少部署哨兵模式，以保证重试队列与幂等窗口在故障时可快速切换。
- **调度 Worker**：`EventFabricDeps.RetryWorker` 支持在多实例下并行运行，推荐配置 2~3 倍于租户数量的 worker 并使用不同实例抢占锁，以提升重试吞吐。
- **配置建议**：生产环境启用 `event_fabric.security.require_tls=true` 与签名校验，Redis 建议使用独立密码与 TLS；观察窗口 `default_max_retry` 与 `ack_timeout_seconds` 可根据业务 SLA 调整。

## 指标与可观测性
- 通过 `deps.EventFabric.Metrics.Snapshot()` 获取实时指标：
  - `DeliverySuccess`：投递成功率（≥99.9% 满足 NFR）。
  - `AvgRetryDelay`、`MaxRetryDelay`：重试延迟，建议 P95 < 200ms。
  - `DLQBacklog`：死信积压量，持续 >100 需告警。
  - `AvgReplayLatency`：回放耗时，结合任务日志分析。
- 参考示例，在服务内部定期写入日志：
  ```go
  snap := deps.EventFabric.Metrics.Snapshot()
  logger.Info(ctx, "event_fabric", "snapshot", metrics.EncodeSnapshot(snap))
  ```
- Prometheus 集成可通过自定义 exporter 读取上述快照或扩展 `RecorderImpl`，将指标上报至 `event_fabric_*` 系列指标（交付时默认以日志输出）。

## 性能与压测
- **基准测试**：
  ```bash
  go test -bench=. ./internal/tests/perf/event_fabric -run=^$
  ```
  `latency_benchmark_test.go` 聚焦发布→Ack 往返延迟，`throughput_benchmark_test.go` 验证多并发场景吞吐。
- **压力工具**：
  ```bash
  go run ./tools/event_fabric_loadtest \
    -endpoint https://corex.example.com/admin/event-fabric/events:publish \
    -tenant tenant-prod -topic tenant-prod.corex.workflow.approved \
    -signature-secret $EVENT_FABRIC_SIGNATURE -events 5000 -concurrency 100
  ```
  工具会输出成功率、平均/分位延迟及吞吐，并可通过 `-report` 生成 JSON 报告。

## 安全校验
- 启用 TLS/签名后，可通过脚本快速验证：
  ```bash
  EVENT_FABRIC_URL=https://corex.example.com/admin/event-fabric/events:publish \
  EVENT_FABRIC_SIGNATURE_SECRET=xxxxxx \
  scripts/security/event_fabric_self_check.sh
  ```
- 脚本会先尝试非 TLS 请求（期望被拒绝），随后使用 HMAC-SHA256 组合（时间戳、HTTP 方法、Path、Body）生成签名并验证 202 响应，结果写入 `reports/event_fabric_security.log`。

## 快速验证脚本
- 按照 `quickstart.md` 可使用自动化脚本完成端到端体验：
  ```bash
  EVENT_FABRIC_API_BASE=http://localhost:8077/admin/event-fabric \
  EVENT_FABRIC_TENANT=tenant-demo \
  scripts/demo/event_fabric_quickstart.sh
  ```
- 脚本包含 Topic 创建、ACL 下发、事件发布与 DLQ 巡检，输出日志保存到 `reports/event_fabric_quickstart.log` 方便留存。
