# L5 - 统一入口调用（preferred_protocol=skill）

## 目标

验证通过统一入口调用 skill 的链路与结果模型。

## 前置条件

1. 租户 Token 可用：`TENANT_TOKEN`
2. Skill 已发布并绑定 capability

## 操作步骤

### 步骤 1：调用统一入口

```bash
curl -sS -X POST "$API_BASE/tenant/invocations" \
  -H "Authorization: Bearer $TENANT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "capability_id":"com.powerx.skill.incident-triage.invoke",
    "preferred_protocol":"skill",
    "payload":{"skill_id":"incident-triage","incident_id":"INC-1001"}
  }'
```

### 步骤 2：记录 trace_id 并查询执行记录

使用返回 `trace_id` 查询 invocation trace（按你们实际查询接口）。

## 预期效果

1. `protocol_used=skill`。  
2. 返回统一 envelope（status/trace/result）。  
3. trace 与审计记录可查。

## 通过标准

1. Gateway 路径结果字段与 Agent 路径一致。  
2. fallback 行为与策略一致。

