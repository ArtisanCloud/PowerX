# Quickstart — PowerX Skills 管理与治理

## 1. 前置条件

- Go 1.24、Node 20、Nuxt 4 开发环境可用
- PostgreSQL、Redis 已启动并可连接
- 已切换到分支 `024-ai-engineering-skills`

## 2. 合同准备

```bash
# 确认本 feature 的合同文件
ls specs/024-ai-engineering-skills/contracts/

# 后续实现阶段同步到 backend proto 合同时执行
make proto-lint
make proto-gen
```

## 3. 数据迁移准备（实现阶段）

```bash
# 迁移脚本接入后执行
make db-migrate
```

预期：Skills registry 与 trace/audit 相关表创建成功，重复执行幂等。

## 4. 管理侧最小闭环验收

1. 访问 Web Admin `设置 -> AI -> Skills`。
2. 查看“官方固有 Skills 目录”（来源为后端内置 catalog）。
3. 上传一个第三方 skill bundle，并填写 `source_url/source_ref` 元数据。
4. 记录应进入 `draft`。
5. 管理员执行人工审批发布。
6. 绑定 capability。

预期：发布前若 checksum 校验失败必须阻断。

## 5. 调用侧最小闭环验收

### 5.1 直接调用

```bash
curl -X POST "$POWERX_BASE_URL/api/v1/tenant/skills/invoke" \
  -H "Authorization: Bearer $TENANT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "skill_id": "incident-triage",
    "payload": {"incident_id": "INC-1001"}
  }'
```

### 5.2 统一入口调用

```bash
curl -X POST "$POWERX_BASE_URL/api/v1/tenant/invocations" \
  -H "Authorization: Bearer $TENANT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "capability_id": "com.powerx.skill.incident-triage.invoke",
    "preferred_protocol": "skill",
    "payload": {"skill_id": "incident-triage", "incident_id": "INC-1001"}
  }'
```

预期：两条路径返回一致的 `status` 语义，且带 `trace_id`。

## 6. 治理与审计检查

- 检查审计日志存在：`import/publish/rollback/bind_capability`。
- 检查调用 trace 包含：`tenant_uuid/skill_id/version/entrypoint/status`。
- 验证跨租户访问 trace 被拒绝。
- 验证未传 `version` 时路由到最新 published。

## 7. 回滚演练

1. 发布版本 `1.1.0`。
2. 触发异常后执行回滚到 `1.0.0`。
3. 再次调用验证默认版本已切回 `1.0.0`。

预期：历史版本保留，latest published 指针正确切换。
