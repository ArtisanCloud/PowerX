# 验收手册：Agent Runtime 持久化调度

**状态**：目标验收步骤，完整业务和生产验收未完成；下方附已实测的本机环境准备命令，各任务证据以 tasks.md 为准。

1. 准备两台 Core、至少两个 Agent Worker、一台声明单物理槽的 Ollama、Redis AOF + `noeviction`、对象存储与可查询的业务受理记录。记录配置版本、部署版本、RPO/RTO 和测试租户。
2. 用同一消息提交两个独立模型任务，保留 `run_id/message_id/trace_id`。验证两任务同时就绪，一个 `running`、一个 `queued`；排队任务的 `request_started_at` 仅在获取槽位后出现，最终只生成一条 assistant 消息。
3. 在第二个模型任务排队时提交不同模型或非模型独立任务，验证它不受该池阻塞；依赖任务须等前置任务完成。确认 `event_seq`、Run 快照、SSE 和 Trace 一致。
4. 中断浏览器、Core 和一个 Worker，分别用 `after_seq` 续订；制造过期租约与重复投递，验证旧 fencing token 拒写且副作用未重复。无幂等回执的模糊结果须进入人工核查。
5. 关闭 Redis 或使归档存储不可用，验证新受理/领取失败关闭、在途状态不虚报成功、归档背压可见。恢复后按受理凭据/回执对账，测试备份恢复落在声明 RPO/RTO 内。
6. 提交超过队列上限与等待时限的请求，分别核对容量拒绝、`queue.timeout`、`provider.timeout` 和 `run.deadline`；抓取队列 P95/P99、模型并发峰值、Redis/DB 写入量与归档积压。

验收记录必须包含实际 API/SSE 输出、Run/Task 状态和 `event_seq`、部署配置及故障演练证据。仅有单元测试或文档不算交付。

## 管理端联调启用条件

先完成配置依赖，再重启后台。仅执行 migrate 或重启不会自动启用新链路。

```yaml
ai:
  runtime:
    durable_sessions:
      enabled: true
      admin_chat_enabled: true
      report_bucket: powerx-agent-runs
      hot_retention: 168h
      archive_max_pending: 1000
      archive_max_age: 1h
      worker_concurrency: 4
      scan_interval: 5s
      lease_ttl: 30s
    run_deadline: 30m
    queue_wait_timeout: 10m
    physical_model_pools:
      - pool_id: ollama-local-qwen3-8b
        provider: ollama
        endpoint: http://127.0.0.1:11434
        model: qwen3:8b
        capacity: 1
        max_waiting: 100
        lease_ttl: 6m
```

- `storage.s3` 必须指向可访问的私有 S3/MinIO，预先创建上述桶并授予指定凭据读写删除权限。启动执行写入/读回校验，不能改用 local 存储绕过检查。
- Event Fabric Redis 必须持久配置 `appendonly yes`、`maxmemory-policy noeviction`，并明确 `appendfsync` 对应的 RPO。共享开发 Redis 的变更应协调其其他消费者。
- 在 `backend` 执行 `go run ./cmd/database migrate -config etc/config.yaml`，然后用原启动方式重启 Core；重启后确认 Worker 启动且没有门禁错误。
- 从营销页面发消息，首个受理 meta 应包含 UUID Run ID 与 `durable_run: true`；以同一 Run ID 和 `after_seq` 续订不得再次提交 q。核对同一消息内任务状态、最终数据库只有一条助手消息，以及追踪报告 `summary.state_backend=redis`。
- 上述配置是模板，不表示当前开发环境已启用或营销案例已验收。

## 2026-10-02 本机联调命令

依赖：本机已有 `redis-cli`、`minio`、Go 与 Python 的 PyYAML/boto3/requests。命令在仓库根目录执行。凭据从忽略提交的开发配置读取，联调配置和证据文件以 0600 权限保存。Python 环境缺依赖时使用独立 venv：

```sh
python3 -m venv /tmp/powerx-agent-runtime/venv
/tmp/powerx-agent-runtime/venv/bin/pip install PyYAML boto3 requests
```

下文 `python3` 可替换为该 venv 的 `bin/python`。源配置的 `auth.jwt_secret` 必须符合现有密钥校验；示例占位值不能启动 Core。使用已配置的私有文件作为 `--config`，或按部署要求持久保存独立随机密钥；不要修改正在运行后台的签名密钥来完成联调。

1. Redis 配置需持久启用 AOF/noeviction，并核对 `INFO persistence` 的 `aof_enabled=1`、`aof_last_write_status=ok`、`aof_last_bgrewrite_status=ok`。2026-10-02 本机已执行 `CONFIG SET appendonly yes` + `CONFIG REWRITE`，保留 everysec，故本机 AOF 的 fsync 周期为约一秒，不作为生产 RPO 演练证据。
2. 本机 MinIO 由 Homebrew 管理并在用户登录后启动，沿用固定数据目录。日常启动与状态检查：

```sh
brew services start minio
brew services info minio
```

2026-10-06 已将本机服务接入 Homebrew：可执行文件 `/opt/homebrew/opt/minio/bin/minio`，数据目录为 `backend/.cache/agent-runtime/minio/objects`，API 监听 `127.0.0.1:9000`，设置 RunAtLoad/KeepAlive。官方 tap 配方没有 service 定义，因此提供自定义服务文件并放入当前安装 keg；`brew services list` 可能不显示它，以 `brew services info minio` 为准。升级 MinIO 后，若新 keg 没有服务文件，应重新放入服务定义再启动，不能默认为升级后仍支持普通 start。

本机私有服务源为 `~/.config/powerx/homebrew/homebrew.mxcl.minio.plist`，凭据文件 `~/.config/powerx/homebrew/minio.env` 权限 0600，日志 `~/Library/Logs/PowerX/minio.log`。已注册文件为 `~/Library/LaunchAgents/sh.brew.minio.plist`，实际服务 label 为 `homebrew.mxcl.minio`。服务从私有凭据文件加载 MinIO 环境，不依赖 Python 前台启动脚本或 PowerX 的生命周期。PowerX `storage.s3` 凭据/区域变更时必须同步私有凭据文件并重启 MinIO。

首次注册或明确使用保留的源文件重启：

```sh
brew services restart minio --file="$HOME/.config/powerx/homebrew/homebrew.mxcl.minio.plist"
```

此服务是当前用户登录后启动，不是无需登录的系统启动服务。`agent-runtime-dev.py minio` 仅保留为其他隔离环境的手动工具，本机日常使用 Homebrew，避免同端口重复启动。

3. 生成独立联调配置并验证报告桶：

```sh
python3 backend/scripts/ops/agent-runtime-dev.py prepare-config --output /tmp/powerx-agent-runtime/config.yaml
python3 backend/scripts/ops/agent-runtime-dev.py --config /tmp/powerx-agent-runtime/config.yaml check --create-bucket
```

MinIO 区域固定与 `storage.s3.region` 一致；桶默认私有。预检执行实际写入、读回摘要和删除。启动命令覆盖仓库 `.env` 的 HTTP/gRPC 端口与日志选项，避免联调误占正式端口。

4. 构建并启动独立 Core：

```sh
(cd backend && go build -o /tmp/powerx-agent-runtime/powerx ./cmd/app)
python3 backend/scripts/ops/agent-runtime-dev.py --config /tmp/powerx-agent-runtime/config.yaml core --binary /tmp/powerx-agent-runtime/powerx
```

默认 HTTP=18077、gRPC=18078，均使用本机地址；正式 8077 后台的重启仍按原启动方式和已确认的持久化配置进行。

5. 用有效管理员 JWT 文件运行营销断线验收。输入文件可使用 `backend/tests/fixtures/agent_runtime/marketing_review.txt` 的合成数据，或营销团队对话手册的完整材料；该命令会创建验收会话和一轮真实消息，记录保留供排障：

```sh
python3 backend/scripts/acceptance/agent-runtime-marketing.py --auth-file /path/to/private-auth.json --tenant-uuid <tenant-uuid> --input-file backend/tests/fixtures/agent_runtime/marketing_review.txt --report-file /tmp/powerx-agent-runtime/marketing-evidence.json
```

验收只提交一次；受理后主动断开，再按同一 Run 和游标续订。断言 Redis 报告来源、唯一用户/助手消息、营销 v4 报告及 completed。保存脱敏事件、任务、Run/Trace/Session 身份。该脚本准备完成不等于真实营销验收通过；仍需有效登录和实际执行。

## 终态归档恢复检查

启用 durable Worker 后，终态结果消息先幂等保存，再归档到报告桶。`agent_admin_run_admissions` 和 `agent_service_invocations` 的 `archive_key/archived_at` 只记录已读回校验的对象定位；任务事件、租约和心跳不写入 SQL。

- 对象存储暂不可用：终态消息可读，Redis 事件保留，锚点 `finished_at` 已有值而 `archived_at` 为空；Worker 每轮扫描会再次归档。
- 对象已完成但 SQL 补写失败：Redis `archive_key` 已有值，扫描重试读回同一对象后补写定位，不重复执行任务或保存消息。
- 保存定位后，锚点退出未完成扫描。追踪报告优先读 Redis；仅当 Run 不存在且 SQL 有已完成归档定位时，按原所有权检查读取对象，报告 `summary.state_backend=object_storage`。
- 归档损坏或身份不一致必须失败，不得读本地日志兜底。管理端 SSE 和服务会话 Query/SSE 在热状态不存在时也从已验证归档读取原 Run。启用生命周期策略后，仅已校验归档并保存数据库定位的运行设置统一绝对到期时间；对象不可用或定位未保存的运行不清理。

## 2026-10-04 清理、背压与刷新恢复

- `hot_retention` 默认 `168h`，范围 `1h..2160h`。到期以最初 `archived_at + hot_retention` 为准，补写重试不续期。Run 的状态、事件、计划、任务/回执、outbox 和投递游标一同设置到期时间。SQL `hot_expires_at` 表示设置成功；空值继续恢复扫描。Redis 已到期而 SQL 补写曾失败时，只确认对象归档和到期索引，绝不重建任务。
- 保存归档定位后移除该 Run 的共享队列、延迟重试及死信残留；共享队列不设置整体 TTL。退休标记阻止迟到投递、NACK 和延迟提升重新入队；队列去重与 fence 元数据按该 Run 的保留期到期。清理按 200 条分页，保留其他 Run 的记录。
- `archive_max_pending` 默认 `1000`，`archive_max_age` 默认 `1h`（允许 `1m..168h`）。Worker 按扫描间隔聚合待归档数量与最老终态时间，并发布 `agent:archive:{<env>}:health`。观测达到任一阈值后，新受理返回 `archive.backpressure`；服务 HTTP 为 429 和 `Retry-After: 30`，管理端 SSE 给出相同原因及建议等待秒数。已受理 Run 的订阅/恢复继续。
- 健康记录有效期为三倍扫描间隔；未初始化、过期、读取失败或 Core 策略不一致返回 `archive.health_stale`。这是周期性容量保护，阈值更新存在一个扫描周期的观测延迟；生产阈值必须按实际内存及并发规模配置。它不替代 Redis 故障切换或容量验收。
- 多 Core 必须使用相同生命周期策略。操作员优先执行 `python3 backend/scripts/ops/agent-runtime-dev.py --config <联调配置> worker-health` 查看共享状态与实际 Redis DB。Core 的连接按 Event Fabric → Queue → Cache 回退，`event.fabric.redis_db=0` 时沿用 Queue DB；本机当前为 DB 5。手动检查示例为 `redis-cli -n 5 GET 'agent:archive:{dev}:health'`，字段包括 `pending/oldest_at/observed_at/blocked/reason/policy`；每个 Core 每轮只做环境级低频聚合，不为 token 或任务心跳写 SQL。

### 2026-10-04 共享执行配额配置与验证

`ai.runtime.max_concurrent_tasks` 现在约束跨 Core 的单 Run 执行槽（默认 4）；`ai.runtime.durable_sessions.tenant_concurrency` 默认 16，范围 `1..10000`，必须不小于 Run 配额。`worker_concurrency` 仍是单 Core 本地槽，不是租户配额。规划与执行都占用共享槽，物理模型容量池独立治理。

配额/租约策略在环境键 `agent:scheduling:{<env>}:policy` 固定，配置不一致时启动失败并返回 `scheduling.policy_drift`。变更流程为：暂停新受理 → 等待活动 Run 排空并确认槽已释放 → 停止所有旧 Worker → 备份并移除该环境的策略键 → 使用一致新配置启动 Worker → 检查状态后恢复受理。不要在活动 Run 存在时删除策略键；旧策略需按相同排空流程回滚。若任务等待，查看 Task `queue_reason=capacity.run|capacity.tenant` 和原始 `queued_at`；不能通过反复提交请求取得新槽。

新计划按 `max_steps/max_capability_calls` 预留完整 DAG 的步骤/`tooling` 节点。超额规划终止为 `budget.exhausted`；重启仍沿用计划里的预算。Workflow 内部、token/费用与计划修订累计预算另行验收。

```sh
(cd backend && go test -race ./internal/service/agent_run ./pkg/event_bus ./internal/server/agent/runtime ./internal/bootstrap ./internal/server/agent/config)
(cd backend && POWERX_AGENT_RUN_TEST_REDIS_ADDR=127.0.0.1:6379 go test ./internal/service/agent_run -run '^TestExecutionCapacityAcrossProcessesRealRedis$' -count=1 -v)
```

第二条命令使用独立测试环境键和虚拟 Run 身份，启动独立测试进程验证共享配额，清理自身键；不会重启 Redis、Core 或提交业务消息。

### 2026-10-04 双完整 Core 启动与共享维护验收

- 使用独立 HTTP `18077/18177`、gRPC `18078/18178`、同一 Redis DB 5 与私有报告桶启动两个完整 Core。二者 HTTP 健康通过，归档共享状态有效，未记录 durable Worker 错误。
- 对测试 A 发送 SIGTERM，B 的 HTTP 仍健康且 `observed_at` 继续增加。重启 A 后停止 B，A 同样继续维护共享状态。
- 已修正联调脚本的 Redis DB/地址/密码回退规则；启动检查同时读取 Redis 权威归档健康状态，拒绝缺失、过期、时钟异常或积压状态，并检查三种端口是否占用。HTTP 健康成功不能独立证明 Worker 启用。
- 私有证据保存于 `/tmp/powerx-agent-runtime/two-core-startup-evidence.json`。演练结束只关闭测试进程，未重启正式 8077 后台。
- 此项只证明完整 Core 的启动、重启和共享归档维护，不证明活动 Run 接管、真实营销 HTTP/SSE、浏览器刷新或生产 HA/RPO。T011/T012 继续保留这些验收项。

### 2026-10-04 正式开发端口启用准备

用户重启 8077 后核对 PID 38800 与启动日志，实际加载 `backend/etc/config.yaml`，但配置仍没有 durable_sessions/physical_model_pools，归档依赖也未启动，因此那次重启没有启用新链。现已启动本机 MinIO，并完成 AOF/noeviction、报告桶写入/读回/删除和隔离 18077 Core 的 Worker 启动预检。

仅更新忽略提交的开发配置 AI runtime：durable/admin enabled、Run 并发 2、租户并发 16、Worker 并发 4、扫描 5s、租约 30s、Run 30m、排队 10m、热保留 168h、归档阈值 1000/1h、qwen3:8b 物理槽 1。认证、模型 Profile、存储凭据和其他模块均未修改；原配置私有备份与启用证据位于 `/tmp/powerx-agent-runtime/`。隔离 Core 已停止，MinIO 保持运行作为开发依赖。

此检查点尚需用户按原启动方式再次重启 8077，再确认 Redis DB 5 的共享健康与新 Worker。配置已写入不表示正在运行的 PID 已加载新链，也不表示营销业务验收完成。停止开发 MinIO 后，下一次持久化 Core 启动会因归档依赖不可用失败关闭；恢复时先启动 MinIO，再启动 Core。

### 2026-10-04 用户重启后生效确认

用户完成再次重启后，8077 为 PID 63594，启动日志确认加载上述开发配置。HTTP 健康通过；Redis DB 5 环境调度策略为 `16:2:30000`，物理模型池 capacity 为 1，归档健康 observed_at 持续按 5s 更新且 pending=0。隔离 18077/18177 均未运行，MinIO 9000 正常。持久化 Worker 与共享配额已生效；实际营销 Run 仍待用户页面提交或有效授权 HTTP 验收。浏览器自动化连接仍不可用，不能把本次生效确认记为浏览器或营销业务通过。
- 刷新恢复仅在浏览器保存用户/租户/环境范围内的 Run、Agent、Session 定位，最多 20 条并淘汰超过七天的记录，不保存输入或凭据。加载服务器历史后，已有最终消息就清除定位；否则只按 Run 从游标零续订，以重建任务视图，不发送 `q/client_msg_id`。切换身份/租户/环境会中断旧订阅。
- 独立进程真实模型容量检查：在 `backend` 执行 `POWERX_TEST_OLLAMA_ENDPOINT=http://127.0.0.1:11434 POWERX_AGENT_RUN_TEST_REDIS_ADDR=127.0.0.1:6379 go test ./internal/server/ai/factory/llm -run '^TestRealOllamaCapacitySharedAcrossRuntimeProcesses$' -count=1 -v`。该测试验证两进程共享一槽、排队后调用真实模型，不等同于两台完整 Core 的营销/HTTP 故障验收。

隔离 AOF 强制重启检查（只操作测试创建的 Redis 子进程和临时目录，不重启共享 Redis）：

```sh
(cd backend && POWERX_TEST_REDIS_SERVER_PATH=/opt/homebrew/bin/redis-server go test ./internal/service/agent_run -run '^TestIsolatedRedisAOFRestartRecoversRunAndPendingDelivery$' -count=1 -v)
```

该用例固定 `appendfsync=always`，检查已确认 Run/事件和待领取队列恢复、另一 Worker 重领与旧 fence 拒绝。记录的恢复时间包含租约等待；不能据此宣称 `everysec`、Redis HA、完整 Core 或业务副作用的生产 RPO/RTO 已通过。

### 2026-10-04 营销 Run 规划阶段故障修复

实际 Run `cb53373e-4b25-4ecc-bef5-95bcdbd2a76b` / Trace `42e56b81-ebaa-4a52-8cad-0ae58690d622` 的 Redis 事件与对象归档一致：排队约 1.3 秒，规划执行约 18 毫秒后 `planner.failed`，没有调用模型。用原冻结输入只读核对 DB（不修改失败 Run、不重放业务），未命中权限缓存时可生成四任务 DAG；先预热权限缓存则稳定复现 `admin run capability contract changed`。

根因是 `EffectivePermissionItem.Policy` 的公开 DTO 标签 `json:"-"` 同样作用于旧内部缓存，缓存命中后执行契约丢失。内部缓存改用独立 result/policies 载荷并升级到 v3，公开 DTO 仍不暴露 Policy；v2 缓存不再读取，无需清空整个 Redis。缺失策略数组的缓存失败关闭。修复后原输入在预热缓存条件下完成四任务规划与内存对象读回检查。

Worker 原因码补充 `authorization.contract_changed`、`planner.input_failed`、`planner.input_invalid`、`planner.build_failed`、`planner.plan_invalid`、`planner.panicked`，经既有 Task 失败事件和归档保留。此检查不是模型执行或页面验收。需要按原方式重启 8077，然后在营销复盘页面点击重试，核对新 Run 的 planning 完成、四任务依赖执行和最终消息；旧失败 Run 保留原历史，不更写其原因码或状态。不需要数据库迁移。

### 2026-10-04 页面主动重试的受理键修复

用户重启后 8077 为 PID 40043（16:44:48 启动）。16:48:54 页面发送了 `regen_from_message_id=348`，但没有 `client_msg_id`；受理服务默认按原用户消息 UUID 去重，返回 15:38 的旧失败 Run `cb53373e-4b25-4ecc-bef5-95bcdbd2a76b`。DB 此会话仍只有这一条受理记录，后续 SSE 都以 `after_seq=6` 续订旧 Run，不能将此现象视为修复后新运行失败。

`regenerateFrom` 每次主动重试生成 `retry_<UUID>` 受理键，并清除占位回复中的旧追踪信息。SSE 恢复继续仅按已受理的 Run 和游标续订，不生成新的受理键。回归验证连续主动重试使用不同键、复用一条用户消息、每次保留一条当前助手结果；后端受理测试验证同键复用 Run、不同键创建新 Run。

本检查点修改前端逻辑与测试，无需迁移或再次重启后台。刷新 3030 的营销复盘页面后点击“重试”，请求应携带 `client_msg_id=retry_...`，新响应必须返回不同 Run ID。实际营销执行仍须依据新 Run 验收；浏览器页面结果未在自动化中验证。

### 2026-10-04 刷新后失败 Run 思考占位修复

旧助手消息被重新生成裁剪后，失败 Run 的受理锚点与 Redis 终态仍保留。续订原 Run 在读取助手消息时收到 record-not-found，先前误报为可重连的订阅中断，页面只拿到任务失败快照而没有结束信号，产生“失败 + 思考中”的冲突。

失败或取消的权威 Run 在最终助手消息缺失时，订阅发送 `agent_run.ended`（success=false、真实 status）及 `end`；不修改旧运行、不重新执行、不伪造成功正文。已完成 Run 缺失结果或数据库连接错误仍不能被当作普通失败吞掉。页面收到失败结束信号后清除思考/生成状态与待恢复定位，保留失败卡片和追踪入口。后端测试覆盖失败 Run 的零游标重放与末尾游标续订、助手消息已软删除；前端测试覆盖一次恢复后结束并且再次恢复不发送请求。

此修复包含后端代码，需要按原方式重启 8077，再刷新 3030；不需要迁移。页面应显示终态失败并停止思考，只有主动点击重试才创建新 Run。上述回归检查不代表新的营销执行已经通过。

### 2026-10-06 营销复盘数据覆盖修复

Run `b8612aff-646c-4e55-acba-3da8e15d49f3` 完成了单 Skill 调度与归档，但事实阶段仅返回部分数据。当前执行器改用内部 evidence-facts/v2 的逐 token required 对象，并验证事实和合格计算字段的完整覆盖；字段匹配和来源去重防止金额/渠道串位；说明 gaps 只能覆盖真实缺失操作数。详见 `docs/guides/develop/agent-response-evidence-v4.md` 的覆盖率回归与真实模型检查。本次不改变历史 Run、已发布 Skill 或团队编排。生效需要重启 8077，随后页面主动重试得到新 Run；不需要数据库迁移。
