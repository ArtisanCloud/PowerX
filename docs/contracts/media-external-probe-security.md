# 媒体外链探测安全边界（PR #441 develop 整合）

## 来源与整合范围

原始补丁来自 Peter Wilfahrt（GitHub `p-w`）：https://github.com/ArtisanCloud/PowerX/pull/441 ，原提交 `d050420af84b5f99c23681e31db6b80704c17b8a`。已在独立分支 `fix/pr441-media-public-probe` 上，以 `origin/develop` 为基线无冲突 cherry-pick，保留原作者与提交说明。没有合入 develop/release，也没有部署。

补充改动仅涉及媒体外链探测的测试注入、空闲连接释放、非单播地址拒绝与回归覆盖，不改变普通文件上传、对象存储驱动或公开资源 token 协议。不宣称其他模块的全部出站请求已受此保护。

## 运行行为

登记 `external_link` 时，Core 通过 HEAD（必要时 GET Range）读取文件大小及 MIME。默认客户端在 socket 拨号时检查实际解析出的目标 IP，而非只在解析 URL 时检查域名。

- 拒绝本机、RFC1918 私网、链路本地、IPv6 unique-local、CGNAT、未指定地址、0/8 和非全局单播地址（包含广播／组播）。IPv4-mapped IPv6 同样检查。
- HTTP/HTTPS 重定向仍经过实际拨号检查，重定向次数上限保留原 PR 规则；其他协议拒绝。
- 域名解析结果变化时，每次新连接都检查；已连接的公网上游连接复用不会变成另一目标。
- 保留探测超时、HEAD→GET 回退、Range、文件大小与 MIME 解析。每次探测持有的 transport 在结束后关闭空闲连接。
- 私有地址即使属于合法内网文件服务，也不属于这个公网外链入口允许的目标。原 PR 的 transport 不使用环境 HTTP(S)_PROXY；依赖环境代理的部署应单独验收，不能通过放开内网拨号绕过检查。

## 测试结构

服务内部增加未导出的 `externalProbeClient` 测试注入点，默认 nil；生产构造函数、配置、请求 DTO、API 和插件均没有入口修改它。nil 始终创建受保护客户端。旧测试只在测试代码中注入 httptest 客户端，生产规则没有内网例外或测试模式开关。

回归包括：默认服务拒绝内网且内网服务收到 0 次请求、IPv4/IPv6 与映射地址、允许公网地址的拨号检查、公网重定向内网拒绝、重定向上限和协议限制、模拟 HEAD/GET 之间解析地址变化、HEAD→GET 的 Range 与元数据，以及原有媒体服务和本地存储驱动测试。公网成功链路使用受控 fixture，不对真实公网资源发起探测；没有用隔离测试冒充实际 CDN、私有网络或云元数据验收。

```sh
cd backend
go test ./internal/service/media ./internal/transport/http/admin/media ./internal/infra/media/... ./pkg/corex/db/persistence/repository/media -count=1
go test -race ./internal/service/media -count=1
go vet ./internal/service/media ./internal/transport/http/admin/media ./internal/infra/media/... ./pkg/corex/db/persistence/repository/media
```

## 后续合入与部署

本分支仅作为 develop 的候选修复。推送后应创建以 develop 为目标的独立 PR，按审阅结果再合并；不得直接合入 release。上线应使用既有后端发布流程，验证真实公网外链登记和普通上传。无需数据库迁移、Capability seed 或私网白名单。用户启动的服务不由本次整合任务重启。
