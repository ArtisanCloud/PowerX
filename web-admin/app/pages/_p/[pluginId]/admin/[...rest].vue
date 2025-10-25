<script setup lang="ts">
// /pages/_p/[pluginId]/admin/[...rest].vue
import PluginWebView from "@/components/PluginWebView.vue"

const route = useRoute()

const pluginId = computed(() => String(route.params.pluginId || ""))
const rest = computed(() => {
  const r = route.params.rest
  if (!r) return ""
  return Array.isArray(r) ? r.join("/") : String(r)
})


// iframe 的 src：/_p/<id>/admin/<rest>（注意 admin/ 尾斜杠）
// const src = computed(() => `http://127.0.0.1:8077/_p/${pluginId.value}/admin/${rest.value || ''}`)
const src = computed(() => {
  const base = `/_p/${pluginId.value}/admin/`
  return rest.value ? base + rest.value : base
})

watch(src, (v) => console.log("[PXAdmin][Page:rest] iframe src ->", v), {immediate: true})
</script>

<template>
  <PluginWebView
    :plugin-id="pluginId"
    :src="src"
    :instance-id="route.fullPath"
  />
</template>
