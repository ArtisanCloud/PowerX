# 实时仪表盘（Realtime Dashboard）

> 概述 `/dashboard` 页面目标、布局、数据来源及后续接入实时数据的规划。目前页面位于 `app/pages/dashboard/index.vue`，使用 ECharts + Nuxt UI 组成卡片和图表。

---

## 1. 目标与角色

- **受众**：平台管理员、运营人员，关注 Agent 运行、租户活跃度、Token 消耗等指标。  
- **目标**：提供实时统计、趋势分析、异常告警入口，并支持数据导出。  
- **实时性要求**：关键指标延迟 < 5 秒，趋势图以分钟粒度更新，历史数据可回溯 30 天。

---

## 2. 页面布局

| 区域 | 内容 | 当前实现 |
| --- | --- | --- |
| 顶部统计卡片 | 用户数、访问量、营收、活跃度 | `stats` / `agentStats` 静态数组展示（`app/pages/dashboard/index.vue:30`） |
| 趋势图 | `访问趋势` 折线图 | `visitTrendOption`（ECharts） |
| 用户分布 | 饼图 | `userDistributionOption` |
| Top 列表 | 对话排行、插件排行 | 需接入 API（可复用 `agentStats` 数据） |
| 实时事件流 | WebSocket/SSE 数据（规划） | 待实现，可使用 `useDualChannelConnection` 扩展 |

---

## 3. 数据来源

| 指标 | 推荐 API/通道 | 更新频率 |
| --- | --- | --- |
| 核心计数 (Total Users, Token 消耗) | REST：`/admin/dashboard/overview` | 30 秒刷新或按需请求 |
| 趋势数据 | REST + SWR 缓存 (`?range=30d`) | 每次切换日期范围刷新 |
| 实时事件/任务 | WebSocket Topic `dashboard.events` | 实时推送 |
| 告警 | SSE Topic `alerts` 或通知中心 | 实时 |

在前端可组合 `useAsyncData`（历史数据） + `useDualChannelConnection`（实时推送），并使用 Pinia store 聚合。

---

## 4. 交互与过滤

- **时间范围选择**：支持今日/近7天/近30天，自定义时间段。  
- **租户过滤**：在多租户场景下，通过下拉选择租户或“全部”，需要调用 `/admin/dashboard?tenantId=`。  
- **指标切换**：卡片支持点击切换视角（例如切换到 Token 消耗趋势）。  
- **告警筛选**：提供严重级别、来源等过滤器。

---

## 5. 实时数据处理（规划）

1. 建立 WebSocket 连接订阅 `dashboard.events`。  
2. 推送结构：
   ```json
   { "type": "metric", "metric": "token_usage", "value": 1234, "timestamp": "..." }
   ```
3. 前端根据 `type` 更新对应图表或将事件写入“实时事件流”列表。  
4. 若连接中断，自动回退到周期轮询，并在 UI 显示“已切换到轮询模式”提示。

---

## 6. 导出与分享

- 提供导出 CSV/Excel（调用后台生成或前端处理）。  
- 截图/分享：调用后端生成快照或利用浏览器 `toDataURL` 导出 PNG。  
- Embed 模式：生成受保护的分享链接，遵守租户范围与权限校验。

---

## 7. 无权限/空数据处理

- **无数据**：显示空态卡片与“暂无数据”说明，可提供“去安装插件”或“开启 Agent”导航。  
- **无权限**：展示 403 空态（参考 `Permission_Guards_and_RBAC.md`），并隐藏敏感指标。

---

## 8. 测试 checklist

- [ ] 时间范围切换后图表刷新正常，无闪烁。  
- [ ] WebSocket 推送断开后能够自动重连或回退轮询。  
- [ ] 导出功能生成正确文件，内容与筛选条件一致。  
- [ ] 不同角色登录时指标可见性符合预期。  
- [ ] 暗色模式下图表、卡片对比度符合要求。  
- [ ] 在移动端或窄屏下布局保持可读。

---

## 9. 后续计划

- 接入后端真实 API 并将静态数据替换为动态接口。  
- 新增“异常监控”面板，展示失败率、响应时间阈值。  
- 与告警中心联动，支持一键跳转到问题详情或日志。  
- 构建自定义小部件系统，让用户定制仪表盘布局。
