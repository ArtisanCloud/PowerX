# 事件骨干授权运行手册

## 1. 功能概览
- 管理能力（Capability）与授予（Grant）生命周期，满足工具最小权限要求。
- 提供 gRPC 网关评估服务，返回 allow/block/challenge 决策。
- 记录授权与评估全链路审计事件，支持导出与合规留存。
- 对策略缺失、越权尝试、评估失败触发实时告警，协助 SecOps 处置。

## 2. 常用操作
### 2.1 能力管理
```bash
# 创建能力
curl -X POST "$ADMIN/api/v1/event-fabric/capabilities" \
  -H 'Content-Type: application/json' \
  -d '{"namespace":"event_fabric","action":"publish","risk_level":"medium"}'
```

### 2.2 授权 Grant
```bash
curl -X POST "$ADMIN/api/v1/event-fabric/grants" \
  -H 'Content-Type: application/json' \
  -d '{
    "tenant_id":"TENANT_UUID",
    "subject":{"type":"agent","id":"SUBJECT_UUID"},
    "capabilities":["event_fabric.publish"],
    "ttl_seconds":7200,
    "conditions":{"resources":["topic://demo"],"context_tags":["prod"]}
  }'
```

### 2.3 缓存失效
```bash
curl -X POST "$ADMIN/api/v1/event-fabric/grants/cache:invalidate" \
  -H 'Content-Type: application/json' \
  -d '{
    "tenant_id":"TENANT_UUID",
    "subject":{"type":"agent","id":"SUBJECT_UUID"}
  }'
```

### 2.4 审计查询与导出
```bash
# JSON
curl "$ADMIN/api/v1/event-fabric/audit/authorization?tenantId=TENANT_UUID&from=1970-01-01T00:00:00Z&to=$(date -u +'%Y-%m-%dT%H:%M:%SZ')&page=1&pageSize=20"

# CSV
curl -o authorization_audit.csv "$ADMIN/api/v1/event-fabric/audit/authorization?tenantId=TENANT_UUID&from=1970-01-01T00:00:00Z&to=$(date -u +'%Y-%m-%dT%H:%M:%SZ')&format=csv"
```

## 3. 监控与告警
| 项目 | 指标/日志 | 阈值 | 响应 |
|------|-----------|------|------|
| 网关评估延迟 | `event_fabric_authorization_latency_ms` | P99 > 200ms 连续5分钟 | 触发性能告警，排查 Redis/DB 性能 |
| 缓存命中率 | `authorization_cache_hit_rate` | 低于 70% | 检查 Grant 版本是否频繁变更 |
| Challenge SLA | `authorization.challenge_timeout` 告警事件 | 任意 | SecOps 介入，追踪审批工单 |
| 越权尝试 | 告警 topic `event_fabric.authorization.alerts` | 任意 | 核对主体是否误配置，必要时回收 Grant |

## 4. 审计留存核查
- 每周执行 `scripts/audit/check_authorization_retention.sh <tenant_uuid>`，确认最早记录不早于 3 年阈值。
- 按需使用 `--days <N>` 指定自定义窗口，用于专项稽核。
- 若 `legacy_rows > 0`，需触发冷存储归档流程并更新运行记录。

## 5. 故障排查
1. **评估返回 BLOCK**：
   - 调用 `/grants/:grantId` 核对条件；
   - 查看审计决策原因 `items[].reason`；
   - 必要时执行缓存失效。
2. **Challenge 长时间 pending**：
   - 查询审计事件，确认是否发出 `challenge_required` 告警；
   - 与 SecOps 工单状态对齐，必要时人工决策 `/challenges/:ticketId/decision`。
3. **审计数据缺失**：
   - 检查 Kafka `secops.challenge` 堆积；
   - 查看 audit service 日志是否熔断；
   - 运行留存脚本确认 ClickHouse 数据完整性。

> 术语说明：`$ADMIN` 默认为 `http://localhost:8077/api/v1/admin`, 所有示例需替换为实际环境地址与 UUID。
