# 插件项目测试操作手册

当你修改插件安装 / 调试 / 治理链路时，请依照本手册执行验证步骤。本指南与
`docs/guides/test/strategy.md` 的策略表互补，聚焦“怎么做”。

## 1. 环境准备

1. 启动 Postgres / Redis / MinIO（`make dev-up` 或 Docker Compose）。  
2. 导出管理员 API Token 供 CLI 使用：`export POWERX_ADMIN_TOKEN=<token>`。  
3. 在 `backend/` 目录执行数据库迁移：`go run cmd/database/migrate.go up`。  
4. 开启所需 Feature Flag 并启动服务：
   ```bash
   PX_PLUGIN_HOST_SIMULATOR=1 PX_VERSION_GOVERNANCE=enabled \
   go run cmd/server/main.go
   ```

## 2. 常用自动化命令

| 目的 | 命令 | 说明 |
|------|------|------|
| CI 聚合入口 | `make ci-all` | 串行跑 `go test ./...` 与 Phase 9–11 回归，供 PR / main push 复用。 |
| 全量回归 | `make regression-pxp` | 依次运行 Phase 9–11 套件，可用 `REGRESSION_FILTER="Phase 10"` 聚焦某一阶段。 |
| Quickstart 端到端演练 | `scripts/ci/run_quickstart.sh` | 构建制品并跑一次发布/安装流水线。 |
| 指定包测试 | `GO_TEST_FLAGS="-run TestSandbox" REGRESSION_FILTER="Phase 10" make regression-pxp` | 覆盖默认 go test 参数，集中验证特定用例。 |
| 覆盖率报告 | `make test-coverage` | 生成 `reports/coverage.html`。 |

## 3. 手工见证步骤

1. **宿主模拟器热更新回路**  
   ```bash
   go run cmd/px/main.go host start --api http://localhost:8077/api --token "$POWERX_ADMIN_TOKEN" --plugin-id com.powerx.demo
   px-plugin dev --watch --host-api http://localhost:8077/api --token "$POWERX_ADMIN_TOKEN" --tenant-id 101 --developer-id 2025 --artifact ./dist/plugin.zip
   ```
   到 Prometheus 中确认 `debug.hot_reload.duration_ms` 等指标被写入。

2. **沙箱回归套件**  
   ```bash
   curl -X POST http://localhost:8077/api/internal/sandbox/test/run \
     -H "Authorization: Bearer $POWERX_ADMIN_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{"suiteId":"order-sync-regression","tenantId":"demo-tenant"}'
   ```
   在 `plugin_sandbox_validation_runs` 查 run 状态及脱敏/性能结果。

3. **版本治理看板**  
   打开 Web Admin `/admin/plugin-release/governance`，确保至少一个租户显示版本偏差卡片，且“扫描/例外”操作可成功触发。

## 4. 报告与检查表

1. 自动化完成后，保存控制台日志并附上 `reports/coverage.html`。  
2. 在 `specs/009-install-plugin-pxp/checklists/regression.md` 记录执行日期、运行的套件以及证据链接。  
3. 发版说明需注明 SCN 覆盖情况，例如“SCN-DEV-PLUGIN-DEBUG-001 于 2025-11-08 回归通过”。

## 5. 常见问题排查

- **宿主相关测试不稳定**：确认没有遗留的 mock host 会话（`DELETE /internal/plugins/host/mock/:id`）。  
- **沙箱数据集报错**：重新同步 `config/plugins/debug/data_suite.yaml` 并确认 MinIO 桶存在。  
- **版本扫描失败**：先本地执行 `px version compat check`，检查兼容矩阵是否缺项，再重跑回归。
