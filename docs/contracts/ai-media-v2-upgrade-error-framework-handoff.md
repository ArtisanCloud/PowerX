# 媒体 v2 升级错误：Framework 对齐清单

日期：2026-10-22。Core 当前分支源码与受控回归交付，尚未提交；运行实例需由维护人员按原方式重启后复验。本轮不修改 Framework 源码、快照或发布包。

## 本次变化

Core 为已有人工升级确认入口发布固定结构化原因码：

`POST /api/v1/admin/capability-registry/capabilities/{capabilityId}/tenants/{tenant_uuid}/upgrade`

来源与精确摘要以 [ai-media-v2-source.json](ai-media-v2-source.json) 为准。合同正文见 [ai-media-v2.md](ai-media-v2.md#升级确认的结构化错误)，权威样例见 [capability-version-upgrade-errors.v1.json](fixtures/capability-version-upgrade-errors.v1.json)。媒体 OpenAPI 与 `fixtures/ai-media-v2.json` 的内容保持原摘要。

此入口仅允许人工 Admin JWT 确认。Framework 的 API Key／STS 媒体调用不能自动审批、修改注册或绕过 `registry.version_locked`。

## Framework 需要修改的文件

以下路径相对 PowerXPlugin 仓库：

| 文件 | 对齐要求 |
|---|---|
| `docs/contracts/ai-media-v2.md` | 同步 Core 最新正文，包含 Agnes 说明及结构化升级错误约定 |
| `docs/contracts/ai-media-v2-source.json` | 同步 Core 来源基线与四份文件摘要；保留 `source_worktree_uncommitted=true` |
| `docs/contracts/fixtures/capability-version-upgrade-errors.v1.json` | 新增 Core 权威样例的原样副本 |
| `docs/contracts/fixtures/ai-media-v2-registration.json` | 更新 `upgrade_confirmation_error`，使用权威样例的 `binding_mismatch` 响应 |
| `framework/backend/go/runtime/powerx/ai/testdata/ai-media-v2-registration.json` | 与文档侧 registration fixture 保持一致 |
| `framework/backend/go/runtime/powerx/ai/registration_test.go` | 移除“Core 尚无结构化 reason”的旧注释，按固定原因码断言 |
| `docs/contracts/evidence/ai-settings-media-framework-validation-20261022.json` | 更新 `core_contract_source` 和 `core_coordination.upgrade_error`，记录实际复验结果 |

`upgrade_confirmation_error.expected_reason_code` 应从 `AI_UPSTREAM_DEPENDENCY` 改为 `capability.upgrade_binding_mismatch`；`machine_reason_published` 改为 `true`。其 HTTP 状态仍为 409，`message` 仍为 `capability.upgrade_failed`。

Core 已用当前 Framework 的 `DecodeHTTPError` 消费全部 10 个权威样例，核验状态、`ErrorCode`、`ReasonCode`、trace、request_id、details 和原始响应均通过。现有通用解析器支持本合同，本次无需新增 SDK 调用方法或为供应商增加硬编码。该结果是当前源码消费验证，不是已发布 SDK 或安装态验收。

## 固定错误和状态

| HTTP | reason_code | retryable |
|---|---|---|
| 400 | `capability.upgrade_invalid` | false |
| 403 | `capability.upgrade_forbidden` | false |
| 404 | `capability.upgrade_not_found` | false |
| 409 | `capability.upgrade_hash_mismatch` | false |
| 409 | `capability.upgrade_binding_mismatch` | false |
| 503 | `capability.upgrade_unavailable` | true |
| 503 | `capability.upgrade_persistence_failed` | true |
| 503 | `capability.upgrade_runtime_apply_failed` | true |
| 500 | `capability.upgrade_audit_finalize_failed` | true |
| 500 | `capability.upgrade_failed` | false |

具体码同时出现在 `error_code`、`reason_code` 和 `details.reason_code`。保留标准 `request_id`，trace 存在时从 `details.trace_id` 读取。不得根据诊断文案推断原因，未知普通错误仍是通用失败。

`runtime_apply_failed` 附带 `audit_id` 和 `approval_persisted=true`，运行锁结果仍未知；`audit_finalize_failed` 额外附带 `runtime_confirmed=true`。不要把错误响应解释为“没有发生任何变更”。`retryable=true` 只表示修复依赖后可以人工重新确认，不授权自动重试审批。

早期 JSON 解析失败的 `message` 是 `capability.upgrade_invalid`，早期 API Key 拒绝为 `capability.upgrade_forbidden`，Service 失败继续为 `capability.upgrade_failed`。以结构化码判断；前置认证／Admin 中间件的错误不属于本 fixture 的覆盖范围。

## Framework 验收

1. 文档、运行时 testdata 的 registration fixture 一致，四份 Core 来源文件摘要准确。
2. 通过真实 `DecodeHTTPError` 覆盖全部 10 个权威样例；检查状态、机器码、trace、request_id、details、原始 body。
3. 特别覆盖两个 409 原因不同、503 已持久化审批和 500 运行锁已确认；未知错误不能通过解析字符串冒充具体业务码。
4. 保留现有 API Key／STS 两种目录调用回归，固定 `core_internal`，无自动重试、无通道回退、无客户端自动确认升级。
5. 更新证据；明确当前源码、消费快照、公开发布、真实 Core 安装态分别验证到了哪一层。

## MediaXStudio 可以继续的工作

已有 v2 媒体客户端和当前快照可继续用于目录与 05／06／08 配置验收，本次错误细化不要求替换媒体 SDK。目录成功不替代导演页面保存／重载、真实生成、任务刷新恢复、播放与安装态 STS 验收。

Qwen 视觉理解、生图、TTS 已有本地配置记录；视频需在 Core 配置有效凭据和型号。Agnes 已有 Core 图像／视频驱动和型号目录，实际凭据、真实任务及播放仍待验证，供应商密钥只保存在 Core。

Core 验证记录见 [capability-upgrade-machine-reasons-validation-20261022.json](evidence/capability-upgrade-machine-reasons-validation-20261022.json)。本轮不执行实际升级确认、不写本地业务库、不调用付费供应商，也不重启用户进程。
