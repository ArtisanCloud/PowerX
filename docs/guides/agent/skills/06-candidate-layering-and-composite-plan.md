# 候选分层与组合规划回归指南（T080/T081）

本文用于对齐当前实现后的可测试指导，覆盖两类关键回归：

1. `T080`：`system + agent` 同名候选去重优先级、未授权候选不可见。
2. `T081`：组合规划 `workflow->skill/tooling` 可执行，且事件含 `node_kind/node_ref/source_scope`。

## 1. 前置条件

1. 已拉取包含 `T080/T081/T082` 的最新代码。
2. 在仓库根目录执行测试。
3. Go 本地缓存使用项目内目录（避免系统目录权限问题）。

## 2. 一键执行（推荐）

```bash
cd backend && GOCACHE=$PWD/.gocache GOMODCACHE=$PWD/.gomodcache \
go test ./tests/integration/skills \
  -run 'TestSkillAgentCandidateLayering_SystemAgentDedupeAndAuthzFilter|TestSkillAgentCompositePlanExecuteWithEventSourceScope' \
  -count=1
```

预期：输出 `ok github.com/ArtisanCloud/PowerX/tests/integration/skills`。

## 3. 测试点说明

## 3.1 T080：候选分层与授权过滤

文件：`backend/tests/integration/skills/skill_agent_candidate_layering_test.go`

验证点：

1. 同名候选（`workflow.same`）冲突时，保留 `agent custom`，覆盖 `system builtin`。
2. 未授权候选（source/tool_grants 不满足）不会进入最终候选池。
3. 已授权候选仍可见，且 `node_kind/node_ref` 与预期一致。

## 3.2 T081：组合规划与事件字段

文件：`backend/tests/integration/skills/skill_agent_composite_plan_test.go`

验证点：

1. 计划中同时出现 `skill`、`tooling`、`workflow` 三类节点。
2. `workflow` 节点依赖 `skill/tooling` 节点（组合规划有效）。
3. `node_start/node_end` 事件包含非空的 `node_kind/node_ref/source_scope`。

## 4. 失败排查

1. 报缓存目录权限错误（如 `operation not permitted`）：
   使用本指南的 `GOCACHE/GOMODCACHE` 命令重新执行。
2. 报测试未找到：
   确认分支已包含以下文件：
   - `backend/tests/integration/skills/skill_agent_candidate_layering_test.go`
   - `backend/tests/integration/skills/skill_agent_composite_plan_test.go`
3. 事件字段断言失败：
   检查 Agent runtime 是否透传 `source_scope` 到 `node_start/node_end` payload。
