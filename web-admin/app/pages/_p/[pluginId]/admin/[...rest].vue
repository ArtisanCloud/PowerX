<script setup lang="ts">
// /pages/_p/[pluginId]/admin/[...rest].vue
import PluginWebView from "@/components/PluginWebView.vue"

definePageMeta({
  key: (route) => `plugin-admin:${String(route.params.pluginId || "")}`,
})

const route = useRoute()

const pluginId = computed(() => String(route.params.pluginId || ""))
const src = computed(() => `/_p/${pluginId.value}/admin/?__px_iframe=1`)
const navigatePath = computed(() => {
  const full = String(route.fullPath || "")
  return full || `/_p/${pluginId.value}/admin/`
})

</script>

<template>
  <PluginWebView
    :plugin-id="pluginId"
    :src="src"
    :navigate-path="navigatePath"
    :instance-id="route.fullPath"
    constrain-to-viewport
  />
</template>
