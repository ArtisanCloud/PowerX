<script setup lang="ts">
// /pages/_p/[pluginId]/admin/index.vue
import PluginWebView from "@/components/PluginWebView.vue"

// 这个页面就是“根 Dashboard”，iframe 通过 /_p/<id>/admin/ 进入后端插件
const route = useRoute()
const pluginId = computed(() => String(route.params.pluginId || ""))

// iframe 基础入口固定为 admin 根路径，避免同插件内跳转时反复重载 iframe
const src = computed(() => `/_p/${pluginId.value}/admin/?__px_iframe=1`)
const navigatePath = computed(() => `/_p/${pluginId.value}/admin/`)

</script>

<template>
  <PluginWebView :plugin-id="pluginId" :src="src" :navigate-path="navigatePath" />
</template>
