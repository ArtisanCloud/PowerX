# PowerX 知识库 · 管理台 UI 规范索引

> 文档状态：Final v1.0  
> 维护者：PowerX CoreX 团队  
> 上次更新：2025-10-13

本目录收敛了 PowerX Web Admin 中与**知识库**相关的 UI/交互规范，覆盖：Agent 上下文、检索、图谱、文档管理、空间配置、权限与审计。  
所有交互仅依赖统一契约：**HTTP** `/api/v1/knowledge/...` 与 **gRPC** `knowledge.v1`（插件侧按 gRPC 规范调用）。

---

## 目录结构

```text

docs/knowledge_base
├── Agent_Context_UI.md # 智能体上下文注入可视化与回放
├── Document_Management_UI.md # 来源/上传/版本/索引/发布
├── Graph_Visualization.md # 实体/概念/文档关系与锚点
├── Knowledge_Query_UI.md # 检索界面、Explain 与调参覆盖
├── Knowledge_Space_UI.md # 空间与默认配置、成员权限、Webhook
├── Permission_and_Audit_UI.md # RBAC 与审计追溯
└── README.md

```

> 相关上游文档（放在其他目录）：
>
> - `00_overview.md`（总体概览）
> - `Knowledge_Domain_Model.md`（领域模型）
> - `Storage_and_Indexing.md`（存储与索引）
> - `Retrieval_and_Ranking.md`（检索与排序）
> - `Knowledge_Graph_Extension.md`（图谱扩展）
> - `API_and_SDK_Design.md`（接口定义，仅 HTTP/gRPC，无独立 SDK）
> - `Agent_Integration.md`（智能体集成）
> - `Workflow_Context_Injection.md`（工作流节点与上下文注入）

---

## 统一约定

- **鉴权与多租户**：所有请求必须携带
  - `Authorization: Bearer <token>`
  - `JWT claims（tid/tenant_uuid）: <uuid>`
- **字段风格**：REST 使用 `snake_case`；UI 层保持与接口字段同名（除非有明确映射）。
- **调参覆盖**：面向单次查询的 `rank_profile_override`，**不修改**空间默认配置。
- **审计与追溯**：重要操作与检索返回包含 `trace_id`、`query_id`、`document_id/version_no/chunk_id`。
- **错误模型**：统一 Envelope

  ```json
  { "code": 0, "message": "ok", "data": {}, "trace_id": "..." }
  ```

- **Mermaid 图写法**：统一使用 `flowchart` 安全写法；节点标签勿含括号；每条连线独占一行；围栏行只写 ` ```mermaid `.

---

## 跨页面关键路径

1. **检索验证 → Explain → 回放对比**
   - 入口：`Knowledge_Query_UI.md`
   - 侧栏 Explain 展示 `scores/explain` 与（可选）图谱路径
   - 通过 `query_id` 跳转到 **Agent 上下文面板** 回放比对

2. **文档治理 → 版本/索引状态 → 发布/回滚**
   - 入口：`Document_Management_UI.md`
   - 看板：解析/分块/向量/关键词/校验/发布
   - 支持重建索引（后端 Job）

3. **空间配置 → Rank Profile → Webhook**
   - 入口：`Knowledge_Space_UI.md`
   - 默认策略版本化、回滚
   - 订阅 `knowledge.*` 事件

4. **图谱增益 → 邻居/路径 → 锚点**
   - 入口：`Graph_Visualization.md`
   - 邻居拓展（depth≤2）、路径分析（max_depth≤3）
   - Workflow Anchor 维护

5. **权限与审计 → RBAC 授权 → 审计追溯/导出**
   - 入口：`Permission_and_Audit_UI.md`
   - Space 维度权限矩阵、模板（Reader/Editor/Publisher/Admin）
   - 审计过滤与 CSV 导出

---

## 关键接口速查

- **检索**：`GET /api/v1/knowledge/search`
- **反馈**：`POST /api/v1/knowledge/feedback`
- **图谱**：
  - `GET /api/v1/knowledge/graph/nodes`
  - `GET /api/v1/knowledge/graph/neighbors`
  - `GET /api/v1/knowledge/graph/paths`
  - `POST /api/v1/knowledge/graph/anchors`（可选写入）

- **空间**：`POST|GET|PATCH|DELETE /api/v1/knowledge/spaces`
- **来源**：`POST|GET /api/v1/knowledge/spaces/{spaceID}/sources`，`POST /api/v1/knowledge/sources/{sourceID}:sync`
- **文档**：`POST /api/v1/knowledge/documents`，`POST /documents/{id}/versions`，`POST /documents/{id}/publish`，`DELETE /documents/{id}`
- **审计**（平台统一）：`GET /api/v1/audit/logs?...`
- **IAM**（平台统一）：`GET /api/v1/iam/spaces/{spaceID}/bindings`，`POST .../bindings:batchUpdate`

> 插件/服务侧如用 gRPC，请按照 `knowledge.v1` 的 `KnowledgeService` 接口（详见 `API_and_SDK_Design.md` 与 `Agent_Integration.md`）。

---

## 质量检查清单（提交前自测）

- [ ] 所有 Mermaid 代码块可在目标渲染器中正常显示
- [ ] 示例接口路径/字段与 `API_and_SDK_Design.md` 一致
- [ ] 示例对象中的字段命名与大小写与接口一致
- [ ] UI 操作均有对应权限要求与错误处理分支
- [ ] 空状态、骨架屏、降级标记（Degraded）已覆盖
- [ ] i18n 文案抽取（至少 `zh-CN`、`en`）
- [ ] 可访问性（键盘可达、ARIA 标签）已标注
- [ ] 所有“调参覆盖”明确声明**仅对本次查询生效**

---

## 版本与变更

- 采用**小步演进**：新增字段与功能优先向后兼容；破坏性变更需跨版本发布（v1 → v2）并保留 ≥6 个月过渡。
- 变更记录（Changelog）请在仓库统一 `CHANGELOG.md` 维护，UI 规范的更新需同步标注“上次更新”日期。

---

## 贡献指南（针对此目录）

1. 分支：从 `dev/*` 创建功能分支，PR 合并至 `dev`。
2. 文档规范：
   - 标题使用 `#` 一级，段落使用 `##/###`；避免过深层级。
   - 代码围栏指定语言：`json`/`yaml`/`mermaid`/`bash`。
   - 图示尽量使用 Mermaid 的 `flowchart` 安全写法。

3. 审阅：至少 1 名 CoreX Reviewer 通过；关键接口对齐由 API 文档 Owner 复核。
4. 通过后更新本 README 的“上次更新”日期与相关条目。

---

## 常见问题（FAQ）

- **Q：为什么没有独立 SDK？**
  A：PowerX 对外只提供 **HTTP/gRPC 契约**，避免多端 SDK 维护成本与侵入，前端直接 `fetch` 调用，插件侧按 gRPC 规范生成客户端。

- **Q：图谱相关写入接口是否稳定？**
  A：当前以读为主，写入（锚点）按阶段逐步开放，详见 `Graph_Visualization.md` 的写入约束。

- **Q：检索调参为何不落到默认配置？**
  A：避免对全局查询产生意外影响；需要长期生效请在 Space 的“默认配置”保存并版本化。

---

## 依赖与环境（渲染/查看）

- Markdown 渲染器需支持代码围栏与 Mermaid（建议 `flowchart`）。
- 若渲染器会在围栏后自动加属性（如 `{data-source-line=...}`），请使用 HTML 容器写法：

```html
<div class="mermaid">flowchart LR A[Node] B[Node] A --> B</div>
```
