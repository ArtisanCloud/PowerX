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
const src = computed(() => {
  const target = navigatePath.value || "/"
  const [pathPart, queryPart = ""] = target.split("?")
  const search = new URLSearchParams(queryPart)
  search.set("__px_iframe", "1")
  const normalizedPath = pathPart.startsWith("/") ? pathPart : `/${pathPart}`
  return `/_p/${pluginId.value}/admin${normalizedPath}?${search.toString()}`
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
