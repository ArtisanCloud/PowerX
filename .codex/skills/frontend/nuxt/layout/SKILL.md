---
name: nuxt-layout
description: PowerX Nuxt 布局与菜单 RBAC 规范。
---

# PowerX Nuxt Layout

## 步骤

1) 打开 `本文件内嵌规则`。
2) 按规则执行实现/校对。
3) 完成后按核对清单验收。

## 核对点

- 与 PowerX 当前代码结构、路径与命名一致。
- 仅在传输层/契约层做职责内改动，不跨层越界。

## 规则（内嵌）

### nuxt_layout.yaml

````yaml
kind: ruleset
name: plugin/crud/frontend/nuxt_layout
version: 1

layout:
  nav:
    - label: "Templates"
      to: "/templates"
      icon: "i-lucide-file"
      permission: "base:template:read"    # UI 可见性关联 RBAC
  files:
    - target: web-admin/app/app.vue
      template: builtin/nuxt_app_shell_with_nav

gates.require: [PG-FE-RBAC-001]
````
