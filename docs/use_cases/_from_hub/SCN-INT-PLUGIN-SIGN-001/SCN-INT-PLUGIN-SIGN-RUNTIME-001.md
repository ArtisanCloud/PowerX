---
scn_id: SCN-INT-PLUGIN-SIGN-RUNTIME-001
title: 安装与运行时签名验证
status: Draft
version: v0.1.0
owners:
  - name: Michael Hu
    role: Plugin Tech Lead
    contact: tech@artisan-cloud.com
domains: [integration]
layers: [ops]
repos:
  - key: powerx
    scope: runtime-security
    responsibility: 安装器、运行时校验、隔离策略
related_usecases:
  - doc_id: UC-INT-PLUGIN-SIGN-RUNTIME-001
    layer: ops
    domain: integration
last_reviewed_at: 2025-02-21

---

# Executive Summary

> 在租户安装和插件运行过程中持续校验签名与哈希，阻止未签名或被篡改的插件加载，并记录可追溯的验证日志。

# Scope & Guardrails

- **In Scope**：安装器校验、运行时周期性校验、证书状态检查、失败阻断与隔离、租户策略、日志。
- **Out of Scope**：构建签名、SOC 事件响应。
- **Environment & Flags**：`PX_PLUGIN_RUNTIME_VERIFY`, `PX_PLUGIN_INSTALL_GUARD`, `PX_PLUGIN_TENANT_POLICY`。

# End-to-End Flow

1. 安装器下载插件包 → 校验签名/哈希 → 验证租户策略。
2. 成功后安装并记录验证日志/证书指纹；失败则阻断。
3. 运行时在加载/更新/定期巡检时重复校验，检测文件被替换。
4. 校验失败触发隔离、自动卸载或提醒租户更新证书。

# Key Interactions & Contracts

- `POST /host/plugins/install`、`POST /host/plugins/verify`, `GET /host/plugins/:id/cert`。
- Logs：`plugin.runtime.verify.pass/fail`, `plugin.runtime.isolation`。

# Acceptance Criteria

1. 未签名或失效签名插件无法安装，阻断记录包含租户、插件、证书信息。
2. 运行时校验覆盖加载/更新/巡检场景，检测到篡改时自动隔离。
3. 验证日志与租户策略关联，可用于审计。

# Telemetry & Ops

- 指标：`plugin.runtime.verify.fail_count`, `plugin.install.block_count`, `plugin.runtime.cert.expiring`。
- 告警：连续校验失败、证书即将过期、隔离动作失败。

# Open Issues & Follow-ups

| 风险/事项 | 影响 | 负责人 | ETA |
|-----------|------|--------|-----|
| 热更新/补丁场景未覆盖 | 可能绕过校验 | Runtime Platform Squad | 2025-03-08 |
| 租户自定义策略缺少 UI | 配置复杂 | Tenant Experience Squad | 2025-03-04 |
