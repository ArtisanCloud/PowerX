<script setup lang="ts">
import type { SetupConnectionKind } from '~/utils/setup-connection-test';

const props = defineProps<{
  kind: SetupConnectionKind;
  testing: boolean;
  ok: boolean;
  message: string;
  diagnosticId?: string;
}>();
const { t } = useI18n();
const title = computed(() => t(`setup.connection.${props.kind}.${props.testing ? 'testing' : props.ok ? 'success' : 'failed'}`));
const description = computed(() => props.testing
  ? props.message ? t('setup.connection.previousResult', { message: props.message }) : t('setup.connection.testingDetail')
  : props.message);
</script>

<template>
  <div class="min-w-0 space-y-2" aria-live="polite" :data-testid="`setup-${kind}-test-result`">
    <UAlert
      v-if="testing || message"
      variant="soft"
      :role="testing || ok ? 'status' : 'alert'"
      :color="testing ? 'info' : ok ? 'success' : 'error'"
      :icon="testing ? 'i-lucide-loader-circle' : ok ? 'i-lucide-circle-check' : 'i-lucide-circle-alert'"
      :title="title"
      :description="description"
    />
    <p v-else class="text-sm text-muted">{{ t(`setup.connection.${kind}.hint`) }}</p>
    <p v-if="!testing && diagnosticId" class="break-all text-xs text-muted">
      {{ t('setup.connection.diagnosticId', { id: diagnosticId }) }}
    </p>
  </div>
</template>
