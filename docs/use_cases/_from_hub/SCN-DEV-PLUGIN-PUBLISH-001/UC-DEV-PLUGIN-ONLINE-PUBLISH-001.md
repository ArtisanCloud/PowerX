doc_id: UC-DEV-PLUGIN-ONLINE-PUBLISH-001
scn_id: SCN-DEV-PLUGIN-PUBLISH-001
title: 在线发布与 Marketplace 即时上架
status: Draft
version: v0.1.0
repo_key: powerx-marketplace
scope: powerx-marketplace
layer: marketplace
domain: dev
scenario_title: "在线发布与 Marketplace 即时上架"
owners:
  - name: Ivy Chen
    role: Marketplace Operations Lead
    contact: marketplace@artisan-cloud.com
  - name: Alex Wei
    role: Release Automation Engineer
    contact: automation@artisan-cloud.com
contributors: []
linked_requirements:
  - SCN-DEV-PLUGIN-ONLINE-PUBLISH-001
code_refs:
  - repo: powerx-plugin
    path: packages/cli/src/commands/plugin/publish.ts
    description: 发布命令、元数据校验、回执管理
  - repo: powerx-marketplace
    path: internal/review/online_pipeline.go
    description: 审核流程、补件任务、SLA 计时
  - repo: powerx-marketplace
    path: apps/market/src/modules/online-publish/index.tsx
    description: 发布表单、状态展示、补件提示
  - repo: powerx
    path: internal/publish/records/manager.go
    description: 发布记录、版本 diff、审计日志、通知关联
feature_flags:
  - plugin-online-publish
  - marketplace-review-v2
optional: false
last_reviewed_at: 2025-11-20

---

# Usecase Overview

- **业务目标**：为供应商提供在线发布通道，在自动校验签名、合规、安全与元数据后快速上架并同步通知。
- **成功度量**：发布成功率 ≥99%；审核 SLA ≤48 小时；通知延迟 ≤5 分钟；补件率 <8%。
- **场景关联**：主场景 Stage G，对应在线发布与 Marketplace 列表同步。

# Context & Assumptions

- 发布版本已通过测试/审批并具备签名与安全报告。
- Feature Flags `plugin-online-publish`、`marketplace-review-v2` 启用。
- 审核员与 Vendor Success 团队具备操作权限。
- 通知与报表服务可用。

# Solution Blueprint

## 体系分解

| 层 | 组件 | 责任 | 代码入口 |
|----|------|------|---------|
| 发布入口 | `packages/cli/src/commands/plugin/publish.ts` | 采集元数据、校验必填项、提交申请 | `packages/cli` |
| 审核管道 | `internal/review/online_pipeline.go` | 自动校验、人工审核、补件流程、SLA | `services/review` |
| 前端界面 | `apps/market/src/modules/online-publish/index.tsx` | 表单校验、状态展示、补件提示 | `apps/market` |
| 发布记录 | `internal/publish/records/manager.go` | 版本 diff、审计、通知关联、回执归档 | `services/publish/records` |
| 通知与报表 | `internal/listing/notification/broadcast.go` | 订阅通知、公告、初始运营报表 | `services/listing/notification` |

## 流程与时序

1. `px-plugin publish` 或 控制台提交版本、定价、支持策略。
2. 审核管道校验签名、兼容、合规、安全项并创建补件任务。
3. 审核员完成审批后更新 Marketplace 列表并同步发布记录。
4. 通知中心推送上线消息，生成初始运营报表并记录审计。

# Implementation Checklist

| 项目 | 描述 | 状态 | 负责人 |
|------|------|------|--------|
| CLI 校验 | 必填字段、智能提示、错误分类 | [ ] | Alex Wei |
| 审核流程 | 自动校验、补件任务、SLA 统计 | [ ] | Ivy Chen |
| 通知模板 | 多语言通知、订阅分群、Webhook | [ ] | Ivy Chen |
| 发布记录 | 版本 diff、审计链路、通知关联 | [ ] | Matrix Ops |

# Testing Strategy

- 单元：CLI 参数解析、审核状态机、通知模板。
- 集成：`marketplace-online-publish.mjs` 验证成功/补件路径。
- 端到端：复现 Meta 用例 G（正向/逆向）。
- 非功能：高并发发布、通知高峰、区域化定价。

# Observability & Ops

- 指标：`marketplace.online.publish_success_rate`、`marketplace.online.review_sla_hours`、`marketplace.notification.delivery_latency`。
- 告警：成功率 <99%、审核超 SLA、通知延迟 >5 分钟、补件率 >8%。
- 日志：审核决策、补件详情、上线回执，存入 `marketplace_online_publish` index。

# Follow-ups & Risks

| 风险/事项 | 影响 | 缓解方案 | 负责人 | ETA |
|-----------|------|----------|--------|-----|
| 区域化定价模板缺失 | 国际上线延迟 | 提供本地化模板与审校流程 | Ivy Chen | 2025-12-30 |
| CLI 缺少重复提交保护 | 数据不一致 | 增加防重复令牌与乐观锁 | Alex Wei | 2025-12-19 |
| 通知通道高峰延迟 | 租户触达体验 | 扩容队列、监控延迟 | Matrix Ops | 2025-12-24 |
