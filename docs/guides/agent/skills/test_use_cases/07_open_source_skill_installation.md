# L7 - 开源 Skill 安装

## 目标

验证仓库安装链路：创建 install-task、任务成功、Registry 可检索。

## 前置条件

1. 可访问左侧菜单 `技能库`。
2. 管理员账号有安装权限。
3. API 留证时可用：`API_BASE/ADMIN_TOKEN/TMP_DIR`。

## UI 详细操作步骤（主流程）

1. 打开左侧菜单 `技能库`。
2. 点击右上角 `导入/安装 Skill`。
3. 在弹窗选择 `仓库安装（推荐）`。
4. 填写仓库信息：`provider/repo(or repo_url)/path/ref/source`。
5. 提交后进入任务列表，观察 install-task 状态。
6. 等待状态从 `pending/running` 变为 `success`。
7. 回到 Registry 按 skill_id 查询，确认已入库。

## UI 通过标准

1. install-task 最终状态为 `success`。
2. Registry 能检索到对应 skill 记录。

## API 留证（可复制）

```bash
set -euo pipefail
: "${API_BASE:?}" "${ADMIN_TOKEN:?}" "${TMP_DIR:?}"

cat > "$TMP_DIR/l7_install_req.json" <<JSON
{
  "provider":"github",
  "repo":"openai/skills",
  "path":"skills/.curated/gh-address-comments",
  "ref":"main",
  "source":"third_party",
  "auto_import":true
}
JSON

curl -sS -X POST "$API_BASE/admin/skills/install-tasks" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d @"$TMP_DIR/l7_install_req.json" | tee "$TMP_DIR/l7_install_resp.json" | jq .

TASK_ID=$(jq -r '.. | .task_id? // empty' "$TMP_DIR/l7_install_resp.json" | head -n1)
[ -n "$TASK_ID" ]

echo "task_id=$TASK_ID"
echo "L7 PASS"
```

## 失败排查

1. 任务卡在 `running`：检查网络访问仓库能力与后台 worker 状态。
2. 任务 `failed`：优先看 `error_summary/stderr_log`。
3. 任务成功但 Registry 无数据：确认 `auto_import=true` 或手动执行导入。
