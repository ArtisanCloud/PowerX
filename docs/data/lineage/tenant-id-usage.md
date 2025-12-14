# Tenant ID 使用清单（Data & Analytics）

| 系统/任务 | 数据集/表 | 是否仍写入 `tenant_id` | Owner | 切换计划 | 备注 |
| --- | --- | --- | --- | --- | --- |
| dbt `fact_usage` | warehouse.fact_usage | ✅ | @leo | 12/15 替换为 `tenant_uuid` |  |
| Airflow job `billing_export` | s3://billing/export.csv | ❌ | @mira | - | 已改 UUID |

### Backend `tenant_id = ?` SQL（2025-12-10 扫描）

> 2025-12-14 更新：除 `tenant_id = ?` 外，新增巡检命令 `rg -n "tenant_id IS NOT DISTINCT FROM" backend`，当前命中 2 处（Tenant Key Pair 仓储），同样纳入本表追踪。

| 文件 | 域 | 现状 | UUID 列 / 回填情况 | 计划/Owner | 状态 |
| --- | --- | --- | --- | --- | --- |
| backend/cmd/database/seed/seed_department.go | Data seed | 重新插入部门时仍按 `tenant_id` 过滤 | ✅ 已改 `tenant_uuid`; 无需回填 | 租户目录组（@ops-seed）计划 12/20 切到 UUID | ✅（2025-12-13：seed helper 改为 `tenant_uuid`） |
| backend/internal/service/system/user_service.go | System Admin | 已改用 `tenant_uuid` 过滤/写入（system users & member 关联） | ✅ Service/Repo 均使用 `tenant_uuid` | 系统团队（@sys-admin） | ✅（2025-12-10） |
| backend/pkg/corex/db/persistence/repository/setting/auth_provider_config_repo.go | System Settings | provider 配置 CRUD 均以 `tenant_id` 条件查询 | ✅ 列已存在；2025-12-13 起只接受 canonical UUID | Settings 组（@sys-admin）T1 处理，改为 `tenant_uuid` 兼容查询 | ✅（2025-12-13：仅接受 canonical `tenant_uuid`，冲突键改为 `tenant_uuid + type`） |
| backend/pkg/corex/db/persistence/repository/setting/domain_binding_repo.go | System Settings | 域名绑定仓储仅支持 `tenant_id` 过滤 | ✅ GORM `tenant_uuid` 列已生效；`tenant_id` 正待迁移下线 | 同上 | ✅（2025-12-13：仓储 API / Upsert 统一走 `tenant_uuid`） |
| backend/pkg/corex/db/persistence/repository/setting/plugin_instance_config_repo.go | System Settings | 已改为仅接受 canonical `tenant_uuid` + `plugin_id` + `key` 作为唯一键 | ✅ `tenant_uuid` 唯一键；旧 `tenant_id` 已移除 | 同上 | ✅（2025-12-13：写入前强制 strings.TrimSpace + ToLower，彻底移除 `tenant_id` 入口） |
| backend/pkg/corex/db/persistence/repository/setting/tls_cert_ref_repo.go | System Settings | TLS 证书引用仓储通过 `tenant_uuid` Scope + kind/ref 查询，系统级证书使用空 UUID | ⚠️ 模型新增 `tenant_uuid` 并默认空串；生产数据需回填 | 同上 | ✅（2025-12-13：冲突键更新为 `tenant_uuid + kind + ref`，Find/Upsert 不再接受 `tenant_id` 指针） |
| backend/pkg/corex/db/persistence/repository/setting/tenant_scope.go | System Settings | `TenantScope` 在缺少 UUID 时仍回落到 `tenant_id = ?` | N/A（helper），现仅支持 canonical UUID | 同上，12/18 内删除回落逻辑 | ✅（2025-12-13：仅允许 UUID，禁用回退） |
| backend/pkg/corex/db/persistence/repository/capability_registry/capability_registry_repo.go | Capability Registry | 仓储已双写 `tenant_uuid` 并在查询中兼容 UUID/旧 ID | ✅ 列完成迁移，`tenant_id` 列已在 010 脚本移除 | 能力团队（@capability） | ✅（2025-12-10） |
| backend/pkg/corex/db/persistence/repository/event_fabric/authorization_repository.go | Event Fabric | Grant/Capability 查询已统一 `tenant_uuid`（`rg -n "tenant_id" backend/internal/service/event_fabric backend/pkg/corex/db/persistence/repository/event_fabric`=0） | ✅ `tenant_uuid` 列生效；旧列通过 011/012 脚本删除 | Event Fabric 团队（@eventfabric），2025-12-14 验证仓储/服务/缓存只消费 UUID | ✅ |
| backend/pkg/corex/iam/reqctx/scope.go | IAM | scope helper 针对 root query 仍支持 `tenant_id` 条件 | N/A（helper）；2025-12-13 起仅写 `tenant_uuid` | IAM 团队（@iam-core）清理，统一调用 `tenant_uuid` | ✅（2025-12-13：ReqDB 仅写入 `tenant_uuid` 条件） |
| backend/pkg/corex/db/persistence/repository/tenant/keypair_repo.go | Tenant Platform | 仍以 `tenant_id IS NOT DISTINCT FROM ?` 过滤 key pair（GetActive/Deactivate） | ⚠️ 表内仅存在 `tenant_id`，待新增 `tenant_uuid` 列并回填 | Tenant Platform（@tenant-core），12/22 前切换查询条件 | ⏳ |
| backend/pkg/corex/db/persistence/repository/capability/**/*、repository/workflow/**/* | Capability & Workflow | 多个仓储仍以 `tenant_id` 作为 where 条件，Service/DTO 同样暴露 ID | ⚠️ 主要表已补 `tenant_uuid` 列；旧列待删除 | Wave-CW：@capability + @workflow，2025-12-27 完成仓储+Service切换并跑 `go test ./backend/internal/service/{capability_registry,workflow}` | ⏳ |
| backend/pkg/corex/db/persistence/repository/integration_gateway/**/*、agent_model_hub/**/*、cost_quota/* | Integration Gateway & Provider Registry & Cost Quota | 仍出现 `tenant_id` 查询、密钥轮转/配额仓储依赖 ID | ⚠️ 列已回填 UUID，仍需移除 ID 兼容层 | Wave-IG：@integration + @agent-hub，2026-01-06 前完成仓储 + 集成测试 `tests/integration/integration_gateway/*`、`agent_model_hub/*`、`go test ./backend/internal/service/cost_quota` | ⏳ |

## 操作指引
1. Data Platform 每周运行一次 `rg -n "tenant_id" data-pipelines/` 和 dbt lineage，填充上述表格。
2. 若 `是否仍写入 tenant_id = ✅`，需创建任务（Jira/Asana）并在“切换计划”列写入目标日期。
3. 完成迁移后，在 `tmp/reports/tenant-uuid-ga-weekly.md#data` 更新状态。
