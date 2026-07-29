# 原生智能体运行机制

## 1. 总体机制

PowerX 原生智能体通过“Agent 入口 + Workflow 编排 + Skill 执行 + Capability 落地 + Knowledge Space 沉淀”的链路运作。

```text
用户/专家/管理员
  -> Agent
  -> Workflow
  -> Skill / Capability / Human Review
  -> Knowledge Space / Plugin / Core Service
  -> Audit / Trace / Version
```

Agent 不直接承载复杂流程。Agent 负责理解业务意图、选择合适的知识库和流程入口，并把长流程任务交给 Workflow。Workflow 负责状态、节点编排、人工审核、重试、补偿、发布和回滚。Skill 负责可复用的单项处理能力。Capability 负责真正调用底座或插件服务。

## 2. Agent 的职责

Agent 是业务入口和职责边界。

Agent 负责：

1. 定义岗位或领域 persona。
2. 绑定可用 Skill、Workflow、Knowledge Space 和权限。
3. 接收用户输入并生成任务计划。
4. 在必要时向用户追问缺失信息。
5. 汇总执行结果并给出业务可理解输出。

Agent 不负责：

1. 保存知识库真相源。
2. 执行长事务状态机。
3. 绕过 Capability Registry 直接调用底层服务。
4. 把未审核材料直接写入正式知识库。

## 3. Skill 与 Workflow 的关系

Skill 和 Workflow 是互补关系，不是二选一。

| 维度 | Skill | Workflow |
| --- | --- | --- |
| 粒度 | 单项能力或任务手册 | 多步骤业务流程 |
| 状态 | 轻状态，围绕一次调用 | 长状态，围绕流程实例 |
| 复用 | 高复用，可被多个 Agent/Workflow 调用 | 复用为业务流程模板 |
| 典型动作 | OCR、转写、摘要、分类、冲突检测、知识卡片生成 | 采集、解析、审核、入库、发布、回滚 |
| 治理重点 | 版本、输入输出 schema、executor、授权 | 节点状态、人工审核、重试、补偿、SLA |

允许的组合方式：

1. Agent 直接调用 Skill：仅适合无长状态、无审核、无发布动作的一次性任务，例如“把这段录音整理成会议纪要”。
2. Workflow 调用多个 Skill：专家知识库持续迭代必须使用该方式。
3. Skill 调用 Capability：Skill 作为语义层，Capability 作为执行层。

规则：只要流程涉及知识库 staging、human review、publish、version、rollback、跨节点重试或补偿，就必须建模为 Workflow，不允许用一个编排型 Skill 替代 Workflow。

## 4. 知识库迭代流程

专家知识库的标准迭代流程：

1. 采集：专家上传文字、图片、录音、文件、链接或业务记录。
2. 解析：调用转写、OCR、文件解析、表格解析等 Skill。
3. 结构化：抽取事实、概念、方法论、SOP、案例、决策规则。
4. 对齐元数据：匹配分类、标签、字典、资源类型和领域模板。
5. 冲突检测：识别与已有知识的重复、冲突、过期和空白。
6. 生成草稿：写入 staging knowledge draft，不直接进入正式知识库。
7. 人工审核：专家或负责人确认新增、合并、替换、废弃。
8. 发布入库：写入 Knowledge Space，更新 chunk、embedding、图谱和引用。
9. 版本审计：记录来源、审核人、版本、变更摘要和回滚点。

## 5. Flow 与 Workflow 边界

PowerX 已有的 Event Fabric、Task Queue、Scheduler 属于底座 Flow，负责可靠投递、重试和系统事件处理。

智能体 Workflow 属于租户业务编排，负责串联 Agent、Skill、Capability 和人工审核。

规则：

1. 系统级可靠性用 Flow。
2. 租户业务流程用 Workflow。
3. Workflow 可以消费 Flow 事件，也可以向 Flow 回推状态事件。
4. 知识库迭代不得使用 Skill + Task Queue 作为替代编排路径；Workflow runtime 不可用时，相关 Agent/页面/能力必须预检失败并提示缺失依赖。

## 6. 治理原则

1. 所有原生智能体必须有明确职责边界。
2. 所有可执行动作必须来自已注册 Skill 或 Capability。
3. 所有知识库写入必须经过 staging/review/version。
4. 所有业务对象引用使用 UUID。
5. 所有用户可见名称显示业务名称，不用 UUID 作为主标签。
6. 所有运行链路必须有 trace_id、tenant_uuid、agent_uuid、skill_id/workflow_id。
