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
const src = computed(() => {
  const base = `/_p/${pluginId.value}/admin/`
  return rest.value ? base + rest.value : base
})

</script>

<template>
  <PluginWebView
    :plugin-id="pluginId"
    :src="src"
    :instance-id="route.fullPath"
  />
</template>
