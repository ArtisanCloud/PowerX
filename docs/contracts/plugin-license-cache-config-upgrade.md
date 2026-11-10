# 插件旧 License cache 注入修复与历史配置升级

更新：2026-11-06。适用 Core 的 Host 配置生成、正式安装/替换、Enable 配置落盘。无数据库 schema 迁移，不修改事件 Topic ACL、Key grant 或 Profile。

## 修复行为

Core 停止生成 `marketplace.license.cache.key_prefix`，也不会改成生成 `runtime.cache.key_prefix`。保留通用 `POWERX_ENVIRONMENT_ID`、`POWERX_REDIS_KEY_PREFIX`、`host.environment_id`、`host.redis.key_prefix` 和原有合法隔离设置。插件的 delegated 缓存通过 Framework `cache.service.v1` 使用 Core，Local 模式参数由插件自己的正式 Cache 合同决定。

删除注入代码不会删除历史文件里的旧区块；旧 seed、旧 registry 和已落盘文件均可能继续携带它。Core 不根据某个插件的新合同批量删除其他插件的业务配置。

生成和启用分别经过同一个通用隔离函数。启用读取实际 host-values 为基础，不把 registry 中已经被移除的旧业务字段合并回来；空文件、非法 YAML 或读取失败明确报错，不覆盖成空配置。配置写成功才替换内存；正式 Enable 将文件写入与 registry 保存纳入同一失败回滚范围。

## 只读预览

在 Core 的 `backend/` 工作目录运行；`-registry` 必须是实际运行配置使用的文件，`-version` 不填时选择 current 版本：

```bash
go run ./cmd/plugin-host-config -registry '<实际 registry.json 路径>' -plugin com.powerx.plugins.ai-craft -version 0.1.78
```

命令只读取 registry 和目标版本的 host-values，不调用 Bootstrap、不扫描/改写插件清单、不连接数据库、不启停服务。

输出包含插件/版本、两个文件路径、状态、cache 字段名和 `plan_digest`；不输出字段值、Redis URL、API Key、STS secret 或其他配置原文。

| 状态 | 处理 |
|---|---|
| `absent` | 没有旧 cache 配置（或值为 null），不需要工具清理 |
| `legacy_default_candidate` | cache 只有 `key_prefix`，并与保存的 Host 环境/插件前缀匹配；这是候选，不是来源证明，需确认由旧 Core 注入生成 |
| `manual_migration_required` | 含 provider、redis_url、TTL、未知前缀、空 `cache: {}` 或其他内容；工具拒绝删除，需要按插件合同迁移 |
| `configuration_drift` | registry Spec 与实际文件不一致；先核对并同步，不直接清理 |

`PLUGIN_HOST_CONFIG_NOT_FOUND` 表示所指定 registry 中没有这个版本的 Host 配置，不能据此猜测其他路径并写文件。安装失败回滚后可能没有登记；应确认目标，或通过正式安装重新生成。

## 受控清理：仅停服窗口使用

先完成新版 Core 编译和插件修复/打包。日常 Core 由所有者自行停止，工具不会代替操作者停服；停止后重新预览，确认版本、路径、候选来源和 digest。

仅对确认由旧 Core 注入生成的候选执行：

```bash
go run ./cmd/plugin-host-config -registry '<实际 registry.json 路径>' -plugin com.powerx.plugins.ai-craft -version 0.1.78 -apply -confirm-generated -core-stopped -expect '<刚预览的 plan_digest>' -backup-dir '<插件安装包外的新私有备份目录>'
```

`-core-stopped` 是操作者声明，不是机器探测结果；不能在日常 Core 运行时使用。全局 registry 被运行进程管理，在线直接修改可能被覆盖。`plan_digest` 绑定完整 registry、host-values、插件和版本；任一源文件在预览后变化都拒绝应用。没有确认、没有备份目录、备份目录已经存在或位于插件包内，也会拒绝。

清理只移除确认过的 `marketplace.license.cache` 候选，并移除因此成为空 map 的 license/marketplace 容器。含其他业务字段的 marketplace/license 保留；手工 provider、redis_url 等不能由此命令迁走或删除。工具同步更新 registry Spec 和实际文件，不改其他版本、事件 ACL、Key、运行凭据或数据库。

备份目录 0700，`registry.json`、`host-values.yaml`、`preview.json` 均为 0600。前两份包含完整原配置，可能含凭据，必须作为私有恢复材料保管，不上传到工单、日志、源码或插件包。

## 失败与恢复

文件写入、registry 更新或保存返回错误时，Core 恢复原文件、内存 HostConfig 和 registry；恢复失败与原错误一起返回，不启动插件或伪报成功。

文件系统与 JSON registry 不是跨文件事务；进程/机器中断不能依赖内存回滚。若发生中断，在同一停服窗口按备份 `preview.json` 记录的原路径恢复两份原文件，保持 0600，再重新预览。若已发生其他插件/配置更新，不要用整份旧 registry 覆盖新变更，应先核对差异。此命令不执行自动崩溃恢复，也不删除备份。

配置提交成功之后的插件启动/健康检查失败，继续沿用原生命周期和安装回滚；成功提交的合法 Host 配置不因业务启动失败而自动撤销。

## AI Craft 与联合验收

AI Craft 仍需处理 `catalogs.events`：按包内正式目录加载事件权限；缺失、读取或解析失败必须阻止启动。除了 parser，还要修改 main 中目前记录警告后继续的调用点。路径检查应拒绝绝对路径、越界及包外符号链接。Core 本轮没有修改插件源码。

双方准备好后：

1. 在停服窗口完成上述历史处理（如有），启动新版 Core。
2. 通过正式插件安装/替换流程安装 AI Craft 新包，不手改安装目录，不重置已准备的事件 ACL。
3. 核对最终文件、registry Spec 和内存配置均无被 Core 新增的旧 cache 或未知 runtime.cache.key_prefix；人工配置按插件合同迁移。
4. 插件持续健康，事件目录权限检查启用，不再出现旧 License cache 拒绝错误。
5. 使用真实插件服务凭据完成一次 Framework Host Cache 写入/读取，核对 tenant/environment 隔离；健康 200 和客户端构造成功不能替代缓存授权及调用验收。

本轮开发回归覆盖新生成/旧 seed、再次启用、多环境/多插件隔离、人工配置保留、确认与过期预览、私有备份、配置漂移、无效文件及写入/registry 失败回滚。运行中的 Core 和实际安装联调需单独验收。

## 代码映射

- `backend/internal/infra/plugin/manager/host_redis.go`：通用隔离，停止业务缓存注入。
- `host_config.go`、`host_config_update.go`、`lifecycle.go`：生成、启用落盘与失败恢复。
- `legacy_license_cache.go`：脱敏预览、候选判定、digest、备份与受控清理。
- `backend/cmd/plugin-host-config/`：默认只读 CLI。
- `host_redis_test.go`、`legacy_license_cache_test.go`：隔离与历史升级回归。
