# L3 - Agent 最小 Skill 执行链路（统一版）

## 目标

统一验证 3 类能力：
1. `hello-echo`：最小连通性冒烟。
2. `prompt-template`：多轮参数变化与缺参行为（主测）。
3. `incident-triage`：可选扩展，不作为主验收。

统一通过标准：事件链完整 `intent -> plan -> node_start/node_end -> final -> end`，且执行节点为 `skill`。

## 前置条件

1. 页面位置：左侧菜单 `技能库`。
2. 以下 skill 状态均为 `published`：
`skill.thirdparty.hello-echo`、`skill.thirdparty.prompt-template`（可选 `incident-triage`）。
3. 使用同一租户上下文进行测试。

## UI 统一操作流程

1. 在 `技能库` 分别搜索 `hello-echo` 与 `prompt-template`，确认 `published`。
2. 打开 Agent 调试/聊天页，选择测试 Agent（建议固定一个 Agent，避免干扰）。
3. 按下面“测试脚本”逐条发送问题。
4. 每条都检查事件流顺序：`intent -> plan -> node_start/node_end -> final -> end`。
5. 在节点详情确认：
`node.kind=skill`，且有 `skill_id/version`。

## 测试脚本（可直接复制）

### A. hello-echo（冒烟）

1. 自然语言：`把 INC-1001 原样返回给我。`
2. 调试回退：`请调用 hello-echo，把文本 "INC-1001" 原样返回。`
3. 预期：最终内容包含 `INC-1001`，链路完整。

### B. prompt-template（主测，多轮）

1. 第 1 轮（基础渲染）：
`请使用 prompt-template 输出：事故 {{id}} 影响 {{scope}}，修复建议 {{action}}。其中 id=INC-1001，scope=华东支付，action=先回滚 v2.3.7。`
预期：输出文本包含 `INC-1001`、`华东支付`、`先回滚 v2.3.7`。

2. 第 2 轮（变量变更）：
`同样模板，把 scope 改成 华南支付，把 action 改成 先限流再灰度回滚。`
预期：输出发生对应变化，不应与第 1 轮相同。

3. 第 3 轮（缺参场景）：
`同样模板，这次只给 id=INC-1002。`
预期：出现可解释结果（缺失变量提示或占位输出），且链路不断。

### C. incident-triage（可选扩展）

1. 自然语言：`我们线上接口 INC-1001 宕机 30 分钟，请先排查并给简短修复建议。`
2. 调试回退：`请调用 incident-triage 分析 INC-1001，并给出修复建议。`
3. 说明：该技能为示例执行器，验链路即可，不作为质量验收基准。

## API 留证（可复制）

```bash
set -euo pipefail
: "${API_BASE:?}" "${TENANT_TOKEN:?}" "${AGENT_ID:?}" "${TMP_DIR:?}"

SSE_FILE="$TMP_DIR/l3_agent_stream.sse"
curl -sS -N -G "$API_BASE/agents/stream/sse" \
  -H "Authorization: Bearer $TENANT_TOKEN" \
  --data-urlencode "agent_id=${AGENT_ID}" \
  --data-urlencode "q=请使用 prompt-template 输出：事故 {{id}} 影响 {{scope}}，修复建议 {{action}}。其中 id=INC-1001，scope=华东支付，action=先回滚 v2.3.7。" \
  | tee "$SSE_FILE"

grep -q '^event: intent' "$SSE_FILE"
grep -q '^event: plan' "$SSE_FILE"
grep -q '^event: final' "$SSE_FILE"
grep -q '^event: end' "$SSE_FILE"
(grep -q 'node_start' "$SSE_FILE" && grep -q 'node_end' "$SSE_FILE")
(grep -q 'skill' "$SSE_FILE" || grep -q 'skill_id' "$SSE_FILE")

echo "L3 PASS"
```

## 失败排查（统一口径）

1. 查不到技能：执行 `make db-seed` 后刷新页面重试。
2. 有 `intent/plan` 但无 `node_start/node_end`：检查 skill 绑定、授权与发布状态。
3. 命中不稳定：先自然语言，再用带 skill 名的调试回退句定位路由问题。
4. 返回内容不变：优先看 `planner` 日志里 `tasks[].params` 是否带上你本轮新变量。
5. 鉴权错误：检查租户 token 是否过期、租户上下文是否一致。
