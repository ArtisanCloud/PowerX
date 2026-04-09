<script setup lang="ts">
// /pages/_p/[pluginId]/admin/index.vue
import PluginWebView from "@/components/PluginWebView.vue"

// 这个页面就是“根 Dashboard”，iframe 通过 /_p/<id>/admin/ 进入后端插件
const route = useRoute()
const pluginId = computed(() => String(route.params.pluginId || ""))

// 统一用插件前缀 + 尾斜杠
const src = computed(() => `/_p/${pluginId.value}/admin/`)

</script>

<template>
  <PluginWebView :plugin-id="pluginId" :src="src" />
</template>
