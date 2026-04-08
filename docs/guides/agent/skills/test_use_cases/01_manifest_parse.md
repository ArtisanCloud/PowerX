# L1 - Manifest 解析与入库

## 目标

验证一个最小 Skill 能在“技能库”页面完成导入，并以 `draft` 状态进入 Registry。

## 前置条件

1. 你当前能访问左侧菜单 `技能库` 页面。
2. 有管理员权限账号（可执行导入）。
3. API 留证时可用：`API_BASE/ADMIN_TOKEN/SKILL_ID/SKILL_V1/TMP_DIR`。
4. 说明：`incident-triage` 是本用例里的“手工导入示例”，不是默认 seed 数据。

## UI 详细操作步骤（主流程）

1. 打开左侧菜单 `技能库`，先搜索 `skill.thirdparty.prompt-template` 验证 seed 是否已执行（可选但推荐）。
2. 保持 tab 在 `已导入技能（Registry）`。
3. 点击右上角 `导入/安装 Skill`。
4. 在弹窗选择 `Upload 导入` 模式。
5. 填写基础字段：
- `skill_id=incident-triage`
- `version=1.0.0`
- `source=plugin`
6. 填写包信息：`bundle_uri` 与 `checksum`（`sha256:` 前缀）。
7. 提交导入，等待成功提示。
8. 回到列表页搜索 `incident-triage`，点击“查询”。
9. 查看该版本状态是否为 `draft`。

## UI 通过标准

1. 列表中能检索到 `incident-triage:1.0.0`。
2. 状态为 `draft`。
3. 没有 manifest 校验错误。

## API 留证（可复制）

```bash
set -euo pipefail
: "${API_BASE:?}" "${ADMIN_TOKEN:?}" "${SKILL_ID:?}" "${SKILL_V1:?}" "${TMP_DIR:?}"

cat > "$TMP_DIR/l1_register.json" <<JSON
{
  "skill_id": "${SKILL_ID}",
  "version": "${SKILL_V1}",
  "source": "plugin",
  "bundle_ref": {
    "uri": "s3://powerx-skills/demo/${SKILL_ID}-${SKILL_V1}.tgz",
    "checksum": "sha256:demo-${SKILL_ID}-${SKILL_V1}"
  },
  "manifest": {
    "name": "${SKILL_ID}",
    "description": "Incident triage workflow",
    "entrypoints": ["runbook.default"]
  }
}
JSON

curl -sS -X POST "$API_BASE/admin/skills" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d @"$TMP_DIR/l1_register.json" | tee "$TMP_DIR/l1_register_resp.json" | jq .

curl -sS "$API_BASE/admin/skills/${SKILL_ID}" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | tee "$TMP_DIR/l1_get_resp.json" | jq .

jq -e 'tostring | test("draft")' "$TMP_DIR/l1_get_resp.json" >/dev/null

echo "L1 PASS"
```

## 失败排查

1. 导入后列表无数据：先清空过滤条件，再点“查询”。
2. `checksum` 相关错误：确认格式是 `sha256:` 或 `sha256-` 前缀。
3. manifest 错误：检查 `name/entrypoints` 是否为空。
