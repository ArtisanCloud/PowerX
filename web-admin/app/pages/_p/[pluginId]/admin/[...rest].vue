<script setup lang="ts">
// /pages/_p/[pluginId]/admin/[...rest].vue
import PluginWebView from "@/components/PluginWebView.vue"

definePageMeta({
  key: (route) => `plugin-admin:${String(route.params.pluginId || "")}`,
})

const route = useRoute()

const pluginId = computed(() => String(route.params.pluginId || ""))
const navigatePath = computed(() => {
  const prefix = `/_p/${pluginId.value}/admin`
  const full = String(route.fullPath || "")
  const target = full.startsWith(prefix)
    ? full.slice(prefix.length)
    : full
  return target && target !== "/" ? target : "/"
})
// Capture the deep link once. Subsequent child routes use the checked bridge.
const initialPath = navigatePath.value;
const src = computed(() => {
  const url = new URL(`/_p/${pluginId.value}/admin${initialPath}`, 'http://localhost');
  url.searchParams.set('__px_iframe', '1');
  return url.pathname + url.search + url.hash;
})

</script>

<template>
  <PluginWebView
    :plugin-id="pluginId"
    :src="src"
    :navigate-path="navigatePath"
    constrain-to-viewport
  />
</template>
