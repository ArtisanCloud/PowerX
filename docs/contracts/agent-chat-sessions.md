# Agent 单聊与团队会话归属

同一个 Agent 可以独立对话，也可以担任多个团队的负责人。负责人相同不代表会话相同。Session 创建后固定归属，切换页面不能改变其执行模式。

## 持久化与查询

继续使用 `agent_chat_sessions`，新增两个字段：

| sessionType | teamId | 含义 |
| --- | --- | --- |
| `agent` | `0` | 独立 Agent 会话，`agentId` 为所选 Agent |
| `team` | 正整数 | 指定团队会话，`agentId` 记录创建时负责人 |
| `legacy` | `0` | 历史归属待确认，只能查看，不能继续执行 |

创建 `POST <API_BASE>/agents/sessions` 的 JSON 使用 `sessionType`、`teamId` 和既有 `agentUuid` / `agentId`。创建团队会话时，后端验证团队租户和负责人；不能通过 `meta` 声明团队归属。新建请求省略类型和团队 ID 时保持旧单聊调用兼容。禁止创建 `legacy` 会话。

列表 `GET <API_BASE>/agents/sessions` 增加 `session_type=agent|team|legacy`，团队列表必须带 `team_id`；既有 Agent、租户、环境过滤继续生效。没有工作区参数的旧列表调用只返回单聊。Singleton 查找也按类型和团队区分。

详情、消息历史、重命名、归档、删除携带同样的查询参数时，必须与存储归属一致。按 ID 进行且未声明工作区的历史读取保持兼容，返回记录本身的归属，不改变它。

## 执行与重试

- SSE 发送、编辑后重试与 Run 续订带 `session_type`，团队请求另带 `team_id`。非流式 invoke 使用 JSON 字段 `session_type`、`team_id`。
- 所选 Agent、类型或团队与 Session 不符时返回 `409 / agent.session_scope_mismatch`，在写消息、截断历史或开始模型执行前拒绝。
- `legacy` 续写返回 `agent.session_legacy_read_only`；显式冒充其他类型同样拒绝。
- 调试追加消息校验 URL Session、JSON `sessionId`、`agentId`、`sessionType`、`teamId`；摘要触发与审批后恢复执行同样遵守会话归属。
- 已受理的 Run 有固定 Session；续订只重放运行状态，不因为切换页面重新发起任务。

## 历史数据升级

增量迁移挂载在既有 `MigrateAgentModels`，由正式 `database migrate` / 安装升级流程调用。只增加字段、索引并回填归属，不删除会话、消息或改写正文。

旧行默认 `legacy`。只有每个用户回合都具有对应的服务端执行快照、显式团队字段，且全部回合指向同一类型／同一团队，才自动归类。团队还必须真实存在、属于同一租户，并与记录的负责人一致。缺失证据、混合模式、多个团队、未完成回合或已压缩摘要都保留待确认。迁移可重复执行。

待确认记录在页面“待确认历史”分区查看；不根据标题、负责人名称或当前菜单猜测归属，也不提供一键猜测迁移。需要重新执行时，在“当前会话”新建正确类型的会话。

前端会话列表、加载状态、选中会话、上次选择的 Agent、消息缓存和后台 Run 续订定位均按用户、租户、环境、类型及团队分区。异步响应回写发起请求的原分区；切换分区不覆盖另一个分区的选中会话。

## 上线顺序与验收

本轮验证记录见 [2026-10-15 会话隔离回归](evidence/agent-session-scope-validation-20261015.json)。后端作用域、真实 Handler 调用链、增量迁移幂等与原文保留测试通过；前端 39 项测试通过、1 项既有外部 fixture 用例跳过；真实 Nuxt 页面在 1440px / 390px 的 API fixture 浏览器测试通过。额外旧 `integration_gateway` 用例在进入会话流程前因既有 agents fixture 缺 `type_id` 失败，未计为通过。尚未执行本次目标数据库迁移或实际模型验收。

先备份目标数据库，停止旧 Core / Worker，再使用本次构建的 database 程序和目标环境的既有配置执行迁移，最后启动新 Core 并刷新前端。开发目录命令为：

```bash
cd backend && go run ./cmd/database migrate -config etc/config.yaml
```

迁移是集中迁移，会执行该构建中已注册的迁移；不运行 reset、refresh 或全量 seed。日常服务由用户自行停启。此修复的源码与隔离测试通过不等于目标数据库已经升级。

验收时用同一个负责人依次建立单聊、团队甲、团队乙会话，确认列表和刷新历史相互独立；切换工作区后返回原工作区，选中会话应保留。将团队甲的 Session 用作单聊、团队乙或重试请求，必须收到 409，且原消息数量不变。检查待确认历史可以阅读、无法续写，并在桌面和窄屏完成分区切换和新建操作。

回滚程序前先停止新服务。旧程序无法理解会话隔离，不能允许其继续写入这些 Session；保留新增字段及原始数据，采用匹配版本恢复或前向修复。
