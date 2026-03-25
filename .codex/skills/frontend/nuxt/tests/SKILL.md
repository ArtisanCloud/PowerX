---
name: nuxt-tests
description: PowerX Nuxt Vitest/E2E 测试规范。
---

# PowerX Nuxt Tests

## 步骤

1) 打开 `本文件内嵌规则`。
2) 按规则执行实现/校对。
3) 完成后按核对清单验收。

## 核对点

- 与 PowerX 当前代码结构、路径与命名一致。
- 仅在传输层/契约层做职责内改动，不跨层越界。

## 规则（内嵌）

### nuxt_tests.yaml

````yaml
kind: ruleset
name: plugin/crud/frontend/nuxt_tests
version: 1

tests:
  runner: vitest
  files:
    - target: web-admin/tests/templates.spec.ts
      template: builtin/nuxt_test_table_page
      params:
        route: "/templates"
        expect:
          - "renders table"
          - "open create modal"
          - "submit form calls API"

gates.require: [PG-FE-UI-001]
````
