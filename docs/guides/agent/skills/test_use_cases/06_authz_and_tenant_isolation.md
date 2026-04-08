# L6 - 权限与租户隔离

## 目标

验证“有权限租户可调用、无权限租户被拒绝、跨租户 trace 不可读”。

## 前置条件

1. 准备租户 A（有 grant）和租户 B（无 grant）。
2. 已有管理员 token（用于 trace 查询校验）。
3. skill 与 capability 已发布且可调用。

## UI 详细操作步骤（主流程）

1. 在左侧菜单 `技能库` 确认目标 skill 已发布并绑定 capability。
2. 切换到租户 A 上下文，发起一次 skill 调用，应成功。
3. 记录租户 A 返回的 `trace_id`。
4. 切换到租户 B 上下文，发起同样调用，应被拒绝。
5. 使用租户 B 的 `tenant_uuid` 查询租户 A 的 trace，应返回 `403/404`。

## UI 通过标准

1. 租户 A 成功，租户 B 拒绝。
2. 跨租户 trace 查询被阻断。
3. 拒绝结果有明确错误语义。

## API 留证（可复制）

```bash
set -euo pipefail
: "${API_BASE:?}" "${TENANT_A_TOKEN:?}" "${TENANT_B_TOKEN:?}" "${TENANT_B_UUID:?}" "${ADMIN_TOKEN:?}" "${CAPABILITY_ID:?}" "${SKILL_ID:?}" "${TMP_DIR:?}"

curl -sS -X POST "$API_BASE/tenant/invocations" \
  -H "Authorization: Bearer $TENANT_A_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"capability_id\":\"${CAPABILITY_ID}\",\"preferred_protocol\":\"skill\",\"tool_grant_ids\":[\"ops.read\"],\"payload\":{\"skill_id\":\"${SKILL_ID}\"}}" \
  | tee "$TMP_DIR/l6_tenant_a.json" | jq .

TRACE_A=$(jq -r '.. | .trace_id? // empty' "$TMP_DIR/l6_tenant_a.json" | head -n1)
[ -n "$TRACE_A" ]

curl -sS -X POST "$API_BASE/tenant/invocations" \
  -H "Authorization: Bearer $TENANT_B_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"capability_id\":\"${CAPABILITY_ID}\",\"preferred_protocol\":\"skill\",\"tool_grant_ids\":[],\"payload\":{\"skill_id\":\"${SKILL_ID}\"}}" \
  | tee "$TMP_DIR/l6_tenant_b.json" | jq .

HTTP_CODE=$(curl -sS -o "$TMP_DIR/l6_cross_resp.json" -w '%{http_code}' \
  "$API_BASE/admin/skills/traces/${TRACE_A}?tenant_uuid=${TENANT_B_UUID}" \
  -H "Authorization: Bearer $ADMIN_TOKEN")

[ "$HTTP_CODE" = "403" ] || [ "$HTTP_CODE" = "404" ]

echo "L6 PASS"
```

## 失败排查

1. 租户 B 也成功：检查是否误配了 grant 或使用了错误 token。
2. 跨租户查询返回 200：优先检查查询接口是否传了错误 tenant_uuid。
3. 无法稳定复现：清理会话后重新调用，避免沿用旧上下文。
