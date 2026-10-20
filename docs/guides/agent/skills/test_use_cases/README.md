# Skills 测试用例（UI 优先，API 留证）

本目录默认先走 UI 操作，再用 API 命令留证与自动化回归。

当前 024 / V5 的单 Agent 页面测试入口为[单 Agent 页面验收指南](../../../features/024-ai-engineering-skills/guide.md)。以下 L1～L8 用例按各自实际安装状态使用。

## 一次性准备（仅 API 留证需要）

```bash
set -euo pipefail

export API_BASE="${API_BASE:-http://127.0.0.1:8077/api/v1}"
export ADMIN_TOKEN="${ADMIN_TOKEN:-REPLACE_ADMIN_TOKEN}"
export TENANT_TOKEN="${TENANT_TOKEN:-REPLACE_TENANT_TOKEN}"

export TENANT_A_TOKEN="${TENANT_A_TOKEN:-$TENANT_TOKEN}"
export TENANT_B_TOKEN="${TENANT_B_TOKEN:-REPLACE_TENANT_B_TOKEN}"
export TENANT_A_UUID="${TENANT_A_UUID:-REPLACE_TENANT_A_UUID}"
export TENANT_B_UUID="${TENANT_B_UUID:-REPLACE_TENANT_B_UUID}"

export SKILL_ID="${SKILL_ID:-skill.thirdparty.prompt-template}"
export SKILL_V1="${SKILL_V1:-1.0.0}"
export SKILL_V2="${SKILL_V2:-1.1.0}"
export CAPABILITY_ID="${CAPABILITY_ID:-REPLACE_CAPABILITY_ID}"
export AGENT_ID="${AGENT_ID:-1001}"

export TMP_DIR="${TMP_DIR:-$(mktemp -d /tmp/powerx-skill-cases.XXXXXX)}"
echo "TMP_DIR=$TMP_DIR"
```

## 先确认所用示例的实际安装状态

1. 本轮单 Agent V5 验收使用 `marketing.review_summarize`，发布及页面准备按上方指南。
2. 旧用例中的 `skill.thirdparty.prompt-template`、`skill.thirdparty.hello-echo` 必须在当前租户已导入、发布且符合当前执行合同；目录存在不代表可运行。
3. `incident-triage` 为可选扩展，不是本轮验收前置。
4. 示例缺失时检查导入、发布与依赖，不通过全量 seed 或重置数据库补齐测试环境。

## 执行顺序

1. [01_manifest_parse.md](./01_manifest_parse.md)
2. [02_publish_and_rollback.md](./02_publish_and_rollback.md)
3. [03_agent_minimal_skill_execution.md](./03_agent_minimal_skill_execution.md)
4. [04_intent_to_planner_decision.md](./04_intent_to_planner_decision.md)
5. [05_gateway_protocol_skill.md](./05_gateway_protocol_skill.md)
6. [06_authz_and_tenant_isolation.md](./06_authz_and_tenant_isolation.md)
7. [07_open_source_skill_installation.md](./07_open_source_skill_installation.md)
8. [08_stability_regression.md](./08_stability_regression.md)
9. 多智能体协作分层用例已迁移至：`docs/guides/agent/multi_agent/09_a2a_team_collab_progressive.md`

## 留证要求

每条用例记录：`trace_id`、执行时间、执行人、结论、失败请求/响应。

## 重要说明：问题模板必须匹配技能用途

1. `hello-echo` / `prompt-template` 这类技能用于回显或模板渲染，不适合 incident 分析问题。
2. `incident-triage` 才适合 `INC-1001` 这类事故分析与修复建议问题。
3. 若问题语义与技能用途不匹配，Agent 可能走普通对话或命中非预期节点，导致测试结论失真。
