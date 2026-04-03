# Skills 安全与治理规范

本文定义 Skill 的安全基线、授权边界与治理流程。

## 1. 来源可信策略

1. 首版仅支持“托管仓库 + 元数据注册”。
2. 禁止直接执行未托管远程 URL 的脚本。
3. 所有发布版本必须携带 `checksum`。
4. `signature` 可按环境策略启用强校验。

## 2. 执行权限边界

1. 默认最小权限，不隐式继承高危命令权限。
2. 需要外部系统操作时，必须声明副作用与权限需求。
3. 敏感操作要求显式授权（ToolGrant/Role）。

## 3. 授权模型

1. Skill 可绑定 capability，再复用现有 ToolGrant 策略。
2. Tenant 调用时必须通过：
   - tenant 身份检查
   - capability 可见性检查
   - tool grant 检查
3. safe mode 开启时可阻断高风险 skill。

## 4. 多租户隔离

1. 执行上下文必须注入 `tenant_uuid`。
2. 数据与审计按租户隔离。
3. 禁止跨租户读取执行记录。

## 5. 审计规范

每次调用至少记录：

- `trace_id`
- `tenant_uuid`
- `skill_id`
- `version`
- `entrypoint`
- `status`
- `latency_ms`
- `error_summary`
- `source`

## 6. 风险分级（建议）

- `L1` 只读技能：文档总结、文本处理
- `L2` 外部读取：查询接口、数据检索
- `L3` 有副作用：写操作、工单、通知
- `L4` 高风险：生产变更、权限修改

不同等级应匹配不同审批与授权要求。

