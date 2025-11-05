doc_id: UC-DEV-PLUGIN-OFFLINE-MARKETPLACE-001
scn_id: SCN-DEV-PLUGIN-PUBLISH-001
title: 离线包送审与 Marketplace 入库
status: Draft
version: v0.1.0
repo_key: powerx-marketplace
scope: powerx-marketplace
layer: marketplace
domain: dev
scenario_title: "离线包送审与 Marketplace 入库"
owners:
  - name: Ivy Chen
    role: Marketplace Operations Lead
    contact: marketplace@artisan-cloud.com
  - name: Grace Lin
    role: Security & Compliance Lead
    contact: compliance@artisan-cloud.com
contributors: []
linked_requirements:
  - SCN-DEV-PLUGIN-OFFLINE-MARKETPLACE-001
code_refs:
  - repo: powerx-plugin
    path: packages/cli/src/commands/plugin/pack.ts
    description: 离线打包命令、签名与依赖清单生成
  - repo: powerx-marketplace
    path: apps/market/src/modules/offline-upload/index.tsx
    description: 离线上传界面、字段校验、补件提示
  - repo: powerx-marketplace
    path: internal/review/offline_pipeline.go
    description: 审核工作流、签名/兼容校验、补件工单、SLA
  - repo: powerx
    path: internal/security/signature/validator.go
    description: 签名验证、证书轮换、许可证校验与告警
feature_flags:
  - marketplace-offline-upload
  - plugin-offline-package
optional: false
last_reviewed_at: 2025-11-20

---

# Usecase Overview

- **业务目标**：为无公网或弱网环境提供标准化离线上传与审核流程，确保包体签名、兼容、许可证合规并快速入库。
- **成功度量**：签名校验通过率 ≥99%；审核 SLA ≤48 小时；补件率 <5%；离线分发库同步延迟 <30 分钟。
- **场景关联**：与主场景 Stage F 对齐，保证离线链路与线上生态一致。

# Context & Assumptions

- Feature Flags `plugin-offline-package`、`marketplace-offline-upload` 已启用。
- 签名、许可证服务与离线分发库可访问。
- Vendor 已准备完整的版本说明、依赖/兼容清单与合规文档。
- 审核团队具备补件沟通渠道与权限。

# Solution Blueprint

## 体系分解

| 层 | 组件 | 责任 | 代码入口 |
|----|------|------|---------|
| 打包层 | `packages/cli/src/commands/plugin/pack.ts` | 生成 `.pxp` 包、签名、依赖清单、校验文件 | `packages/cli` |
| 上传层 | `apps/market/src/modules/offline-upload/index.tsx` | 上传 UI、元数据校验、补件提示 | `apps/market` |
| 审核层 | `internal/review/offline_pipeline.go` | 签名/兼容/许可证校验、补件任务、SLA 打点 | `services/review` |
| 安全层 | `internal/security/signature/validator.go` | 签名解析、证书轮换、许可证校验、告警 | `services/security` |
| 分发层 | `internal/marketplace/repo/offline_sync.go` | 入库离线分发库、生成指纹、监控下载 | `services/marketplace/repo` |

## 流程与时序

1. `px-plugin pack --sign` 生成离线包与签名。
2. 管理员在离线上传入口提交包体与元数据。
3. 审核流程校验签名、兼容矩阵、许可证并处理补件。
4. 审核通过后同步版本到离线分发库，返回下载指纹。

# Implementation Checklist

| 项目 | 描述 | 状态 | 负责人 |
|------|------|------|--------|
| 包体结构标准化 | 统一 `.pxp` 结构、依赖清单、指纹文件 | [ ] | Michael Hu |
| 审核流水线 | 并行校验、补件任务、SLA 指标 | [ ] | Ivy Chen |
| 许可证校验 | 多区域许可证策略映射 | [ ] | Grace Lin |
| 分发同步 | 增量同步、指纹记录、访问监控 | [ ] | Matrix Ops |
| 通知模板 | 多语言补件/通过模板、Webhook | [ ] | Ivy Chen |

# Testing Strategy

- 单元：打包参数解析、签名/许可证校验、元数据规则。
- 集成：`marketplace-offline-review.mjs` 覆盖成功与补件场景。
- 端到端：复现 Meta 用例 F（正向/逆向）。
- 非功能：大包体、断点续传、证书过期、并发审核。

# Observability & Ops

- 指标：`marketplace.offline.upload_success_rate`、`marketplace.offline.review_sla_hours`、`marketplace.offline.rework_rate`。
- 告警：签名失败率 >1%、审核超 SLA、补件率 >5%、分发库同步失败。
- 日志：审核记录、补件原因、签名指纹；写入 `marketplace_offline_review` index。

# Follow-ups & Risks

| 风险/事项 | 影响 | 缓解方案 | 负责人 | ETA |
|-----------|------|----------|--------|-----|
| 证书即将过期导致批量校验失败 | 审核阻断 | 证书轮换提醒与自动续期 | Grace Lin | 2025-12-23 |
| 离线分发库容量不足 | 下载稳定性 | 生命周期管理与分层存储 | Matrix Ops | 2026-01-08 |
| 补件沟通缺乏模板 | 工作量增加 | 统一运营工作流与模板化邮件 | Ivy Chen | 2025-12-20 |
