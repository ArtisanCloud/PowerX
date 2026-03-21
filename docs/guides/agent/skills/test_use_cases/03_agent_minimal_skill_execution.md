# L3 - Agent 最小 Skill 执行链路

## 目标

验证单个已发布 skill 能被 Agent 正常识别并执行，事件链完整：`intent -> plan -> node_start/node_end -> final -> end`。

## 前置条件

1. 你当前页面在左侧菜单 `技能库`。
2. Registry 中已存在并发布任一 skill（推荐直接用 seed 自带的 `skill.thirdparty.prompt-template`）。
3. 有可用租户 token（用于 API 留证）。

## 技能与问题模板映射（先选一套）

1. 若测试 `skill.thirdparty.hello-echo`（回显类）：
- 建议问题：`请调用 hello-echo，把文本 "INC-1001" 原样返回。`
2. 若测试 `incident-triage`（事故分析类）：
- 建议问题：`请调用 incident-triage 分析 INC-1001，并给出修复建议。`

## UI 详细操作步骤（主流程）

1. 在 `左侧菜单 -> 技能库` 页面，把搜索词设为 `skill.thirdparty.prompt-template`，点“查询”。
2. 确认列表里该 skill 的状态是 `published`；如果不是，先在该行执行发布。
3. 打开 Agent 调试页面（左侧进入 Agent 对话/调试入口，使用同一租户上下文）。
4. 选择一个可测试 Agent（建议 `agent_id=1001`）。
5. 按上面的“技能与问题模板映射”发送匹配问题（不要混用）。
6. 在事件流中按顺序检查：`intent`、`plan`、`node_start/node_end`、`final`、`end`。
7. 打开执行节点详情，确认 `node.kind=skill` 且包含 `skill_id/version`。

## UI 通过标准

1. 出现完整事件链：`intent -> plan -> node_start/node_end -> final -> end`。
2. 执行节点是 `skill`，且能看到对应 `skill_id/version`。
3. 最终回答成功返回，不是空结果或报错中断。

## API 留证（可复制）

```bash
set -euo pipefail
: "${API_BASE:?}" "${TENANT_TOKEN:?}" "${AGENT_ID:?}" "${TMP_DIR:?}"

SSE_FILE="$TMP_DIR/l3_agent_stream.sse"
curl -sS -N -G "$API_BASE/agents/stream/sse" \
  -H "Authorization: Bearer $TENANT_TOKEN" \
  --data-urlencode "agent_id=${AGENT_ID}" \
  --data-urlencode "q=请调用 incident-triage 分析 INC-1001，并给出修复建议" \
  | tee "$SSE_FILE"

grep -q '^event: intent' "$SSE_FILE"
grep -q '^event: plan' "$SSE_FILE"
grep -q '^event: final' "$SSE_FILE"
grep -q '^event: end' "$SSE_FILE"
(grep -q 'node_start' "$SSE_FILE" && grep -q 'node_end' "$SSE_FILE")
(grep -q 'skill' "$SSE_FILE" || grep -q 'skill_id' "$SSE_FILE")

echo "L3 PASS"
```

## 失败排查

1. Skills 页查不到 `skill.thirdparty.prompt-template`：先执行 `make db-seed`，再刷新页面重查。
2. 有 `intent/plan` 但无 `node_start/node_end`：检查该 agent 的授权与 skill 绑定。
3. SSE 直接报鉴权错误：检查租户 token 是否过期、是否与当前租户一致。
4. 命中 skill 不稳定：确认提问文本是否和技能用途匹配（回显类别用回显问题，incident 类别用事故问题）。
