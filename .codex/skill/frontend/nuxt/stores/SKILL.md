---
name: nuxt-stores
description: PowerX Nuxt Pinia Store 规范。
---

# PowerX Nuxt Stores

## 步骤

1) 打开 `本文件内嵌规则`。
2) 按规则执行实现/校对。
3) 完成后按核对清单验收。

## 核对点

- 与 PowerX 当前代码结构、路径与命名一致。
- 仅在传输层/契约层做职责内改动，不跨层越界。

## 规则（内嵌）

### nuxt_stores.yaml

````yaml
kind: ruleset
name: plugin/crud/frontend/nuxt_stores
version: 1

stores:
  create_pinia: true
  files:
    - target: web-admin/app/stores/useTemplateStore.ts
      template: builtin/nuxt_pinia_crud
      params:
        resource: "templates"

gates.require: [PG-FE-UI-001]
````
