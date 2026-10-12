# Agent Session 与插件授权交付台账

状态（2026-09-08）：P1 正式 Session 已通过本地真实 STS/HTTP/runtime 验收，可交给 Framework 对齐；P3 的 manifest 撤权及 PostgreSQL 并发验收通过。P2 正式 REST binding 与具体 API Key 映射已补齐本轮发现的缺口，但不将静态 runtime、所有动态插件能力和远程部署宣称为全部通过。

## P1：Session Host Contract（本地真实运行验收通过）

以下边界已经确认，不得通过保留旧请求格式规避：

- 外部引用为 `agent_uuid`、`session_uuid`、`message_uuid`。
- tenant 与插件服务主体取自已认证 STS；本轮不以 tenant 身份替代 API Key 的凭证主体。
- 人工历史会话不自动归属插件；同租户插件之间默认隔离。
- Session 可见性与 Agent 执行授权分别校验。
- 外部消息追加只允许 `user`。
- 追加与 Invoke 必须声明幂等作用域、有效期及相同键不同内容的冲突。
- SSE 订阅不是执行入口；断开不等同于取消，重连不得再次执行。
- 需要单独取消操作、明确终止事件与流式错误事件。

人工会话与插件服务会话不是同一资源集合：

- `internal/server/agent/persistence/model/session_gorm.go`：仍有 numeric Agent/User 关联。
- `internal/server/agent/persistence/model/chat_gorm.go`：消息仍缺自身 UUID，并使用 numeric Session/Agent 关联。
- `internal/transport/http/admin/agent/agent_session_handler.go`：输入与既有 OpenAPI 不一致。
- `internal/transport/http/admin/agent/chat_handler.go`：Session Invoke 在执行前直接追加消息；现有 Session SSE 会启动执行。

上述历史人工记录不回填插件归属，也不通过新 Host API 访问。它们的全量 numeric 关联迁移不在本次独立服务会话模型变更中；不得声称历史人工模型已经 UUID 化。

新增 `agent_service_sessions`、`agent_service_messages`、`agent_service_invocations`，集中挂载在 `pkg/corex/db/database/migration.go`。新关联为 UUID，外部响应使用独立 DTO，不序列化 GORM 模型。

正式规格：`specs/007-integration-gateway-and-mcp/contracts/agent-session.http-openapi.yaml`。
统一前缀：`/api/v1/tenant/agent/sessions`。

| Method | 路径后缀 | Capability |
| --- | --- | --- |
| GET / POST | 空 | `com.corex.agent.session.manage` |
| GET / PATCH / DELETE | `/{session_uuid}` | `com.corex.agent.session.manage` |
| POST | `/{session_uuid}/archive` | `com.corex.agent.session.manage` |
| GET / POST | `/{session_uuid}/messages` | `com.corex.agent.session.manage` |
| POST | `/{session_uuid}/invocations` | `com.corex.agent.invoke` 且 `com.corex.agent.session.manage` |
| GET | `/{session_uuid}/invocations/{invocation_uuid}` | `com.corex.agent.session.manage` |
| POST | `/{session_uuid}/invocations/{invocation_uuid}/cancel` | `com.corex.agent.session.manage` |
| GET | `/{session_uuid}/invocations/{invocation_uuid}/events` | `com.corex.agent.session.manage` |

授权位置：`internal/service/agent_session/service.go` 的 `owner`，每次从数据库核对发布记录、tenant registration、启用凭证与实时 allowed 集合；STS subject 必须匹配凭证 client_id，JWT 必须含有效 exp。创建和执行额外检查 Agent 同租户、active 且归属当前插件；本轮不提供跨插件共享授权。API Key 和人工 JWT 不作为该合同的调用主体。

`agent.yaml` 的旧 `/agents/sessions`、`/agents/invoke`、`/agents/stream/sse` REST binding 不再声明 STS direct。人工用户入口保留，不作为插件服务合同兼容路径。

执行适配：`internal/server/agent/runtime/service_session_executor.go` 调用 Core runtime；只有正式 final envelope 可产生完成响应，不从 token 片段拼出替代成功。2026-09-08 已通过本地 8077 后端、9001 STS Exchange 和现有模型配置的真实调用，任务进入 succeeded，SSE 返回 final/end。

执行适配已接入 Core 持久化历史及结构化 pending task：Invoke 在 Session 行锁内冻结当前消息之前的历史，最多 128 条、内容合计 256 KiB；超限明确 409，不裁剪或降级为无上下文执行。历史通过 locale 模板作为对话数据输入 Runtime。pending task 与 assistant 消息和成功 invocation 在同一事务中提交；失败/取消不覆盖状态。pending 期限 72 小时，过期为 409 AGENT_SESSION_CONTEXT_EXPIRED，状态损坏为依赖错误；插件无状态写入入口。新字段为 Session 的 pending_task JSONB、pending_task_expires_at，仍通过集中模型迁移创建；不调用人工 numeric SkillStateStore。

多轮 Service 回归覆盖首次保存、下一轮读取历史/已收集参数、完成清除、过期拒绝；真实 provider 多轮执行仍属于部署验收，不能用测试执行器替代。

### 状态与重试

- Session：active → archived/deleted；archived → deleted。删除保留 tombstone，重复删除成功，其余读取返回 404。存在活跃执行时，变更返回 409。
- 追加消息和 Invoke 必须提供 Idempotency-Key；作用域为 session UUID + 操作 + key，24 小时有效。同键不同内容返回 409；过期键明确失败，不重新执行。
- Invoke 先持久化 execution，返回 202，状态为 running/cancelling/succeeded/failed/cancelled。每会话最多一个活跃执行。
- 执行期限 10 分钟；进程中断后过期记录明确失败，不自动恢复执行。取消为协作式，不承诺撤销已经完成的外部副作用。
- SSE 是持久化状态／最终结果订阅，不是 token delta 流。事件为 state、final、error、end；重连读取同一 invocation，不启动执行，不提供历史事件 cursor。
- 断开订阅不取消执行；取消必须调用独立 cancel。撤权后新查询／订阅失败；已启动执行不等同于可回滚事务。

### 已运行验证

在 backend：

```bash
go test ./internal/service/agent_session ./internal/transport/http/openapi/agent_session ./internal/http -count=1
go test -race ./internal/service/agent_session ./internal/transport/http/openapi/agent_session -count=1
go test ./internal/server/agent/runtime -run TestServiceSessionFinalEnvelope -count=1
go test ./cmd/app ./cmd/database -run '^$'
POWERX_SESSION_TEST_POSTGRES_DSN='host=/tmp dbname=postgres sslmode=disable' go test ./internal/service/agent_session -run TestPostgresMigrationAndConcurrentIdempotency -count=1
```

以上定向测试已通过。PostgreSQL 测试使用随机临时 schema，结束删除；覆盖重复模型迁移、12 并发同键追加与 Invoke，并验证只产生一个用户消息和一次执行。不是在 PowerX 业务数据库执行完整 make migrate。

2026-09-08 续收尾：新增多轮 ExecutionMemory、具体 Gateway key 授权与认证缓存上下文测试通过；相关 Session/GrantStatus/认证包竞态测试通过。Runtime 参数用例曾因共用全局候选注册表导致整包/重复运行失败；已将四个相关用例改成独立 Manager，Runtime 整包连续三次通过，没有修改生产参数解析行为。全部 OpenAPI 包及 app/database 编译与测试通过；无测试文件的子包仅计作编译通过。

HTTP 测试经实际 JWT middleware 和实际 Handler/Service；执行器使用可控测试替身。覆盖 STS、401、身份覆盖/非法 UUID 400、跨租户/跨插件 404、冲突 409、旧 token 撤权 403、依赖 503、成功终止事件；Service 测试覆盖断连不取消与显式取消。

`make capability-check` 通过：declared=972、referenced=43、rest_routes=855、candidates=926、ignored_route_rules=4。另有 OpenAPI 校验和每条已注册路由的 capability binding 检查。

本地真实验收已覆盖 STS Exchange、Session CRUD、仅 user 消息、UUID/身份覆盖校验、跨租户/跨插件 404、幂等冲突、真实 Invoke succeeded、重复 Invoke 不再次执行、SSE final/end、断连不取消、显式取消、执行依赖失败 error/end、旧 token 撤权。远程生产环境的迁移与部署不在此结论内；多轮 pending task 目前是 Service 合同回归证据，不宣称已做业务场景的真实多轮补参验收。

## P2：能力映射与统一入口授权（部分落地）

88 条显式 STS direct REST binding 的声明快照见 `sts-direct-binding-map.md`，包含 method/path、capability、声明鉴权、资源范围、源文件。未把静态入口或 API Key 的未验证语义混入该快照。

正式 YAML 中 `sts_direct: true` 的 REST binding 现在由 `internal/http/auth_subject_validator.go` 自动解析 capability，并在 JWT 校验回调中调用 `DirectGrantService.AuthorizeSTS`。该检查不是 URL 放行：逐次查询 active tenant、已发布能力、最新版本 tenant registration、启用的插件凭证与当前 allowed 集合，同时验证 STS subject 与 credential client_id 一致。数据库故障为稳定 503，未授权为 403，不使用缓存中的旧授权。

同一路径命中多个正式能力时全部校验；静态 runtime 入口仍由各自服务校验业务授权，不借此自动授予任何能力。API Key 不经过该 STS 检查，保持其独立授权链路，不能据此宣告 API Key 全模块一致性已经验收。

`TestSTSDirectGateRevokesExistingTokenBeforeHandler` 经生产 JWT subject callback 验证 AI Invoke 与 Session 两类正式路由：先允许进入 handler，凭证撤权后同一 token 为 403；最新 registration 停用，即使历史版本仍 published 也为 403；数据库关闭为 503。这里的业务 handler 是测试探针，不是 LLM 成功执行证明。

grant-status 同样仅采纳最新 registration；旧 published 版本不能覆盖最新停用状态，返回 unknown / CAPABILITY_TENANT_NOT_REGISTERED（以代码常量为准）。正式 tenant Customer Auth 的 auth 路径片段不再被误当成根级认证命名空间；根级 auth 与 admin/internal 前缀仍被拒绝。

仍须逐操作核对 method/path、capability、认证方式、tenant registration、实际 grant 校验位置和测试。
扫描一致性不等于执行授权正确，不允许通过给开发 Key 扩权替代验收。

API Key 已改为具体凭证授权：认证中间件（含缓存命中）将已验证 key hash 放入私有类型的 request context；grant-status 不再从 profile permission 推断授权。它只使用已发布权限元数据中的显式 api_key binding 和该 key 的实时 Gateway permission，并与 Host 的 HasPermission 共用匹配函数。缺少可信 key 标识返回 401，停用 key 返回 401；同 profile 的其他 key 不继承该 key 的 grant。没有显式 API-key binding 的能力不会因生成的路由 scope 被报告为 granted。

发布时须运行既有 seed 流程更新 Permission.Meta 的 api_key_explicit 标记；这只是标注正式 binding，不授予 key 新权限。旧元数据未同步会明确显示 not_granted 或 grant-status 自身 403，不回退到 profile 权限。当前覆盖正式 Host binding；插件动态能力和静态 runtime 入口仍需分别验收，不能把这轮测试扩大为所有 API Key 调用已对齐。

2026-09-08 真实库核验发现：Customer/Media/Plugin Release 的 11 项能力缺少 title_i18n/description_i18n，权限模板同步忽略了目录加载错误，造成能力发布成功但显式权限映射未写入。已补齐 locale 元数据、将目录加载错误改为 fail-fast，并在 capability-seed 中同步权限定义（不绑定现有 key、不授予新 grant）。IAM、Metadata、Media、Plugin Release 的 API-key 精确映射已按各 Host authorizer 的实际 scope/action/resource 补入正式 REST binding。本地 capability seed 已执行成功；真实临时 API Key 的 IAM 目录请求由 200 变为撤权后 403，grant-status 对应由 granted 变为 not_granted。

精确映射见 `host-api-key-binding-map.md`：23 项 capability、50 条 binding，数据库数量已核对一致。权限生成回归直接加载仓库正式目录，防止仅用测试 YAML 而遗漏真实目录错误。较大范围的 Integration Gateway 合同回归另发现共享 OpenAPI 的 components.parameters 重复定义，已合并并验证解析通过。

统一授权验证命令：

```bash
cd backend
go test ./internal/http ./internal/service/capability_registry ./pkg/auth/middleware -count=1
```

## P3：授权同步（本地撤权与 PostgreSQL 并发验收通过）

### 本轮代码

`TenantPluginInstanceService` 不再对 `allowed_capabilities` 做只增不减的 union。
安装同步和启用按凭证行加事务锁；停用不再把读到的旧凭证 JSON 整体写回。
首次凭证创建使用冲突不覆盖，不能用并发创建生成的未持久化 secret 替换现有凭证。

凭证文档字段：

| 字段 | 来源与用途 |
| --- | --- |
| `manifest_capabilities` | 当前成功同步的 manifest required，包括空数组 |
| `independent_capability_grants` | Core 审批表生成的 capability → approval UUID 映射，不能从输入 JSON 自证 |
| `unattributed_capabilities` | 旧有效集合中无法归属的能力，仅供审核，不产生授权 |
| `allowed_capabilities` | 当前 manifest 与有效独立审批的并集 |
| `capability_grant_schema_version` | 当前为 1 |

新增 `plugin_capability_approvals`：稳定 UUID、tenant UUID、plugin ID、capability ID、审批者 user UUID、状态、撤销者 user UUID 和撤销时间。
同一 tenant/plugin/capability 仅允许一条 active 审批；撤销保留记录，重新审批创建新 UUID。
集中迁移入口：`pkg/corex/db/database/migration.go`。

`SetIndependentCapabilityApproval` 是仅允许可信 root 用户上下文的 **Service 方法**，不是已发布 HTTP Host API。
它检查审批能力已发布且当前 tenant 已注册，更新审批与有效凭证使用同一事务。
尚未交付管理员审批界面或 HTTP 操作入口，插件没有独立审批权限。

### 历史授权处理

1. 部署前备份凭证和 manifest，记录原 allowed 集合；不要记录或输出 secret/hash。
2. 先运行集中迁移，创建审批表，再部署使用该表的二进制。
3. 对已安装插件通过正式安装/升级或重新启用链路同步当前 manifest。
4. 当前 manifest 声明的能力归为 manifest 来源；其余无来源历史能力进入待审核集合并失去有效授权。
5. 确需独立保留的权限必须经管理员显式审批；不能把历史全部推断为管理员授权。
6. 单纯 restart、migrate、seed 不等同于插件 manifest grant 同步。

审批表为空不会保留凭证 JSON 自填的独立来源。缺表、JSON 损坏或依赖故障明确失败，不降级为旧的 union 逻辑。

### 已运行验证

在 `backend` 目录：

```bash
go test ./internal/service/plugin -count=1
go test -race ./internal/service/plugin -count=1
go test ./tests/contract/plugin_grants -count=1
go test ./cmd/app ./cmd/database -run '^$'
```

已通过。覆盖新增、manifest 删除/清空、重复同步、来源自证拒绝、独立审批保留/撤销/再次审批及非管理员拒绝。
另有跨服务测试复用同一个已认证 STS 上下文：撤销 Metadata 能力后，grant-status 返回 not_granted，Metadata 授权返回 403。

另已通过 `TestManifestRevocationWithExistingSignedToken`：同一枚测试密钥签名的 STS claims token，经真实 JWT middleware、已注册的 grant-status 和 Metadata HTTP handler，先成功读取，再在 manifest 同步撤权后返回 not_granted/403；required 全部清空后 grant-status 自身也为 403。缺失/非法凭证为稳定 401。

该测试使用进程内 HTTP recorder 和 SQLite，没有注入认证 claims 或替换业务 service，但不包含部署环境的 STS 签发/keyring，也不是所有 Host 入口旧 token 撤权的证明。

### 2026-09-08 本地验收

用户已执行业务库迁移和重启；只读核验确认三个 Session 表及 pending_task 字段存在，关键能力为 published。随后执行了修正后的 capability seed：972 项能力、4 个租户 registration；没有重跑全量业务 seed，也没有重启 VS Code 服务或修改已有插件授权。

```bash
cd backend
POWERX_LIVE_TEST_DSN='host=/tmp dbname=powerx sslmode=disable' \
POWERX_LIVE_TEST_URL=http://127.0.0.1:8077 \
POWERX_LIVE_TEST_STS=127.0.0.1:9001 \
POWERX_LIVE_TEST_INVOKE=1 \
go test ./tests/contract/plugin_grants -run TestLive -count=1 -v

POWERX_SESSION_TEST_POSTGRES_DSN='host=/tmp dbname=postgres sslmode=disable' \
go test ./internal/service/plugin -run TestPostgresConcurrentApprovalAndManifestRemoval -count=1 -v
```

两组通过。Live 测试只接受本地地址；专属凭证通过 Core Enable 生成，真实 STS Exchange 签发，密钥不输出。它验证同一枚旧 token 在 required 删除和清空后立即失权；API Key 测试只为新建临时 key 授予明确的两项权限。PostgreSQL 测试使用随机隔离 schema，覆盖重复模型迁移、12 路并发审批/空 manifest 同步、独立来源保留、重复撤销和审计记录保留。

最终补充回归：Session/Plugin/GrantStatus/权限定义/认证中间件/HTTP 合同的定向 race 测试通过；Session 与审批两个 PostgreSQL 用例在 race 下通过；Integration Gateway、Skills 合同包通过；所有 OpenAPI 子包及 app/database 编译/测试通过。Agent Lifecycle 合同文件带 ignore build tag，本次命令显示 no tests to run，不计作验收通过。make capability-check 与 git diff --check 通过；未声称全仓库所有测试均已运行。

清理核验：临时插件凭证、Agent/Session/消息/执行记录、临时 API Key/profile/权限记录，以及临时 PostgreSQL schema 均已移除。未改动 AI Craft 等既有安装实例授权。

### 结论边界

- Framework 可以依据本文规格接入新的 Agent Session，不再等待本地基础 CRUD/Invoke/SSE/撤权验收。
- 尚未发布管理员独立审批 HTTP/UI，现有交付为受限 Service 方法与审计模型；这不等于插件可自行审批授权。
- 静态 runtime 队列/总线和动态插件能力不是本次 Live 测试覆盖对象；不得将正式 Session 和 IAM 的通过外推为这些入口已逐项验收。
- 远程 prod/dev 部署、本地真实多轮业务补参不包含在上述通过结果内。
