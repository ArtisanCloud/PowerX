<script setup lang="ts">
const props = defineProps<{ pluginId: string; currentVersion?: string }>()
const emit = defineEmits<{ close: [string | null] }>()

const loading = ref(true)
const error = ref<string>('')
const versions = ref<string[]>([])
const selected = ref<string>('')
const enableAfter = ref<boolean>(true)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const { useAdminPluginsService } = await import('~/composables/api/services/adminPluginsService')
    const svc = useAdminPluginsService()
    const s: any = await svc.status(props.pluginId)
    // 尝试多种字段名
    const list = s?.availableVersions || s?.versions || s?.releases || []
    versions.value = Array.isArray(list) ? list.map((v: any) => String(v.version || v)) : []
  } catch (e: any) {
    error.value = e?.message || '加载版本列表失败'
  } finally {
    loading.value = false
  }
  // 默认选中当前或第一项
  selected.value = props.currentVersion || versions.value[0] || ''
}

onMounted(load)

function onDismiss() { emit('close', null) }
function onConfirm() { emit('close', selected.value || null) }
</script>

<template>
  <UModal :title="`切换版本`" :description="`插件：${pluginId}`" :close="{ onClick: onDismiss }">
    <template #content>
      <div class="space-y-4">
        <UCard :ui="{ body: { padding: 'p-4 sm:p-5' }, footer: { padding: 'p-3 sm:p-4' } }">
          <div class="space-y-3">
            <div v-if="loading" class="text-sm text-[var(--text-secondary)]">加载版本中…</div>
            <div v-else-if="error" class="text-sm text-red-600">{{ error }}</div>
            <template v-else>
              <div class="text-sm text-[var(--text-secondary)]">当前版本：<b>{{ currentVersion || '-' }}</b></div>
              <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 items-end">
                <div>
                  <label class="block text-xs text-[var(--text-secondary)] mb-1">选择版本</label>
                  <USelect v-if="versions.length" v-model="selected" :items="versions" />
                  <UInput v-else v-model="selected" placeholder="手动输入版本号，如 0.1.0" />
                </div>
                <div class="flex items-center gap-2">
                  <USwitch v-model="enableAfter" />
                  <span class="text-sm text-[var(--text-secondary)]">切换后立即启用</span>
                </div>
              </div>
            </template>
          </div>

          <template #footer>
            <div class="flex justify-end gap-2">
              <UButton color="neutral" variant="subtle" @click="onDismiss">取消</UButton>
              <UButton :disabled="!selected" color="primary" @click="onConfirm">切换</UButton>
            </div>
          </template>
        </UCard>
      </div>
    </template>
  </UModal>
</template>

