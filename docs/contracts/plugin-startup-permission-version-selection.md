# 插件启动权限同步的版本选择

## 故障与修复

旧实现对 `plugins/installed/<plugin_id>/` 下的每个版本依次执行
`PostInstallManifest`。各版本共享 `source=plugin:<plugin_id>` 的权限目录，
同步会将本次声明缺少的权限标记为 `deprecated`。
例如目录遍历顺序为 `0.1.10 → 0.1.11 → 0.1.8` 时，旧版会废弃新版新增的权限，
网关随即无法找到 active 路由绑定，返回 403。

启动现在先发现、登记并保存全部安装产物，再为每个插件最多同步一份声明：

| 状态 | 同步来源 |
|---|---|
| 注册表有明确的 `current` | 该版本本次成功加载并校验的磁盘声明 |
| 没有 `current`，仅登记一个版本 | 唯一版本本次成功加载并校验的声明 |
| 没有 `current`，登记了多个版本 | 跳过并记录告警，先明确选择版本 |
| 当前包缺失或无效 | 跳过并记录告警，不使用历史包或旧快照替代 |

不会按语义版本号、目录名称或扫描时重写的安装时间猜选。
主动选定旧版本回滚时，同步该旧版本，不强制采用最大版本号。
历史版本文件和登记、当前版本选择、安装升级流程与网关权限判断规则保持原样。
同一回调中的 required capability grants 也只接收这份选定声明。

注册表保存失败时不执行本轮声明同步。声明同步失败仍按既有行为记录错误，
保持 Core 管理器可用；不能将此类告警视为插件权限恢复成功。

## 部署和现场恢复

1. 按目标环境的原部署流程构建并部署包含本修复的 Core，再重启 Core。
2. 核对注册表中受影响插件的 `current` 是实际选定版本，且对应安装包存在、有效。
   多版本却没有明确选择时，应通过现有版本切换流程选定版本，不能仅调整目录排序。
3. 启动会沿用正式同步流程，根据选定声明恢复被误标为废弃的权限。
   已有权限 ID、UUID 和角色关联继续复用，不需要执行迁移、修复 SQL 或重新安装插件。
4. 确认没有 `plugin permission sync failed`、`no selected version` 或
   `selected package unavailable` 告警。使用正常身份访问新接口，确认不再出现
   `no registered permission binding for this route`。
5. 再次重启并复验，确认状态持续正确。接口恢复登记不代替用户自身的业务授权检查。

本轮未重启日常或 stage 服务，未修改实际数据库或授权数据；现场恢复尚待部署验收。

## 验证记录（2026-11-12）

- 修复前，版本选择回归复现了三个历史版本被依次同步，以及包缺失时错误采用旧版。
- 修复后，七种版本选择场景分别执行三轮启动，通过。
- 隔离 SQLite 使用真实 FSLoader、JSONRegistry、权限同步及网关路由解析，验证误废弃恢复、
  三轮重启、权限 UUID 保持、Root 与业务角色关联保持、API Key 不扩权、已移除接口不复活、
  跨插件路由不匹配。
- manager、router、IAM repository 全包测试通过；相关 bootstrap 和 required capabilities 回归通过。
- 指定包 `go vet`、`go build -o /dev/null ./cmd/app`、`make capability-check` 通过。
- 没有修改 capability 合同、执行 capability seed 或进行数据库 schema 迁移。

可复制的验证命令：

```bash
cd backend && GOWORK=off go test ./internal/infra/plugin/manager ./internal/infra/plugin/manager/router ./pkg/corex/db/persistence/repository/iam -count=1
cd backend && GOWORK=off go test ./internal/bootstrap -run '^Test(PluginPermissionSyncRestart|PluginIAMAuthorizer|BuildPluginPermissionRows|EffectivePermissionCode)' -count=1
cd backend && GOWORK=off go test ./internal/service/plugin -run '^TestSyncManifestRequiredCapabilities' -count=1
cd backend && GOWORK=off go vet ./internal/infra/plugin/manager ./internal/bootstrap
cd backend && GOWORK=off go build -o /dev/null ./cmd/app
make capability-check
```

上述每行均从仓库根目录单独运行。测试只使用临时文件、隔离数据库及受控辅助进程；
辅助进程由测试 cleanup 停止，测试不接管日常服务。
