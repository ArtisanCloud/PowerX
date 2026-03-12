# 管理员生命周期管理（Skills）

本文用于管理员（`admin root`）执行 Skills 生命周期闭环：
登记 -> 发布 -> 回滚 -> 绑定 Capability。

## 步骤 1：查看官方目录

```bash
curl -sS "$POWERX_HTTP_BASE/admin/skills/catalog" \
  -H "Authorization: Bearer $ROOT_TOKEN"
```

预期：返回 `items` 列表（`skill_id/recommended_version/risk_level`）。

## 步骤 2：登记 Skill（draft）

```bash
curl -sS -X POST "$POWERX_HTTP_BASE/admin/skills" \
  -H "Authorization: Bearer $ROOT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "skill_id":"skill.demo.lifecycle",
    "version":"1.0.0",
    "source":"plugin",
    "bundle_ref":{"uri":"s3://skills/skill.demo.lifecycle-1.0.0.tgz","checksum":"sha256:abc123"},
    "manifest":{"name":"demo lifecycle"}
  }'
```

预期：HTTP `201`，状态为 `draft`。

## 步骤 3：查询 registry

```bash
curl -sS "$POWERX_HTTP_BASE/admin/skills?skill_id=skill.demo.lifecycle" \
  -H "Authorization: Bearer $ROOT_TOKEN"
```

预期：可看到刚登记版本。

## 步骤 4：发布版本

```bash
curl -sS -X POST "$POWERX_HTTP_BASE/admin/skills/skill.demo.lifecycle/publish" \
  -H "Authorization: Bearer $ROOT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"version":"1.0.0","approval_note":"approved by root"}'
```

预期：HTTP `200`，返回 `status=published`。

## 步骤 5：回滚版本

先准备并发布第二个版本（如 `2.0.0`），再回滚：

```bash
curl -sS -X POST "$POWERX_HTTP_BASE/admin/skills/skill.demo.lifecycle/rollback" \
  -H "Authorization: Bearer $ROOT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"target_version":"1.0.0","reason":"regression rollback"}'
```

预期：HTTP `200`，`is_latest_published=true` 指向回滚目标版本。

## 步骤 6：绑定 Capability

```bash
curl -sS -X POST "$POWERX_HTTP_BASE/admin/skills/skill.demo.lifecycle/bind-capability" \
  -H "Authorization: Bearer $ROOT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"version":"1.0.0","capability_id":"cap.skill.demo","tool_grants":["grant.read"]}'
```

预期：HTTP `200`，返回 `binding_id` 与 `status`。

## Web Admin 操作路径

1. 打开 `设置 -> AI -> Skills`。
2. 在 Registry 区域查看列表，执行发布/回滚。
3. 在导入区完成 bundle 导入（见导入手册）。
4. 点击“审计 -> 查看”打开审计抽屉。
