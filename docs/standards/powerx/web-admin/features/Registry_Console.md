# 能力目录（Registry Console）

> 面向产品、QA 与前端，描述插件/能力目录页面的交互流程、数据来源及角色差异。当前实现主要集中在 `/plugins/market`（`app/pages/plugins/market.vue:1`）与 `components/plugins/*`。

---

## 1. 页面定位

- **入口**：侧边栏「插件市场」，路由 `/plugins/market`。  
- **目标用户**：平台 Root 管理员、租户管理员（浏览），普通成员（只读）。  
- **核心能力**：搜索、分类筛选、安装状态查看、插件详情、安装/升级操作。

---

## 2. 顶部导航

- 左侧切换标签：`应用市场`（当前页）、`已安装`（跳转 `/plugins/installed`）。  
- 右侧操作：  
  - `安装`：仅 Root 用户可见，触发 `InstallDialog`（`app/components/plugins/InstallDialog.vue`）。  
- `刷新`：调用 `refresh()` 重新拉取 `getMarketplace()`。

角色判断通过 `useUserStore().isRoot`（`app/pages/plugins/market.vue:74`），普通用户看不到安装按钮。

---

## 3. 筛选区

- 组成：搜索输入框 + 分类/状态/排序下拉。  
- `q` 支持名称、描述、作者模糊匹配。  
- 分类、状态、排序数据来自本地数组，可与后端约定字段后迁移至 API。  
- 交互：任意筛选项变化都会触发 `computed` 重新计算 `filtered`，并将分页重置到第一页。

---

## 4. 列表与卡片

- 使用 `PluginCard` 组件渲染，每页默认 10 条。卡片展示：名称、描述、版本、作者、标签、安装状态。  
- 状态字段：  
  - `__sys.isSystemInstalled`、`__sys.isSystemEnabled` 由后端返回，控制徽标与操作按钮。  
  - Root 用户可看到 “安装 / 升级” 操作；普通用户仅展示状态。  
- 空态：当 `filtered.length === 0` 时展示提示图标与引导文案。

分页使用 `UPagination`（`app/pages/plugins/market.vue:134`），确保读屏器可识别 `aria` 属性。

---

## 5. 插件安装流程

1. 点击卡片操作或顶部“安装”。  
2. 打开 `InstallDialog`：  
   - 展示插件详情、版本选择、安装说明。  
   - 调用 `adminPluginsService.installPlugin()`（具体逻辑在 `app/composables/api/services/adminPluginsService.ts`）。  
3. 安装成功触发 `@installed`，页面调用 `refresh()`，并显示通知（依赖 `useOneShotAlert`）。  
4. 若安装失败，使用 `normalizeApiError` 显示后端返回的错误信息。

> 需要 Root 权限；后续可增加“提交安装申请”流程供普通用户使用。

---

## 6. 数据来源

- `useAdminPluginsService().getMarketplace()` 拉取后端数据，映射为统一结构（`app/pages/plugins/market.vue:90`）。  
- 如果后端暂不可用，可考虑在服务层提供 Mock（参考 `docs/environment/Local_Mocks_and_Fixtures.md`），确保 UI 可回归。

---

## 7. 权限差异

| 角色 | 权限 | 行为 |
| --- | --- | --- |
| Root 管理员 | `plugin.market.install` | 可安装/启停/升级插件；看到所有操作按钮。 |
| 租户管理员 | `plugin.market.view` | 仅浏览与查看细节，不可安装。 |
| 普通成员 | `plugin.market.view` | 只读，无法看到安装入口。 |

建议后端在返回数据中附带 `allowedActions`，前端根据列表动态展示操作。

---

## 8. 验收 checklist

- [ ] 搜索、筛选、排序联动正确，分页统计与实际显示数量一致。  
- [ ] Root 用户可完成安装流程，普通用户无法触发。  
- [ ] 失败时展示规范的错误提示并保留弹窗。  
- [ ] 切换语言后菜单/筛选文本正确翻译。  
- [ ] 切换主题时卡片背景、文字对比度达标。  
- [ ] 安装成功后刷新列表，状态实时更新。

---

## 9. 未来扩展

- 插件详情页：展示版本历史、权限、所需凭证等。  
- 批量操作：支持批量启停或批量安装离线包。  
- 审批流：普通用户发起安装申请，管理员审核后自动触发安装。  
- 运行监控：显示插件健康状态、错误日志、更新提醒。
