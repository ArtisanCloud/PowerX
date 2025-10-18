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
