# 审计与租户隔离指南（Skills）

本文用于运维/管理员验证 Skills 审计可追溯与跨租户隔离。

## 场景 1：按 Skill 查询管理动作审计

```bash
curl -sS "$POWERX_HTTP_BASE/admin/skills/audits?skill_id=skill.demo.lifecycle&limit=20" \
  -H "Authorization: Bearer $ROOT_TOKEN"
```

预期：

1. 返回 `items` 数组。
2. 可见关键动作：`import/publish/rollback/bind`（按实际操作出现）。
3. 每条含 `action/skill_id/version/operator/result/trace_id/created_at`。

## 场景 2：按 trace_id 查询执行链路

```bash
curl -sS "$POWERX_HTTP_BASE/admin/skills/traces/<trace_id>?tenant_uuid=<tenant_uuid>" \
  -H "Authorization: Bearer $ROOT_TOKEN"
```

预期：

1. 返回该 trace 的执行信息。
2. 可见 `skill_id/version/tenant_uuid/protocol_used/invoke_path/status`。
3. 可见摘要字段 `request_payload_digest/response_payload_digest`。

## 场景 3：跨租户阻断验证

1. 准备一条 `tenant_uuid=A` 的 trace。
2. 使用 `tenant_uuid=B` 查询该 trace：

```bash
curl -i "$POWERX_HTTP_BASE/admin/skills/traces/<trace_id>?tenant_uuid=tenant-b" \
  -H "Authorization: Bearer $ROOT_TOKEN"
```

预期：HTTP `404`（阻断跨租户读取）。

## UI 操作路径（审计抽屉）

1. 打开 `左侧菜单 -> 技能库`。
2. 在 Registry 行点击“审计 -> 查看”。
3. 抽屉会加载最近审计记录并支持刷新。

## 常见排查

1. 审计为空：确认是否实际执行了 `import/publish/rollback/bind/invoke`。
2. trace 查询 404：先确认 `trace_id` 存在，再确认 `tenant_uuid` 是否匹配。
3. 响应缺字段：检查后端是否运行最新代码分支（US4 实现后版本）。
