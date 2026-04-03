---
name: nuxt-api-client
description: PowerX Nuxt API Client 规则。
---

# PowerX Nuxt API Client

## 步骤

1) 打开 `本文件内嵌规则`。
2) 按规则执行实现/校对。
3) 完成后按核对清单验收。

## 核对点

- 与 PowerX 当前代码结构、路径与命名一致。
- 仅在传输层/契约层做职责内改动，不跨层越界。

## 规则（内嵌）

### nuxt_api_client.yaml

````yaml
kind: ruleset
name: plugin/crud/frontend/nuxt_api_client
version: 1

client:
  runtime_config:
    public_keys:
      apiBase: "NUXT_PUBLIC_API_BASE"
      pluginId: "NUXT_PUBLIC_PLUGIN_ID"
      powerxProxy: "NUXT_PUBLIC_POWERX_PROXY"
    defaults:
      apiBase: "/_p/<plugin-id>/api/v1"
      powerxProxy: 1
  # 约定：前端服务统一放置在 web-admin/app/composables/api/services 下
  files:
    - target: web-admin/plugins/api.ts
      template: builtin/nuxt_api_plugin_fetch
    - target: web-admin/composables/useTemplates.ts
      template: builtin/nuxt_composable_crud
      params:
        resource: "templates"
        dto:
          item: "TemplateItem"
          create: "TemplateCreateReq"
          update: "TemplateUpdateReq"

gates.require: [PG-FE-API-001]
````
