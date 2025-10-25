<script setup lang="ts">
const props = defineProps<{ pluginId: string }>()
const loading = ref(true)
const logs = ref<any>(null)
const error = ref<string>('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    const { useAdminPluginsService } = await import('~/composables/api/services/adminPluginsService')
    const svc = useAdminPluginsService()
    logs.value = await svc.logs(props.pluginId, { limit: 200 })
  } catch (e: any) {
    error.value = e?.message || '加载日志失败'
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <UModal :title="`运行日志 - ${pluginId}`" description="最近运行日志">
    <template #content>
      <div class="min-h-[240px] space-y-3">
        <div v-if="loading" class="text-sm text-[var(--text-secondary)]">加载中…</div>
        <div v-else-if="error" class="text-sm text-red-600">{{ error }}</div>
        <div v-else class="bg-black text-green-200 rounded-md p-3 overflow-auto max-h-[50vh] text-xs leading-6 whitespace-pre-wrap">
          <pre v-if="Array.isArray(logs)">{{ logs.map(l => (typeof l === 'string' ? l : JSON.stringify(l))).join('\n') }}</pre>
          <pre v-else-if="typeof logs === 'string'">{{ logs }}</pre>
          <pre v-else>{{ JSON.stringify(logs, null, 2) }}</pre>
        </div>
        <div class="flex justify-end">
          <UButton variant="ghost" icon="i-heroicons-arrow-path" @click="load">刷新</UButton>
        </div>
      </div>
    </template>
  </UModal>
</template>

