<script setup lang="ts">
const props = defineProps<{
  title?: string
  description?: string
  placeholder?: string
  defaultValue?: string
  confirmLabel?: string
  cancelLabel?: string
  multiline?: boolean
  rows?: number
}>()

const emit = defineEmits<{ close: [string | null] }>()
const value = ref(props.defaultValue || '')
const onDismiss = () => emit('close', null)
const onConfirm = () => emit('close', value.value || '')

const safeDescription = computed(() => {
  const d = props.description ?? ''
  return typeof d === 'string' ? d.replace(/\n+/g, ' ').trim() : ''
})

const resolvedRows = computed(() => {
  const r = Number(props.rows || 0)
  return r > 0 ? r : 3
})
</script>

<template>
  <UModal :title="props.title || '请输入'" :description="safeDescription || '请输入内容'" :close="{ onClick: onDismiss }">
    <template #content>
      <UCard :ui="{ body: { padding: 'p-4 sm:p-5' }, footer: { padding: 'p-3 sm:p-4' } }">
        <div class="space-y-3">
          <UTextarea
            v-if="props.multiline"
            v-model="value"
            :rows="resolvedRows"
            :placeholder="props.placeholder || '输入内容…'"
            class="w-full"
          />
          <UInput
            v-else
            v-model="value"
            :placeholder="props.placeholder || '输入内容…'"
            class="w-full"
            autofocus
            @keyup.enter="onConfirm"
          />
          <div class="text-xs text-[var(--text-tertiary)]">
            {{ props.multiline ? '支持换行编辑；确认后将触发重新生成。' : '按回车可直接确认。' }}
          </div>
        </div>

        <template #footer>
          <div class="flex justify-end gap-2">
            <UButton color="neutral" variant="outline" @click="onDismiss">{{ props.cancelLabel || '取消' }}</UButton>
            <UButton color="primary" @click="onConfirm">{{ props.confirmLabel || '确定' }}</UButton>
          </div>
        </template>
      </UCard>
    </template>
  </UModal>
</template>
