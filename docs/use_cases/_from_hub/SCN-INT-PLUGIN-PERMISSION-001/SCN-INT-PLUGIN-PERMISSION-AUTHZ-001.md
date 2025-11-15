---
scn_id: SCN-INT-PLUGIN-PERMISSION-AUTHZ-001
title: 安装授权与租户策略落地
status: Draft
version: v0.1.0
owners:
  - name: Michael Hu
    role: Plugin Tech Lead
    contact: tech@artisan-cloud.com
domains: [integration]
layers: [service]
repos:
  - key: powerx
    scope: tenant-policy
    responsibility: 授权向导、策略生成、租户隔离
related_usecases:
  - doc_id: UC-INT-PLUGIN-PERMISSION-AUTHZ-001
    layer: service
    domain: integration
last_reviewed_at: 2025-02-22

---

# Executive Summary

> 租户管理员在安装/更新插件时需要可视化授予权限，并将配置转化为租户策略，保证细粒度隔离与审计。

# Scope & Guardrails

- **In Scope**：安装向导、权限说明、策略生成/同步、撤销与复核。
- **Out of Scope**：Manifest 审核、运行时校验、越权响应。
- **Environment & Flags**：`PX_PLUGIN_TENANT_POLICY`, `PX_PLUGIN_AUTHZ_WIZARD`。

# End-to-End Flow

1. 安装向导展示权限清单、用途、数据范围。
2. 管理员选择授权范围（组织/租户实例/数据分类）。
3. 生成租户策略 `policy-<tenant>-<plugin>` 写入策略引擎并激活。
4. 推送审计记录与复核计划。

# Key Interactions & Contracts

- `POST /tenant-policies`, `PATCH /tenant-policies/:id`, `DELETE /tenant-policies/:id`, `POST /tenant-policies/:id/review`。

# Acceptance Criteria

1. 授权过程有可视化确认，未确认不生效；
2. 策略与租户/插件绑定，支持有效期；
3. 审计记录授权人、范围、理由。

# Telemetry & Ops

- 指标：`tenant.policy.sync_latency`, `tenant.policy.revoke.count`, `tenant.policy.review.complete_rate`。
- 告警：策略同步失败、授权冲突。

# Open Issues & Follow-ups

| 风险/事项 | 影响 | 负责人 | ETA |
|-----------|------|--------|-----|
| 租户策略 UI 未支持批量更新 | 大租户配置繁琐 | Platform Policy Squad | 2025-03-10 |
