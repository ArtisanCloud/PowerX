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

依赖：本机已有 `redis-cli`、`minio`、Go 与 Python 的 PyYAML/boto3/requests。命令在仓库根目录执行。凭据从忽略提交的开发配置读取，联调配置和证据文件以 0600 权限保存。

1. Redis 配置需持久启用 AOF/noeviction，并核对 `INFO persistence` 的 `aof_enabled=1`、`aof_last_write_status=ok`、`aof_last_bgrewrite_status=ok`。2026-10-02 本机已执行 `CONFIG SET appendonly yes` + `CONFIG REWRITE`，保留 everysec，故本机 AOF 的 fsync 周期为约一秒，不作为生产 RPO 演练证据。
2. 启动固定数据目录的 MinIO（前台命令；Ctrl+C 停止后数据保留）：

```sh
python3 backend/scripts/ops/agent-runtime-dev.py minio
```

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
