# PowerX × 插件「能力统一与双向调用」设计规范（v1.0 草案）

> 目标：**一次定义能力（Single Source of Truth）**，支持 **PowerX→插件** 与 **插件→PowerX** 的双向调用；在不增加重复实现的前提下，通过 **Selector 策略 + Adapter 层** 实现 **gRPC / MCP / Agent / HTTP** 的多后端路由与治理。

---

## 0. 术语与角色

* **能力（Capability / Tool）**：系统可被调用的“动作”，如 `template.get`、`template.create`。
* **PowerX（宿主）**：编排器/能力网关的提供方。
* **插件（Plugin）**：域能力提供方；同时也可消费宿主底座能力（AI、IAM、KB 等）。
* **Adapter**：协议适配层（gRPC/MCP/Agent/HTTP），**薄**，不承载业务。
* **Selector**：能力选择器；基于策略/健康/成本/SLA 为每次调用选择后端并实现回退。
* **PowerXClient（Host Client）**：插件侧调用宿主的 gRPC SDK 的**薄封装**（注入上下文、重试、观测等）。

---

## 1. 范围（Scope）

* 覆盖两条方向：

  1. **PowerX → 插件**：宿主编排调用插件的业务能力。
  2. **插件 → PowerX**：插件调用宿主底座能力（AI、IAM、向量、对象存储、审计、事件）。
* 覆盖四种方式：**gRPC、MCP、Agent、HTTP（仅 UI/探活/极少量 API）**。

---

## 2. 核心原则

1. **能力只定义一次**：名称、I/O Schema、RBAC、租户、事务/幂等、SLA 由插件侧清单统一声明。
2. **单入口，多后端**：编排层只认能力名 → `CapabilityGateway.Invoke(tool, args, ctx)`。
3. **读写分流**：读优先 MCP（可与 gRPC 竞速），写强制 gRPC（强一致 + 幂等 + 审计）。
4. **策略外置**：多步/不确定任务交给 Agent 封装（内部再调 MCP/gRPC）。
5. **非对称**：插件→宿主以 **gRPC** 为主；宿主内部可 MCP/Agent 化以便编排，**不要**强行做完全对称的三层互调。
6. **治理优先**：上下文、鉴权、多租户、幂等、观测在网关/客户端统一收口，避免散落。

---

## 3. 能力模型（Capability Model）

### 3.1 能力命名与 I/O

* **命名**：`<domain>.<verb>_<object>`（如 `template.create`、`template.search`）。
* **Schema**：输入/输出使用统一 JSON Schema（或 Proto + JSON Schema 镜像），能力清单引用 `inputSchemaRef` / `outputSchemaRef`。
* **治理元数据**（随能力声明）：

  * `rbac.required[]`：权限码（如 `template.write`）。
  * `tenancy`：`scope`（tenant/system/mixed）、`dataIsolation`（schema/row/none）。
  * `policy`：`transactional`、`idempotent`、`timeoutMs`、`prefer`、`fallback`。
  * `costProfile`（可选）：CPU/tokens/典型时延，供调度与限流参考。

### 3.2 Template 领域推荐能力集（示例）

* 读：`template.get` / `template.list` / `template.search`
* 写：`template.create` / `template.update` / `template.upsert` / `template.delete`
* 智能：`template.generate_from_prompt` / `template.review_and_fix` / `template.bulk_import`

---

## 4. 调用矩阵（方向 × 方式）

| 调用方式 \ 方向 | **PowerX → 插件**（宿主调用插件）                            | **插件 → PowerX**（插件调用宿主）                              |
| --------- | -------------------------------------------------- | ---------------------------------------------------- |
| **gRPC**  | **强烈推荐**：写路径唯一通道；读为兜底或与 MCP 并发竞速；支持流式/批量。          | **强烈推荐**：AI、IAM、审计、对象存储、向量、事件回调等底座能力的首选。             |
| **MCP**   | **推荐（读优先）**：可发现、易编排；与 gRPC 并发竞速；写能力在目录可见但默认转 gRPC。 | **一般不建议**：宿主可将底座能力做成 MCP 供**宿主内部**编排；插件调用仍以 gRPC 为主。 |
| **Agent** | **按需**：多步智能/策略性任务；内部再调用 MCP/gRPC。                  | **谨慎**：仅当插件希望“让宿主代跑策略流程”时使用；常规 CRUD/AI 不走 Agent。     |
| **HTTP**  | **次选**：UI 反代、健康探针、极少量查询；能力层不依赖。                    | **次选**：上传/下载、Webhook；强一致/流式仍优先 gRPC。                 |

---

## 5. Selector（选择器）默认策略

* **读类能力**：`prefer = ["mcp","grpc"]`（可并发竞速，先回先用，另一路取消）。
* **写类能力**：`prefer = ["grpc"]`；`fallback = []`；必须携带 `idempotency_key`。
* **智能类能力**：显式 `prefer=["agent"]`；Agent 内部再按任务分解调用 MCP/gRPC。
* 运行时可结合**健康/时延/错误率/配额**动态调节；支持租户级/环境级覆写。

---

## 6. Adapter 层职责（统一薄封装）

* **MCP Adapter**（Server/Client）

  * `/tools/list` 从能力清单与 Schema 自动生成；
  * `/call_tool` 做入参校验、RBAC、上下文注入、调用 app/service；
  * 只做**原子动作**；长任务转 Job/事件。

* **gRPC Adapter**（Server/Client）

  * 强类型方法对齐能力名（按命名规则映射）；
  * 统一**超时/重试/熔断/幂等键**；
  * 统一错误映射（领域错误到 canonical codes）。

* **Agent Adapter**（Server/Client）

  * `Execute(Task)` 接收意图/槽位/约束，调用内部 Planner/Service/MCP/gRPC；
  * 回传中间产物引用、评分、仲裁说明。

* **HTTP Adapter**

  * 仅用于前端静态/反代与健康/metrics；不承载能力。

---

## 7. 上下文与安全（两端一致）

* `Ctx` 统一字段：`tenant_uuid`（必要时宿主可额外注入 `tenant_id` 仅用于兼容审计）、`actor`、`scopes[]`、`trace_id`、`locale/currency`、`idempotency_key`。
* **鉴权**：PowerX 网关统一校验 `rbac.required[]`；插件→宿主通过 STS/Service Account 获取最小权限。
* **多租户**：宿主注入 `tenant_uuid`；插件按 `schema/row` 隔离；日志与审计落**租户维度**。
* **幂等**：写类能力必须支持；宿主负责生成/转发；插件在仓储层实现去重。
* **审计**：统一事件名=能力名（如 `template.upsert`）；南北向都落账。
* **配额与限流**：按租户/能力/方向实现令牌桶；AI 推理单列配额。

---

## 8. 观测与运维

* **Tracing**：W3C/OTel，跨南北向单一 Trace；每次调用附 `tool`、`backend_kind`、`attempt` 标签。
* **Metrics**：`invoke_count`、`latency_ms`、`error_rate`、`cost_tokens/cpu_ms` 按（能力×方向×后端）维度暴露。
* **Health**：gRPC 健康检查、MCP 自检、HTTP `/healthz`。
* **审计**：能力级审计表（actor、tenant、tool、status、artifact_ref、cost）。

---

## 9. 版本与兼容

* **能力级版本**：`tool.version`，仅在**不兼容变更**时递增大版本；支持 deprecations。
* **协议演进**：gRPC 方法/消息向后兼容；MCP Schema 采用 `oneOf`/`nullable` 方式平滑升级。
* **灰度**：Selector 支持按版本/租户/环境路由（可 A/B）。

---

## 10. 双向调用边界与建议

### 10.1 PowerX → 插件

* 读：MCP 优先，允许与 gRPC 竞速。
* 写：只走 gRPC；拒绝多路径写。
* 智能：走 Agent（内部再调 MCP/gRPC）。
* HTTP：仅 UI/探活。

### 10.2 插件 → PowerX

* 以 **gRPC** 为主：AI、IAM、KB、对象存储、审计、事件。
* 插件内引入 **PowerXClient** 薄封装：上下文注入、重试/限流、错误映射、观测。
* 异步优先：结果/后续流程尽量通过**事件总线**解耦；必要时宿主回调插件极少数 gRPC。

---

## 11. Template 领域：路由建议（示例）

| 能力                              | 事务 | 幂等 | 推荐路由         | 备选/说明                                 |
| ------------------------------- | -: | -: | ------------ | ------------------------------------- |
| `template.get`                  |  否 |  是 | **MCP**（读）   | 与 gRPC 竞速；失败回退 gRPC                   |
| `template.list` / `search`      |  否 |  是 | **MCP**      | 同上                                    |
| `template.create`               |  是 |  是 | **gRPC**（唯一） | MCP 目录可见但 Selector 强制转 gRPC           |
| `template.update`               |  是 |  是 | **gRPC**（唯一） | 同上                                    |
| `template.upsert`               |  是 |  是 | **gRPC**（唯一） | idempotency_key 必须                    |
| `template.delete`               |  是 |  是 | **gRPC**（唯一） | 软删+审计                                 |
| `template.generate_from_prompt` | 复合 | 外围 | **Agent**    | Agent 内：AI→自检→`template.upsert(gRPC)` |
| `template.review_and_fix`       | 复合 | 外围 | **Agent**    | 读（MCP）+ 评审（AI）+ 写（gRPC）               |
| `template.bulk_import`          | 复合 | 外围 | **Agent**    | 大文件解析/分批写/失败重试/报告                     |

---

## 12. 交付清单（落地所需）

### 12.1 PowerX 侧

* `CapabilityGateway`：统一入口（鉴权/上下文/幂等/观测）。
* `Selector`：默认策略 + 可配置偏好/回退；并发竞速支持。
* `Plugin Client`：MCP/gRPC/Agent 三适配；可按插件能力目录自动生成调用绑定。
* 事件总线：跨边界异步协作/回调。

### 12.2 插件侧

* **业务 Service**：唯一事实来源（Template CRUD/校验/去重）。
* **Adapters**：gRPC/MCP/Agent/HTTP **薄壳**，映射到 Service。
* **PowerXClient**：宿主 gRPC SDK 的薄封装（上下文、限流、重试、观测、错误映射）。
* **能力清单与 Schema**：仅一份（供 MCP `/tools/list`、宿主注册与 UI 校验复用）。

---

## 13. 验收清单（Checklist）

* [ ] 所有 Template 能力均在清单中定义且仅定义一次（含 RBAC/租户/策略/Schema）。
* [ ] PowerX Gateway 仅通过能力名调用；Selector 可按默认策略路由并支持覆盖。
* [ ] 写类能力实际只走 gRPC，具备幂等校验与审计记录。
* [ ] 读类能力可 MCP/gRPC 并发竞速，正确取消落败分支。
* [ ] 插件具备 gRPC/MCP Server 与（可选）Agent 执行端，三者仅做薄适配。
* [ ] 插件内使用 PowerXClient 调宿主 AI/IAM/KB 等；上下文/限流/重试/观测统一。
* [ ] 跨边界异步流程通过事件总线解耦；必要回调路径最小化。
* [ ] Tracing/Metrics/Audit 在两端贯通并按（能力×方向×后端）可观测。

---

### 附：两条典型时序（ASCII）

**A. 宿主读模板（竞速）**

```
FlowNode(template.search) → Gateway → Selector
   ├─ MCP.call_tool(template.search) ──► 插件.MCP → Service
   └─ gRPC.SearchTemplates() ─────────► 插件.gRPC → Service
<─ 先返回者被采纳；另一路取消
```

**B. 插件智能生成并入库**

```
PowerX.Agent(GenerateTemplate) → 插件.Agent.Execute
  → 插件.PowerXClient.AI.Chat()（gRPC）
  → 插件.Service.校验/去重
  → 插件.gRPC.UpsertTemplate()  ←（写路径强一致）
  → 插件.Emit(TemplateCreated) → PowerX.Bus → 索引/审计/监控
```

---

**一句话总结**：
**能力一次定义、编排单入口、读写分流、策略外置、非对称互通、治理先行**。
这样 PowerX 与插件既能灵活协作，又不会落入“三套协议三套实现”的维护陷阱。
