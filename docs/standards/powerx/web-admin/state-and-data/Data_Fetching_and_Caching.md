# 数据获取与缓存策略

> 本指南阐述 PowerX Web Admin 中的请求方式选择、缓存策略以及在 Nuxt 4 环境下的 SSR/ISR 考量，帮助前端工程师在页面和组件中高效加载数据。

---

## 1. 请求层选择

| 场景 | 推荐方案 | 说明 |
| --- | --- | --- |
| 业务接口调用 | `useApiClient()` + Pinia store | 统一拦截器、全局 Loading、错误归一化（`app/composables/api/index.ts:139`）。 |
| 页面级数据预取 | `useAsyncData()` / `useFetch()` | 内建缓存、依赖追踪，适合菜单、导航等数据（`app/components/layout/Sidebar.vue:108`）。 |
| 非 API 资源（静态 JSON、第三方） | 原生 `$fetch` 或 `ofetch` | 可自定义 headers/缓存，注意同源策略。 |
| WebSocket / SSE | 请参考 `docs/realtime/SSE_WS_Client_Guide.md` | 与本文数据缓存策略配合时，可在 store 层增量更新。 |

> 默认优先通过 store 暴露数据，组件读取响应式状态；只在页面首次渲染或一次性数据时使用 `useAsyncData`。

---

## 2. useAsyncData / useFetch 约定

### 2.1 基础用法

```ts
const { data, pending, error, refresh } = await useAsyncData(
  "user-menus",
  () => menuService.getUserMenus(),
  {
    default: () => ({ data: [], categories: [] }),
    transform: normalizeMenuResponse,
    watch: [locale],
  }
);
```

- **Key 唯一**：使用具语义的字符串（`user-menus`），避免与其他页面冲突。  
- **default**：提供初始值保证 SSR 和客户端首次渲染不会出现 `undefined`。  
- **transform**：将后端返回转换为组件期望结构（`Sidebar.vue:118`）。  
- **watch**：依赖变更（如语言切换）时自动刷新。

### 2.2 参数与缓存

- `server`: 当前项目是 SPA，可保持默认 `true`。若未来开启 SSR，可在仅需客户端执行的请求上设为 `false`。  
- `staleTime`: 配合 SWR 模式，设置缓存有效期（毫秒）；超时后下次访问触发后台刷新。  
- `lazy`: 与 `lazy: true` 组合可延迟请求至客户端，适合低优先级数据。  
- `immediate`: 设为 `false` 时，可以在条件满足后通过 `refreshNuxtData(key)` 触发请求。

### 2.3 使用 useFetch

`useFetch` 是 `useAsyncData` 的语法糖。适合简短内联调用：

```ts
const { data, pending } = await useFetch("/api/v1/ping", {
  key: "health-check",
  server: false,
});
```

仍需提供 `key`，并留意 `$fetch` 已被插件包装，会计入全局 Loading（`app/plugins/global-loading.ts:4`）。

---

## 3. SWR 与缓存失效策略

### 3.1 何时刷新

- **显式刷新**：调用 `refresh()` 或 `refreshNuxtData("user-menus")`，适合设置、权限等变更后立即更新侧边菜单。  
- **事件驱动**：通过 Pinia store 监听 WebSocket 事件，刷新对应数据（例如 Agent 会话新增后调用 `messageStore.setMessages()` 并手动更新缓存）。  
- **路由守卫**：在中间件中根据路由元信息决定是否刷新缓存。

### 3.2 缓存分层

1. **请求级**：`useAsyncData` 内置缓存，依赖 key 与参数。  
2. **状态级**：Pinia store 持有长时缓存，提供 `clear()`/`fetch()` 控制。  
3. **存储级**：`localStorage` 记住用户选择（见 `app/stores/envStore.ts:126`），非结构化数据不建议持久化。

### 3.3 数据一致性

- 确保写操作成功后更新 store，再刷新 `useAsyncData` 缓存，避免 UI 显示旧数据。  
- 在 `useApiClient` 的响应拦截器中统一处理 401/403，必要时清空所有缓存并跳转登录。  
- 对于高频更新数据（通知、实时任务），优先采用 WebSocket 推送 + store，也可在后台定时轮询。

---

## 4. SSR / ISR 考量

当前配置 `ssr: false`（`nuxt.config.ts:7`），但设计文档需兼容后续迁移：

| 模式 | 建议 | 注意事项 |
| --- | --- | --- |
| SSR | 使用 `useAsyncData` 获取首屏数据，将必要的 store 状态通过 `pinia.state.value` 序列化。 | 避免在服务端访问 `window` 与 `localStorage`。 |
| SSG (`npm run generate`) | 为动态路由提供 `prerender.routes` 或在页面内使用 `defineGenerateRoutes`。 | 若数据需实时更新，改用客户端请求。 |
| ISR | 借助 Nuxt Nitro 的 `cachedEventHandler` 或部署平台（如 Vercel）的 revalidate 机制；前端依旧通过 `useAsyncData` + `refresh` 获取最新数据。 |

当 SSR 启用时，请审视 store 的持久化逻辑，将 `process.client` 守卫保留，并在 `useAsyncData` 中避免访问浏览器 API。

---

## 5. 缓存失效与并发控制

- **幂等请求**：POST/PUT 操作完成后主动刷新相关缓存，传入 `useGlobalLoading: false` 则需要组件层处理 Loading。  
- **并发合并**：对同一资源的并行 `useAsyncData` 调用会自动去重（相同 key）；若需要独立请求，追加参数或随机后缀。  
- **后台刷新**：结合 `staleTime` 和 `initialCache`，可在页面保持旧数据的同时后台请求新数据，再通过 `pending` 识别状态。

---

## 6. 工具与最佳实践

- `refreshNuxtData(key)`：在提交表单、切换租户后刷新页面缓存。  
- `clearNuxtData()`：调试时可清空所有缓存（谨慎使用）。  
- `useIntervalFn`（来自 VueUse，可 npm 添加）：配合 `useApiClient` 做轮询，注意在组件卸载时停止。  
- 记录缓存命中率与 API 延迟，可在 `useApiClient` 的响应拦截器中统计，上传到监控日志。

---

## 7. Review Checklist

- [ ] 是否为每个 `useAsyncData/useFetch` 提供唯一 key 和 default 值。  
- [ ] 是否避免在默认值为 `null/undefined` 的情况下直接渲染，导致闪动。  
- [ ] 更新操作后是否刷新了相关缓存或 store。  
- [ ] 组件是否正确处理 `pending`/`error` 状态并给出空态提示。  
- [ ] 缓存是否会在租户切换、语言切换等情况下更新。  
- [ ] 是否考虑了 SSR/SSG 的 `server` 选项和浏览器 API 限制。  
- [ ] 新增全局缓存策略时，是否更新此文档并通知团队。
