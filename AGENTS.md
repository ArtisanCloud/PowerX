# PowerX Agent Governance

## Git 分支管理

- 默认在用户当前所在的 Git 分支上进行修改。
- 创建、拆分或切换 Git 分支前，必须先获得用户的明确允许；不得擅自执行。
- 创建独立 worktree、checkout 或使用 detached HEAD 进行开发，同样必须先获得用户的明确允许，不得以隔离任务为由绕过上述规则。

## 调试服务进程管理

- Agent 可以为开发、联调和验收临时启动服务，启动时必须记录进程 PID、启动命令、工作目录及监听端口，明确进程归属。
- 调试结束后、向用户交付前，必须关闭 Agent 自己启动的服务及其子进程，并确认进程已经退出、相关监听端口已经释放。只有用户明确要求保留运行时，才可以继续运行。
- 日常服务由用户自行启动。需要重新编译或重启时，告知用户具体操作，不得在调试结束后遗留临时服务进程。
- 不得擅自关闭或重启用户启动的进程；清理前必须核实 PID 和进程归属。

## Framework delegated Host capability rule

When a Core capability is intentionally consumed by PowerX Framework through a
local Host simulation or a delegated Core binding, it MUST declare both
authorization planes in the same formal platform capability contract:

- An Admin plane, when an admin surface exists: `admin_user` + user JWT + RBAC.
- A service plane: a fixed typed `core_internal` binding with `service_actor`.
- When local Host simulation supports `PX_GATEWAY_API_KEY`, the service binding
  MUST include explicit `api_key` metadata. Core MUST materialize it into an
  `allow_api_key=true`, `api_key_explicit=true` IAM permission and evaluate it
  through published capability + tenant registration + live API-key grant.

Do not use an Admin REST permission as the service grant. Do not grant a broad
`admin_manage` capability merely for a Framework selector; define a
least-privilege `service_read` capability. A `core_internal` capability MUST
use a stable `core://` endpoint, method `INVOKE`, and a typed operation DTO;
it MUST reject arbitrary HTTP method, endpoint, headers, or raw proxy payloads.

Changing this contract is a breaking change: callers using an old free-form
payload must fail explicitly and be migrated to the typed binding. After a
capability change, run `make capability-check`, `make capability-seed`, the
affected API-key/grant tests, and verify `grant-status` for the development
Host API key.

## 页面设计与产品体验

- 默认禁止将独立功能区、说明、表单和列表以瀑布式长页连续堆叠。多个独立主题默认采用“左侧纵向 TabView 导航 + 右侧当前内容”；已有顶部分类 Tab 时，各分类内部同样必须按任务分区，不能以顶部 Tab 替代内部信息层次。
- 一次只展示当前分区；切换时保留必要上下文、表单和进行中的任务。列表／表格／时间线等同类内容可以自然滚动，但不能借此堆叠不同任务。文章或用户明确要求连续阅读的页面除外。
- 页面设计必须具备产品思维：先明确目标用户、主要任务和查看／操作流程，再组织功能与信息；不得以“所有功能和元素都已放到页面上”作为完成标准。
- 信息呈现必须有层次。优先展示核心信息、关键状态和主要操作，辅助说明、低频操作及详细数据按需展开，避免同等突出地堆叠所有内容。
- 按照用户的查看逻辑组织“概览 → 分块 → 详情”。可以通过明确的按钮打开弹层、抽屉或详情页，让用户逐层查看，并保留当前操作上下文。
- 对具有独立主题的信息或不同任务阶段，可以使用 TabView 分块呈现；标签名称和默认选中项必须符合用户理解与使用频率，核心信息和主要操作应易于发现，避免为完成同一个任务频繁切换 Tab。
- 使用标题、分组、间距和视觉权重表达主次关系。弹层、Tab 和折叠应服务于阅读与操作效率，避免不必要的嵌套和跳转，也不得隐藏完成当前任务所必需的信息。
- 页面验收必须沿实际用户流程检查：能否快速理解当前状态、找到主要操作、按逻辑查看详情并完成任务；同时保证加载、空数据、失败与成功状态有清晰反馈。

## 异步任务状态与事件推送

- 备份、恢复、导入、导出等异步任务的页面状态必须使用 WebSocket／SSE 推送；轮询仅限临时诊断，不作为常驻方案。不得用 `setInterval` 或递归 `setTimeout` 周期查询任务状态替代推送接入。
- 初次载入、用户手动刷新、收到变更事件和断线重连后的状态同步可以读取接口。连接保活与有上限退避的断线重连不能夹带周期性状态查询；连接异常必须明确反馈，并保留当前任务与表单。
- 恢复涉及 Core 重启或维护切换时，进度必须由独立于业务数据库、插件及业务 worker 的维护执行器通过只读、单任务、限时票据推送。默认复用同一个 PowerX 程序和原端口，通过维护启动模式完成恢复；不得要求用户另外管理常驻协调器、第二实例或另一套端口。不得依赖已停止的业务路由，也不得把票据写入 URL 或浏览器持久存储。
- 后台统一 TaskBus 的阻塞式消费、业务定时调度及任务重试与页面状态轮询分别评审；消费者必须遵守正式 TaskBus 合同，不得自行新增私有轮询队列。
- 验收应检查实际请求记录：无状态变更时不周期请求详情／列表，事件到达能更新，分区切换保留上下文，离开页面或切换登录身份后释放旧连接，维护期间独立进度仍可见。
