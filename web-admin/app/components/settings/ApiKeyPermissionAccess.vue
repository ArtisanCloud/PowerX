<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from '#imports';
import type { IntegrationGatewayPermissionCatalogItem } from '~/composables/api/services/integrationGatewayApiKeyService';

const props = defineProps<{ permission: IntegrationGatewayPermissionCatalogItem }>();
const { t } = useI18n();
const access = computed(() => {
  const meta = props.permission.meta;
  if (meta?.type === 'capability_service' && meta.api_key_explicit === true) {
    return { key: 'service', endpoint: String(meta.core_internal_endpoint || '') };
  }
  if (meta?.type === 'api' && meta.api_endpoint) {
    return {
      key: 'http',
      endpoint: [meta.http_method, meta.api_endpoint].filter(Boolean).join(' '),
    };
  }
  return null;
});
</script>

<template>
  <div v-if="access" class="mt-1 min-w-0 space-y-1">
    <span
      class="inline-flex rounded border border-default bg-elevated px-1.5 py-0.5 text-xs font-medium text-default"
    >{{ t(`integrationApiKeyAccess.${access.key}`) }}</span>
    <p v-if="access.endpoint" class="break-all font-mono text-[11px] text-muted">
      {{ access.endpoint }}
    </p>
  </div>
</template>
