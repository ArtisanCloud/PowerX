# Core 角色管理授权与升级

对应 [issue #434](https://github.com/ArtisanCloud/PowerX/issues/434)。角色管理需要用户登录身份和实时管理权限；登录成功、处于同一租户以及 JWT 中包含 `role_admin` 等角色名称均不构成管理授权。

## 权限和边界

| Core 权限 | 允许的操作 |
| --- | --- |
| `corex.iam.role_management:read` | 查看当前租户角色、角色权限及权限 ID |
| `corex.iam.role_management:write` | 创建、修改角色；按 ID 或 triple 授权、撤权、覆盖权限集合 |
| `corex.iam.role_management:delete` | 删除符合现有规则的租户角色 |
| `corex.iam.role_management:bind` | 绑定、解绑当前租户成员的角色 |

正式能力为 `com.corex.iam.role_management.admin_read/admin_write/admin_delete/admin_bind`，声明见 `backend/config/platform_capabilities/iam_role_management.yaml`。仅用户 JWT、`admin_user`、当前租户范围；不开放 STS direct、API Key 或 agent 调用。

HTTP 在读取请求体和执行操作前检查权限，Service 再次检查。授权使用 UUID 角色关系、有效时间、active/allow 状态及正式 `platform_capability` 所有者；同一会话的权限撤销和角色绑定过期会在下一次调用生效。真正的 Root 用户会话保持原有管理能力；只有 `ActorContext.IsRoot=true` 不能制造 Root 身份。

来源仍为 `core` 或正式 `platform_capability` 的历史 `iam.role:<action>` 授权继续有效。来源为插件的同名权限不放行，不能通过插件权限、JWT 角色名称或角色代码创建来取得 Core 管理能力。

系统角色、其他租户角色和 Core 管理的 Root 角色继续受原保护限制。租户管理员通过 triple 授权时只能选择已登记权限，不能顺便创建或改写全局权限目录。创建角色并附带权限在同一事务中完成，初始授权失败会回滚角色。

成员管理的角色变更入口仍额外要求成员账号管理权限；其角色分配检查同时识别新 `role_management:bind` 与有效的历史 Core `iam.role:bind`。本次不改插件服务能力、插件 API Key／STS grants、订单或客户数据。

## 已部署环境的影响

2026-11-15 只读检查确认 200 服务器 dev／prod 运行 `dev-20261113-1002`／`prod-20261113-1002`，构建提交同为 `c84848176809279e78b8ef8dd7ab22381e42f139`；其中角色授权与绑定入口仍缺少管理授权检查，属于受影响版本。此结论不代表已经发生利用；本轮没有在两站执行提权请求或完整入侵取证。

两站历史 `iam.role:read/write/delete` 的权限来源为 `plugin:com.powerx.plugins.ai-craft`，`iam.role:bind` 为 `core`。修复使用独立的 `role_management` 命名空间，保留旧记录及其 UUID、来源和授权关联，不在启动时夺回插件权限或自动给角色增加新权限。

## 发布顺序和验收

1. 发布包含本次修正与正式能力 YAML 的 Core。使用当前环境原配置，先 dev 后 prod；本项不增加数据库表、列或索引，不要求全量 `make migrate`，不要求重新安装插件。
2. 在各环境按明确配置发布以下四项正式能力目录。以下以已切换的 dev 后端目录为例，prod 使用 `/opt/powerx/backend` 与 `/etc/powerx/config.yaml`：

   ```bash
   cd /opt/powerx-dev/backend
   ./platform_capability_seed -config /etc/powerx-dev/config.yaml \
     -capability-ids com.corex.iam.role_management.admin_read,com.corex.iam.role_management.admin_write,com.corex.iam.role_management.admin_delete,com.corex.iam.role_management.admin_bind
   ```

   保持默认 `preserve-installed-state=true`；该命令只发布目录与权限元数据，不给角色、插件或凭据补授权。源码环境可使用同配置的 `make capability-seed CAPABILITY_SEED_CONFIG=/absolute/config.yaml CAPABILITY_SEED_FLAGS='-capability-ids ...'`。不得运行全量 seed 替代精确升级。
3. Root 登录对应站点，切换到需管理的租户，在角色权限配置中为明确需要管理的角色分配 `corex.iam.role_management` 对应操作。`write` 与 `bind` 都是高权限，应明确授权；不得默认授予 `role_user`。历史授权来源已冲突的租户管理员在完成这一步前会收到 403，Root 仍能完成配置。
4. 在隔离测试账号／专用验收租户中验证：无管理权限的用户在 13 条角色管理路由收到 403，权限和绑定无变化；合法管理员按所授操作成功；撤销管理权限后同一会话立即收到 403。正式站不使用真实业务成员执行提权或破坏性测试。
5. 本次不自动删除任何历史授权。若发现疑似异常授权，应先比对审批／审计记录，再通过正式 Root 接口处理；不能根据当前角色名称推断授权来源并批量撤销。

上线前源码测试和隔离验证不替代上述运行验收。回滚至未修复 Core 会恢复该漏洞，应优先停止受影响的管理写入口，而不是以旧版本长期运行。
