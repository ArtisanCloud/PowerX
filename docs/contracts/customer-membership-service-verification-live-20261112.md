# 客户核验能力：日常联调交接（2026-11-12）

## ① Core 已完成

已在本机日常 Core（`http://127.0.0.1:8077`、数据库 `powerx`、`dev` 环境）发布客户核验能力、完成租户登记、构建并重启 Core，并给电商自己的 STS 服务身份附加精确只读授权。

公开 Go SDK `github.com/ArtisanCloud/PowerXPlugin/framework/backend/go@v0.0.30` 已实际调用，没有使用本地 replace、Mock 或 Admin Bearer 代替服务核验。

| 项目 | 实际值 |
|---|---|
| Core 运行版本 | `v1.0.0`，本地 development 构建 |
| Core PID | `64437` |
| 基础源码提交 | `1ead89105483b3d5c60205348afe9fe2483d9a82` |
| Core Build ID | `a2fcd6afb0f81d4f90c83d8a5627f6dddcc9378a3ba6a0ba67ff229905f67d46` |
| 能力 | `com.corex.customer.memberships.service_read` |
| 目录提供方版本（现有 Core 目录体系） | `2025.01` |
| 目标租户 registration | `published`，版本 `1` |
| 合同 schema_version | `1` |
| catalog hash | `711c90c4cea94d1474ec0fa02d3829338dfd02c2510977dd00a3f05e941f0849` |
| protocol hash | `24efe7cf4e6abe254559fe02317f3b870ab080ad005c99fd981ef294203e495a` |

目录提供方版本、registration 版本、请求 schema 版本和 Core 运行版本含义不同；上表记录原值，没有把它们互相替换。

正式限定发布命令只同步了这一项能力及对应权限。现有发布流程为 4 个活跃租户建立 registration；服务授权只增加到下面的电商身份，不向其他身份附加 grant。

## 电商可以继续接线的实际对象

| 项目 | 实际值 |
|---|---|
| 电商服务 plugin_id | `com.powerx.plugins.ecommerce` |
| 当前已安装电商版本 | `0.4.6`（本轮没有升级电商） |
| 现有 Lab 所属租户 | `System` |
| tenant_uuid | `6b5d0240-9920-46da-b707-88200e0f51ea` |
| Lab 客户显示名 | `WorkspaceX Lab Checkout efe26bfd` |
| customer_uuid | `cf2cf9f0-938d-474b-af63-9cfc7aab0162` |
| membership_uuid | `99c935c3-b4b8-4922-add6-d716b530e58b` |
| primary_contact_uuid | `f529d578-bf3f-4f13-8bc0-847f173b9d40` |
| 最终独立审批 UUID | `4fd9a911-4e91-4537-b70b-9d467faf8492` |
| 最终状态 | active，电商已获客户核验只读能力 |

这里沿用已有 Lab 客户，不新建租户或客户。已有商品读取、商品配置 Key 的名称、状态、Profile 和 owner policy 已前后核对保持一致；没有复用它们进行本次 SDK 核验。

## 真实 SDK 结果

三次请求使用同一个电商 STS 令牌；撤权测试结束后已恢复授权。

| 阶段 | 结果 | 实际 trace |
|---|---|---|
| 已授权 | 核验成功，active | `69a88781-13d8-4ba7-8854-89723b68c215` |
| 撤权后的下一次调用 | HTTP 403，`CAPABILITY_FORBIDDEN` | `fce96881-8f96-44af-aebd-d1dee3cbf97f` |
| 恢复授权 | 核验成功，active | `578c62a3-f6ee-4b6f-a6b0-a63e19389bae` |

两次成功调用在 Core invocation trace 表中均为 `completed`，协议 `core_internal`，调用主体为电商自身的 `sts:com.powerx.plugins.ecommerce:client:...`。撤权请求在 Gateway 授权阶段拒绝，没有进入核验执行，因此不要求存在业务 invocation 行；SDK 返回的 trace 为实际 HTTP 请求追踪号。

完整对象、摘要、发布记录和保留范围见[机器收据](evidence/customer-membership-service-verification-live-20261112.json)。临时保存的 STS secret／token 文件已移除，未在聊天或交接中提供。

## 运行构建与维护边界

本次没有执行数据库迁移。构建以当前 HEAD 为基础，加上客户核验实现；使用只读 Go overlay 排除工作区中尚未交付的 IAM／通知改动，没有切分支、创建 worktree、回退或覆盖其他源码。

当前运行文件：`backend/bin/customer-verification-20261112/powerx`。运行时能力目录固定为同目录的 `platform_capabilities/`，其中保留基础目录并增加客户核验正式声明。进程按本次用户明确要求保持日常服务运行。旧二进制副本 `rollback-app` 仅作为本轮应急回滚文件保留，不是另一个运行实例。

后续正常源码构建仍需由相应维护者完成其他工作区改动的验收；本次收据只证明客户核验这项限定构建和实际调用。

## ②／③ 尚未完成与下一步

当前日常目录还没有电商的五项 `checkout.*` 正式能力，因此没有创建 WorkspaceX 交易 Key，也没有提前授予未发布能力。

下一项由电商维护者执行：升级 SDK v0.0.30，通过 `customerfw.NewServiceMembershipVerifier(...).Verify(...)` 实现其现有适配器，并在实际启动绑定 `Deps.CheckoutCustomers`；完成电商迁移、安装，正式发布和登记报价、订单创建、订单查询／恢复、支付创建、支付查询／恢复五项能力，回传实际版本、hash、Lab 测试购买选项和支付渠道。

五项能力完成后，Core 再创建独立的 WorkspaceX Lab 交易身份，精确授予五项插件能力和 SDK 确实需要的最小只读查询许可；私有 Key 文件另行交接。客户核验由电商自己的身份执行，不因此给 WorkspaceX 交易 Key 增加客户核验或客户管理权限。电商随后按实际 caller_subject 配置业务允许范围。

本轮没有迁移电商数据库、回填支付 UUID、创建／修改购买选项、创建订单、创建支付或生成付款码；这些仍属于②及后续 WorkspaceX 验收。
