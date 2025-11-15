---
scn_id: SCN-INT-PLUGIN-SIGN-BUILD-001
title: 构建产物签名与上传校验
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
    scope: plugin-signing
    responsibility: 签名 CLI/SDK、CI 集成、Marketplace 验证接口
related_usecases:
  - doc_id: UC-INT-PLUGIN-SIGN-BUILD-001
    layer: service
    domain: integration
last_reviewed_at: 2025-02-21

---

# Executive Summary

> CI/CD 必须对每个插件构建产物进行签名并在上传 Marketplace/分发平台时校验签名、哈希和证书状态，防止伪造包或篡改包进入生态。

# Scope & Guardrails

- **In Scope**：CI 签名脚本、哈希指纹、上传 API 验证、证书 CRL/OCSP 检查、审计。
- **Out of Scope**：证书申请（子场景 A）、运行时校验（子场景 C）、事件监控（子场景 D）。
- **Environment & Flags**：`PX_PLUGIN_ARTIFACT_SIGN`, `PX_PLUGIN_UPLOAD_VERIFY`, `PX_PLUGIN_HASH_LEDGER`。

# End-to-End Flow

1. 构建完成 → CI 调用 KMS `sign` 生成签名与哈希。
2. 上传插件包 + 签名 + 元数据至 Marketplace。
3. Marketplace 校验签名、哈希、证书吊销状态，通过后进入审核/发布。
4. 所有动作写入审计，并生成可查询的指纹。

# Key Interactions & Contracts

- `POST /signing/sign`, `POST /marketplace/plugins`、`POST /marketplace/plugins/:id/verify`。
- Artefacts：`.sig`, `.hash`, 证书序列号。
- Logs：`plugin.sign.upload.success/fail`, `plugin.sign.hash.collision`。

# Acceptance Criteria

1. 未签名/哈希不匹配的包无法上传；
2. 证书吊销/过期时上传被拒且告警；
3. 审计日志记录签名者、指纹、上传时间。

# Telemetry & Ops

- 指标：`artifact.sign.fail_rate`, `artifact.verify.fail_rate`, `artifact.hash.collision`。
- 告警：签名失败、验证失败率 >1%、哈希重复、吊销检查失败。

# Open Issues & Follow-ups

| 风险/事项 | 影响 | 负责人 | ETA |
|-----------|------|--------|-----|
| Marketplace 缺少哈希查询 API | 难以追踪版本 | Build Infra Squad | 2025-03-05 |
| CI 未强制签名脚本 | 可能漏签 | Dev Productivity Squad | 2025-02-28 |
