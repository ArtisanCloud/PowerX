doc_id: UC-DEV-PLUGIN-LOCAL-DEBUG-001
scn_id: SCN-DEV-PLUGIN-PUBLISH-001
title: 本地调试与快速迭代保障
status: Draft
version: v0.1.0
repo_key: powerx-plugin
scope: powerx-plugin
layer: dev
domain: dev
scenario_title: "开发者本地调试链路"
owners:
  - name: Michael Hu
    role: Plugin Tech Lead
    contact: tech@artisan-cloud.com
  - name: Lily Zhang
    role: Developer Experience Engineer
    contact: devexp@artisan-cloud.com
contributors: []
linked_requirements:
  - SCN-DEV-PLUGIN-LOCAL-DEBUG-001
code_refs:
  - repo: powerx-plugin
    path: packages/cli/src/commands/plugin/build.ts
    description: 构建命令、依赖校验、错误分类、缓存策略
  - repo: powerx-plugin
    path: packages/cli/src/commands/plugin/dev.ts
    description: 热更新命令、watcher、日志流与失败回退
  - repo: powerx
    path: internal/plugin/local/install_handler.go
    description: 本地安装 API、签名/权限校验、审计记录
  - repo: powerx-admin
    path: apps/admin/src/modules/plugin/local-install.tsx
    description: Web Admin 安装向导、调试面板、健康检查
feature_flags:
  - px-plugin-local-dev
  - plugin-local-install
optional: false
last_reviewed_at: 2025-11-20

---

# Usecase Overview

- **业务目标**：支撑开发者在本地完成构建、安装、调试与热更新闭环，提前发现依赖、权限与接口问题。
- **成功度量**：单轮迭代 ≤15 分钟；安装成功率 ≥97%；热更新成功率 ≥95%；调试日志可追溯。
- **场景关联**：位于主场景前置阶段，为测试租户发布与审批提供可信 artifacts。

# Context & Assumptions

- **前置条件**
  - `px-plugin-local-dev` 与 `plugin-local-install` Feature Flag 已启用。
  - 开发者具备 PowerX CLI 凭证与本地/内网环境。
  - 插件工程遵循 PowerXPlugin 模板并配置最小权限。
  - 调试日志与审计渠道可用。
- **输入/输出**
  - 输入：源码、构建配置、调试参数、权限清单。
  - 输出：构建产物、安装/热更新状态、调试日志、健康检查结果、权限建议。
- **边界**
  - 不处理远程测试租户部署与审批流程。
  - 不覆盖插件业务逻辑测试。

# Solution Blueprint

## 分层设计

| 层 | 组件 | 责任 | 代码入口 |
|----|------|------|---------|
| 构建层 | `packages/cli/src/commands/plugin/build.ts` | 构建产物、依赖校验、错误分类、缓存清理 | `packages/cli` |
| 调试层 | `packages/cli/src/commands/plugin/dev.ts` | Watcher、热更新、日志流、迭代耗时统计 | `packages/cli` |
| 安装层 | `internal/plugin/local/install_handler.go` | 本地安装/激活、签名与权限校验、审计写入 | `internal/plugin/local` |
| UI 层 | `apps/admin/src/modules/plugin/local-install.tsx` | 安装向导、调试面板、健康检查、权限提示 | `apps/admin` |
| 遥测层 | `internal/plugin/telemetry/local_dev.go` | 调试日志聚合、指标上报、告警触发 | `services/plugin/telemetry` |

## 流程概述

1. 执行 `px-plugin build`，完成依赖检验与产物生成。
2. Web Admin 选择本地目录触发安装，Backend 验证签名/权限后激活插件。
3. `px-plugin dev --watch` 推送热更新，调试面板展示日志与健康检查。
4. 导出调试日志与权限建议，为测试租户发布与审批做准备。

# Implementation Checklist

| 项目 | 描述 | 状态 | 负责人 |
|------|------|------|--------|
| 构建缓存优化 | 支持增量构建与缓存命中统计 | [ ] | Michael Hu |
| 热更新稳定性 | Watcher 错误回退、日志提炼 | [ ] | Lily Zhang |
| 权限提示 | 自动比对最小权限模板并生成建议 | [ ] | Grace Lin |
| 日志脱敏 | 调试日志脱敏与审计对接 | [ ] | Matrix Ops |

# Testing Strategy

- 单元：构建命令、热更新队列、权限 diff。
- 集成：`plugin-local-dev-smoke.mjs` 覆盖成功/失败路径。
- 端到端：演练 Meta 用例 E 正/逆向。
- 非功能：跨平台兼容、长时间 watch、并发多个插件。

# Observability & Ops

- 指标：`plugin.local.iteration_cycle_time`、`plugin.local.install_success_rate`、`plugin.local.hot_reload_success_rate`。
- 告警：安装连续失败、热更新成功率 <95%、日志写入失败 >10 分钟。
- Dashboard：Local Dev Loop dashboard、CLI Telemetry、`workflow-metrics.mjs`。

# Rollback & Failure Handling

- 保留上一版本产物，提供 `px-plugin dev --reset` 清理缓存。
- 自动导出日志/权限建议供分析；脚本修复失败记录。
- `plugin-local-dev-reconcile.mjs` 对齐调试历史与审计记录。
