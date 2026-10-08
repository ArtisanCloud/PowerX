# 插件运行 STS 凭证修复

## 故障与修复范围

适用于 Core 已有 `auth.credentials` 记录，但插件的 `host-values.yaml`、注册表缺少或保存了无效 STS 凭证，安装最终在 `enable` 阶段失败的情况。

本次定位实例：正式环境 `com.powerx.plugins.ai-craft@0.1.75`，系统租户 `8421e1d2-e571-432a-8431-273396f3e8a3`；开发与正式 Core 运行二进制 SHA256 相同，但正式配置文件缺 Client ID、注册表缺 Client ID 和 secret。关联失败 trace：`1ee7ed5b-c687-4d85-8287-1b37a60952a8`。当前证据不能确定是哪次历史操作首次丢失了凭证。

永久修复覆盖 Core 的插件安装、强制覆盖和启用流程：

- 删除旧版本目录前取得并验证完整运行凭证；失败立即返回，不删除旧目录和注册记录。
- 有效凭证另存到 `<plugin.installed_dir>/<plugin_id>/.runtime-credentials.json`，权限 `0600`，不依赖版本目录；已有安装仅在原凭证通过当前数据库校验后写入这个持久文件。
- 首次生成凭证时，持久文件写入失败会回滚新建数据库凭证记录。
- 生成和启用配置时明确返回凭证／文件写入错误，不保存缺凭证的成功结果。
- `host-values.yaml` 和含凭证的注册表使用受限权限和原子替换；发布切换脚本保持注册表 `0600`。
- 错误区分字段缺失、身份不符、secret 无效、禁用、过期和 audience/scope 拒绝。正常安装不会自动轮换已有凭证。

## 一次性工具合同

重新编译并部署 **Core `powerx` 和 `database` 两个二进制**。现有 `make dist` 已包含二者，无需修改插件包。命令：

```bash
database repair-plugin-runtime-credentials -plugin-id com.powerx.plugins.ai-craft -config /etc/powerx/config.yaml
```

### 先验证实际执行的 database

新版提供 `database --version` 和 `database --help`，在加载配置、校验 JWT 配置或连接数据库前直接返回。版本 JSON 包含 `version`、`git_commit`、`build_time`、`source_modified`、`go_version` 和 `commands`。发布构建 `make dist` 会为数据库工具注入发布标签、完整 commit 和 UTC 构建时间；普通 `go build` 仍可从 Go 自带构建信息取得 VCS commit，缺少的发布标签／时间明确显示 `unversioned`／`unknown`。

更新后直接查询实际部署文件，无需 EnvironmentFile 或数据库参数：

```bash
/opt/powerx/backend/database --version
```

核对 `git_commit` 与本次部署源码的 `git rev-parse HEAD` 一致，`commands` 包含要执行的命令；`source_modified=false` 才能按 commit 对应干净源码。若为 true，说明编译包含未提交改动，不能只凭 commit 判断代码一致；为 null 表示没有可用的 Go VCS 修改状态。

如果服务器旧二进制尚不支持 `--version`，在服务器仓库根目录做以下只读检查：

```bash
git rev-parse HEAD
```

```bash
readlink -f /opt/powerx/backend/database
```

```bash
go version -m /opt/powerx/backend/database | grep -E 'vcs.revision|vcs.modified'
```

```bash
grep -n 'case .*prepare-plugin-runtime-credentials' backend/cmd/database/main.go
```

最后一条无输出时，当前服务器源码不包含安装前准备命令，需要先同步追加补丁再编译。源码有命令但实际二进制不包含时，需要检查制品部署路径及 release 切换。`--version` 的命令列表可以直接确认已部署二进制是否支持准备操作。

默认只读预览。工具从这份配置及进程环境连接实际数据库，读取已有系统租户；不创建租户，不接受调用方覆盖租户 UUID。它只处理指定插件的系统租户运行凭证，以及该插件已登记版本的配置。

| `action` | 含义 | 后续执行参数 |
| --- | --- | --- |
| `healthy` | 三处运行文件与数据库校验一致 | 无需执行修复 |
| `restore` | 找到了通过数据库校验的原 secret，可恢复缺失配置 | `-confirm` |
| `rotation_required` | 文件及注册表均不能提供有效原 secret | `-confirm -rotate` |
| `rotate` + `applied: true` | 已显式轮换并同步完成 | 启动 Core，继续插件启用验收 |

`-rotate` 表示允许在无法恢复时轮换；存在有效原 secret 时仍优先恢复。重复执行达到 `healthy` 后不再次轮换。

确认执行前需停止匹配的 Core 服务，避免覆盖其内存中的注册表；工具发现配置的后端端口仍在监听时拒绝写入。禁用、draining、过期、身份不符或 scope/audience 不允许的记录也明确拒绝，不借修复改变授权。

### 正式服务器步骤

以下命令在服务器终端逐条执行。每条都是一行；沿用正式服务的实际 User/Group、EnvironmentFile 和工作目录，避免用开发环境的配置或身份。

**1. 部署新版二进制后先预览，可以保持服务运行：**

```bash
sudo systemd-run --wait --pipe --collect --property=User="$(systemctl show powerx-backend -p User --value)" --property=Group="$(systemctl show powerx-backend -p Group --value)" --property=WorkingDirectory=/opt/powerx/backend --property=EnvironmentFile=/etc/powerx/powerx.env /opt/powerx/backend/database repair-plugin-runtime-credentials -config /etc/powerx/config.yaml -plugin-id com.powerx.plugins.ai-craft
```

核对输出中的 `deployment_env=prod`、实际 `database`、`tenant_uuid`、`plugin_id`、`current_version` 和 `action`。输出不包含 secret 或 hash。

**2. 当需要执行修复时，停止正式后端：**

```bash
sudo systemctl stop powerx-backend
```

停止期间正式 API 和插件服务不可用；带 `Requires=powerx-backend.service` 的管理前端也可能被停止。

**3a. 若预览为 `restore`，恢复原凭证：**

```bash
sudo systemd-run --wait --pipe --collect --property=User="$(systemctl show powerx-backend -p User --value)" --property=Group="$(systemctl show powerx-backend -p Group --value)" --property=WorkingDirectory=/opt/powerx/backend --property=EnvironmentFile=/etc/powerx/powerx.env /opt/powerx/backend/database repair-plugin-runtime-credentials -config /etc/powerx/config.yaml -plugin-id com.powerx.plugins.ai-craft -confirm
```

**3b. 若预览为 `rotation_required`，显式轮换：旧 secret 的后续 STS Exchange 会立即失败；已经签发的短期 token 仍按现有令牌有效期处理。**

```bash
sudo systemd-run --wait --pipe --collect --property=User="$(systemctl show powerx-backend -p User --value)" --property=Group="$(systemctl show powerx-backend -p Group --value)" --property=WorkingDirectory=/opt/powerx/backend --property=EnvironmentFile=/etc/powerx/powerx.env /opt/powerx/backend/database repair-plugin-runtime-credentials -config /etc/powerx/config.yaml -plugin-id com.powerx.plugins.ai-craft -confirm -rotate
```

成功必须返回 `applied: true`，并输出受限备份目录 `backup_dir`。轮换保留 Client ID、系统租户、权限状态、audience/scope 和其他租户凭证；同步持久凭证文件、已登记版本的 `host-values.yaml`、注册表。数据库事务内记录 `PLUGIN_RUNTIME_CREDENTIAL_REPAIR` 审计。

**4. 仅在修复成功后启动服务：**

```bash
sudo systemctl start powerx-backend powerx-web-admin
```

```bash
curl -fsS http://127.0.0.1:8080/api/v1/health
```

再次执行步骤 1 应返回 `healthy`。然后在插件管理页启用已安装插件，检查实际插件健康状态及一次真实 STS 业务调用。若还有 `healthcheck_failed`，用新 trace 和插件进程退出日志诊断；凭证修复不代表其他启动错误也已消失。

## 失败、备份与恢复

- 目录检查失败发生在凭证查询／写入前。`PLUGIN_RUNTIME_INSTALLED_ROOT_UNAVAILABLE`、`PLUGIN_RUNTIME_PLUGIN_DIR_UNAVAILABLE` 和 `PLUGIN_RUNTIME_CONFIG_DIR_UNAVAILABLE` 会给出具体路径及操作系统原因；`*_OUTSIDE_*` 会给出软链接解析后的越界路径。不要只根据这类错误执行迁移或轮换。
- 排查正式配置路径：`sudo grep -nE '^[[:space:]]*(installed_dir|registry_file):' /etc/powerx/config.yaml`。检查目录、软链接和逐级权限：`sudo namei -l /opt/powerx/plugins/installed/com.powerx.plugins.ai-craft`。若配置使用另一条路径，检查该配置指向的目录；目录不存在时先确认插件产物是否还在原持久目录，不能用空目录代替缺失的安装产物。
- `-confirm` 不带 `-rotate` 且无原凭证时，明确拒绝，不修改数据库和文件。
- DB 事务或文件写入失败时，回滚数据库并恢复原文件；`applied` 保持 false。
- 写入前创建 `0700` 备份目录，`backup.json` 为 `0600`，保存原文件、原数据库凭证记录及路径。备份含敏感运行配置，不要上传或粘贴其内容。
- 如果进程被中断，保持对应 Core 停止，重新预览：文件采用原子替换，工具能通过当前数据库重新判断可恢复凭证；根据新的 `action` 再确认恢复或轮换。
- 若返回 `recovery_needed: true`，说明 DB 提交结果不明确、DB 无法读取或文件补偿失败；保持服务停止，保留输出和备份目录，核对数据库与各文件状态后再处理。不能只恢复旧文件而保留新数据库 hash。
- CLI 不修改客户资料、插件业务库、JWT 签名密钥或 API Key／能力 grant。

## 安装目录也已丢失时

如果配置指向 `/opt/powerx/plugins/installed`，而目录已经不存在，凭证修复无法代替安装产物恢复。先用下列只读命令确认旧产物和注册表是否还在：

```bash
sudo find /opt/powerx -type d -name com.powerx.plugins.ai-craft -print
```

```bash
sudo ls -l /opt/powerx/plugins/registry.json
```

有原安装产物时先核对并恢复正式环境的持久目录。没有可恢复的产物、数据库仍保留旧凭证时，使用独立的安装前准备命令 `prepare-plugin-runtime-credentials`。普通 `repair-plugin-runtime-credentials` 继续要求已有安装配置；二者不隐式互相切换。

部署包含这个子命令的新版 **database 工具**后，先只读预览：

```bash
sudo systemd-run --wait --pipe --collect --property=User="$(systemctl show powerx-backend -p User --value)" --property=Group="$(systemctl show powerx-backend -p Group --value)" --property=WorkingDirectory=/opt/powerx/backend --property=EnvironmentFile=/etc/powerx/powerx.env /opt/powerx/backend/database prepare-plugin-runtime-credentials -config /etc/powerx/config.yaml -plugin-id com.powerx.plugins.ai-craft
```

输出明确包含 `current_config_available: false` 和 `next_step: install_plugin_package`。如果 `action=install_required`，说明当前系统租户没有旧凭证记录，直接正常安装插件，由 Core 首次签发凭证即可。

若 `action=restore`，停止正式后端后，用上面命令追加 `-confirm`。若为 `rotation_required`，停止后追加 `-confirm -rotate`，旧 secret 的后续交换会失效。完整的轮换执行命令：

```bash
sudo systemctl stop powerx-backend
```

```bash
sudo systemd-run --wait --pipe --collect --property=User="$(systemctl show powerx-backend -p User --value)" --property=Group="$(systemctl show powerx-backend -p Group --value)" --property=WorkingDirectory=/opt/powerx/backend --property=EnvironmentFile=/etc/powerx/powerx.env /opt/powerx/backend/database prepare-plugin-runtime-credentials -config /etc/powerx/config.yaml -plugin-id com.powerx.plugins.ai-craft -confirm -rotate
```

准备命令只恢复／轮换现有系统租户运行凭证，写入 `0600` 持久凭证文件，并同步已存在的该插件注册表配置；缺少注册记录时不创建假的安装版本。它不会生成版本目录、`host-values.yaml`、插件二进制或启用状态。保留原有的预览、离线检查、授权边界、备份、审计和失败回滚。重复准备达到 `healthy` 后，凭证已就绪，但 `next_step` 仍为安装插件包。

准备成功后启动 Core 和管理前端，再上传正式插件包重新安装；完成插件健康检查及真实 STS 业务调用才算恢复完成。

```bash
sudo systemctl start powerx-backend powerx-web-admin
```

## 验证记录与实现位置

本地回归覆盖：Force 前取得凭证；恢复失败保留旧安装；首次持久化失败回滚 hash；版本目录缺失仍恢复原凭证；缺字段报错且不轮换；恢复／轮换后的真实 bcrypt 校验；其他租户不变；重复执行幂等；在线拒绝写入；文件失败回滚数据库、注册表和配置；审计与输出不包含 secret。

- `backend/internal/bootstrap/plugin.go`：取得、校验并持久保存全局运行凭证。
- `backend/internal/infra/plugin/runtimecredential/files.go`：稳定文件位置与受限原子写入。
- `backend/internal/infra/plugin/manager/install.go`、`host_config.go`、`lifecycle.go`：覆盖前检查、生成及启用错误处理。
- `backend/internal/service/plugin_credential/repair.go`：预览、恢复、轮换、备份和事务补偿。
- `backend/internal/service/plugin_credential/prepare_install.go`：安装产物丢失时的显式凭证准备。
- `backend/cmd/database/main.go`：工具入口。

正式站的修复执行、插件健康检查和真实 STS 调用仍需按上述步骤验收；本地测试不替代线上验收。
