# 租户客户端应用登记

应用入口：系统管理 → 设置 → 客户端应用，页面 `/settings/client-apps`。
登记顺序为“应用概览 → 平台凭证 → 授权插件”。前两步使用“下一步”，仅最后一步显示“保存应用”；每步只校验当前内容，同一次登记中回退会保留输入。
应用形态 `mini_program / mobile_app / web_app` 与 Provider `wechat / alipay / douyin` 分别建模。
当前只允许创建微信小程序；其他选项明确标为暂未支持。连接按数组表达，当前只支持一个微信连接。

## 归属和调用链

每个租户独立登记应用、凭证和插件授权。同一 Provider/AppID 可以在不同租户独立登记。
管理请求只接受用户 JWT 和实时 RBAC，租户取自已验证上下文；请求体不能传 tenant_uuid。
管理入口拒绝 STS 与 API Key。凭证管理没有新增 `core_internal` 或 API Key 服务绑定。

客户登录／手机号认证：插件 STS + 原有能力授权 → 已验证 tenant/plugin → 数据库应用登记 → 应用已启用 + 本租户插件已启用并获授权 → 解密本租户 AppSecret → Provider 校验。
手机号认证仍要求当前客户 JWT。登记应用不替代插件能力发布、租户注册及调用授权。

正式管理能力 `com.corex.customer.client_apps.admin_manage` 覆盖以下权限：

| 操作 | 权限 |
| --- | --- |
| 应用列表 | `customer.admin_customers_client_apps:list` |
| 分类和本租户插件目录 | `customer.admin_customers_client_apps_catalog:list` |
| 应用详情 | `customer.admin_customers_client_apps_app_uuid:read` |
| 登记应用 | `customer.admin_customers_client_apps:create` |
| 修改、启停、轮换凭证、调整插件授权 | `customer.admin_customers_client_apps_app_uuid:update` |
| 迁入旧配置 | `customer.admin_customers_client_apps_import:create`，同时需要登记权限 |
| 菜单可见 | `menu.settings.client_apps:view` |

菜单权限不授予接口写权限。普通租户角色不会因升级自动获得新权限。系统 Root 目录会按现有同步规则补齐新增权限，既有授权关系保持不变。
完整 HTTP 契约见 [client-applications.openapi.yaml](client-applications.openapi.yaml)。

## 存储与修改边界

- `customer_client_applications` 保存 tenant、形态、状态、版本和插件授权。
- `customer_client_app_bindings` 保存 Provider、AppID 和租户密钥加密后的凭证。使用现有租户密钥设施及 `server.secret_key` 包装密钥，不新增用户 `.env` 配置。
- 租户 RSA 密钥首次初始化及轮换按作用域串行；密文按自身 KID 和 tenant/env 读取保留的密钥版本，不随当前活跃 RSA 密钥切换而失效。`server.secret_key` 的包装主密钥更换仍须沿现有迁移流程，不能直接替换配置。
- 只返回 `secret_configured`，不返回 AppSecret、密文或私钥；审计仅记录字段名、版本及是否更换凭证。
- 更新必须提供 `expected_version`；同版本并发修改只接受一次，其余返回 409，页面保留草稿，不自动重试写入。
- 应用形态、Provider、AppID 和连接 UUID 不可修改；新身份另行登记。省略 AppSecret 保留原值，空值拒绝；启用前必须存在凭证。
- 运行时一次查询读取版本与对应密文；微信 token 缓存按 tenant/platform/AppID 隔离并比较版本。每次调用先查应用启停和插件授权。
- 停用、缺表、权限错误或密钥不可用均不回退 YAML。

## 开发环境接入

以下命令会写入指定数据库，需要在用户安排的维护时间执行；本次开发仅在隔离测试库执行过。
先保留数据库备份和现有 `server.secret_key`，确认配置文件确实指向目标库。

```sh
# 仓库根目录；仅迁移两个新模型，dev/test 可用，不夹带其他模型迁移。
cd backend
go run ./cmd/database migrate -config /absolute/path/config.yaml -scope customer-client-apps
# 新增菜单权限并保留已有权限；系统 Root 自动补齐新项，其他角色不自动授权。
go run ./cmd/database seed-client-apps-menu -config /absolute/path/config.yaml
cd ..
make capability-seed CAPABILITY_SEED_CONFIG=/absolute/path/config.yaml \
  CAPABILITY_SEED_FLAGS='-capability-ids=com.corex.customer.client_apps.admin_manage'
```

已安装正式环境沿现有完整升级计划/apply 协议迁移，不能绕过其准入运行开发限定迁移。
随后由管理员在角色管理中明确授予所需菜单与操作权限；用户自行重新编译、重启 Core 并更新管理端。
进入页面登记应用，或从折叠的“旧部署配置迁入”中明确选择迁入当前租户。
旧配置带 tenant_uuids 的仅对应租户可迁入；未限定租户的仅 root 可迁入。迁入授权取旧配置与本租户已启用插件的交集；无可授权插件时需先启用插件。

旧 `customer_auth.client_apps` 只保留为显式迁移来源，不再作为登录真源。
不要在尚未迁移已使用的应用时切换生产认证流量。回退旧版本代码可能重新启用 YAML 授权，须同时核对旧配置和停用状态；新表默认保留供恢复，不执行删除。

## 验证范围

Go 服务与 HTTP 边界测试验证租户隔离、权限撤销、密文存储、凭证不回显、禁用拒绝和无配置回退；原生 PostgreSQL 验证幂等迁移、唯一约束和并发保存。
浏览器 fixture 使用虚拟后端和虚拟凭证验证桌面／窄屏布局、分区切换保留表单、409 草稿保留、凭证掩码及关闭后清空、分类筛选和失败反馈。
这些验证不代表真实微信 code、设备授权、生产密钥或部署运行验收。真实环境须使用本租户小程序完成登录、手机号认证、停用拒绝与重新启用检查。

浏览器回归先构建隔离输出，再运行（不覆盖日常 `.nuxt`/`.output`）：

```sh
cd web-admin
POWERX_NUXT_BUILD_DIR=.nuxt-client-apps-review \
POWERX_NITRO_OUTPUT_DIR=/tmp/powerx-client-apps-output npm run build
POWERX_WEB_OUTPUT=/tmp/powerx-client-apps-output \
  npx playwright test --config playwright.client-apps.config.ts
```

测试 fixture 自动记录并关闭自己启动的服务，不连接日常 Core 或业务数据库。
