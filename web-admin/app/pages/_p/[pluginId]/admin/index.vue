<script setup lang="ts">
// /pages/_p/[pluginId]/admin/index.vue
import PluginWebView from "@/components/PluginWebView.vue"

// 这个页面就是“根 Dashboard”，iframe 直指 /_p/<id>/admin/
const route = useRoute()
const pluginId = computed(() => String(route.params.pluginId || ""))

// 统一用干净前缀 + 尾斜杠
// const src = ref(`/_p/${pluginId.value}/admin/`)
// const src = computed(() => `http://127.0.0.1:8077/_p/${pluginId.value}/admin/`)
const src = computed(() => `/_p/${pluginId.value}/admin/`)

</script>

<template>
  <PluginWebView :src="src" />
</template>
