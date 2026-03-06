# L4 - Intent 候选 Skill 到 Planner 定案

## 目标

验证三层决策链：`Intent 候选 -> Planner 定案 -> Node 分发`。

## 前置条件

1. 至少两个语义相近的 skill（例如 `incident-triage`、`incident-summary`）
2. 已开启 Intent 策略链（rule/embedding/llm）

## 操作步骤

### 步骤 1：输入多候选语义消息

示例：“帮我排查这个事故并给一段简短总结。”

### 步骤 2：查看 Intent 输出

确认存在 `candidate_skills[]`（top-k）。

### 步骤 3：查看 Planner 结果

确认最终 plan 是否落了 `kind=skill` 节点，以及选择了哪个 skill。

### 步骤 4：执行并核对

确认真正执行的是 Planner 定案 skill，而不是所有候选 skill。

## 预期效果

1. Intent 给候选，不直接执行。  
2. Planner 综合上下文选择最终节点。  
3. Executor 仅按 `node.kind/use` 分发。

## 通过标准

1. 候选与定案可解释（有分数或理由）。  
2. 误选率在可接受阈值内（团队自定义）。

