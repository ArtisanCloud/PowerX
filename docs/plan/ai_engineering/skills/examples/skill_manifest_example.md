# Skill Manifest 示例

## 1. 标准 SKILL.md 示例

```yaml
name: incident-triage
version: 1.0.0
description: Incident triage and summary workflow
scope: ops
inputs:
  incident_id:
    type: string
    required: true
outputs:
  summary:
    type: string
dependencies:
  - rg
  - go
entrypoints:
  - runbook.default
references:
  - https://example.com/runbook/incident-triage
```

## 2. PowerX 扩展示例

```yaml
name: incident-triage
version: 1.0.0
description: Incident triage and summary workflow
entrypoints:
  - runbook.default
x_powerx:
  source: plugin
  tenant_scope: tenant
  bundle_ref:
    uri: s3://powerx-skills/plugin-a/incident-triage-1.0.0.tgz
    checksum: sha256:8ec2f0...
  policy:
    tool_grants:
      - ops.incident.read
      - ops.incident.comment.write
```

## 3. Tenant 调用示例

```json
{
  "skill_id": "incident-triage",
  "version": "1.0.0",
  "payload": {
    "incident_id": "INC-1001"
  }
}
```

## 4. 统一入口调用示例

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

