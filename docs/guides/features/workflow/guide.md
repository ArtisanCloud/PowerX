# PowerX Workflow 操作指导手册

## 1. 功能背景与目标

Workflow 是 PowerX 用来编排租户业务流程的运行时能力。它面向智能体、知识库增量迭代、插件复合能力、人工审核和补偿回滚，不替代 EventBus、Task Queue、Scheduler 这类底座 Flow。

本手册用于指导管理员、研发和 QA 在当前 Web Admin 与 Admin API 中完成工作流的创建、编辑、运行、审核查看和排障。

当前目标：

- 在 Web Admin 查看 Workflow Definition 列表。
- 创建 draft 工作流定义并打开编辑器。
- 在编辑器中拖入节点、连线、配置节点、保存和运行测试。
- 通过 Admin API 创建、发布、启动和查询 Workflow。
- 校验内置 Workflow Pack Catalog，并按租户显式启用内置 WorkflowDefinition。
- 通过实例详情和 Review Task 页面进行基础运行巡检。

当前边界：

- Web Admin 编辑器已接入真实 `/api/v1/admin/workflows` API 和 Node Catalog。
- 内置 Workflow Pack Catalog 通过 `make seed` 的 `db-seed` 阶段校验；租户 WorkflowDefinition 只在显式启用 Pack 时写入。
- 当前 Runner、节点适配器、Human Review 处理链路仍需按 `docs/plan/ai_engineering/workflow` 继续完善；手册中标注的验收必须以当前实现为准。

## 2. 角色与适用范围

| 角色 | 主要用途 | 环境 |
| --- | --- | --- |
| 平台管理员 | 创建、发布、运行工作流，查看审核任务和实例状态 | Web Admin、本地/测试/远程 dev |
| 研发 | 联调 Admin API、Node Catalog、seed、Runner 和节点适配器 | 本地开发环境 |
| QA | 按页面和 API 验证成功链路、失败链路和权限边界 | 测试环境 |
| 运维 | 在部署后执行 migrate/seed、检查日志、回滚异常发布 | systemd dev / production |

适用页面：

- `/workflow`
- `/workflow/workspace?id=<workflow_definition_uuid>`
- `/workflow/instances/<workflow_instance_uuid>`
- `/workflow/review-tasks`

适用 API：

- `/api/v1/admin/workflows/*`

不适用：

- 不把 `/workflow` 当前前端页面路径当作后端 API 前缀。
- 不通过 numeric id 引用工作流定义、实例、用户、智能体或审核任务。
- 不把未通过发布校验的 draft 当作可执行生产工作流。

## 3. 整体架构与模块关系

```mermaid
flowchart LR
  Admin[Web Admin /workflow] --> API[Admin API /api/v1/admin/workflows]
  API --> WorkflowSvc[Workflow Service]
  WorkflowSvc --> DefRepo[(WorkflowDefinition)]
  WorkflowSvc --> InstRepo[(WorkflowInstance)]
  WorkflowSvc --> StepRepo[(WorkflowStepRecord)]
  WorkflowSvc --> ReviewRepo[(HumanReviewTask)]
  WorkflowSvc --> Catalog[Node Adapter Registry / Node Catalog]
  WorkflowSvc --> Packs[Workflow Pack Catalog / Installation]
  Catalog --> Skill[Skill Adapter]
  Catalog --> Capability[Capability Adapter]
  Catalog --> Knowledge[Knowledge Adapter]
  Catalog --> Metadata[Metadata Adapter]
  Catalog --> EventBus[Event Adapter]
```

关键模块：

| 模块 | 职责 |
| --- | --- |
| Web Admin Workflow List | 搜索、筛选、分页、创建、打开编辑器、启动实例 |
| Workflow Workspace | Vue Flow 画布、组件库、节点配置、节点说明、运行面板 |
| Workflow Admin API | 定义、实例、节点目录、审核任务、Workflow Pack |
| Workflow Service | 定义校验、发布、实例启动、控制、seed、事件记录 |
| Node Catalog | 将语义节点 `skill.invoke`、`human.review` 等暴露给编辑器 |
| Workflow Pack Catalog | 校验内置可复用流程模板；租户显式启用后生成自己的流程定义 |

## 4. 核心流程

```mermaid
flowchart TD
  A[输入: 管理员创建 Workflow] --> B[创建 draft definition]
  B --> C[打开 workspace 编辑节点和连线]
  C --> D[保存 / validate]
  D -->|通过| E[发布 definition]
  D -->|失败| F[显示校验错误]
  F --> C
  E --> G[启动 instance]
  G --> H[Runner / StepRecord 推进]
  H -->|成功| I[输出: succeeded / step records / trace_id]
  H -->|等待人工审核| J[Review Task pending]
  J --> K[审核通过或拒绝]
  K --> H
  H -->|失败| L[输出: failed / last_error]
  L --> M[排障后 retry_step / compensation / 修改定义]
```

失败分支处理原则：

- 定义校验失败：回到 workspace 修正节点、连线或必填配置。
- 实例启动失败：检查 definition 状态是否 `published`，以及 API Token/RBAC。
- 节点执行失败：从实例详情读取 `trace_id`、`step_id`、`error_message`。
- 审核卡住：进入 `/workflow/review-tasks`，按状态筛选 pending 任务。

## 5. 跨角色协作流程

```mermaid
flowchart LR
  subgraph AdminLane[管理员 / Web Admin]
    A1[创建工作流]
    A2[拖拽节点并配置]
    A3[发布并运行测试]
    A4[查看实例和审核任务]
  end

  subgraph BackendLane[PowerX Backend]
    B1[CreateDefinition]
    B2[Node Catalog / Validate]
    B3[PublishDefinition / StartInstance]
    B4[StepRecord / HumanReviewTask / WorkflowEvent]
  end

  subgraph OpsLane[研发 / QA / 运维]
    C1[make migrate / make seed]
    C2[curl API 验证]
    C3[查日志和 trace_id]
    C4[回滚或重新 seed]
  end

  C1 --> B1
  A1 --> B1
  A2 --> B2
  B2 --> A2
  A3 --> B3
  B3 --> B4
  A4 --> B4
  B4 --> C3
  C2 --> B3
  C4 --> B1
```

## 6. 前置条件与依赖

### 6.1 服务依赖

- PostgreSQL 已完成迁移。
- 后端服务已启动，并加载包含 Workflow 模块的配置。
- Web Admin 已启动并指向当前后端。
- 当前用户拥有访问 `/api/v1/admin/workflows/*` 的后台权限。

### 6.2 数据依赖

初始化数据库：

```bash
make migrate
make seed
```

说明：

- `make migrate` 执行数据库迁移。
- `make seed` 顺序执行 `db-seed` 和 `capability-seed`。
- `db-seed` 当前包含 CoreX、Metadata 等基础种子数据，并校验 Workflow Pack Catalog。
- Workflow Pack Catalog 校验不会给所有租户批量写入 WorkflowDefinition；当前租户需要通过页面或 `POST /api/v1/admin/workflows/packs/seed` 显式启用。

远程 systemd dev 环境不要在没有 Makefile 的发布包 backend 目录里直接跑 `make seed`。应使用发布包实际包含的二进制或部署脚本约定，例如：

```bash
cd /opt/powerx-dev/backend
sudo -u powerx ./database migrate --config /etc/powerx-dev/config.yaml
sudo -u powerx POWERX_CONFIG=/etc/powerx-dev/config.yaml ./database seed
sudo -u powerx POWERX_CONFIG=/etc/powerx-dev/config.yaml ./powerx capability-seed
```

具体命令以当前发布包和 systemd 文档为准。

### 6.3 权限与身份

- 页面访问使用用户 JWT、租户成员和 RBAC。
- API 示例中的 `<TOKEN>` 必须是可访问 Admin Workflow API 的用户态 token。
- 不使用插件 STS token 直接调用 `/api/v1/admin/workflows/*` 绕过用户权限。

## 7. 操作步骤（按场景拆分）

### 7.1 页面操作：查看工作流列表

动作：打开工作流管理页面。

入口：

```text
Web Admin -> /workflow
```

操作：

1. 查看顶部统计：总流程、已发布、草稿。
2. 使用搜索框按名称或描述搜索。
3. 使用状态筛选：全部、草稿、已发布、已归档。
4. 使用分页控件切换每页数量。

预期结果：

- 页面展示 Workflow Definition 卡片。
- 卡片展示名称、描述、版本、状态和更新时间。
- 内置 Workflow Pack 显示“内置包”标识。

失败处理：

- 页面空白：检查 `GET /api/v1/admin/workflows/definitions` 是否 200。
- 没有内置流程：先确认已执行 `make seed` 或远程环境的 `./database seed` 完成 catalog 校验，再对当前租户显式启用内置 Workflow Pack。
- 401/403：检查当前登录用户的后台权限。

### 7.2 页面操作：新建工作流

动作：创建 draft Workflow Definition。

入口：

```text
/workflow -> 新建流程
```

操作：

1. 点击“新建流程”。
2. 输入流程名称。
3. 可选输入描述。
4. 点击“创建”。

预期结果：

- 创建成功后打开新窗口：

```text
/workflow/workspace?id=<workflow_definition_uuid>
```

- 新定义默认包含一个 `input.capture` 起始节点。

失败处理：

- 创建按钮不可点：确认流程名称非空。
- 400：检查后端返回的校验错误，常见原因是 `steps` 为空或 `node_kind` 不合法。
- 新窗口被拦截：允许浏览器弹窗，或从列表卡片重新点击进入编辑。

### 7.3 页面操作：编辑工作流画布

动作：通过 Workflow Workspace 编辑节点和连线。

入口：

```text
/workflow/workspace?id=<workflow_definition_uuid>
```

操作：

1. 左侧“组件库”搜索节点。
2. 将节点拖到画布中。
3. 拖拽节点 handle 建立连线。
4. 点击节点，右侧“节点配置”显示 schema 表单。
5. 点击右侧“节点说明”查看节点类型、必填字段和输出端口。
6. 使用顶部工具栏放大、缩小、适应视图、居中、重置缩放、全屏和切换小地图。
7. 点击“保存”。

预期结果：

- 节点在画布可拖拽、可连线。
- 右侧属性面板随选中节点更新。
- 小地图和缩放百分比与画布状态联动。
- 保存后定义的 `step_graph` 与画布节点/连线一致。

失败处理：

- 左侧组件库为空：检查 `GET /api/v1/admin/workflows/node-catalog`。
- 右侧属性为空：先点击一个节点。
- 画布空白：确认 URL 中 `id` 是有效 `workflow_definition_uuid`。
- 节点不能发布：检查节点必填字段、能力授权、Skill/Capability/Knowledge Space 引用是否存在。

### 7.4 页面操作：运行实例和查看详情

动作：从列表启动 Workflow Instance。

入口：

```text
/workflow -> 卡片菜单 -> 运行
```

预期结果：

- 跳转到：

```text
/workflow/instances/<workflow_instance_uuid>
```

- 页面显示实例状态、定义 UUID 摘要、trace 和步骤记录。

失败处理：

- 启动失败：确认定义状态是 `published`。
- 详情页无步骤：确认请求包含 `include_steps=true`，并检查后端 StepRecord 是否写入。
- 实例失败：复制 `trace_id` 和失败步骤，查后端日志。

### 7.5 页面操作：营销知识采集运行测试表单

动作：用业务表单启动 `marketing_knowledge_capture` 测试运行。

入口：

```text
/workflow -> 营销知识采集 -> 编辑 -> 运行测试
```

操作：

1. 在“目标知识库”中选择一个已启用的 Knowledge Space。
2. 选择“素材类型”：文本、音频、文档或链接。
3. 按素材类型填写对应输入：
   - 文本：填写“营销材料文本”。
   - 链接：填写“素材链接”。
   - 音频/文档：填写“素材资产引用”。
4. 填写可选的业务场景、内容语言和运行备注。
5. 点击“开始运行”。

提交到后端的结构：

```json
{
  "knowledge_space_uuid": "<knowledge_space_uuid>",
  "source": {
    "type": "text",
    "content": "新品发布复盘：高意向客户识别不足导致转化延迟...",
    "context": "活动复盘",
    "language": "zh"
  },
  "note": "验证营销方法论抽取效果"
}
```

预期结果：

- 表单缺少必填项时不会创建实例，并显示明确校验错误。
- 知识空间下拉显示空间名称和部门，不把 UUID 作为主要可见标签。
- 成功提交后创建 Workflow Instance，并在底部运行记录中展示节点状态。

当前边界：

- 本阶段完成 Web Admin 输入表单化和结构化 payload 构造。
- `skill.invoke`、`metadata.classify`、`knowledge.stage`、`knowledge.publish` 的真实业务执行仍依赖后续接入 `SkillInvoker`、`MetadataClassifier` 和 `KnowledgeOperator`。
- 如果后端未接入上述依赖，流程会在对应节点明确失败，而不是静默降级。

失败处理：

- 知识库列表为空：先进入 `/knowledge-spaces` 创建并启用 Knowledge Space。
- 运行后卡在或失败于 Skill 节点：检查 `marketing.audio_or_document_parse` 和 `marketing.extract_methodology` 是否已登记并授权。
- 运行后失败于 Metadata/Knowledge 节点：检查 metadata seed 和知识库发布接口接入状态。

### 7.6 页面操作：配置营销知识采集 Skill 节点

动作：给 `marketing_knowledge_capture` 的 `skill.invoke` 节点选择已发布 Skill，并按节点指定可选模型 Profile。

入口：

```text
/workflow -> 营销知识采集 -> 编辑 -> 点击 parse_source 或 extract_marketing 节点
```

操作：

1. 在右侧“节点配置”中选择“执行技能”。
2. 可选：选择“模型模态”，例如大语言模型、视觉语言模型、语音识别或文档解析。
3. 可选：选择“模型 Profile”。列表来自 AI Settings 中已保存的模型配置。
4. 点击顶部“保存”保存工作流定义。
5. 需要排障时展开“高级参数”，查看 `input_path`、`output_path` 等引擎变量路径。

写入节点配置的结构：

```json
{
  "skill_id": "marketing.extract_methodology",
  "skill_version": "1.0.0",
  "skill_source": "builtin",
  "skill_status": "published",
  "model_override": {
    "modality": "llm",
    "profile_uuid": "<ai_model_profile_uuid>",
    "provider": "openai",
    "model": "gpt-4o-mini"
  },
  "input_path": "$.vars.parsed",
  "output_path": "$.vars.extracted"
}
```

预期结果：

- Skill 下拉只显示已发布 Skill，并展示名称、来源和版本。
- 模型 Profile 下拉显示可读名称和 `provider/model`，UUID 只作为保存值。
- `$.vars.extracted` 等变量路径不作为普通配置入口，只在高级参数和技术诊断中出现。

当前边界：

- 本阶段完成 Web Admin 节点配置入口和定义保存。
- 后端 `WorkflowService` 尚未注入真实 `SkillInvoker` 时，运行到 `skill.invoke` 会明确失败为 `workflow.skill_invoker_unavailable`。
- `model_override` 是节点级模型选择合同，真实执行还需要后续在 `backend/internal/service/workflow/adapter_skill.go` 接入 Skill 调用与模型策略透传。

失败处理：

- Skill 列表为空：进入 AI 设置 > Skills，确认目标 Skill 已导入并发布。
- 模型 Profile 列表为空：进入 AI 设置 > 模型配置，保存对应模态的 Provider/Model。
- 保存后运行仍失败：检查后端是否已将 `SkillInvoker` 注入 `WorkflowService`，并查看实例 `trace_id` 对应日志。

### 7.7 页面操作：配置营销知识采集元数据分类节点

动作：给 `metadata.classify` 节点选择分类策略和已启用的元数据治理对象。

入口：

```text
/workflow -> 营销知识采集 -> 编辑 -> 点击 classify_metadata 节点
```

操作：

1. 在右侧“节点配置”中选择“分类策略”。
2. 选择“分类体系”。
3. 选择“标签命名空间”。
4. 选择“数据字典”。
5. 选择“资源类型”。
6. 点击顶部“保存”保存工作流定义。
7. 需要排障时展开“高级参数”，查看 `input_path`、`output_path` 等引擎变量路径。

写入节点配置的结构：

```json
{
  "classification_strategy": "rule_based",
  "taxonomy_namespace": "corex.marketing.methodology",
  "tag_namespace": "corex.marketing",
  "dictionary_namespace": "corex.marketing",
  "resource_type_namespace": "corex.knowledge",
  "input_path": "$.vars.extracted",
  "output_path": "$.vars.metadata"
}
```

预期结果：

- 下拉选项来自设置 > 元数据治理中的已启用 Taxonomy、Tag、Dictionary 和 Resource Type。
- 列表显示可读名称和 namespace，不把 UUID 当作主要可见标签。
- 保存后的节点仍使用后端 `metadata.classify` Adapter 需要的 namespace 合同。

当前边界：

- 本阶段完成 Web Admin 配置入口和 seed 默认策略。
- 后端 `WorkflowService` 尚未注入真实 `MetadataClassifier` 时，运行到 `metadata.classify` 会明确失败，不会静默跳过分类。

失败处理：

- 下拉为空或提示治理对象不完整：进入设置 > 元数据治理，启用营销相关分类体系、标签、数据字典和资源类型。
- 保存后运行仍失败：检查 metadata seed、`MetadataClassifier` 注入和实例 `trace_id` 对应日志。

### 7.8 页面操作：人工审核任务干预

动作：当实例运行到 `human.review` 节点并进入 `waiting` 状态后，由审核员人工通过或拒绝。

入口：

```text
/workflow -> 人工审核
/workflow/review-tasks
```

操作：

1. 选择任务状态：pending、approved、rejected、changes_requested、canceled。
2. 点击刷新。
3. 对 pending 任务点击“通过”或“拒绝”。
4. 在确认弹窗中核对审核类型、实例和节点，填写可选审核意见。
5. 点击“确认通过”或“确认拒绝”。
6. 点击任务右侧查看按钮进入实例详情。

预期结果：

- 列表展示可读审核类型、实例摘要、节点和任务状态。
- 通过后工作流沿 `approved_route` 继续执行，例如进入 `publish_knowledge`。
- 拒绝后工作流沿 `rejected_route` 继续执行，例如进入 `emit_rejected`，不发布候选知识。
- 可跳转到对应 Workflow Instance。

失败处理：

- pending 为空：确认工作流中是否存在 `human.review` 节点，以及实例是否推进到该节点。
- 审核动作不可见：确认任务状态仍是 pending；非 pending 任务只允许查看，不允许重复提交动作。
- 审核动作失败：复制实例 `trace_id`，检查 `/api/v1/admin/workflows/review-tasks/:review_task_uuid/actions` 返回和后端日志。

### 7.9 接口调用：创建、发布、运行和查询

准备：

```bash
export POWERX_BASE_URL="http://127.0.0.1:8077"
export ADMIN_TOKEN="<TOKEN>"
```

创建定义：

```bash
curl -sS -X POST "$POWERX_BASE_URL/api/v1/admin/workflows/definitions" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "marketing-knowledge-capture",
    "description": "营销知识采集流程",
    "steps": [
      {
        "id": "capture",
        "type": "system",
        "node_kind": "input.capture",
        "config": {
          "artifact_output_path": "$.artifacts.source"
        },
        "next_step_ids": ["extract"]
      },
      {
        "id": "extract",
        "type": "system",
        "node_kind": "skill.invoke",
        "node_ref": "knowledge.extract.marketing",
        "depends_on": ["capture"],
        "config": {
          "skill_id": "knowledge.extract.marketing",
          "output_path": "$.vars.extracted"
        }
      }
    ]
  }'
```

响应片段：

```json
{
  "uuid": "<definition_uuid>",
  "name": "marketing-knowledge-capture",
  "status": "draft",
  "version": 1
}
```

校验定义：

```bash
curl -sS -X POST "$POWERX_BASE_URL/api/v1/admin/workflows/definitions/<DEFINITION_UUID>/validate" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"steps":[]}'
```

预期失败：

- 空 `steps` 或非法 `node_kind` 应返回明确校验错误。

发布定义：

```bash
curl -sS -X POST "$POWERX_BASE_URL/api/v1/admin/workflows/definitions/<DEFINITION_UUID>/publish" \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

启动实例：

```bash
curl -sS -X POST "$POWERX_BASE_URL/api/v1/admin/workflows/instances" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "definition_uuid": "<DEFINITION_UUID>",
    "input": {
      "source_asset_uuid": "<SOURCE_ASSET_UUID>"
    }
  }'
```

查询实例：

```bash
curl -sS "$POWERX_BASE_URL/api/v1/admin/workflows/instances/<INSTANCE_UUID>?include_steps=true" \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

查询节点目录：

```bash
curl -sS "$POWERX_BASE_URL/api/v1/admin/workflows/node-catalog" \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

当前租户显式启用 Workflow Pack：

```bash
curl -sS -X POST "$POWERX_BASE_URL/api/v1/admin/workflows/packs/seed" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"keys":[]}'
```

### 7.7 本地联调步骤

动作：本地启动并验证主链路。

命令：

```bash
make migrate
make seed
```

启动后端和前端：

```bash
cd backend
go run ./cmd/powerx
```

```bash
cd web-admin
npm run dev
```

验证前端类型和 workflow 合同测试：

```bash
cd web-admin
npx vue-tsc --noEmit --pretty false
npm run test -- workflow-contract.test.ts --run
```

验证后端相关测试：

```bash
cd backend
go test ./internal/service/workflow -count=1
go test ./internal/transport/http/admin/workflow -count=1
```

预期结果：

- `workflow-contract.test.ts` 通过，证明前端使用真实 Admin Workflow API 和 Node Catalog。
- 后端 workflow service 测试通过。
- `/workflow` 页面能显示内置 Workflow Pack 或新建的定义。

失败处理：

- `go test` 找不到包或编译失败：先跑 `make migrate`，检查 Go module 和生成代码。
- 前端 API 404：确认后端已注册 `backend/internal/transport/http/admin/workflow/routes.go`。
- 页面 locale key 裸露：检查 `web-admin/i18n/locales/zh.json` 和 `en.json`。

## 8. 预期结果与验收标准

| 验收项 | 操作 | 成功标准 |
| --- | --- | --- |
| 列表加载 | 打开 `/workflow` | 能看到定义卡片、分页和状态筛选 |
| 创建定义 | 点击“新建流程” | 成功打开 `/workflow/workspace?id=<uuid>` |
| 节点目录 | 打开 workspace | 左侧组件库显示真实 Node Catalog |
| 画布编辑 | 拖入节点并连线 | 节点和连线显示，右侧配置面板可切换 |
| 缩放联动 | 缩放画布 | 顶部百分比随 viewport 变化 |
| API 创建 | `POST /definitions` | 返回 `uuid/status/version` |
| API 发布 | `POST /definitions/:uuid/publish` | 状态变为 `published` |
| API 运行 | `POST /instances` | 返回 `workflow_instance_uuid` 或实例 `uuid` |
| 实例查看 | 打开实例页 | 能看到 state、trace、steps |
| 失败路径 | 提交非法 steps | 返回明确错误，不静默创建错误定义 |

## 9. 代码实现映射

| 能力 | 代码路径 |
| --- | --- |
| Admin Workflow 路由 | `backend/internal/transport/http/admin/workflow/routes.go` |
| Admin Workflow Handler | `backend/internal/transport/http/admin/workflow/handler.go` |
| Workflow Service | `backend/internal/service/workflow/service.go` |
| 实例控制 | `backend/internal/service/workflow/service_control.go` |
| Runner | `backend/internal/service/workflow/runner.go` |
| Node Catalog / Adapter Registry | `backend/internal/service/workflow/node_catalog.go`、`node_adapter.go` |
| Human Review | `backend/internal/service/workflow/human_review.go` |
| Workflow Pack catalog / install state | `backend/cmd/database/seed/seed_workflow_packs.go`、`backend/config/workflow_packs`、`workflow_pack_installations` |
| 数据库 seed 入口 | `backend/cmd/database/seed/seed.go` |
| Make seed | `make_files/database.mk` |
| 前端 API client | `web-admin/app/composables/api/services/workflowService.ts` |
| 工作流列表页 | `web-admin/app/pages/workflow/index.vue` |
| 工作流编辑器 | `web-admin/app/pages/workflow/workspace.vue`、`web-admin/app/components/workflow/WorkflowEditor.vue` |
| 工作流节点组件 | `web-admin/app/components/workflow/nodes/GenericNode.vue` |
| 实例详情页 | `web-admin/app/pages/workflow/instances/[instance_uuid].vue` |
| 审核任务页 | `web-admin/app/pages/workflow/review-tasks/index.vue` |
| i18n | `web-admin/i18n/locales/zh.json`、`web-admin/i18n/locales/en.json` |
| 规划文档 | `docs/plan/ai_engineering/workflow` |
| Spec | `specs/006-workflow-and-agent` |

## 10. 常见问题与排障

### 10.1 `/workflow` 页面为空

检查：

```bash
curl -sS "$POWERX_BASE_URL/api/v1/admin/workflows/definitions?page_size=20&offset=0" \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

处理：

- 如果 API 返回空列表但预期有内置流程，先执行 `make seed` 或远程发布包 seed 校验 catalog，再调用 `POST /api/v1/admin/workflows/packs/seed` 启用当前租户的 Pack。
- 如果 API 401/403，检查登录用户权限。
- 如果 API 404，确认后端注册了 workflow 路由。

### 10.2 组件库为空

检查：

```bash
curl -sS "$POWERX_BASE_URL/api/v1/admin/workflows/node-catalog" \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

处理：

- 如果返回空，检查 `RegisterWorkflowNodeAdapters` 是否执行。
- 如果返回 500，查看后端日志中的 workflow/node catalog 错误。

### 10.3 创建工作流后 workspace 空白

检查：

- URL 是否包含 `id=<workflow_definition_uuid>`。
- `GET /api/v1/admin/workflows/definitions/<uuid>` 是否返回 `step_graph`。
- 前端是否加载了 `workflowFromDefinition` 转换逻辑。

### 10.4 发布失败

常见原因：

- 无起始节点。
- `steps[*].id` 重复。
- `node_kind` 不在 Node Catalog。
- 必填配置缺失。
- 引用的 Skill、Capability、Knowledge Space 或 Metadata namespace 不存在。

处理：

- 在 workspace 选中节点，查看右侧“节点说明”的必填字段。
- 调用 `/validate` 获取校验结果。

### 10.5 实例一直 waiting

可能原因：

- 到达 `human.review` 节点，等待人工处理。
- 某个节点适配器未实现或依赖不可用。

处理：

- 打开 `/workflow/review-tasks?status=pending`。
- 查询实例详情，查看 `current_step_id`、`last_error` 和 steps。

### 10.6 systemd dev seed 没生效

检查：

```bash
sudo journalctl -u powerx-dev-backend -n 300 --no-pager | grep -Ei "workflow|seed|migrate"
```

处理：

- 确认 seed 使用正确 config：

```bash
sudo -u powerx POWERX_CONFIG=/etc/powerx-dev/config.yaml ./database seed
```

- 不要在缺少 Makefile 的发布包目录里执行 `make seed`。

## 11. 回滚与风险控制

### 11.1 页面配置错误

处理：

- 不发布 draft。
- 删除或重新创建错误定义。
- 使用 `/validate` 找到错误节点和字段。

### 11.2 已发布定义异常

处理：

- 将新实例切回上一个已验证的 Workflow Definition 版本。
- 暂停或取消异常实例：

```bash
curl -sS -X POST "$POWERX_BASE_URL/api/v1/admin/workflows/instances/<INSTANCE_UUID>/actions" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"action":"pause","reason":"rollback"}'
```

### 11.3 Seed 写入错误

处理：

- 先备份数据库。
- 修正 `backend/config/workflow_packs` 后重新执行 seed。
- 对生产环境，优先通过迁移或运维脚本显式修正，不手工编辑数据库。

### 11.4 权限风险

规则：

- `/api/v1/admin/workflows/*` 必须走用户 JWT、租户成员、RBAC。
- 插件服务态 STS 不得绕过 Admin 用户权限直接执行后台工作流管理接口。
- 工作流节点中的 Capability 必须来自正式 Capability Registry，不允许硬编码绕过。

## 12. 变更记录

| 版本 | 日期 | 责任人 | 变更 |
| --- | --- | --- | --- |
| 0.1 | 2026-07-20 | PowerX Core | 新增 Workflow 操作指导手册，覆盖页面、API、联调、验收、排障和代码映射。 |
