# Core 客户主联系人与补录修复证据（2026-10-11）

## 交付范围

在当前 `024-ai-engineering-skills` 工作树完成 Core 修复；合同为 [customer-contact-maintenance.openapi.yaml](../customer-contact-maintenance.openapi.yaml)，操作和部署步骤为 [主联系人维护与补录指南](../../guides/features/030-customer-contact/usecase-primary-maintenance.md)。

- 显式设主：原引用比较、同租户/客户 active 校验、membership 锁、唯一 primary 角色、事务审计；禁止直接停用当前主联系人或用普通角色写入换主引用。
- 详情页：当前 tenant/customer 重置，旧弹窗/表单/字典/身份结果清理，迟到响应丢弃，逐项权限控制。补录组件额外比较实时租户，避免卸载前迟到事件污染新页面。
- 字段：Customer/Contact/CSV 预览和提交共用 Unicode 校验，姓名 128 字符；控制字符和无效 UTF-8 拒绝。
- 存量补录：持久化预览、显式类型与联系人方案、24 小时期限、创建者/租户隔离、来源指纹、准确确认、幂等结果和事务审计；CSV 与 repair 的任务模式互斥。
- 治理：新 primary_manage 同一正式 capability 包含 Admin 用户与 typed service 两个平面。repair 为 Admin-only；发布不自动授予凭证。Framework 扩展清单已写入 spec Core internal 合同，AI Craft 已存在的候选和订单校验状态已对齐。

## 自动与原生数据库验证

从真实 Go 模块执行的 8 个相关包全部通过：customer service、customer HTTP、apikeypermissions、capability_registry service、capability_registry HTTP、database、customer repository、IAM repository。

以下四包 `go test -race` 通过：customer service、customer HTTP、capability_registry service、apikeypermissions。对应 service/handler/repository 定向 `go vet` 通过。

Nuxt Vitest `tests/unit/customers tests/unit/iam`：**35 tests / 5 files passed**，覆盖主引用确认、停用按钮保护、各项权限、客户/租户切换、迟到写入/字典/身份/补录响应、恢复记录客户校验、确认准确 checksum 以及 Unicode 边界。

原生 PostgreSQL 14.15、真实 Gin Handler/SQL、独立临时 schema，最终带 race 的 HTTP 验证通过：

| 证据 | 值 |
|---|---|
| schema | `customer_admin_test_7d57c3ba76d64ad3b85a7b7c15cca4b6` |
| import UUID | `542e1dad-2df3-4dac-a659-828c489b52e7` |
| repair UUID | `d3c9f1da-1c43-4445-8137-2208447e551f` |
| Trace | `customer-admin-http-9cbd8ae1-8718-41c3-8913-d17433aeb465` |
| 停用当前主联系人 | HTTP 409 / `CONTACT_PRIMARY_CHANGE_REQUIRED` |
| 设主与目标停用并发 | 返回成功或确定冲突；回读 membership 引用始终指向 active Contact |
| 补录跨租户回读 | HTTP 404 |
| 补录重复确认 | HTTP 200 / completed；同一客户仅一位新增 Contact |
| Contact 43 个汉字 | HTTP 201；另有 CSV 43/128 成功、129 拒绝的预览/提交/回读断言 |

服务测试还验证来源变化、过期、选人歧义、公司自然人要求、明确复用、类型不可更改、审计失败整笔回滚、重试、不同任务模式拒绝、独立 primary capability 与不接受 proxy 参数。HTTP 使用预置用户上下文的 recorder，不替代真实用户登录或浏览器验收。

## 集中迁移与发布

独立数据库 `powerx_contact_test_ad826c1862d7`，仅使用合成密钥和任务私有配置。

- 正式 `database migrate` 成功；`make capability-seed` 连续两次成功。
- 模拟旧账本表不含 mode，再跑集中 migrate：旧行 `2467884c-aabc-45e7-a51a-e3dd1e2a64db` 的 UUID、`before-upgrade.csv`、64 位原 checksum、preview 状态保留，新增字段为 `mode=create`。配置 SHA256 不变。
- 回读两个新 capability 为 published。
- 实际 IAM：4 个 Admin REST permission 的 `allow_api_key=false`；typed primary service permission 为 `customer / contacts / primary_manage`、`allow_api_key=true`、`api_key_explicit=true`。
- `make capability-check` 成功：declared=1103、referenced=65、rest_routes=1032、candidates=1054、ignored_route_rules=26。
- `upgrade_payloads.json` 已重新生成；`upgrade_registry.py --check` 和 `git diff --check` 通过。

## 当前 8077 的真实 Host 凭证回读

从 Framework 开发配置读取现有 API Key，仅用于只读请求；没有输出凭证。

`POST /api/v1/tenant/capabilities:grant-status`：HTTP 200，request ID `4ea8945e-95e5-4caf-8b04-abb8c3d2646c`。

| Capability | 运行中实例结果 |
|---|---|
| com.corex.customer.contacts.service_read | granted / CAPABILITY_GRANTED |
| com.corex.customer.contacts.service_manage | granted / CAPABILITY_GRANTED |
| com.corex.customer.contacts.primary_manage | unknown / CAPABILITY_UNKNOWN |

这表示运行中实例尚未发布本次新增目录。更新后执行目标实例迁移、能力发布及重启；需要 Framework 设主时再授予精确 primary_manage 并复查 granted。原有联系人选择不需要该新权限。不能用旧运行结果声称新增能力已部署。

## 清理与待部署验收

确认自建测试数据库无活动连接后已删除；数据库和私有临时目录不存在。两次 HTTP 临时 schema 均已清理，回读剩余数 0。本轮未启动临时应用服务。

生产实例的迁移、能力发布、准确用户角色授权、前后端更新，以及普通用户/切租户/导入回读/Core 客户 → AI Craft 联系人选择 → 保存订单的浏览器验收仍由部署后执行。对应 T034/T040 保持待验收，不把本机测试写成生产闭环完成。
