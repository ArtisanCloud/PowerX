# 错误边界与统一处理策略

> 本文总结 PowerX Web Admin 的错误处理体系，包括 Nuxt 全局错误页面、组件级边界、API 错误归一化与用户提示策略。

---

## 1. 渲染层级错误

| 层级 | 机制 | 位置 | 说明 |
| --- | --- | --- | --- |
| 全局错误页面 | `app/error.vue` | 使用 Nuxt `error.vue` | 显示状态码、错误详情、复制按钮、返回操作。 |
| 布局/组件级 | 建议使用 `<ErrorBoundary>` 或自定义包装组件 | 待实现 | 针对工作流编辑器等复杂区域，避免局部错误导致整个页面崩溃。 |
| Composable 异常 | 手动 `try/catch` + Toast | `useAgentManager`、`useChatSessions` 等 | 捕获后调用 `useOneShotAlert` 提示用户。 |

> 当前仅提供全局错误页，后续需要在关键页面引入局部边界，避免小范围错误导致跳转到 “红屏”。

---

## 2. API 错误归一化

- `normalizeApiError(err, fieldMap)` 将后端返回结构转化为 `{ title, description, fields }`（`app/composables/api/normalizeApiError.ts:1`）。  
- 典型用法（组件中）：
  ```ts
  try {
    await permissionStore.fetchList();
  } catch (err) {
    const { title, description, fields } = normalizeApiError(err);
    toast.error(title, { description });
    form.setErrors(fields);
  }
  ```
- 统一处理 400/422 等校验错误，同时保留 `raw` 便于上报日志。

---

## 3. Toast 与警报

- `GlobalAlertNotification` + `useOneShotAlert()` 用于在视图外部弹出 UAlert（`app/components/GlobalAlertNotification.vue`）。  
- 对于表单错误，优先在组件内展示；全局 Toast 用于流程性提示（登录失效、权限不足等）。  
- 建议错误信息遵循《UX Writing for Errors》规范（另见文档）。

---

## 4. 重试与降级

- 网络错误/超时：提供“重试”按钮，调用相同 API；若连续失败，提示用户检查网络或联系管理员。  
- SSE/WS 断线：`useDualChannelConnection` 内部处理重连并在 UI 上显示状态图标。  
- 批量操作失败：保留成功/失败列表，避免一次错误导致数据丢失。

---

## 5. 记录与上报（规划）

- 引入 Sentry/LogRocket 等工具捕获前端异常，关联后端 trace（见 `Sentry_Logging_and_Traces.md`）。  
- 在 `app/error.vue` 提供“复制错误详情”，便于用户反馈。  
- 组件内部错误应包含 `err.code`、`requestId` 等信息，方便排查。

---

## 6. 审查清单

- [ ] 所有 API 调用均包裹 `try/catch`，并使用 `normalizeApiError`。  
- [ ] UI 在错误发生时提供明确提示与后续行动。  
- [ ] 长时间操作（导入、安装插件）支持取消和重试。  
- [ ] SSE/WS 断线提示清晰，可手动重连。  
- [ ] 新增错误场景是否在文档和 QA 用例中登记。

---

## 7. 后续计划

- 实现组件级错误边界（`<ErrorBoundary>`），提供“刷新局部”功能。  
- 在 store/composable 层记录最近一次错误，方便全局监控侧采集。  
- 将错误信息本地化，确保多语言环境提示准确。  
- 与监控平台联动，异常高频时自动通知维护人员。
