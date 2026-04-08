# L4 - Intent 候选到 Planner 定案

## 目标

验证同一条用户问题会经过：`intent 候选 -> planner 定案 -> 节点执行`，且最终只执行被定案的 skill。

## 前置条件

1. 已完成 L1/L2，且 `incident-triage` 已发布可用。
2. 管理员与租户 token 可用。
3. Agent 调试页面可正常使用。
4. 本用例仅测试“语义接近技能之间的候选与定案”，不要使用 `hello-echo` 这类回显技能做候选比较。
5. 策略口径：`LLM 意图识别/Tool-Calling` 是首要路径；`rule` 仅用于以 `/` 开头的快捷命令（如 `/help`、`/rerun`），不用于普通自然语言。
6. 在 `左侧菜单 -> AI 设置` 确认当前环境存在可用的 `Provider + Model`（否则会回退到非 LLM 规划路径）。

## UI 详细操作步骤（主流程）

1. 进入页面：`左侧菜单 -> 技能库`。
2. 在 Skills 列表确认 `incident-triage` 为 `published`。
3. 在同一页面导入并发布第二个语义接近技能（建议：`incident-summary`，版本 `1.0.0`）。
4. 进入 Agent 调试页面（Agent 聊天/调试入口）。
5. 选择测试 Agent（建议 `agent_id=1001`）。
6. 新建一次会话，输入：`帮我排查这个事故并给一段简短总结`（incident 语义问题）。
7. 发送后先看 `intent` 区域：应出现候选技能列表（至少包含 `incident-triage` 或 `incident-summary`）。
8. 再看 `plan` 区域：应出现最终执行节点，且 `node.kind=skill`。
9. 最后看执行事件流：应出现 `node_start -> node_end`，并且节点引用的 skill 与 plan 定案一致。
10. 查看最终回答：应有 `final` 输出，且内容来自已执行 skill 的结果，而不是“把多个候选都执行一遍”。

## UI 通过标准

1. `intent` 有候选，`plan` 有唯一或明确可解释的 skill 定案。
2. 事件流中存在 `node_start/node_end`，并能对上 plan 里的 skill。
3. 最终输出正常返回，且未出现“候选全执行”。

## API 留证（可复制）

```bash
set -euo pipefail
: "${API_BASE:?}" "${ADMIN_TOKEN:?}" "${TENANT_TOKEN:?}" "${AGENT_ID:?}" "${TMP_DIR:?}"

# 1) 准备第二个候选 skill：incident-summary
cat > "$TMP_DIR/l4_register_b.json" <<JSON
{
  "skill_id":"incident-summary",
  "version":"1.0.0",
  "source":"plugin",
  "bundle_ref":{
    "uri":"s3://powerx-skills/demo/incident-summary-1.0.0.tgz",
    "checksum":"sha256:demo-incident-summary-1.0.0"
  },
  "manifest":{
    "name":"incident-summary",
    "description":"summary",
    "entrypoints":["summary.default"]
  }
}
JSON

curl -sS -X POST "$API_BASE/admin/skills" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d @"$TMP_DIR/l4_register_b.json" | tee "$TMP_DIR/l4_register_b_resp.json" | jq .

curl -sS -X POST "$API_BASE/admin/skills/incident-summary/publish" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"version":"1.0.0"}' | tee "$TMP_DIR/l4_publish_b_resp.json" | jq .

# 2) 触发 Agent SSE 流并落盘
SSE_FILE="$TMP_DIR/l4_agent_stream.sse"
curl -sS -N -G "$API_BASE/agents/stream/sse" \
  -H "Authorization: Bearer $TENANT_TOKEN" \
  --data-urlencode "agent_id=${AGENT_ID}" \
  --data-urlencode "q=帮我排查这个事故并给一段简短总结" \
  | tee "$SSE_FILE"

# 3) 最小断言：有 intent/plan，有 node 执行，有 incident 候选关键词
grep -q '^event: intent' "$SSE_FILE"
grep -q '^event: plan' "$SSE_FILE"
(grep -q 'node_start' "$SSE_FILE" && grep -q 'node_end' "$SSE_FILE")
(grep -q 'incident-triage' "$SSE_FILE" || grep -q 'incident-summary' "$SSE_FILE")

echo "L4 PASS"
```

## 失败排查

1. 没有 `intent` 事件：先确认 Agent 入口是统一编排模式，不是旧直出模式。
2. 有 `intent` 但没有 `plan`：检查该 Agent 是否启用了 planner。
3. `plan` 有 skill 但无 `node_start/node_end`：检查 skill 是否发布、是否授权、是否被 hard filter 拦截。
4. 看不到第二个候选技能：检查 `incident-summary` 是否导入并发布成功。
5. 问题文本不命中候选：避免使用回显类问题（如 hello-echo），保持 incident 语义问题。
6. 日志出现 `[intent-tool-calling] fallback: provider/model missing`：先去 `左侧菜单 -> AI 设置` 保存可用模型，再重试。
7. 日志出现 `strategies=[rule]` 且输入不是 `/command`：说明当前实例未启用 LLM/classifier 或 tool-calling 连通失败，需先修复模型配置。
