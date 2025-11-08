# Mock 数据集索引

> 汇总项目中现有的 Mock 数据集、用途与适用场景，便于开发和 QA 快速找到示例数据或扩展新的假数据。

---

## 1. Mock 清单

| 名称 | 文件位置 | 内容 | 用途 |
| --- | --- | --- | --- |
| 全局搜索数据 | `app/composables/api/services/searchService.ts:10` | `mockSearchData`, `mockSuggestions` | 模拟搜索结果、分面、建议，支撑 `/test/search-showcase` 页面与搜索组件调试。 |
| 工作流节点规格 | `app/composables/api/services/workflowService.ts:158` | `mockKinds`、`mockPalette` | 当后端不可用时，提供 Workflow 节点类型、工具箱数据。 |
| 通知中心 | `app/composables/useNotifications.ts:17` | `mockNotifications` | 模拟通知列表、统计、操作按钮，便于 Notification 组件开发。 |
| Agent 流 Mock（注释） | `app/composables/agent/useDualChannelConnection.ts:249` | `/agents/stream/mock`（预留） | 本地可切换至 Mock 流，实现离线演示。 |
| 测试页面数据 | `app/pages/test/*.vue` | 例如 `mockResults`、`mockSuggestions` | `/test/connection`, `/test/search-showcase` 等测试页的静态数据。 |
| 插件市场 | 无（待补充） | — | 可通过 `useAdminPluginsService().getMarketplaceV2` 返回后端数据；若需要 Mock，请在此文档登记。 |

---

## 2. 维护规范

1. 新增 Mock 时，放在相关 composable 或 `app/composables/api/mocks/`（建议后续创建）。  
2. 保持类型与真实 API 一致，便于切换到真实服务。  
3. 在文档中说明 Mock 适用场景和切换方式。  
4. 当后端已提供接口时，及时移除或标记废弃。

---

## 3. Mock 切换策略

- 在 composable 中优先发起真实请求，失败时回落到 Mock（见 `workflowService` 实现）。  
- 对需要本地控制的 Mock，可暴露配置项（例如 `useGlobalLoading({ mock: true })`）。  
- 对测试页面，明确 Mock 数据与真实 API 的差异，避免 QA 混淆。

---

## 4. TODO

- [ ] 为 Agent 聊天流实现可选的 mock SSE/WS 服务。  
- [ ] 整理插件市场、仪表盘的示例数据，方便无后端展示。  
- [ ] 在 `scripts/check-refactor.sh` 中检测 Mock 文件的变更，提醒更新文档。  
- [ ] 结合 `msw`/`@vitest/browser` 搭建统一 Mock 层，供测试与 Storybook 共用。
