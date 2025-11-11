# 权限守卫与 RBAC 策略

> 该文面向前端、产品与 QA，解释 PowerX Web Admin 前端如何消费 RBAC 数据、控制页面/菜单/按钮显示，并处理无权访问时的空态或拒绝态。

---

## 1. 数据来源与核心模块

| 模块 | 位置 | 职责 |
| --- | --- | --- |
| `useUserStore()` | `app/stores/user.ts: "7` | 持有用户上下文：是否 Root、当前租户、成员角色等。 |"
| `usePermissionStore()` | `app/stores/permission.ts: "60` | 拉取权限目录/列表、租户授权关系、角色权限缓存。 |"
| 菜单服务 | `app/composables/api/services/menuService.ts: "1` | 根据后端返回的菜单配置与权限元信息生成侧边栏数据。 |"
| 权限校验 | `useMe().hasPermission()`（`app/composables/api/services/meService.ts: "273`） | 后端鉴权接口，判断用户对资源/动作的可用性。 |"
| 路由守卫 | `app/middleware/auth.ts: "1` | 认证守卫，后续可在此引入权限检查。 |"

### 权限模型快速回顾

- 后端以 `plugin/resource/action` 组合定义权限，并在 `meta` 中标记类型（菜单、按钮、API、数据）。  
- UI 侧遵循“**菜单列表 → 权限目录 → 按钮控件**” 三层控制，遇到无权操作需给出解释与重试/申请入口。

---

## 2. 路由级守卫

1. **认证**：现阶段全局 `auth` 中间件只做登录检查，后续可根据路由元信息附带 `requiredPermissions`。  
   ```ts
   definePageMeta({
     middleware: ["auth"],
     permissions: ["agent.session.view"],
   });
   ```
2. **实现建议**：  
   - 在自定义中间件中读取 `to.meta.permissions`，调用 `usePermissionGuard().ensure(permissions)`；若失败返回 403 页面。  
   - 对多租户路由，先确认 `useUserStore().currentTenantId` 可用，再执行鉴权。  
3. **回退页面**：提供统一的 `/errors/403` 组件，文案包含“联系管理员开通权限”按钮。

---

## 3. 菜单与导航控制

- 菜单数据来自 `menuService.getUserMenus()`（`app/components/layout/Sidebar.vue:108`），后端已按权限过滤。  
- 仍建议在前端进行双重校验：  
  ```ts
  const visibleItems = rawItems.filter(
    (item) => !item.permissions || item.permissions.every(allow)
  );
  ```
  其中 `allow` 可调用 `usePermissionStore()` 或 `useMe().hasPermission()`。
- 分类和排序在 `Sidebar.vue` 内处理，并根据 `permissions` 生成空态。无可用菜单时展示“暂无可访问模块”提示与返回首页按钮。

---

## 4. 按钮/操作级权限

### 4.1 基础方案

- 封装 `usePermission()` composable：  
  ```ts
  const { allow } = usePermission();
  const canDelete = computed(() => allow("user.delete", currentTenantId));
  ```
- 在模板中使用条件渲染或禁用状态：  
  ```vue
  <UButton :disabled="!canDelete">删除用户</UButton>
  <p v-if="!canDelete" class="text-xs text-gray-500">缺少 user.delete 权限</p>
  ```

### 4.2 指令（可选）

- 可新增 `v-permission` 指令，支持 `v-permission="'agent.plan.edit'"` 或数组，用于简化模板判断。  
- 需要在指令中处理禁用/隐藏策略：  
  - 默认隐藏：彻底不渲染。  
  - 可配置禁用：保留按钮但展示 Tooltip。

---

## 5. 空态与拒绝态设计

| 场景 | 表现 | 行动建议 |
| --- | --- | --- |
| 页面无权限 | 展示 403 Illustration + “申请权限”按钮 | 跳转权限申请表单或指导联系管理员 |
| 菜单无任何入口 | 展示空菜单页，提供“返回首页”/“刷新”按钮 | 记录日志，提示用户检查账号或租户切换 |
| 按钮禁用 | 显示 Tooltip：“需要 {角色/权限}” | 若允许申请，则弹出权限申请抽屉 |
| API 403 | 调用 `normalizeApiError` 并显示 Toast：“当前操作无权限” | 根据业务决定是否自动刷新页面或回退 |

空态组件应放在 `app/components/common/EmptyStateForbidden.vue`（可新增），所有调用处保持一致。

---

## 6. 权限数据同步与缓存

1. **权限目录**：`permissionStore.fetchCatalog()` 会缓存树形结构，并记录 `lastSyncTime`（`app/stores/permission.ts:134`）。  
2. **租户切换**：触发 `useUserStore().switchTenant()` 后，需要重新加载权限目录与租户授权。  
3. **本地缓存**：在 `permissionStore` 中使用 `roleSelection` / `roleInitialSelection` 暂存编辑态，提交成功后刷新列表。  
4. **API 校验**：对于关键操作，可在提交前再次调用后端 `checkPermission`，防止权限变更导致的竞态。

---

## 7. 测试与验收

- **单元测试**：对权限 composable 编写测试，模拟不同权限集下的返回值。  
- **E2E**：使用 Playwright（见 `docs/testing/E2E_Testing_with_Playwright.md`）模拟普通用户/管理员角色，验证菜单与按钮显隐。  
- **回归清单**：  
  - [ ] 切换租户后菜单是否更新。  
  - [ ] 禁止访问的页面是否跳转至 403。  
  - [ ] 按钮禁用时是否提供提示。  
  - [ ] 后端 403 是否正确显示错误消息并清空 Loading。  
  - [ ] Root 用户是否不受限制。

---

## 8. 后续计划

- 引入“权限申请”工作流：前端收集操作意图，自动生成 Ticket。  
- 在 `auth` 中间件中支持 `meta.requiredRole`/`requiredScopes` 写法。  
- 与后端约定权限版本号，当版本更新时自动刷新缓存并弹出提示。  
- 将权限列表映射表沉淀至 `docs/appendix/API_Route_Map.md`，便于跨团队协作。
