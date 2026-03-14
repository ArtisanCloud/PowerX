# PowerX 底座能力查看指南

本指南面向需要调试 PowerX 底座（`source=corex`）开放能力的开发者，帮助你在本地或测试环境中快速定位能力元数据、确认协议定义，并拿到可以直接调试的 API/CLI 命令。

> **前提条件**
>
> - 你需要一个 `IsRoot=true` 的管理员账号才能访问底座能力目录。
> - Web Admin 与 Backend 均已运行（前端默认 `http://127.0.0.1:3030`，Backend 默认监听 `http://127.0.0.1:8077`，前缀 `/api/v1`）。
> - 具备 Admin JWT（下文用 `ADMIN_TOKEN` 表示）与目标租户 UUID（`TENANT_UUID`）。

## 1. Web Admin 入口

1. 以 Root 管理员登录 Web Admin。
2. 进入 **设置 > AI > 能力注册表**：
   - 将“来源”筛选器保持在 “插件能力”，即可专注插件/租户透出的能力。
   - 切换为 “平台能力” 可快速确认底座能力是否同步完好。
3. 进入 **设置 > 开放能力**：
   - 仅展示 `source=corex` 的平台能力，按模块（Media、Event、Workflow、Knowledge）聚合。
   - 每个卡片提供协议标签、`capabilities_hash`、调试链接，方便直接跳入对应文档。

> 若你要联调网关鉴权（`API Key / Token`）与 ws-bus，请先阅读：
> [API Key / Token 联调指南](../api_key_token_playbook.md)

## 2. 查询接口（供自动化脚本使用）

```bash
export API_ORIGIN="http://127.0.0.1:8077"
export ADMIN_TOKEN="<root-admin-jwt>"
```

### 列出全部平台模块

```bash
curl -sS -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$API_ORIGIN/admin/platform-capabilities" | jq .
```

### 查看指定模块（例如 media）

```bash
MODULE_KEY=media
curl -sS -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$API_ORIGIN/admin/platform-capabilities/$MODULE_KEY" | jq .
```

返回结果包含：

- `module`：模块键（如 `media`、`workflow`）
- `capabilities[]`：内含 `capability_id`、协议矩阵、优先策略、Docs 链接等

### 对比插件与平台能力

`/admin/capabilities` 暴露统一的能力列表，可通过 `source` 参数区分。

```bash
# 仅列出插件或租户同步的能力
curl -sS -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$API_ORIGIN/admin/capabilities?source=plugin&page=1&page_size=50" | jq .

# 查看底座能力（与“开放能力”页面相同数据源）
curl -sS -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$API_ORIGIN/admin/capabilities?source=corex" | jq .
```

## 3. 如何调试

1. 根据模块（Media/Event/Workflow/Knowledge）打开对应的文档：
   - [Media 能力](./media.md)
   - [Event Fabric 能力](./event_fabric.md)
   - [Workflow & Scheduler 能力](./workflow.md)
   - [Knowledge Space 能力](./knowledge_space.md)
   - [AI 能力总览](./ai/README.md)
2. 文档中包含：
   - 可直接复制的 REST `curl` 与 `grpcurl` 命令
   - 所需 Header（`Authorization`）与典型请求体（租户由 JWT claims 提供）
   - 对应的 `capability_id`、意图、协议优先级
   - 统一的资源访问入口（例如 Media 的 `GET /api/v1/media/assets/{uuid}/resource`），方便你在调试阶段直接下载或跳转外链
3. 若需要走统一 Selector，可以使用 `/tenant/capabilities` / `/tenant/invocations`：

```bash
curl -sS -H "Authorization: Bearer $TENANT_TOKEN" \
  "$API_ORIGIN/api/v1/tenant/capabilities?source=corex"
```

更多细节请参考各模块文档，确保在调用前已为目标租户授予对应 Tool Grant/Feature Flag。这样即可在插件或宿主场景中直接复用 PowerX 底座提供的开放能力。

### 关于 `/api/v1/tenant/invocations`

这是 PowerX 的“能力调度”入口，插件可以通过它调用 REST/gRPC/MCP 等不同协议的开放能力，而无需自己维护多套协议栈。使用方式如下：

```bash
curl -sS -X POST "$API_ORIGIN/api/v1/tenant/invocations" \
  -H "Authorization: Bearer $TENANT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
        "capability_id": "com.corex.media.assets.read",
        "preferred_protocol": "rest",
        "payload": {
          "method": "GET",
          "endpoint": "/api/v1/media/assets",
          "query": { "page": 1, "page_size": 20 }
        }
      }'
```

- `capability_id`：要调用的能力 ID，来自 Registry（如 `com.corex.media.assets.read`）。
- `preferred_protocol`：可选 `rest`、`grpc`、`mcp` 等，Selector 会按 Registry 的协议优先级路由。
- `payload`：描述具体调用。  
  - REST：提供 `method`、`endpoint`、可选 `headers`、`query`、`body`。  
  - gRPC：提供 `endpoint`（Service 名）+ `rpc`（方法），以及 `body`（JSON 序列化后的请求）。
- 返回结果：Gateway 会在统一的 Envelope 中附带真实业务响应与元信息。例如：

  ```json
  {
    "code": 200,
    "message": "success",
    "data": {
      "payload": {
        "items": [/* 这里就是 GET /media/assets 的 JSON */],
        "pagination": { "page": 1, "page_size": 20, "total": 1 }
      },
      "trace_id": "b28a79b9-a653-4ed3-b7aa-ea063432df6d",
      "protocol_used": "http",
      "fallback_used": false
    }
  }
  ```

  - `data.payload`：直接复用底层 REST/gRPC/MCP 响应体，插件无需关心协议差异。
  - `data.trace_id` / `protocol_used` / `fallback_used`：用于调试与观测；当发生 gRPC fallback 或 Workflow 补偿时，这些字段会反映最终采用的协议。
  - 错误场景下同样会在 `data.payload` 中返回底层错误内容，同时 HTTP 状态码与 `code` 字段会指示失败原因。

因此，当你希望“统一入口 + 自动协议适配”时就用 `/tenant/invocations`；若只是简单的 REST 调试，也可以直接调用文档中的业务接口。两者可以并行使用。

## 4. 追溯能力定义位置

若需要从代码或文档层面追踪底座能力的来源，可按以下路径查找（所有文件都在仓库内）：

| 背景 | 文件/目录 | 作用 |
| --- | --- | --- |
| 能力注册定义 | `backend/config/platform_capabilities/*.yaml`（建议结构：`base.yaml`, `media.yaml`, `workflow.yaml`, …）<br>`backend/internal/service/integration_gateway/base_capabilities.go` | YAML 文件用于声明各模块的能力条目；`base_capabilities.go` 负责加载并写入 Registry。新增/禁用能力时优先修改 YAML，再通过进程重启生效，可通过环境变量 `PLATFORM_CAPABILITIES_DIR` 指定自定义目录。 |
| OpenAPI 契约 | `specs/001-docs-media-storage/contracts/http-openapi.yaml` 等 `specs/<module>/contracts/*.yaml` | 描述对外 REST 契约，Web Admin/CLI/Scripts 都引用这些文件生成示例。 |
| gRPC 契约 | `backend/api/grpc/contracts/...` | 包含 Media/Event/Workflow/Knowledge 等 gRPC 服务定义，与 Registry 中 `protocols` 字段一一对应。 |
| Web Admin 指南 | `docs/guides/develop/open_capability/<module>.md` | 本目录下的各模块文档（media/event_fabric/workflow/knowledge）提供调试示例。 |
| 路由与实现 | `backend/internal/transport/http/openapi/<module>/`、`backend/internal/transport/grpc/<module>/` | 可直接阅读 Handler 代码了解参数校验和实际调用流程。 |

通过上述线索，可以从“文档 → 契约 → 代码”逐层定位能力详情，确保开放接口与底层实现一致。

### 能力与接口的映射规则

- **一个 `capability_id` = 一个授权单元。** 例如 `com.corex.media.assets.read` 表示“媒体资产读取”能力，它在 `media.yaml` 中聚合了多个只读接口（`GET /media/assets`、`GET /media/assets/{uuid}`、`POST /media/assets/{uuid}/presign` with `operation=download` 等），租户只要获批该能力即可调用这些接口。
- **写入/管理能力单独建一条。** `com.corex.media.assets.manage` 则聚合了写入链路（`POST`、`PATCH`、`DELETE`、`POST .../presign` with `operation=upload`），方便在授权策略中单独开关可变更的接口。
- **协议字段列出全部触点。** 每条能力下的 `protocols.http.endpoints[]` 与 `protocols.grpc.methods[]` 会把路由或 RPC 方法一并列出，形成“能力 → 接口清单”的映射，便于 Selector/Registry 精确校验。
- **新增接口先选定归属能力。** 若要扩展新的 REST/gRPC 触点，先判断它属于现有的 read/manage 等能力还是需要新建能力；在 YAML 中更新 `endpoints[]` 后，配套修改 OpenAPI/gRPC 契约与文档即可。

借助上述约定，读写授权边界清晰，平台与插件对接时只需基于能力 ID 进行授权，而无需逐条维护接口白名单。

> 推荐的配置目录示例
>
> ```
> backend/config/
>   platform_capabilities/
>     base.yaml            # 全局元数据/默认策略
>     media.yaml           # 媒体模块能力
>     event_fabric.yaml
>     workflow.yaml
>     knowledge.yaml
> ```
> 每个 `*.yaml` 文件使用统一字段描述 capability 列表，便于在不同环境下通过配置管理平台能力。程序加载逻辑可参考 `base_capabilities.go`。
