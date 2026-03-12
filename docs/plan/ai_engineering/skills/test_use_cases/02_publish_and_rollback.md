# L2 - 发布、升级、回滚状态机

## 目标

验证同一 `skill_id` 的多版本发布与回滚行为是否正确。

## 前置条件

1. 已完成 L1，存在 `incident-triage:1.0.0(draft)`
2. 管理员 Token 可用

## 操作步骤

### 步骤 1：发布 `1.0.0`

```bash
curl -sS -X POST "$API_BASE/admin/skills/incident-triage/publish" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"version":"1.0.0"}'
```

### 步骤 2：注册并发布 `1.1.0`

重复 L1 注册流程，版本改为 `1.1.0`，然后执行 publish。

### 步骤 3：回滚到 `1.0.0`

```bash
curl -sS -X POST "$API_BASE/admin/skills/incident-triage/rollback" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"target_version":"1.0.0"}'
```

## 预期效果

1. 版本并存：`1.0.0` 和 `1.1.0` 都存在。  
2. 回滚后默认使用 `1.0.0`。  
3. 历史不丢失，可审计。

## 通过标准

1. 发布与回滚都有审计记录。  
2. 回滚后调用链路实际命中目标版本。

