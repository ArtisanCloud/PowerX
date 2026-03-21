# L2 - 发布、升级、回滚状态机

## 目标

验证同一个 skill 的 `1.0.0 -> 1.1.0 -> 回滚到 1.0.0` 状态机完整可用。

## 前置条件

1. 已完成 L1，存在 `incident-triage:1.0.0(draft)`。
2. 管理员有发布/回滚权限。
3. API 留证时可用：`API_BASE/ADMIN_TOKEN/SKILL_ID/SKILL_V1/SKILL_V2/TMP_DIR`。

## UI 详细操作步骤（主流程）

1. 打开左侧菜单 `技能库`，搜索 `incident-triage`。
2. 在 `1.0.0` 版本行执行“发布”。
3. 再导入 `1.1.0`（操作同 L1）。
4. 在 `1.1.0` 版本行执行“发布”。
5. 在 `incident-triage` 行执行“回滚”，目标版本选择 `1.0.0`。
6. 刷新列表，检查版本指针与状态。

## UI 通过标准

1. `1.0.0` 和 `1.1.0` 都存在于 Registry。
2. 回滚后默认命中版本为 `1.0.0`。
3. 发布和回滚动作都有成功反馈。

## API 留证（可复制）

```bash
set -euo pipefail
: "${API_BASE:?}" "${ADMIN_TOKEN:?}" "${SKILL_ID:?}" "${SKILL_V1:?}" "${SKILL_V2:?}" "${TMP_DIR:?}"

curl -sS -X POST "$API_BASE/admin/skills/${SKILL_ID}/publish" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"version\":\"${SKILL_V1}\"}" | tee "$TMP_DIR/l2_publish_v1.json" | jq .

cat > "$TMP_DIR/l2_register_v2.json" <<JSON
{
  "skill_id":"${SKILL_ID}",
  "version":"${SKILL_V2}",
  "source":"plugin",
  "bundle_ref":{"uri":"s3://powerx-skills/demo/${SKILL_ID}-${SKILL_V2}.tgz","checksum":"sha256:demo-${SKILL_ID}-${SKILL_V2}"},
  "manifest":{"name":"${SKILL_ID}","description":"v2","entrypoints":["runbook.default"]}
}
JSON

curl -sS -X POST "$API_BASE/admin/skills" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d @"$TMP_DIR/l2_register_v2.json" | tee "$TMP_DIR/l2_register_v2_resp.json" | jq .

curl -sS -X POST "$API_BASE/admin/skills/${SKILL_ID}/publish" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"version\":\"${SKILL_V2}\"}" | tee "$TMP_DIR/l2_publish_v2.json" | jq .

curl -sS -X POST "$API_BASE/admin/skills/${SKILL_ID}/rollback" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"target_version\":\"${SKILL_V1}\"}" | tee "$TMP_DIR/l2_rollback.json" | jq .

echo "L2 PASS"
```

## 失败排查

1. 发布按钮不可用：确认当前版本为 `draft` 且账号有管理员权限。
2. 回滚失败：确认目标版本已发布过，不是未发布草稿。
3. 回滚后命中仍是 `1.1.0`：刷新页面并重新查询，确认未缓存旧状态。
