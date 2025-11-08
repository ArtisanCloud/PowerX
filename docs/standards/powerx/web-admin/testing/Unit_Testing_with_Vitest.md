# 使用 Vitest 进行单元测试

> 介绍 PowerX Web Admin 中编写组件、Composable、Store 单元测试的最佳实践。当前项目尚未配置 Vitest，请在引入时遵循本文指南。

---

## 1. 初始化环境

1. 安装依赖：
   ```bash
   npm install -D vitest @vitest/ui @vue/test-utils@next @nuxt/test-utils edge-runtime
   ```
2. 在根目录创建 `vitest.config.ts`：
   ```ts
   import { defineConfig } from "vitest/config";
   import Vue from "@vitejs/plugin-vue";
   import VueJsx from "@vitejs/plugin-vue-jsx";

  export default defineConfig({
     plugins: [Vue(), VueJsx()],
     test: {
       globals: true,
       environment: "jsdom",
       setupFiles: ["./tests/setup/vitest.setup.ts"],
       coverage: {
         provider: "v8",
         thresholds: { statements: 80, branches: 70, functions: 80, lines: 80 },
       },
     },
   });
   ```
3. 创建 `tests/setup/vitest.setup.ts`，注册 `@testing-library/jest-dom`、mock `i18n`、`useRuntimeConfig()` 等。

---

## 2. 目录与命名

- 所有测试放在 `tests/` 或组件同级 `__tests__/` 目录，命名 `*.spec.ts`。  
- 组件测试文件示例：`app/components/agent/__tests__/ChatComposer.spec.ts`。  
- Composable/Store 测试可放在 `tests/composables/`、`tests/stores/`，例如 `tests/stores/messageStore.spec.ts`。

---

## 3. 常用测试场景

| 场景 | 工具 | 示例 |
| --- | --- | --- |
| Vue 组件 | `@vue/test-utils` + `@testing-library/vue` | 渲染 `AgentSidebar`, 断言列表项与事件发射 |
| Pinia Store | `createPinia`, `setActivePinia` | 测试 `useMessageStore` 的增删改逻辑 |
| Composable | 组合 `mountComposable`（来自 `@nuxt/test-utils`） | 验证 `useDualChannelConnection` 状态机 |
| API 工具 | Mock `$fetch` 或使用 `msw` | 测试 `useApiClient` 的拦截器行为 |

---

## 4. Mock 与依赖

- 使用 `vi.mock()` mock 掉网络请求、Nuxt 插件（如 `useColorMode`）。  
- 对于全局 runtime config，可在 `tests/setup/vitest.setup.ts` 注入：
  ```ts
  vi.stubGlobal("useRuntimeConfig", () => ({
    public: { apiBase: "/api/v1" },
  }));
  ```
- WebSocket/SSE：可借助 `mock-socket` 或自定义 stub；复杂场景可将逻辑拆到纯函数后单独测试。

---

## 5. 快照与覆盖率

- 推荐使用组件快照 (`expect(wrapper.html()).toMatchSnapshot()`)，但需谨慎更新。  
- 目标覆盖率：Statements ≥80%，Branches ≥70%。执行 `npx vitest run --coverage` 查看报告。  
- 对于难以测试的第三方 UI，可通过 `shallowMount` 并在快照中忽略子组件。

---

## 6. 与 Nuxt 集成

- 借助 `@nuxt/test-utils/runtime` 的 `mockNuxtImport` 模拟 Nuxt helper，如 `useRouter()`：
  ```ts
  import { mockNuxtImport } from "@nuxt/test-utils/runtime";
  mockNuxtImport("useRouter", () => () => ({ push: vi.fn() }));
  ```
- 对于 `definePageMeta` 等编译期 API，测试时可直接忽略或在组件外包一层用于测试的 wrapper。

---

## 7. 运行与调试

- 命令：`npx vitest`（交互式）或 `npx vitest run`（CI 模式）。  
- 搭配 `@vitest/ui` 查看测试面板：`npx vitest --ui`.  
- 在 VS Code 中配置 `vitest` 插件以获得断点调试体验。

---

## 8. CI 集成

- 在 CI 工作流中增加步骤：
  ```yaml
  - name: Run unit tests
    run: npx vitest run --coverage
  ```
- 结合 `coverage-summary.json` 上报覆盖率 (Codecov、SonarQube)。  
- 失败时保留 `tests/results` 目录供开发者下载分析。

---

## 9. Review Checklist

- [ ] 断言覆盖核心逻辑与异常分支。  
- [ ] Mock 是否合理，避免过度依赖实现细节。  
- [ ] 运行 `npm run lint`、`npx vitest run` 均通过。  
- [ ] 新增依赖/脚本已写入 `package.json` 与文档。  
- [ ] 快照是否清晰，包含必要的可读内容。  
- [ ] 更新后的覆盖率满足阈值，或在 PR 中说明例外原因。

---

## 10. 后续计划

- 引入 `msw` 与 `@vitest/browser` 测试服务端事件、WebSocket。  
- 建立组件 Storybook，与 Vitest 共享 mock 数据。  
- 使用测试工厂（fixtures）统一构造 Agent/插件等示例数据。
