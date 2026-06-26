# Skills 使用指南（Agent）

本目录提供 Skills 功能的分场景操作手册，覆盖管理、导入、调用一致性、审计与隔离。

统一页面入口：左侧菜单 `技能库`（导入安装在页面右上角按钮）。

开发设计入口：

1. [Agent Runtime 标准服务设计](../../../plan/ai_engineering/skills/agent_runtime_standard_services.md)
   定义 Core 为 Agent/Skill 提供的 session、context、skill state、capability invocation、trace、artifact、progress event、model policy 和权限服务。

2. [PowerX Agent Skill Bridge 机制设计](../../../plan/ai_engineering/skills/agent_skill_bridge.md)
   定义插件 Skill 如何通过 Agent Runtime 和 Capability Invocation 执行业务。

3. [Agent Run State Protocol 设计](../../../plan/ai_engineering/skills/agent_run_state_protocol.md)
   定义 Web Admin 与 PowerXPlugin 调试页统一展示任务状态、缺参、结果和 trace 的协议。

## 文档导航

1. [01-admin-lifecycle.md](./01-admin-lifecycle.md)  
   管理员如何完成 Skills 生命周期管理（登记、发布、回滚、绑定）。

2. [02-import-third-party-skill.md](./02-import-third-party-skill.md)  
   开发者/第三方如何受控导入 Skill（仅 upload 模式）。

3. [03-tenant-invoke-consistency.md](./03-tenant-invoke-consistency.md)  
   如何验证 Skill 映射到 capability 后通过 `tenant/invocations` 统一执行，并与 Agent 主入口的 trace/status/result 对齐。

4. [04-audit-and-isolation.md](./04-audit-and-isolation.md)  
   如何查询 Skills 审计、执行 trace，并验证跨租户隔离。

5. [05-import-and-install.md](./05-import-and-install.md)  
   如何区分 upload 导入与 install-tasks 仓库安装，并完成任务状态追踪。

6. [06-candidate-layering-and-composite-plan.md](./06-candidate-layering-and-composite-plan.md)  
   如何回归验证 `system+agent` 候选去重优先级、未授权候选不可见，以及 `workflow->skill/tooling` 组合规划事件字段。

7. [test_use_cases/README.md](./test_use_cases/README.md)  
   从简单到复杂（L1-L8）的开发验收与操作测试用例。

## 统一前置条件

1. 服务已启动，且可访问 API（示例：`http://localhost:8080/api/v1`）。
2. 已有可用 JWT：
   - 管理端接口：需要 `admin root` 用户。
   - 租户端接口：需要带租户上下文的 token。
3. 建议准备环境变量：
   - `POWERX_HTTP_BASE=http://localhost:8080/api/v1`
   - `ROOT_TOKEN=<root_jwt>`
   - `TENANT_TOKEN=<tenant_jwt>`

## 快速回归命令（T080/T081）

```bash
cd backend && GOCACHE=$PWD/.gocache GOMODCACHE=$PWD/.gomodcache \
go test ./tests/integration/skills \
-run 'TestSkillAgentCandidateLayering_SystemAgentDedupeAndAuthzFilter|TestSkillAgentCompositePlanExecuteWithEventSourceScope' \
-count=1
```
