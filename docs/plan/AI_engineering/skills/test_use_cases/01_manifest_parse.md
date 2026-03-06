# L1 - Skill Manifest 解析与入库

## 目标

验证 `SKILL.md` 能被正确解析、校验并写入 `draft` 版本。

## 前置条件

1. 已有管理员 Token：`ADMIN_TOKEN`
2. 已启动后端：`API_BASE=http://127.0.0.1:8077/api/v1`
3. 准备一个最小 Skill 包（含 `SKILL.md`）

## 操作步骤

### 步骤 1：准备最小 `SKILL.md`

```yaml
name: incident-triage
version: 1.0.0
description: Incident triage workflow
entrypoints:
  - runbook.default
```

### 步骤 2：注册 Skill（示例）

```bash
curl -sS -X POST "$API_BASE/admin/skills" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "skill_id":"incident-triage",
    "version":"1.0.0",
    "source":"plugin",
    "bundle_ref":{"uri":"s3://powerx-skills/demo/incident-triage-1.0.0.tgz","checksum":"sha256:demo"},
    "manifest":{"description":"Incident triage workflow","entrypoints":["runbook.default"]}
  }'
```

### 步骤 3：查询详情

```bash
curl -sS -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$API_BASE/admin/skills/incident-triage"
```

## 预期效果

1. 注册成功，状态为 `draft`。  
2. `skill_id=incident-triage`，`version=1.0.0`。  
3. 返回结构中含 manifest 与 bundle_ref。

## 通过标准

1. 无 `skill.invalid_manifest` 错误。  
2. 可进入 L2 发布流程。

## 记录模板

- 执行人：  
- 执行时间：  
- trace_id：  
- 结果：通过 / 失败  
- 备注：

