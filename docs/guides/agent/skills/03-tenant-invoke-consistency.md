# 租户统一调用一致性指南

目标：验证 Skill 映射到 capability 后，租户侧统一通过 Capability Invocation 调用，且 trace/status/result 与 Agent 主入口一致。

标准路径：

1. Agent 主入口：`POST /agents/invoke` 或 `GET /agents/stream/sse`
2. 执行层入口：`POST /tenant/invocations`

非标准路径：

1. 不再使用 `POST /tenant/skills/invoke` 作为新插件业务执行入口。
2. 不再使用 `/api/v1/plugin/skills/invoke` 作为插件业务执行入口。

## 前置条件

1. 对应 Skill 版本已发布。
2. 已完成 `bind-capability`（用于统一入口路径）。
3. 使用租户 token：`$TENANT_TOKEN`。

## 场景 A：统一入口路径

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

## 场景 B：默认版本解析

请求不传 `version`，系统应自动命中最新已发布版本（`is_latest_published=true`）。

验证方式：

1. 发布两个版本，切换 latest 指针。
2. 调用 `tenant/invocations`。
3. 通过审计/trace 核对实际命中的 version。
