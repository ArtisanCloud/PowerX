---

# CoreX Flow（精简版规范）

## 顶层字段

| 字段        | 说明        | 运行时作用                      |
| --------- | --------- | -------------------------- |
| `flow_id` | Flow 唯一标识 | Plan 构建布线、Agent.Invoke 的目标 |
| `name`    | 可读名称      | 仅展示（UI/日志）                 |

## steps（一个 Step = 一个节点/Node）

| 字段        | 说明                               | 运行时作用                            |
| --------- | -------------------------------- | -------------------------------- |
| `use`     | `"namespace.action"`             | 通过 HandlerRegistry 精确路由到 handler |
| `kind`    | 节点类型（`biz_logic`/`llm`/`http`/…） | UI 分类、监控、trace                   |
| `subtype` | 子类型（`transform`/`template`/…）    | 更细粒度展示/统计                        |
| `params`  | 静态参数（支持模板）                       | 传给 handler；与运行时参数合并              |
| `io`      | 输入输出声明                           | 入参绑定（`in_map`）+ 出参收敛（`out_map`）  |

### `io` 子字段（统一命名）

* `inputs`: "端口声明（`name/type/required/desc`），供 UI/校验。"
* `in_map`: "**端口名 → 绑定模板**（执行前解析为 `_inputs`）。"
* `out_map`: **端口名 → 原始返回字段名**（执行后标准化输出，只保留声明端口）。

#### 绑定模板可用命名空间（建议）

* `task.<taskId>.output.<field>`：上游任务输出（Plan 的 `param_refs` 也会指向这里）
* `flow.vars.<key>`：Flow/Step 级变量
* `history[N].data.<field>`：历史第 N 条的字段
* `input.<name>`：Flow 对外入参

> 目的：handler 只“吃端口”，与上游是谁无关；布线与数据源由执行器解析模板完成。

## metadata（Flow 级）

| 字段                       | 说明                          | 运行时作用                         |
| ------------------------ | --------------------------- | ----------------------------- |
| `io.inputs`/`io.outputs` | Flow 入口/出口端口                | 生成 API/表单、文档                  |
| `extra_info.requires`    | 依赖的上游 Flow ID（逗号或数组）        | Plan 构建时生成 `depends_on` 与参数布线 |
| `intent.matchers`        | 规则匹配（keyword/pattern）       | 第一判决层（快、可控）                   |
| `intent.examples`        | few-shot（positive/negative） | LLM 分类用（第二判决层）                |

---

# 执行链路（从意图到结果）

## 1) 意图 → Flow

* 先跑 `matchers`（keyword/pattern）→ 命中且得分 > high → 直接选中
* 未命中/有冲突 → 用 `examples` 做 LLM classify 打分
* 过阈值 → 入选；仍无结论 → 兜底 defaultFlow

> 与你 `Manager.intentStrategies + intentLow/High` 兼容。

## 2) 计划构建（Plan）

* 按 `extra_info.requires` 拓扑补全依赖，生成 `depends_on / stage`。
* 对齐 Flow/Step `io.inputs`，将“上游输出端口”自动映射为本步骤 `param_refs`（如 `{{task.t1.output.id}}`）。
* 若配置了 `WireRule`，优先按 Map 或 AutoByName 线束。

## 3) 执行（Manager.ExecutePlan）

每个任务 `t`：

1. `vars ← t.Params`（运行时参数）
2. `histView ← buildHistoryView(results, t.FlowID)`（已执行历史）
3. **计算 \_inputs**：解析 `in_map`（模板命名空间：task/flow\.vars/history/input）
4. **参数合并（给 handler）**

    * `_inputs`（解析后端口）
    * `step.params`（静态）
    * `t.Params`（运行时覆盖）
5. 调用 handler（由 `use: ns.action` 在 `HandlerRegistry` 解析）
6. 得到 raw 输出后用 `out_map` 收敛 → `taskOutputs[t.TaskID]["output"]`
7. trace 记录：`node_kind/subtype + inputs + outputs`

> 这样 trace 即可显示“每个节点的类型 + 实际入参/出参”。

---

# 代码落点（你项目里的具体改动位）

1. **YAML 解析补全**（已经提醒过）

* 文件：`pkg/corex/flow/schemas/blueprint.go`
* 在 `stepAlias` 加：`Kind/Subtype/IO/History` 字段
* 在 `Step.UnmarshalYAML` 赋回：`s.Kind/s.Subtype/s.IO/s.History = aux...`

> 解决你 `lookupStepIO()` 一直为 `nil`、node\_kind 空的问题。

2. **统一命名**

* 将 `io.in` 改为 `io.in_map`，你的 `lookupStepIO` 和执行器取 `st.IO.InMap`。

  > 可向下兼容：解析时如果 `in_map` 为空而 `in` 不为空 → 复制到 `in_map`。

3. **Handler Registry**

* 新建：`services/agent/contract/registry.go`（或你已有 contract 包内）

    * `Register(ns, name string, h HandlerFunc)`
    * `Resolve("ns.action") (HandlerFunc, bool)`
* 在 `eino.Agent` 初始化时注册内建 handler：`toy.process`、`response.format`、`http.request` 等。

4. **参数合并顺序**（ExecutePlan 内部）

* `_inputs := resolve(in_map)` → `params["_inputs"] = _inputs`
* `params["_vars"] = vars`（t.Params）
* `params["_history"] = histView`
* 给 handler 的 `effectiveParams` = `merge(_inputs, step.params, t.Params)`（同名后者覆盖）

5. **Trace**

* 追加 `lookupFinalNodeType(flowID)`（已实现）
* `trace = append(trace, {task_id, flow_id, node_kind, node_subtype, inputs, outputs})`
* `attachTrace(final, trace, plan.PlanID)`

---

# 示例：`task_2.yaml`

```yaml
flow_id: task_2
name: 任务2：基于上游ID处理

steps:
  - use: toy.process
    kind: biz_logic
    subtype: transform
    params:
      mode: "passthrough"
    io:
      inputs:
        - name: upstream_id
          type: string
          required: true
      in_map:
        upstream_id: "{{ task.${UP}.output.id }}"  # Plan 会把 ${UP}替换成实际上游
      out_map:
        id: id
        timestamp: timestamp

  - use: response.format
    kind: text_proc
    subtype: template
    params:
      template: '{ "id": "{{ .input.upstream_id }}", "from": "t2" }'
    io:
      inputs:
        - name: upstream_id
          type: string
          required: true
      in_map:
        upstream_id: "{{ task.${UP}.output.id }}"
      out_map:
        id: id
        timestamp: timestamp

metadata:
  io:
    inputs: ["upstream_id"]
    outputs: ["id"]
  extra_info:
    requires: "task_1"
  intent:
    matchers:
      - type: keyword
        any: ["执行任务2","任务2","run task 2","t2"]
    examples:
      positive: ["执行任务2"]
      negative: ["执行任务1","执行任务5"]
```

> 规划器把 `${UP}` 换成真正前置的 taskId（比如 `t_auto_1`），并在 `plan.tasks[].param_refs` 写入：
> `upstream_id: "{{task.t_auto_1.output.id}}"`

---

# Handler 侧的最小约定

```go
type HandlerFunc func(ctx context.Context, inputs map[string]any, params map[string]any) (map[string]any, error)

// Manager.ExecutePlan 内部（调用前）：
inputs := resolveInMap(step.IO.InMap, ns) // ns 含 task/vars/history/input
effective := merge3(inputs, step.Params, t.Params) // 后者覆盖前者
outRaw, err := h(ctx, inputs, effective)
outStd := applyOutMap(outRaw, step.IO.OutMap) // 只保留标准端口
```

> 你要“打印输入/输出”，就在每个 handler 里 `log.Debug("inputs", inputs, "params", effective)`；
> 或者在 `Agent.Invoke` debug 模式下，把 `_inputs/_vars/_history` 回传到 `result.Data.__debug`（已给过示例）。

---

# intent：规则 + LLM 协作

* **第一层**：`matchers`（keyword/pattern）→ 高置信度命中直接用（给个 `priority`/`weight`）。
* **第二层**：把候选 flow 的 `examples` 喂给 LLM 分类（One-vs-All），得分≥`intentHigh` 入选。
* 阈值区间：`intentLow`、`intentHigh` 与你 `Manager` 现有字段一致。
* 无结论 → `defaultFlow`。

---

# 最后给你一份简短清单（Checklist）

* [ ] `stepAlias` 补齐 `Kind/Subtype/IO/History` 并在 `UnmarshalYAML` 赋回
* [ ] 统一 `io.in_map/out_map`（保留 `in` 兼容迁移）
* [ ] 新增 `HandlerRegistry`，`use="ns.action"` 解析并路由
* [ ] ExecutePlan 合并参数顺序：`_inputs` ← `in_map`、`step.params`、`t.Params`
* [ ] 每节点 trace：`kind/subtype + inputs + outputs`（按 `out_map` 收敛）
* [ ] 规划器：用 `metadata.extra_info.requires` 补全依赖；`io.inputs` + WireRule 做参数布线
* [ ] 意图：`matchers`→LLM `examples`→fallback，阈值与你现有字段一致

按这个走，你就能 **在每个节点上看到类型、真实入参/出参**，并且 handler 可控、模板绑定可解释。需要我把 `task_1~task_6` 批量转成 `in_map/out_map` 风格，或贴 `HandlerRegistry` 的完整实现。
