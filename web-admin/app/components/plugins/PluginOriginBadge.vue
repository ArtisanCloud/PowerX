<template>
  <UBadge :color="kind === 'unknown' ? 'warning' : 'neutral'" variant="subtle" class="inline-flex shrink-0 items-center gap-1.5 whitespace-nowrap" :title="origin?.source_id">
    <UIcon :name="kind === 'marketplace' ? 'i-heroicons-building-storefront' : kind === 'local' ? 'i-heroicons-computer-desktop' : 'i-heroicons-question-mark-circle'" class="size-3.5 shrink-0" />
    {{ t(`plugins.presentation.origin.${kind}`) }}
  </UBadge>
</template>

<script setup lang="ts">
import type { PluginInstallOrigin } from '~/composables/api/services/adminPluginsService';
const props = defineProps<{ origin?: PluginInstallOrigin }>();
const { t } = useI18n();
const kind = computed(() => props.origin?.type || 'unknown');
</script>
