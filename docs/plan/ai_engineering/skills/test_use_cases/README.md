# Skills 测试用例索引（独立文件版）

本目录将每个 test use case 拆成独立文档，便于逐条执行、分工与回归。

## 1. 用例清单（从简单到复杂）

1. [01_manifest_parse.md](./01_manifest_parse.md)  
L1：Skill Manifest 解析与入库

2. [02_publish_and_rollback.md](./02_publish_and_rollback.md)  
L2：发布、升级、回滚状态机

3. [03_agent_minimal_skill_execution.md](./03_agent_minimal_skill_execution.md)  
L3：Agent 内最小 skill 执行链路

4. [04_intent_to_planner_decision.md](./04_intent_to_planner_decision.md)  
L4：Intent 候选 skill 到 Planner 定案

5. [05_gateway_protocol_skill.md](./05_gateway_protocol_skill.md)  
L5：统一入口 `preferred_protocol=skill`

6. [06_authz_and_tenant_isolation.md](./06_authz_and_tenant_isolation.md)  
L6：权限与租户隔离

7. [07_open_source_skill_installation.md](./07_open_source_skill_installation.md)  
L7：开源 Skill 包安装到 PowerX

8. [08_stability_regression.md](./08_stability_regression.md)  
L8：稳定性与全链路回归

## 2. 统一执行要求

1. 每条用例都要记录：`trace_id`、执行时间、执行人、结论。  
2. 失败必须附：请求参数、错误码、关键日志片段。  
3. 未达到“通过标准”不得进入下一层用例。

## 3. 建议执行顺序

1. 首轮：L1 -> L5（主功能链路）。  
2. 次轮：L6 -> L8（安全、接入、稳定性）。  
3. 发版前：至少重跑 L5、L6、L8。
