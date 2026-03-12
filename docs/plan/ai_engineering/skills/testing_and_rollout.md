# Skills 测试与上线计划

本文定义 Skill 功能的测试矩阵、灰度发布与回滚策略。

## 1. 测试目标

1. 确保 `SKILL.md` 解析与校验稳定。
2. 确保双路径调用（Agent/Gateway）结果一致。
3. 确保授权、安全、租户隔离正确。

## 2. 测试矩阵

### 2.1 单元测试

1. Manifest 字段映射
2. 版本状态机迁移
3. 错误码与异常分类

### 2.2 集成测试

1. Admin 注册 -> 发布 -> 回滚全流程
2. Tenant 调用 `skills/invoke`
3. `tenant/invocations + preferred_protocol=skill`

### 2.3 契约测试

1. API 请求响应 JSON 结构
2. 错误码稳定性
3. 鉴权错误语义稳定性

### 2.4 回归测试

1. 不影响现有 `http/grpc/mcp/agent` 路由
2. 不破坏现有 capability selector 行为

## 3. 验收标准

1. 技能注册成功率达到既定目标（示例：99.9%）。
2. 调用链 trace 可完整回放。
3. 授权拒绝有明确错误码与审计记录。
4. 回滚可在不删历史的前提下完成。

## 4. 灰度策略

1. 灰度开关：按租户与环境控制。
2. 先灰度只读 Skill，再灰度有副作用 Skill。
3. 观察指标通过后全量开启。

## 5. 监控指标（建议）

- `skill_invocations_total`
- `skill_invocation_error_total`
- `skill_invocation_latency_ms`
- `skill_registry_publish_total`
- `skill_registry_rollback_total`

## 6. 回滚策略

1. 配置回滚：关闭 `protocol=skill` 选择权重。
2. 版本回滚：切换到上一个 `published` 版本。
3. 紧急回滚：将目标版本标记 `disabled`。

## 7. 里程碑

1. M1：文档与契约冻结
2. M2：注册与管理接口上线
3. M3：双路径运行时打通
4. M4：插件/第三方接入上线
5. M5：全量发布与运维交接

