<template>
  <div class="rounded-xl border border-default bg-elevated/40 p-4" data-testid="archive-policy-control">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div class="min-w-0"><h4 class="font-medium">{{ t('storageGov.policyControl.title') }}</h4><p class="mt-1 text-sm text-muted">{{ t('storageGov.policyControl.' + authority) }}</p></div>
      <UButton v-if="authority === 'legacy'" :disabled="disabled || !configuration?.ready" data-testid="policy-adopt-open" @click="$emit('adopt')">{{ t('storageGov.policyControl.adopt') }}</UButton>
    </div>
    <dl v-if="authority === 'database'" class="mt-3 grid grid-cols-3 gap-2 text-sm">
      <div v-for="(value, label) in revisions" :key="label"><dt class="text-muted">{{ t('storageGov.policyControl.' + label) }}</dt><dd class="mt-1 font-medium tabular-nums">{{ value ?? '—' }}</dd></div>
    </dl>
    <p v-if="authority === 'database' && configuration?.state !== 'applied'" role="status" class="mt-3 text-sm text-warning">{{ t('storageGov.policyControl.pending') }}</p>
    <div v-if="configuration?.problem" class="mt-3 text-sm"><p role="alert" class="text-error">{{ t('storageGov.policyControl.problemHint') }}</p><details class="mt-2 text-muted"><summary class="cursor-pointer">{{ t('storageGov.policyControl.diagnostics') }}</summary><code class="mt-1 block break-all text-xs">{{ configuration.problem }}</code></details></div>
  </div>
</template>
<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { ArchivePolicyConfiguration } from '~/composables/api/services/archiveOpsService';
const props = defineProps<{ configuration?: ArchivePolicyConfiguration; disabled?: boolean }>();
defineEmits<{ adopt: [] }>();
const { t } = useI18n();
const authority = computed(() => ['legacy', 'database'].includes(props.configuration?.authority || '') ? props.configuration!.authority : 'unavailable');
const revisions = computed(() => ({ desired: props.configuration?.desired_revision, applied: props.configuration?.applied_revision, snapshot: props.configuration?.snapshot_revision }));
</script>
