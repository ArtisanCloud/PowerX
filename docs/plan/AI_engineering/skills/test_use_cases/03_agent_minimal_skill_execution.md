# L3 - Agent 内最小 Skill 执行链路

## 目标

验证 `node.kind=skill` 能被正确分发到 SkillRunner，并返回统一结果。

## 前置条件

1. 已有发布版 skill（如 `incident-triage:1.0.0`）
2. Agent Runtime 已启动

## 操作步骤

### 步骤 1：准备最小 flow（包含 skill 节点）

flow 仅需一条 skill 节点，参数带 `skill_id/version`。

### 步骤 2：发送一条触发消息到 agent 对话入口

通过现有聊天入口发起请求，并在上下文中绑定该 flow。

### 步骤 3：观察流式事件

检查 SSE/WS 事件中 `data.metadata` 是否含 `skill_id/version`。

## 预期效果

1. 节点分发命中 SkillRunner。  
2. 返回 `status/output/latency` 等统一字段。  
3. 产生 trace 与审计记录。

## 通过标准

1. 非 skill 执行器未误触发。  
2. 前端或调用方可直接消费返回结果。

