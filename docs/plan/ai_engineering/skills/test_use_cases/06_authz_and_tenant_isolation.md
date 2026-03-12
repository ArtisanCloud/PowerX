# L6 - 权限与租户隔离

## 目标

验证 ToolGrant、租户隔离、安全模式在 Skill 调用中的生效情况。

## 前置条件

1. 准备两个租户：`TENANT_A`、`TENANT_B`
2. 仅 `TENANT_A` 拥有目标 skill 的 grant

## 操作步骤

### 步骤 1：TENANT_A 调用（有 grant）

预期成功，返回 `completed`。

### 步骤 2：TENANT_B 调用（无 grant）

预期拒绝，返回权限错误码（如 `skill.permission_denied`）。

### 步骤 3：跨租户查 trace

用 `TENANT_B` 去查 `TENANT_A` 的 trace，预期拒绝或 not found。

### 步骤 4：打开 safe mode 后再调用

验证高风险 skill 被阻断。

## 预期效果

1. 授权与未授权行为差异明确。  
2. 跨租户访问不可见。  
3. safe mode 能生效阻断。

## 通过标准

1. 无“越权成功”样例。  
2. 所有拒绝都有可追踪错误码与审计记录。

