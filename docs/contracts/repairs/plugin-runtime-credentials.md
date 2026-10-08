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

- `-confirm` 不带 `-rotate` 且无原凭证时，明确拒绝，不修改数据库和文件。
- DB 事务或文件写入失败时，回滚数据库并恢复原文件；`applied` 保持 false。
- 写入前创建 `0700` 备份目录，`backup.json` 为 `0600`，保存原文件、原数据库凭证记录及路径。备份含敏感运行配置，不要上传或粘贴其内容。
- 如果进程被中断，保持对应 Core 停止，重新预览：文件采用原子替换，工具能通过当前数据库重新判断可恢复凭证；根据新的 `action` 再确认恢复或轮换。
- 若返回 `recovery_needed: true`，说明 DB 提交结果不明确、DB 无法读取或文件补偿失败；保持服务停止，保留输出和备份目录，核对数据库与各文件状态后再处理。不能只恢复旧文件而保留新数据库 hash。
- CLI 不修改客户资料、插件业务库、JWT 签名密钥或 API Key／能力 grant。

## 验证记录与实现位置

本地回归覆盖：Force 前取得凭证；恢复失败保留旧安装；首次持久化失败回滚 hash；版本目录缺失仍恢复原凭证；缺字段报错且不轮换；恢复／轮换后的真实 bcrypt 校验；其他租户不变；重复执行幂等；在线拒绝写入；文件失败回滚数据库、注册表和配置；审计与输出不包含 secret。

- `backend/internal/bootstrap/plugin.go`：取得、校验并持久保存全局运行凭证。
- `backend/internal/infra/plugin/runtimecredential/files.go`：稳定文件位置与受限原子写入。
- `backend/internal/infra/plugin/manager/install.go`、`host_config.go`、`lifecycle.go`：覆盖前检查、生成及启用错误处理。
- `backend/internal/service/plugin_credential/repair.go`：预览、恢复、轮换、备份和事务补偿。
- `backend/cmd/database/main.go`：工具入口。

正式站的修复执行、插件健康检查和真实 STS 调用仍需按上述步骤验收；本地测试不替代线上验收。
