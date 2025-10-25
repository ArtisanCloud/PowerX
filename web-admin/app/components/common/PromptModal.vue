<script setup lang="ts">
const props = defineProps<{
  title?: string
  description?: string
  placeholder?: string
  defaultValue?: string
  confirmLabel?: string
  cancelLabel?: string
}>()

const emit = defineEmits<{ close: [string | null] }>()
const value = ref(props.defaultValue || '')
const onDismiss = () => emit('close', null)
const onConfirm = () => emit('close', value.value || '')
</script>

<template>
  <UModal :title="props.title || '请输入'" :description="props.description" :close="{ onClick: onDismiss }">
    <template #content>
      <div class="space-y-3">
        <UInput v-model="value" :placeholder="props.placeholder || '输入内容…'" />
        <div class="flex justify-end gap-2 pt-1">
          <UButton color="neutral" variant="subtle" @click="onDismiss">{{ props.cancelLabel || '取消' }}</UButton>
          <UButton color="primary" @click="onConfirm">{{ props.confirmLabel || '确定' }}</UButton>
        </div>
      </div>
    </template>
  </UModal>
</template>

