<template>
  <div class="space-y-4" data-testid="storage-log-cleanup">
    <div v-if="!compact" class="rounded-lg border border-default p-4">
      <h3 class="font-semibold">{{ source.resource.name }}</h3>
      <p class="mt-2">{{ t('storageGov.context.currentUsage') }} {{ formatBytes(usage?.attributed_bytes) }}</p>
      <p v-if="source.cleanup?.native_managed" class="mt-2">{{ t('storageGov.runtime.nativeManaged') }}</p>
      <p v-else class="mt-2">{{ t('storageGov.logs.existing', { days: source.cleanup?.retention_days || t('storageGov.logs.unset'), count: source.cleanup?.retention_count || t('storageGov.logs.unset') }) }}</p>
      <p v-if="!source.cleanup?.native_managed" class="mt-2 text-sm text-muted">{{ t('storageGov.logs.manualOnly') }}</p>
    </div>
    <UAlert v-if="error" color="error" :title="t('storageGov.requestFailed')" :description="error" />
    <UForm v-if="!compact" :schema="schema" :state="form" class="space-y-4" data-testid="storage-log-form" @submit="preview">
      <UFormField :label="t('storageGov.logs.days')" name="days"><UInput v-model="form.days" type="number" min="1" max="3650" :disabled="busy" class="w-full max-w-xs" data-testid="storage-log-days" /></UFormField>
      <p class="text-sm text-muted">{{ t('storageGov.logs.protection') }}</p>
      <UButton type="submit" :loading="busy" data-testid="storage-log-preview">{{ t('storageGov.logs.preview') }}</UButton>
    </UForm>
    <div v-if="compact" class="space-y-2 border-t border-default pt-4">
      <p class="text-xs text-muted">{{ t('storageGov.resourcePolicy.manualHint') }}</p>
      <UButton color="neutral" variant="outline" :disabled="busy" data-testid="storage-log-open" @click="openManual">{{ t(needsReview ? 'storageGov.resourcePolicy.reviewResult' : 'storageGov.resourcePolicy.manualCheck') }}</UButton>
      <p v-if="needsReview" class="text-sm text-warning">{{ t('storageGov.logs.unconfirmed') }}</p>
    </div>
    <UModal v-model:open="open" :dismissible="!busy" :close="!busy" :title="t(inputMode ? 'storageGov.resourcePolicy.manualCheck' : 'storageGov.logs.preview')" :description="compact ? `${source.resource.name} · ${t('storageGov.logs.batch')}` : t('storageGov.logs.batch')" :ui="{ content: 'max-w-2xl' }">
      <template #body>
        <UForm v-if="compact && inputMode" :schema="schema" :state="form" class="space-y-4" data-testid="storage-log-form" @submit="preview">
          <UFormField :label="t('storageGov.logs.days')" name="days"><UInput v-model="form.days" type="number" min="1" max="3650" :disabled="busy" class="w-full" data-testid="storage-log-days" /></UFormField>
          <p class="text-sm text-muted">{{ t('storageGov.resourcePolicy.manualHint') }}</p><p class="text-sm text-muted">{{ t('storageGov.logs.protection') }}</p>
          <UAlert v-if="error" color="error" :title="t('storageGov.requestFailed')" :description="error" />
          <UButton type="submit" :loading="busy" data-testid="storage-log-preview">{{ t('storageGov.logs.preview') }}</UButton>
        </UForm>
        <div v-else-if="record" class="space-y-4" data-testid="storage-log-plan">
        <p class="font-medium">{{ record.resource.name }} · {{ t(`storageGov.logs.states.${attempted && error ? 'unconfirmed' : record.state}`) }}</p>
        <p>{{ t('storageGov.logs.candidates', { count: record.items.length }) }}</p>
        <p v-if="record.blocked_reason" class="text-warning">{{ t(`storageGov.logs.reasons.${record.blocked_reason}`) }}</p>
        <UAlert v-if="attempted && error" color="error" :title="t('storageGov.logs.checkRecord')" :description="error" />
        <p v-if="attempted && !error && ['completed', 'partial'].includes(record.state)" class="text-success" data-testid="storage-log-result">{{ t('storageGov.logs.result', { count: record.deleted_files, bytes: formatBytes(record.removed_file_bytes) }) }}</p>
        <p v-if="record.state === 'unconfirmed'" class="text-warning">{{ t('storageGov.logs.unconfirmed') }}</p>
        <div class="max-h-64 overflow-auto"><table class="w-full text-sm"><thead><tr><th class="p-2 text-left">{{ t('storageGov.logs.file') }}</th><th class="p-2">{{ t('storageGov.logs.outcome') }}</th></tr></thead><tbody><tr v-for="item in record.items" :key="item.name"><td class="break-all p-2">{{ item.name }} · {{ formatBytes(item.bytes) }}</td><td class="p-2">{{ t(`storageGov.logs.outcomes.${item.outcome}`) }}</td></tr></tbody></table></div>
        <details><summary class="cursor-pointer text-sm">{{ t('storageGov.logs.skipped') }}</summary><ul class="mt-2 space-y-1 text-sm text-muted"><li v-for="(count, reason) in record.skipped" :key="reason">{{ t(`storageGov.logs.reasons.${reason}`) }}：{{ count }}</li></ul></details>
        <p class="text-sm text-muted">{{ t('storageGov.logs.reclaimedHint') }}</p>
        <div class="flex flex-wrap gap-3"><UButton v-if="needsReview" :loading="busy" data-testid="storage-log-review" @click="reviewResult">{{ t('storageGov.resourcePolicy.reviewResult') }}</UButton><UButton v-if="record.can_execute && !attempted" color="error" :loading="busy" data-testid="storage-log-execute" @click="execute">{{ t('storageGov.logs.execute') }}</UButton><UButton color="neutral" variant="outline" :disabled="busy" @click="open = false">{{ t('storageGov.logs.close') }}</UButton></div>
      </div></template>
    </UModal>
  </div>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import * as v from 'valibot';
import { useStorageGovernanceService, type StoragePolicyOption, type StorageCleanupRecord, type StorageUsage } from '~/composables/api/services/storageGovernanceService';
import { formatStorageBytes as formatBytes } from '~/utils/storage-capacity';
const props = defineProps<{ source: StoragePolicyOption; usage?: StorageUsage; compact?: boolean }>();
const emit = defineEmits<{ busy: [value: boolean]; changed: []; review: [value: { sourceId: string; pending: boolean }] }>();
const { t } = useI18n(); const api = useStorageGovernanceService();
const form = reactive({ days: (props.source.cleanup?.retention_days || props.source.preset.target_days || 7) as string | number });
const schema = computed(() => v.object({ days: v.pipe(v.union([v.string(), v.number()]), v.transform(Number), v.check(n => Number.isInteger(n) && n >= 1 && n <= 3650, t('storageGov.drafts.invalid'))) }));
const busy = ref(false), open = ref(false), attempted = ref(false), error = ref(''); const record = ref<StorageCleanupRecord>();
const inputMode = ref(false);
const needsReview = computed(() => !!record.value && (record.value.state === 'unconfirmed' || (attempted.value && !!error.value)));
watch(needsReview, pending => emit('review', { sourceId: props.source.resource.id, pending }));
function openManual() { inputMode.value = !needsReview.value; open.value = true; }
let controller: AbortController | undefined;
async function reviewResult() {
  if (busy.value || !record.value) return;
  const id = record.value.id, request = controller = new AbortController(); setBusy(true);
  try { const value = await api.cleanupRun(id, request.signal); if (value.id !== id) throw new Error(t('storageGov.logs.checkRecord')); if (!request.signal.aborted) { record.value = value; if (['completed', 'partial'].includes(value.state)) error.value = ''; emit('changed'); } }
  catch (e) { if (!request.signal.aborted) error.value = e instanceof Error ? e.message : t('storageGov.logs.checkRecord'); }
  finally { if (!request.signal.aborted) setBusy(false); }
}
function setBusy(value: boolean) { busy.value = value; emit('busy', value); }
async function preview() {
  if (busy.value) return;
  if (needsReview.value) { inputMode.value = false; open.value = true; return; }
  if (!v.safeParse(schema.value, form).success) return;
  controller?.abort(); const request = controller = new AbortController(); setBusy(true); error.value = ''; attempted.value = false;
  try { const result = await api.previewCleanup(props.source.resource.id, Number(form.days), request.signal); if (!request.signal.aborted) { record.value = result; inputMode.value = false; open.value = true; } }
  catch (e) { if (!request.signal.aborted) error.value = e instanceof Error ? e.message : t('storageGov.requestFailed'); }
  finally { if (!request.signal.aborted) setBusy(false); }
}
async function execute() {
  if (busy.value || attempted.value || !record.value?.can_execute) return;
  const id = record.value.id; const request = controller = new AbortController(); setBusy(true); attempted.value = true; error.value = '';
  try {
    await api.executeCleanup(id, request.signal);
    const readback = await api.cleanupRun(id, request.signal);
    if (!request.signal.aborted) { if (readback.id !== id || !['completed', 'partial', 'unconfirmed'].includes(readback.state)) throw new Error(t('storageGov.logs.checkRecord')); record.value = readback; emit('changed'); }
  } catch (e) {
    if (!request.signal.aborted) {
      error.value = e instanceof Error ? e.message : t('storageGov.logs.checkRecord');
      try { const readback = await api.cleanupRun(id, request.signal); if (!request.signal.aborted && readback.id === id) { record.value = readback; if (['completed', 'partial'].includes(readback.state)) error.value = ''; } } catch { /* 保留错误与批次上下文，不重发删除。 */ }
      emit('changed');
    }
  } finally { if (!request.signal.aborted) setBusy(false); }
}
onBeforeUnmount(() => { controller?.abort(); emit('busy', false); });
</script>
