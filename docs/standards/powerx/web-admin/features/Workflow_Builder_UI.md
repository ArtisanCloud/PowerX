# Workflow Builder UI

> 描述工作流编排器的页面结构、节点交互、属性编辑与运行调试行为，对应实现位于 `app/components/workflow/WorkflowEditor.vue` 与相关 composable。

---

## 1. 页面结构

| 区域 | 组件 | 功能 |
| --- | --- | --- |
| 工具栏 | `WorkflowEditor` 顶部按钮区域 | 撤销/重做、适应视图、保存、运行。 |
| 左侧面板 | 节点清单（Palette） | 搜索节点、拖拽到画布。 |
| 中央画布 | Vue Flow (`<VueFlow>`) | 显示节点和连线，支持缩放、拖拽、连接。 |
| 右侧面板 | 属性面板 | 调整节点属性、Schema 驱动表单。 |

页面路由 `/workflow/workspace` 使用专用布局 `layout: 'workflow'`，并通过 `useWorkflowManager()` 在挂载时加载工作流数据（`app/pages/workflow/workspace.vue:19`）。

---

## 2. 节点清单（Palette）

- 节点数据来自 `useWorkflowService().getPalette()`，若后端不可用则落到 Mock（`app/composables/api/services/workflowService.ts:115`）。  
- 搜索框 `paletteSearch` 支持关键字过滤。  
- 每个节点项支持拖拽：`@dragstart="onDragStart($event, item.id)"`，携带 `kind` 信息。

---

## 3. 画布交互

- 使用 `@vue-flow/core`，开启以下能力：  
  - `@connect`：处理节点连线，默认允许多出度。  
  - `@node-drag-stop`：保存节点位置。  
  - `@node-click`：选中节点并打开属性面板。  
  - `Background`、`MiniMap`、`Controls` 提供视觉辅助与缩放控件。  
- 自定义节点类型 `node-generic` 绑定 `GenericNode`，用于展示节点抬头、状态、错误信息等。

> 未来若需要不同节点外观，可在 `<template #node-xxx>` 中扩展专用组件。

---

## 4. 属性面板

- `selectedNode` 变化时渲染表单，根据 `selectedNode.data.schema` 动态选择控件：  
  - 布尔：`USwitch`。  
  - 数字：`UInput type="number"`，`getNumberStep()` 依据 schema `minimum/maximum/step`。  
  - 枚举：`USelect` (`isEnumField`)。  
  - 长文本：`UTextarea`。  
  - 对象：转为 JSON 文本编辑（`objectProps` 临时存储）。  
- 更新时调用 `updateNodeProps(nodeId, newProps)`，并触发 `useVueFlow().updateNode()` 同步到画布。

---

## 5. 工具栏与命令

- `undo/redo`：使用内部历史栈 (`canUndo`, `canRedo`) 控制按钮状态。  
- `handleFitView()`：调用 Vue Flow `fitView()` 自动缩放。  
- `handleSaveWorkflow()`：待对接 API，当前可序列化 `{ nodes, edges }` 并传给 `useWorkflowService().saveWorkflow()`.  
- `runWorkflow()`：占位实现，后续触发后端执行接口并展示日志输出。

---

## 6. 数据加载与 Mock

- `useWorkflowManager().loadWorkflow(id)` 负责从后端拉取流程定义（TODO）。  
- 未传 `id` 时创建演示工作流，用于本地调试。  
- `workflowService` 在 `fetchKinds`、`fetchPalette` 失败时返回 Mock 数据，保证 UI 可演示（`app/composables/api/services/workflowService.ts:116`）。

---

## 7. 测试建议

- `/workflow/test-workspace` 可作为沙箱页面（若存在）展示 Mock 数据。  
- 手动验证：  
  - 拖拽节点、连接、删除、撤销/重做。  
  - 编辑属性后保存并刷新，确认数据持久化。  
  - 切换暗色主题，确保画布背景与节点对比度良好。  
  - 错误场景：加载失败、保存失败时提示 `UAlert` 或 Toast。

---

## 8. 后续规划

- 多选操作、对齐辅助线、节点分组。  
- 节点版本管理与模板库（引用 `workflowService.getTemplates()`）。  
- 调试回放：展示运行轨迹、高亮执行节点。  
- 协同编辑：结合实时服务实现多人同步。  
- 快捷键：`Delete` 删除节点、`Cmd+Z`/`Shift+Cmd+Z` 撤销/重做。
