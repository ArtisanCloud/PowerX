doc_id: UC-INT-PLUGIN-SIGN-RUNTIME-001
scn_id: SCN-INT-PLUGIN-SIGN-001
title: 安装与运行时签名验证（ops/integration）
status: Draft
version: v0.1.0
repo_key: powerx
scope: powerx
layer: ops
domain: integration
scenario_title: "插件签名与验证"
owners:
  - name: Michael Hu
    role: Plugin Tech Lead
    contact: tech@artisan-cloud.com
contributors:
  - name: Grace Lin
    role: Security & Compliance Lead
    contact: compliance@artisan-cloud.com
linked_requirements:
  - id: SCN-INT-PLUGIN-SIGN-RUNTIME-001
    description: 子场景《安装与运行时签名验证》
code_refs:
  - repo: powerx
    path: services/runtime/plugin_installer/verify_service.ts
    description: 下载后签名/哈希校验、租户策略比对、阻断逻辑
  - repo: powerx
    path: services/runtime/plugin_guard/runtime_checker.ts
    description: 运行时周期性校验、文件变更检测、隔离动作
feature_flags:
  - PX_PLUGIN_RUNTIME_VERIFY
  - PX_PLUGIN_INSTALL_GUARD
  - PX_PLUGIN_TENANT_POLICY
optional: false
last_reviewed_at: 2025-02-21

---

# Usecase Overview

- **业务目标**：在插件安装、更新与运行过程中持续校验签名/哈希，确保未签名、失效或被篡改的插件无法进入租户环境，并提供审计与隔离能力。
- **成功度量**：未签名/失效签名阻断率 100%；运行时校验失败触发 P1 告警；验证日志 100% 可追溯；隔离/恢复操作自动化 ≥ 80%。
- **场景关联**：对应 `SCN-INT-PLUGIN-SIGN-RUNTIME-001`，与证书/构建/响应子场景协同形成供应链防线。

# Context & Assumptions

- Feature Flags `PX_PLUGIN_RUNTIME_VERIFY`, `PX_PLUGIN_INSTALL_GUARD`, `PX_PLUGIN_TENANT_POLICY` 已开启。
- 安装器与运行时可访问 KMS/CRL/OCSP，获取证书状态；租户策略已配置。
- 插件包随上传阶段提供签名/哈希与指纹元数据。

# Solution Blueprint

## 体系分解

| 层 | 模块 | 责任 | 代码入口 |
|----|------|------|---------|
| Installer Verify Service | 校验签名/哈希、租户策略、阻断未签名插件 | `services/runtime/plugin_installer/verify_service.ts` |
| Runtime Checker | 定期/加载时校验文件完整性、触发隔离 | `services/runtime/plugin_guard/runtime_checker.ts` |
| Policy Manager | 租户策略、白名单、日志 | (共享 `tenant_policy` 服务) |

## 流程与时序

1. **Install Verify**：安装器下载插件→校验签名/哈希/证书→匹配租户策略→记录验证日志。
2. **Block/Allow**：成功则继续安装并记录指纹；失败阻断并告警。
3. **Runtime Check**：运行时在加载/更新/巡检时重复校验，检测文件变更。
4. **Isolate & Audit**：校验失败→隔离或卸载插件→写入审计并通知租户。

```mermaid
sequenceDiagram
  participant Installer
  participant VerifySvc
  participant RuntimeGuard

  Installer->>VerifySvc: 下载包 + 签名数据
  VerifySvc-->>Installer: 验证通过/失败
  RuntimeGuard->>VerifySvc: 周期性校验
  VerifySvc-->>RuntimeGuard: 结果 + 指纹
  RuntimeGuard->>Installer: 隔离/恢复
```

# Contracts & Interfaces

- `POST /host/plugins/install`（含签名校验结果）、`POST /host/plugins/verify`、`POST /host/plugins/isolate`。
- Config：`runtime_verify_policy.yaml`, `tenant_plugin_policy.yaml`。

# Implementation Checklist

| 项目 | 描述 | 完成状态 | 负责人 |
|------|------|----------|--------|
| Installer Verify | 下载后签名/哈希校验、证书状态 | [ ] | Runtime Platform Squad |
| Runtime Checker | 周期性校验、隔离动作、日志 | [ ] | Runtime Platform Squad |
| Policy/Logging | 租户策略、审计、告警 | [ ] | Security Ops Squad |

# Testing Strategy

- 单元：证书校验、哈希比对、租户策略决策。
- 集成：安装正向/逆向、运行时篡改检测、隔离/恢复。
- 端到端：沙箱插件安装→运行→异常注入→隔离→复原。

# Observability & Ops

- 指标：`plugin.install.block_count`, `plugin.runtime.verify.fail_count`, `plugin.runtime.isolation.count`。
- 告警：连续校验失败、证书过期、隔离失败。

# Rollback & Failure Handling

- 校验服务故障→启用“只读缓存”模式并记录；
- 误报隔离→白名单恢复并重新验证；
- 证书过期→通知租户/插件提供者更新。

# Follow-ups & Risks

| 风险/事项 | 影响 | 缓解方案 | 负责人 | ETA |
|-----------|------|----------|--------|-----|
| 热补丁场景未覆盖 | 可绕过校验 | Runtime Platform Squad | 2025-03-08 |
| 租户策略缺 UI | 配置难度大 | Tenant Experience Squad | 2025-03-04 |

# References & Links

- 场景：`docs/scenarios/integration/SCN-INT-PLUGIN-SIGN-001.md`
- 子场景：`docs/scenarios/integration/SCN-INT-PLUGIN-SIGN-RUNTIME-001.md`
- 配置：`runtime_verify_policy.yaml`
- 发布指引：`npm run publish:usecases -- --scn-id SCN-INT-PLUGIN-SIGN-001 --validate-only`
