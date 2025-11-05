# API Client 与类型化 SDK 指南

> 说明当前 `useApiClient()`、Typed Service 模块与错误码映射策略，帮助在 PowerX Web Admin 中构建稳定的前端数据层。

---

## 1. 架构概览

- **请求核心**：`useApiClient()`（`app/composables/api/index.ts:139`）封装 Nuxt `$fetch`，统一 headers、拦截器与错误处理。  
- **类型定义**：位于 `app/composables/api/types/types.ts:1`，覆盖分页、响应结构、请求配置、拦截器接口。  
- **业务 SDK**：按域拆分在 `app/composables/api/services/**`，例如 `aiSettingService.ts`、`workflowService.ts`。每个服务导出类型定义和带 `Promise<T>` 返回值的函数/类。  
- **错误规范**：`normalizeApiError()`（`app/composables/api/normalizeApiError.ts:1`）负责将后端异常映射为 UI 文案和字段错误。  
- **全局 Loading**：拦截器调用 `useGlobalLoading()`（`app/composables/useGlobalLoading.ts`）管理请求中的“全局加载”体验。

---

## 2. useApiClient 配置

### 2.1 全局配置

- 初始 `baseURL="/api"`，会拼接到所有相对路径（结合 `nuxt.config.ts:21` 的 `public.apiBase="/api/v1"` 形成代理链）。  
- 默认 `timeout=30000`，可通过 `setApiConfig({ timeout })` 动态覆盖。  
- 全局 headers 包含 `Content-Type: application/json` 与 `Accept: application/json`，上传场景会自动移除 `Content-Type`。

### 2.2 请求拦截器

- **认证头**：客户端读取 `localStorage` 中的 `access_token` 与 `token_type`，除非 `config.skipAuth=true`（`app/composables/api/index.ts:34`）。  
- **全局 Loading**：除非显式传入 `useGlobalLoading: false`，都会在请求开始时调用 `useGL_ReqPending()` 递增计数、展示 Loading，响应/错误时递减（`app/composables/api/index.ts:52` 与 `115`）。

### 2.3 响应与错误拦截器

- 成功：执行 `responseInterceptors` 链，可变换响应结构（当前仅处理 Loading）。  
- 失败：`applyErrorInterceptors()` 遍历 `responseInterceptors.onResponseError`，允许提前处理或转换错误。  
- 最终将错误抛给调用者，由组件或 `normalizeApiError()` 解析。

---

## 3. 请求方法族

`useApiClient()` 返回 `request/get/post/put/delete/patch/upload` 六类方法：

```ts
const { get, post } = useApiClient();
const res = await get<ApiResponse<User[]>>("/admin/users", {
  params: { page: 1, pageSize: 20 },
});
```

- `GET/DELETE` 将 `data` 合并到 Query；其它方法将 `data` 放入 body。  
- 支持透传 `config` 中的 `headers`、`params`、`responseType` 等 `$fetch` 选项。  
- `upload()` 会自动去掉 `Content-Type`，以便浏览器为 `FormData` 增加 multipart 边界。

---

## 4. Typed SDK 组织方式

### 4.1 目录结构

```
app/composables/api/
├── index.ts                # useApiClient 与 setApiConfig
├── config.ts               # API 路径常量（如 ApiEndpoints）
├── normalizeApiError.ts    # 错误归一化
├── types/
│   └── types.ts            # 通用类型
└── services/
    ├── agentService.ts     # 每个文件专注单一领域
    └── ...
```

### 4.2 服务模块规范

- **导出类型**：在顶部声明请求/响应接口，复用 `PaginationParams`、`ApiResponse<T>` 等基础类型（`app/composables/api/services/workflowService.ts:5`）。  
- **函数签名**：统一返回 `Promise<T>`，命名使用动词 + 名词，如 `fetchCatalog`、`saveSettings`。  
- **使用 ApiEndpoints**：避免魔法字符串，将路径集中在 `app/composables/api/config.ts:4`。  
- **处理包装结构**：后端若返回 `{ code, message, data }`，在服务层解析后只返回 `data`，减轻组件负担（`AISettingService.getProviders()` 返回 `response.data.providers || []`）。

### 4.3 类 vs. 工具函数

- 遇到需要缓存内部状态的 SDK，可使用 `class` + `static` 方法（现有服务大多如此），或者导出工厂函数返回闭包。  
- 推荐在文件末尾导出单例（`export const workflowService = useWorkflowService();`）以提升 DX。

---

## 5. 错误处理与全局映射

1. **捕获错误**：组件调用 API 时使用 `try/catch`，并将错误传入 `normalizeApiError()`。  
   ```ts
   try {
     await permissionStore.fetchList();
   } catch (error) {
     const { title, description, fields } = normalizeApiError(error);
     toast.error(title, { description });
     form.setErrors(fields);
   }
   ```
2. **字段映射**：`normalizeApiError` 支持 `fieldMap`，可将后端字段映射为表单字段（例如 `meta -> metaText`，`app/composables/api/normalizeApiError.ts:15`）。  
3. **特殊分支**：文案中处理常见错误码（如 400 invalid key），根据业务扩展。  
4. **全局 Loading**：当 `useGlobalLoading` 开启时，拦截器会在请求结束时自动清空进度条；若组件自带 Loading，可传 `useGlobalLoading: false` 或自定义 `loadingMessage`。

---

## 6. 错误码与返回值约定

- **成功结构**：推荐后端返回 `ApiResponse<T>`，前端服务层剥离包装后返回 `T` 或具名对象。  
- **错误结构**：`{ code, message, error, errors: Record<string,string|string[]> }`，配合 `normalizeApiError` 转换。  
- **可选字段**：在 `ApiRequestConfig` 中扩展 `useGlobalError`、`skipAuth` 等语义开关，便于在服务层细粒度控制行为。

---

## 7. 与 OpenAPI 的衔接

- 当前 SDK 手写，方便迭代；若接入 OpenAPI Generator，可将生成产物放入 `app/composables/api/generated/`，再在服务层进行轻量封装。  
- 生成脚本需在 `package.json` 的 `scripts` 中记录，并在本目录补充“生成命令 + 忽略文件”说明。  
- 引入自动生成后，仍需通过 `normalizeApiError` 和自定义拦截器，使体验保持一致。

---

## 8. 最佳实践清单

- [ ] 所有请求都通过 `useApiClient()`，避免直接调用 `$fetch`。  
- [ ] 新增服务文件时，整理公共类型到 `types/`，路径常量放在 `config.ts`。  
- [ ] 组件层捕获错误后调用 `normalizeApiError`，并结合 UI 组件展示。  
- [ ] 上传/下载场景确认 `isNativeBody()` 判断逻辑是否覆盖新的数据类型。  
- [ ] 多请求并发时，合理关闭全局 Loading 或在 UI 层自定义。  
- [ ] 新增拦截器要考虑 `async` 链路（返回值必须是配置对象或 `Promise`）。  
- [ ] 提交 PR 前同步更新本文档及相关 README，确保团队成员了解新的 API 约定。
