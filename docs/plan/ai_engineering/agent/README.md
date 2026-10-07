# Agent 设计文档

本目录承载 PowerX Agent Runtime、运行态协议、原生 Agent 与多 Agent 的整体设计；它们不是某一个 Skill 的附属说明。

## 阅读入口

- [Agent Runtime 闭环设计](./agent_runtime_loop_design.md)：面向业务 Agent 的观察、规划、执行、验证与受控演进目标架构。
- [Agent Runtime 实施计划](./agent_runtime_implementation_plan.md)：基于当前代码的差距、代码改造顺序、合同、验收与首个实施切片。
- [持久化调度与模型容量](./agent_runtime_durable_scheduling.md)：Redis 默认运行态、跨实例队列、资源池、预算、恢复与验收门槛。
- [Runtime 标准服务](./agent_runtime_standard_services.md)：运行时资源观察、计划控制、验证与恢复服务的边界。
- [回复规划](./agent_response_planning.md)：在向用户回复前完成恢复、校验与可解释输出的规则。
- [运行状态协议](./agent_run_state_protocol.md)：Run、阶段、计划修订与资源快照事件的协议。
- [运行追踪报告](./agent_run_trace_report.md)：可观测性、证据与恢复轨迹的展示约定。
- [Agent 与 Skill 桥接](./agent_skill_bridge.md)：Agent Runtime 如何调用、约束和沉淀 Skill。
- [多 Agent A2A 协议](./multi_agent_a2a.md)：Agent 间协作的正式协议。
- [多 Agent 设计](./multi-agent/README.md)：多 Agent 场景、实现映射与目录说明。
- [原生 Agent 设计](./native-agent/README.md)：原生 Agent 的机制、规则、样例与规格。

## 与 Skills 的边界

`../skills/` 只维护 Skill 的定义、注册、运行、治理、测试与对外契约。Agent Runtime 可以优先选择和执行已发布 Skill，但 Runtime 的计划循环、资源观察、失败恢复和多 Agent 协作在本目录维护。
