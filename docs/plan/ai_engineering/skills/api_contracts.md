# Skills API 契约（Admin / Tenant / Plugin）

本文定义 Skill 管理与调用接口契约，供后端、前端、插件侧统一实现。

## 1. Admin API

### 1.1 注册 Skill

`POST /api/v1/admin/skills`

请求体（示例）：

```json
{
  "skill_id": "incident-triage",
  "version": "1.0.0",
  "source": "plugin",
  "bundle_ref": {
    "uri": "s3://powerx-skills/plugin-a/incident-triage-1.0.0.tgz",
    "checksum": "sha256:xxxx"
  },
  "manifest": {
    "description": "故障分诊流程",
    "entrypoints": ["runbook.default"]
  }
}
```

### 1.2 查询 Skill 列表

`GET /api/v1/admin/skills?skill_id=&status=&source=&page=&page_size=`

### 1.3 发布与回滚

- `POST /api/v1/admin/skills/{skill_id}/publish`
- `POST /api/v1/admin/skills/{skill_id}/rollback`

## 2. Tenant API

### 2.1 直接调用 Skill

`POST /api/v1/tenant/skills/invoke`

```json
{
  "skill_id": "incident-triage",
  "version": "1.0.0",
  "payload": {
    "incident_id": "INC-1001"
  },
  "context": {
    "tool_scope": "ops"
  }
}
```

### 2.2 统一入口调用

`POST /api/v1/tenant/invocations` with `preferred_protocol=skill`

```json
{
  "capability_id": "com.powerx.skill.incident-triage.invoke",
  "preferred_protocol": "skill",
  "payload": {
    "skill_id": "incident-triage",
    "incident_id": "INC-1001"
  }
}
```

## 3. Plugin / 第三方接口

### 3.1 导入 Skill

`POST /api/v1/admin/skills/import`

### 3.2 绑定 capability

`POST /api/v1/admin/skills/{skill_id}/bind-capability`

```json
{
  "capability_id": "com.powerx.skill.incident-triage.invoke",
  "tool_grants": ["ops.incident.read"]
}
```

## 4. 统一响应模型

```json
{
  "trace_id": "trc_xxx",
  "status": "completed",
  "protocol_used": "skill",
  "fallback_used": false,
  "result": {
    "summary": "..."
  }
}
```

## 5. 错误码

- `skill.not_found`
- `skill.version_not_found`
- `skill.permission_denied`
- `skill.execution_failed`
- `skill.source_untrusted`

## 6. 鉴权与多租户

1. 所有 Tenant 调用必须从请求上下文解析 `tenant_uuid`。
2. 管理接口仅 Admin 可调用。
3. Skill 调用需通过 ToolGrant 或 Policy 检查。

