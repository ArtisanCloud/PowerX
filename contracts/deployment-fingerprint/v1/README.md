# 部署指纹共享协议：M0 v1.1

本目录固定开发计划 SH-FP-01、SH-FP-02 的协议输入。规范来源为
`PowerXPluginMarket/docs/marketplace/powerx-deployment-fingerprint-development-plan.md` v1.1；
PowerX 与 Marketplace 的 `contracts/deployment-fingerprint/v1/` 保存逐字节一致的快照。
修改协议先更新本目录、通过两种语言的检查，再同步另一仓库并更新 `bundle.sha256.json`。
目录校验清单用于发现漂移，不是发布签名或运行时验真。

本轮包含可执行 JSON Schema、OpenAPI、摘要样例、时间/入口样例和真实 Core HTTP 客户端兼容测试。
它不代表产品信息/公开证明端点、发布构建接线、指纹库、挑战数据库或后台页面已经实现。
不能据此关闭 A01–A24 的真实部署验收。

## 文件与校验

- `protocol.schema.json`：JSON Schema 2020-12；明确基础请求、扩展请求、响应、完整清单和摘要声明。
- `openapi.yaml`：拟议端点及原有实例接口的兼容扩展。实例认证头保持不变。
- `golden-manifests.json`：固定 canonical payload 和 build_id，包含 Unicode、跨语言路径排序及安全整数边界。
- `schema-cases.json`、`semantic-cases.json`：字段缺省/空值、路径、入口和单调时间的正负例。
- `compatibility-cases.json`：普通 400 在线回退和负面场景；由 PowerX 实际 HTTP 客户端测试读取。
- `constants.json`、`errors.json`：两端使用的预算、版本和受控错误码。
- `conformance.go`、`conformance.mjs`：跨语言一致性参考及测试。Go 使用 RFC 8785 实现；Node 使用 UTF-16 排序及严格 JSON 解析。

在本目录执行，Go 1.23+、Node 22+：

```sh
go test ./... -count=1
npm ci --ignore-scripts --no-audit --no-fund
npm test
```

在任一仓库根目录执行：

```sh
python3 scripts/check-deployment-fingerprint-contracts.py
python3 scripts/check-deployment-fingerprint-contracts.py --peer /path/to/other/repository
```

PowerX 根目录的实际客户端回归：

```sh
cd backend && go test -race ./internal/infra/plugin/marketplace -run '^TestDeployment' -count=1
```

Marketplace 根目录的旧处理器隔离联调（需要同级 PowerX 源码）：

```sh
cd backend && MARKET_DEPLOYMENT_M0_POWERX_CHECK=1 go test ./internal/http/identity -run '^TestDeploymentM0ActualPowerXAgainstOldMarket$' -count=1 -v
```

该联调仅使用测试私有 SQLite 和 httptest，测试结束关闭请求服务；不使用用户配置、服务凭证或应用数据库。

## 稳定摘要与严格输入

1. 稳定载荷取 `schema/product/component/release_id/source_commit/target/artifacts`；清单里的 `build_id/built_at` 不进入摘要。
2. 制品数组按 kind、path 的 UTF-8 字节顺序排列，再对载荷执行 JCS（对象属性仍按 RFC 8785 的 UTF-16 规则）；哈希输入为 `powerx.build-fingerprint/v1`、一个 LF、canonical UTF-8 字节。
3. `Manifest` 是完整清单；`BuildInfo` 是登记/心跳中的精简声明。二者共享构建 schema 版本标记，但分别使用 `$defs/Manifest` 和 `$defs/BuildInfo` 校验，不能拿摘要声明当完整清单导入。
4. size 为 0 到 9007199254740991 的整数；输入拒绝重复 JSON 属性、孤立 surrogate、非有限数字及未知清单字段。
5. 路径必须是组件根目录下的相对路径，区分大小写；拒绝空段、`.`、`..`、反斜杠、绝对路径、冒号、查询、fragment 及 percent 编码。现阶段不支持需要 percent 编码解释的资源路径。
6. 重复资源路径拒绝；backend 使用 binary，web-admin 使用 public_asset。计算摘要之后，导入者还必须比较计算结果与声明 build_id；schema 本身不能证明摘要匹配。
7. 样例是合成制品，不能作为官方发布或真实插件安装包使用。

算法依据：[RFC 8785](https://www.rfc-editor.org/rfc/rfc8785)。Go 的 JCS 与 schema 校验依赖单独锁在本协议工具模块，不改变两边应用模块依赖。

## 基础与扩展请求

只有 discovery 确认 service、基础 protocol 及目标 features 后，才添加扩展字段。
首次登记必须先确认基础协议；能力未知时，已登记实例仍可按旧格式心跳。
`public_site_url` 缺省表示不修改；显式 `""` 表示清除；`null` 无效。
清空与替换入口不会改变 EnvironmentID、实例凭证或 instance_id。

扩展 POST 返回 `400 invalid_input` / `400 instance_extension_unsupported` 后：

1. 暂停证明发布，针对同一市场强制 discovery 一次；并发刷新合并。
2. 只有 discovery 明确不再声明扩展，才删除全部扩展字段并重试原 POST 一次，保留凭证和基础字段。
3. discovery 仍支持、失败或无效时保留原调用错误，不用降级掩盖参数错误。
4. 每次逻辑调用至多一次强制 discovery、一次旧格式重试；重复失败采用 5 秒至 5 分钟退避。
5. 基础请求 400、401/403、429、5xx 和网络错误不进入参数回退。成功 POST 的扩展响应无效时，停止发布/重新协商，不立即重放已成功 POST。

Core 新增的是显式 `*WithDeployment` 客户端入口，供后续 PX-FP-08/09 接线；现有 worker 暂时仍走原方法，不提前声明未交付的端点能力。

## 单入口、时间和公开证明

- 普通网站识别允许 HTTP/HTTPS；实例关联只接受 HTTPS。
- 基址使用显式 ASCII/IDNA 主机，不猜测原始 Unicode 主机；保留入口路径，补末尾 `/`，去掉默认端口。拒绝歧义数字主机、userinfo、query、fragment、反斜杠、percent 编码和路径跳转。
- 规范化不等于访问授权。DNS、多结果、保留地址、实际拨号、代理、重定向及连接复用限制仍由 MK-FP-04 的受限抓取器落实。本轮参考函数不执行网络读取。
- 首版一个实例只有一个当前主动验证入口。替换/清空时停止本地旧证明，下一次心跳同步；市场使旧挑战和当前关联失效，保留历史，不影响其他实例。
- `ChallengeSet` 必含 server_time 和完整 site_challenges；空数组清空，缺字段/null 是协议错误，网络失败不清空仍有效集合。
- server_time、expires_at 为 UTC 毫秒精度，毫秒为零时兼容 Go 省略小数的 RFC3339 输出。expires_in_ms 必须等于两者差值且处于 1–600000；重发不更新绝对期限。
- 请求开始/完整解析后分别记单调时间 t0/t1；本地剩余为 `ttl - RTT - 1000ms`，非正值不发布。重复挑战取旧期限与新期限中较早者。本机墙上时间偏差不参与判断。
- 公开 Proof 原样保留市场 expires_at；仅含 schema/challenge_id/nonce/expires_at，不公开实例 ID、环境摘要、令牌或租户资料。入口绑定由已认证下发集合与市场挑战记录保持。最终消费由市场时间与事务状态决定。
- Core API 采用现有 `code=200` 成功 envelope；前端固定代理验证并解出 data 后，向 `.well-known` 返回裸 Proof。市场实例接口仍采用 `code=0` envelope，不混用两边回包格式。动态证明两端均必须 no-store。

## 后续接线与验收边界

M0 的测试覆盖协议/摘要/时间计算和实际 Core 客户端的旧新协议互操作。
后续仍需 PX-FP-01–12 与 MK-FP-01–14 的运行时、构建、迁移和页面实现。
挑战的本地内存生命周期、配置变更时立即清除、worker 启动刷新/能力过期调度、Marketplace 数据库消费事务以及真实公开入口，均不得凭本轮测试标为已交付。
