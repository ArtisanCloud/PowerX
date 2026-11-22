# 插件数据集开发联调

Core 提供插件数据集登记、统一目录、预览、单次删除与归档批次、执行回执和运维页面。用合成数据集验证 Core 合同。目录与预览见[观测合同](../../contracts/plugin-dataset-lifecycle-review.md)，写批次见[执行合同](../../contracts/plugin-dataset-execution.md)。

## 启动位置与对接地址

本地开发从 `PowerX-storage-datasets/backend` 启动 Core，显式传入绝对路径的隔离配置。配置、安装目录、PostgreSQL、Redis 和运维存储均属于同一个开发实例。Framework 运行在插件后端进程内，通过这个 Core 的网关和 STS 接口对接，无需单独启动一个存储治理服务。

| 组件 | 本地示例 | 使用方 |
|---|---|---|
| Core HTTP | `http://127.0.0.1:18077` | Framework 和 PowerX Admin。API 前缀按 Core `server.api_prefix` 配置 |
| Core gRPC STS | `127.0.0.1:19001` | 合成插件后端，用已授权的插件凭据换取短期 STS token |
| PostgreSQL 和 Redis | 该开发实例的独立地址或 Unix socket | Core；托管插件使用 Core 安装时分配的数据库身份 |
| PowerX Web Admin | 需要看现有运维界面时从本 worktree 的 `web-admin` 启动 | 设置 `POWERX_BACKEND=http://127.0.0.1:18077/api/v1` 和 `NUXT_PUBLIC_WS_ORIGIN=ws://127.0.0.1:18077`。从“监控中心 → 存储治理 → 数据集目录”进入 |

使用正式开发配置，修改 `server.host`、`server.port`、`server.grpc.host`、`server.grpc.port`、`server.api_prefix`，并将 `deployment.env` 设为 `dev` 或 `test`。配置各自的实例安装目录、快照目录及 Redis。启用 `storage_governance.postgres_enabled` 并配置唯一的 `postgres_cluster_id`。这只开启 PostgreSQL 计量。首次接入可以从已初始化的隔离开发实例开始，保持原运行数据与当前日常实例分离。

已有隔离实例执行增量迁移和定向 capability seed：

```sh
cd /path/to/PowerX-storage-datasets/backend
export POWERX_CONFIG=/absolute/path/to/isolated/config.yaml
go run ./cmd/database migrate -scope storage-governance -config "$POWERX_CONFIG"
cd ..
make capability-seed CAPABILITY_SEED_CONFIG="$POWERX_CONFIG" CAPABILITY_SEED_FLAGS="-capability-ids com.corex.storage.datasets.register,com.corex.storage.datasets.preview_worker,com.corex.storage.datasets.execution_worker,com.corex.runtime.taskqueue.service_consume"
cd backend
go run ./cmd/app
```

新建空实例先完成 PowerX 的正常 setup 或全量开发迁移、基础 seed 与 Root 登录。定向 seed 发布目录和租户注册，插件凭据仍需为当前租户明确允许 `com.corex.storage.datasets.register`。

从最小配置启动时也要配置 `agent.flow_spec.business_dir`。从 `backend` 启动时可用 `./internal/server/agent/blueprints`，或填写该目录的绝对路径。Core 会在监听端口前加载此目录。配置 `POWERX_LINKS_ROOT` 指向该开发实例的专用目录，可隔离未显式配置的运行路径。

## 安装样本并通过 Framework 登记

1. 合成插件的 `plugin.yaml` 在 `capabilities.required` 声明 `com.corex.storage.datasets.register`，并在 `data_lifecycle.<dataset_key>` 填 `sha256` 与 `observation_tables`。声明文件固定放在 `contracts/storage-datasets/<dataset_key>.json`。SHA-256 按文件原始字节计算，例如 `shasum -a 256 contracts/storage-datasets/fixture_report.json`。使用[声明样例和保护合同](../../contracts/plugin-dataset-lifecycle-review.md#通用保护合同与验收样本)。
2. 通过 Core 插件安装流程安装并启用合成插件，在测试租户启用插件实例。管理端上传接受 `.tar.gz`、`.tgz` 或目录。安装器执行清单中 `migrations.entry` 指向的迁移，创建目标插件 schema 中的合成表和源账本。插件后端使用 Host 注入的网关 origin 和 API prefix。手动开发时对应 `PX_GATEWAY_BASE_URL=http://127.0.0.1:18077` 和 `PX_GATEWAY_API_PREFIX=/api/v1`。
3. 用该租户的合成插件 STS 凭据，通过官方 STS Exchange 获取 `audience=powerx:api` 的短期 token。联调凭据同时获准 `com.corex.capabilities.grant_status.read` 时，可请求 `POST <prefix>/tenant/capabilities:grant-status`，发送 `{"capability_ids":["com.corex.storage.datasets.register"]}`，确认已发布、已登记且授权有效。
4. Framework 或开发测试客户端向 `POST <prefix>/tenant/invocations` 发送以下类型化请求，认证头为 `Authorization: Bearer <STS>`。第一次登记后保存返回的 request UUID，重发同一操作沿用它。

```json
{
  "capability_id": "com.corex.storage.datasets.register",
  "preferred_protocol": "core_internal",
  "payload": {
    "body": {
      "contract_version": "powerx.storage-dataset-host/v1",
      "operation": "register",
      "input": {
        "request_uuid": "0d54cfd3-8274-4b45-890b-9374afb83baa",
        "dataset_key": "fixture_report",
        "definition_version": 1,
        "definition_digest": "<安装清单中 64 位小写 SHA-256>",
        "source_binding_ref": "database"
      }
    }
  }
}
```

5. 用平台 Root 用户登录后取得的 JWT 请求 `GET <prefix>/admin/ops/storage/datasets?owner_kind=plugin`，再按返回的 `dataset_uuid` 查询详情。用 `owner_kind=system&available_action=open_existing_policy` 验证系统条目仍指向原控制域。相同物理表登记为两个样本时，每条保留自身大小，列表的 `page_physical_bytes` 按当前页去重。有数据未能测量时合计为 `null`。

托管来源识别安装器分配的 `database` 绑定。Core 只读取目录元数据，并核对插件角色对目标 schema 和表的权限。独立 PostgreSQL 使用下文的部署绑定。声明未接纳、测不到数据或指纹变化时，目录显示具体状态。

## 用正式 TaskBus 联调保留预览

1. 在隔离实例启动 Redis 与 Core，确保 Core 的 TaskBus 使用该实例的 Redis。测试租户的插件凭据需要 `com.corex.storage.datasets.preview_worker` 和 `com.corex.runtime.taskqueue.service_consume`。只做预览消费的插件不用申请队列入队权限。
2. 从数据集目录选中可预览的数据集，填写启用该插件的租户和保留天数。API 对应 `POST <prefix>/admin/ops/storage/datasets/<dataset_uuid>/previews`。新请求返回 `202`，同一操作者提交相同的原请求返回 `200`。保留 `request_uuid`，用 `GET <prefix>/admin/ops/storage/datasets/<dataset_uuid>/previews/<request_uuid>` 查询回执。原系统表专项仍在系统表管理入口。
3. 插件后端通过 Framework Host TaskQueue 以该租户的 STS 主体阻塞领取订阅 `powerx.storage.dataset-preview.v1`。Core 预览通知主题为 `storage.dataset.preview.requested`，载荷只含 `contract_version` 和 `request_uuid`。用消息的 `lease.receipt_uuid` 与 `lease.fence` 经正式 tenant invocation 调用 `claim_preview`。
4. 领取返回固定租户、截止时间、最迟完成时间和扫描上限。将返回的 `definition_digest` 与当前安装包内资产的原始字节 SHA-256 对照，核对结构版本、适配器版本和实际数据源指纹，再在源端只读事务内执行。合成样本用租户与完成时间索引、断言明细和引用关系验证候选保护。业务记录无法核验资格时归入 `unknown_eligibility`。
5. 用 `report_preview` 上报聚合计数。回执落账后再 ACK。上报响应丢失时在原凭据和同一租约下重发相同报告。管理端通过现有运行 SSE 刷新原请求。

Framework Go SDK 已使用合成插件验证共享预览执行器。业务插件的正式保护适配器随各插件接入。请求和回执字段见 [Host Schema](../../../specs/027-monitor-center/contracts/plugin-dataset-host.v1.schema.json) 与 [管理 OpenAPI](../../../specs/027-monitor-center/contracts/plugin-datasets.openapi.yaml)。

## 独立 PostgreSQL

在隔离部署配置里为插件登记专用绑定。下例的环境变量由部署进程提供，只存放观察角色 DSN。插件执行器通过自己的来源配置使用相同物理库及 schema。观察连接只用于 Core 目录计量。绑定 UUID 和集群 ID 在创建后保持稳定。

```yaml
storage_governance:
  plugin_postgres_sources:
    - plugin_id: com.powerx.plugins.storage-fixture
      binding_uuid: 4d133c0f-c919-4a39-8488-27b01318c4b8
      cluster_id: isolated-plugin-postgres
      schema: dataset_fixture
      worker_role: dataset_fixture_worker
      observer_dsn_env: POWERX_FIXTURE_DATASET_OBSERVER_DSN
```

## 删除、归档和取回

1. 先完成只读预览，核对目标租户、候选与保护数量。选择「执行策略与批次」，按实际声明启用删除或归档策略。显式派发一个批次才会产生执行任务。
2. 隔离实例需要启用 `storage_governance.advanced_operations_enabled`，并使用已配置的全局存储运行控制。通过 `admission_enabled` 接入运行控制时，填写独立的主机、文件系统、运行目录与空间门槛，保持 `allow_enforce` 和旧清理接管关闭。正式实例还须满足原治理的恢复锚和全局执行准入。
3. Framework 以当前租户和安装主体订阅执行队列。按 [执行合同](../../contracts/plugin-dataset-execution.md) 领取短授权、提交源事务并上报持久回执。暂停后，有未知结果的批次保留在「等待核对」。原租约失效时可用同一 STS 主体与原请求 UUID 查取过期快照，源端封存或读取原回执后按 `source_sealed_at` 上报。
4. 归档使用现有存储归档管理中已验证的私有版本化 S3 目标。归档完成后，从批次详情「取回已校验归档」。合成验收检查对象版本、长度、SHA-256 与源记录保留。管理接口见[执行 OpenAPI](../../../specs/027-monitor-center/contracts/plugin-dataset-execution.openapi.yaml)。

## 自动化验收和排障

- 在 worktree 根目录运行 `python3 scripts/dev/check-storage-datasets.py`，可自动启动和回收两套隔离 PostgreSQL 与 Redis，运行带 race 检查的 Core 回归。安装有 MinIO 与 Python boto3 时，加 `--with-minio` 验证真实目标探测、归档和取回。已有临时实例时，可从 `backend` 单独使用 `go test ./internal/bootstrap -run '^TestStorageDatasetsNativeSTSAndHTTP$' -count=1 -v`，需设置 `POWERX_STORAGE_CONTROL_TEST_DSN`。测试只接受拥有私有 Unix socket 的临时 PostgreSQL，使用 `/tmp/powerx-control-owned-*` 路径、`storage_control_test` 用户和 `postgres` 数据库，固定使用 UTF-8。另设 `POWERX_STORAGE_DATASET_TEST_REDIS_SOCKET` 指向同目录的 `redis.sock` 以覆盖预览与执行。若同时设置同路径的第二个 PG DSN `POWERX_STORAGE_DATASET_TEST_EXTERNAL_DSN`，还会测试独立插件源。HTTP 和 STS 使用系统分配的 127.0.0.1 端口，日志记录 PID、工作目录和实际端口。测试退出时关闭监听。
- 从 Framework 仓库根目录运行 `python3 scripts/testing/storage-dataset-framework.py /path/to/PowerX-storage-datasets --with-package`，可验证托管 PostgreSQL 的完整安装链路。检查 `make package` 生成的 ZIP 保留声明原始字节和二进制，再用同一份 `dist` 打包为管理端接受的 `.tar.gz`。脚本使用安装任务 SSE 等待终态，校验包内迁移、安装代次、worker 登记及版本化归档重传和取回。
- 登记返回 403 时核对调用方的租户、插件 STS 身份、启用状态与授权。返回 409 时核对原请求与已安装声明摘要，尤其检查文件重新打包后摘要是否更新。无法确认来源时先按目录提示核对 schema、角色和实例绑定。
- 管理端用 Root 用户会话，Framework 用短期 STS。从各自入口验证，测试账号和 token 只留在隔离环境。拒绝领取或上报时核对原任务、当前租约、插件安装版本和租户授权。
