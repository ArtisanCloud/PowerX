# L5 - 统一入口调用（preferred_protocol=skill）

## 目标

验证统一入口 `tenant/invocations` 在 `preferred_protocol=skill` 时可正确路由并返回 trace。

## 前置条件

1. 左侧菜单 `技能库` 中目标 skill 已发布。
2. 已完成 capability 绑定。
3. 租户 token 可用。

## UI 详细操作步骤（主流程）

1. 打开左侧菜单 `技能库`，确认 `incident-triage` 已 `published`。
2. 在对应 skill 的版本详情中确认已绑定 capability。
3. 打开租户调用调试入口（统一调用表单）。
4. 设置参数：
- `capability_id=com.powerx.skill.incident-triage.invoke`
- `preferred_protocol=skill`
- payload 包含 `skill_id=incident-triage`
5. 发起调用并查看返回。
6. 记录 `trace_id`，用于后续审计查询。

## UI 通过标准

1. 返回中可见 `trace_id`。
2. 响应语义显示命中 skill 协议路径。
3. 无鉴权或路由错误。

## API 留证（可复制）

```bash
set -euo pipefail
: "${API_BASE:?}" "${TENANT_TOKEN:?}" "${CAPABILITY_ID:?}" "${SKILL_ID:?}" "${TMP_DIR:?}"

RESP="$TMP_DIR/l5_unified_resp.json"

curl -sS -X POST "$API_BASE/tenant/invocations" \
  -H "Authorization: Bearer $TENANT_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"capability_id\":\"${CAPABILITY_ID}\",\"preferred_protocol\":\"skill\",\"payload\":{\"skill_id\":\"${SKILL_ID}\",\"incident_id\":\"INC-1001\"}}" \
  | tee "$RESP" | jq .

TRACE_ID=$(jq -r '.. | .trace_id? // empty' "$RESP" | head -n1)
[ -n "$TRACE_ID" ]

echo "trace_id=$TRACE_ID"
echo "L5 PASS"
```

## 失败排查

1. `trace_id` 为空：先检查是否命中错误分支（鉴权/参数校验失败）。
2. 返回非 skill 协议：检查 `preferred_protocol` 是否正确传入。
3. 调用报 capability 不存在：先回到技能库确认 capability 绑定。
