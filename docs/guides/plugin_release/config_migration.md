# Plugin 配置聚合迁移指南

本指南解释如何从旧版 `config.yaml` 迁移到新的 `plugin` 聚合结构。开发分支 `007-integration-gateway-and-mcp` 起，所有插件相关的配置都位于顶层 `plugin` 区域，包含以下子对象：

```yaml
plugin:
  enabled: true
  registry_file: ./plugins/registry.json
  base_prefix: /_p
  installed_dir: ./plugins/installed
  market_cache_dir: ./plugins/market_cache
  read_timeout_sec: 15
  write_timeout_sec: 15
  dev_mode: false
  release: { ... }
  dev_hotload: { ... }
  bootstrap: { ... }
  debug: { ... }
```

## 迁移步骤

1. **删除旧顶层块**：确保 `plugin_release`、`dev_hotload`、`plugin_bootstrap`、`plugin_debug` 等顶层对象已经移除，仅保留一个 `plugin` 块。
2. **复制原内容到子对象**：
   - `plugin_release` → `plugin.release`
   - `dev_hotload` → `plugin.dev_hotload`
   - `plugin_bootstrap` → `plugin.bootstrap`
   - `plugin_debug` → `plugin.debug`
3. **校验默认值**：如果某些字段在旧版中缺失，可参考 `backend/etc/config_example.yaml` 的最新结构，或运行 `powerx config print-defaults`（未来 CLI todo）来获取默认值。
4. **更新部署脚本**：若 CI/CD 或 K8s ConfigMap 仍引用旧键名，务必同步更新。
5. **重新加载服务**：修改配置后重启 Core 服务，并执行 `scripts/ci/run_quickstart.sh` 以回归插件发布/热更新关键路径。

## 验证

- `backend/internal/bootstrap/app.go` 现在只从 `cfg.Plugin.*` 读取配置，若缺少字段将直接 panic，以便及早发现遗漏。
- `go test ./internal/service/plugin_release/...` 与 `go test ./internal/service/dev_hotload/...` 可验证业务逻辑仍然通过。
- 日志中出现的 `plugin_release.*`、`dev.hotload.*` 指标不受此变更影响，Prometheus/Grafana 配置无需调整。

## 启动并发开关（新增）

插件自动恢复支持配置项与环境变量覆盖：

- `plugin.auto_restore_parallelism`
- `CORE_X_PLUGIN_AUTORESTORE_PARALLELISM`（优先级高于 config）
- `PX_PLUGIN_AUTORESTORE_PARALLELISM`（兼容旧变量，优先级同上）

行为说明：

- 控制 Core 启动时“已启用插件 auto-restore”的并发 worker 数。
- 默认 `1`（串行），与历史版本一致。
- 最大 `8`（超出会自动钳制）。
- 建议本地开发设置 `2~4`，以缩短多插件启动时间。

示例：

```bash
## 覆盖 config.yaml 中的 plugin.auto_restore_parallelism
export CORE_X_PLUGIN_AUTORESTORE_PARALLELISM=3
# 兼容旧变量
export PX_PLUGIN_AUTORESTORE_PARALLELISM=3
```

如在迁移过程中遇到字段含义不确定的问题，可以查阅 `backend/config/plugin.go` 及其子配置文件获取注释。
