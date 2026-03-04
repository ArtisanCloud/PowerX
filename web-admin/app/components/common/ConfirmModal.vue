<script setup lang="ts">
const props = defineProps<{
  title?: string
  description?: string
  message?: string
  confirmLabel?: string
  cancelLabel?: string
  confirmColor?: 'primary' | 'neutral' | 'red' | 'green' | 'blue' | 'yellow'
  tone?: 'danger' | 'warning' | 'success' | 'info'
  showIcon?: boolean
  open?: boolean
  defaultOpen?: boolean
}>()

const emit = defineEmits<{ close: [boolean]; 'update:open': [boolean] }>()

const onDismiss = () => emit('close', false)
const onConfirm = () => emit('close', true)

const safeDescription = computed(() => {
  const d = props.description ?? props.message ?? ''
  return typeof d === 'string' ? d.replace(/\n+/g, ' ').trim() : ''
})

const toneColor = computed(() => {
  switch (props.tone) {
    case 'danger':
      return { bg: 'bg-red-50 dark:bg-red-950/20', border: 'border-red-200 dark:border-red-800/40', icon: 'text-red-600 dark:text-red-400', badge: 'text-red-700 dark:text-red-300' }
    case 'warning':
      return { bg: 'bg-yellow-50 dark:bg-yellow-950/20', border: 'border-yellow-200 dark:border-yellow-800/40', icon: 'text-yellow-600 dark:text-yellow-400', badge: 'text-yellow-700 dark:text-yellow-300' }
    case 'success':
      return { bg: 'bg-green-50 dark:bg-green-950/20', border: 'border-green-200 dark:border-green-800/40', icon: 'text-green-600 dark:text-green-400', badge: 'text-green-700 dark:text-green-300' }
    case 'info':
      return { bg: 'bg-blue-50 dark:bg-blue-950/20', border: 'border-blue-200 dark:border-blue-800/40', icon: 'text-blue-600 dark:text-blue-400', badge: 'text-blue-700 dark:text-blue-300' }
    default:
      return { bg: 'bg-gray-50 dark:bg-gray-900/20', border: 'border-gray-200 dark:border-gray-800/40', icon: 'text-gray-600 dark:text-gray-300', badge: 'text-gray-700 dark:text-gray-300' }
  }
})

const computedConfirmColor = computed(() => {
  if (props.confirmColor) return props.confirmColor
  switch (props.tone) {
    case 'danger':
      return 'red'
    case 'warning':
      return 'yellow'
    case 'success':
      return 'green'
    case 'info':
      return 'blue'
    default:
      return 'primary'
  }
})

const resolvedDefaultOpen = computed(() => {
  if (props.open !== undefined) return undefined
  if (props.defaultOpen !== undefined) return props.defaultOpen
  return true
})
</script>

<template>
  <UModal
    :title="props.title || '确认操作'"
    :description="safeDescription"
    :close="{ onClick: onDismiss }"
    :open="props.open"
    :default-open="resolvedDefaultOpen"
    @update:open="(value: boolean) => emit('update:open', value)"
  >
    <template #content>
      <UCard
        :ui="{
          base: 'border ' + toneColor.border + ' ' + toneColor.bg,
          body: { padding: 'p-4 sm:p-5' },
          footer: { padding: 'p-3 sm:p-4' }
        }"
      >
        <div class="flex items-start gap-3">
          <span v-if="props.showIcon !== false" class="inline-flex h-9 w-9 items-center justify-center rounded-full bg-white/70 dark:bg-white/5" :class="toneColor.icon">
            <UIcon :name="props.tone === 'danger' ? 'i-heroicons-exclamation-triangle' : props.tone === 'warning' ? 'i-heroicons-exclamation-circle' : props.tone === 'success' ? 'i-heroicons-check-circle' : 'i-heroicons-information-circle'" />
          </span>
          <div class="text-[var(--text-secondary)] whitespace-pre-line leading-relaxed">
            {{ props.message || props.description || '是否继续当前操作？' }}
          </div>
        </div>

        <template #footer>
          <div class="flex justify-end gap-2">
            <UButton v-if="props.cancelLabel !== ''" color="neutral" variant="subtle" @click="onDismiss">{{ props.cancelLabel || '取消' }}</UButton>
            <UButton :color="computedConfirmColor" @click="onConfirm">{{ props.confirmLabel || '确定' }}</UButton>
          </div>
        </template>
      </UCard>
    </template>
  </UModal>
  
</template>
