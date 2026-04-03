---
name: nuxt-pages
description: PowerX Nuxt CRUD 页面与主题规范。
---

# PowerX Nuxt Pages

## 步骤

1) 打开 `本文件内嵌规则`。
2) 按规则执行实现/校对。
3) 完成后按核对清单验收。

## 核对点

- 与 PowerX 当前代码结构、路径与命名一致。
- 仅在传输层/契约层做职责内改动，不跨层越界。

## 规则（内嵌）

### nuxt_pages.yaml

````yaml
kind: ruleset
name: plugin/crud/frontend/nuxt_pages
version: 1

pages:
  resource: template
  files:
    - target: web-admin/app/pages/templates/index.vue
      template: builtin/nuxt_page_table
      params:
        title: "Templates"
        columns: ["name","description","updated_at"]
        actions: ["create","edit","delete","view"]
    - target: web-admin/app/pages/templates/[id].vue
      template: builtin/nuxt_page_detail
      params:
        title: "Template Detail"

theme:
  description: |
    浅色模式：使用浅色面板（如 `bg-white/95`）+ 深色前景（不超过 `text-gray-900`），禁止在浅色面板上放白色文字或图标。
    深色模式：完全对齐模板 CRUD 组件，面板使用深海军蓝 (`#0f192a`~`#111a2b`) 并辅以低饱和边框；标题 `#fdfcff`，正文/描述 `#d6e2ff` 附近，不得出现深灰文字。
    图标/按钮：所有状态图标（参考租户配置左侧纵向卡片）在浅色模式用 `text-primary-600`；暗色模式需改为高亮浅色（`#fdfcff` 或 `#93c5fd`），不可保留灰黑图标。
    页面开发必须参照模板 CRUD 组件的色阶搭配（面板、按钮、表格 hover 状态等），保证在明暗主题下对比度一致。

gates.require: [PG-FE-UI-001, PG-FE-RBAC-001]
````
