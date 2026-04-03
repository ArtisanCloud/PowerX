# 租户双路径调用一致性指南

目标：验证两条调用路径语义一致。

1. `POST /tenant/skills/invoke`
2. `POST /tenant/invocations` 且 `preferred_protocol=skill`

## 前置条件

1. 对应 Skill 版本已发布。
2. 已完成 `bind-capability`（用于统一入口路径）。
3. 使用租户 token：`$TENANT_TOKEN`。

## 场景 A：直接调用路径

```bash
curl -sS -X POST "$POWERX_HTTP_BASE/tenant/skills/invoke" \
  -H "Authorization: Bearer $TENANT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "skill_id":"skill.demo.lifecycle",
    "payload":{"input":"hello-direct"}
  }'
```

预期关键字段：

1. `protocol_used=skill`
2. `status=completed`
3. `trace_id` 非空

## 场景 B：统一入口路径

```bash
curl -sS -X POST "$POWERX_HTTP_BASE/tenant/invocations" \
  -H "Authorization: Bearer $TENANT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "capability_id":"cap.skill.demo",
    "preferred_protocol":"skill",
    "payload":{"input":"hello-unified"}
  }'
```

预期关键字段：

1. `protocol_used=skill`
2. `status=completed`
3. `trace_id` 非空
4. 返回中可见绑定后的 `skill_id/version`（实现中已透出）

## 场景 C：默认版本解析

请求不传 `version`，系统应自动命中最新已发布版本（`is_latest_published=true`）。

验证方式：

1. 发布两个版本，切换 latest 指针。
2. 调用 `tenant/skills/invoke`（不传 version）。
3. 通过审计/trace 核对实际命中的 version。
