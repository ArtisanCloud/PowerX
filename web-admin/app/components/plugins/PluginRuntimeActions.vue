<template>
  <div class="flex shrink-0 flex-wrap items-center gap-1.5">
    <UButton v-if="mode !== 'read'" size="sm" :color="enabled ? 'neutral' : 'primary'" :variant="enabled ? 'ghost' : 'soft'" :icon="enabled ? 'i-heroicons-pause' : 'i-heroicons-play'" :loading="busy" @click="emit('toggle')">
      {{ t(mode === 'tenant' ? 'plugins.presentation.unsubscribe' : enabled ? 'plugins.presentation.disable' : 'plugins.presentation.enable') }}
    </UButton>
    <UButton :to="`/plugins/${pluginId}`" size="sm" color="neutral" variant="outline" :disabled="busy" trailing-icon="i-heroicons-chevron-right">{{ t('plugins.sources.manage') }}</UButton>
  </div>
</template>

<script setup lang="ts">
defineProps<{ pluginId: string; enabled: boolean; mode: 'system' | 'tenant' | 'read'; busy: boolean }>();
const emit = defineEmits<{ toggle: [] }>();
const { t } = useI18n();
</script>
