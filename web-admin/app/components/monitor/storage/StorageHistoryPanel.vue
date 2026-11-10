<template>
  <div class="space-y-4" data-testid="storage-history-panel">
    <div class="flex flex-wrap items-center justify-between gap-3"><h2 class="text-lg font-semibold">{{ t('storageGov.history') }}</h2><UButton color="neutral" variant="outline" :loading="loading" @click="load">{{ t('storageGov.refresh') }}</UButton></div>
    <div class="flex flex-wrap gap-2" role="group" :aria-label="t('storageGov.usageViews.choose')">
      <UButton v-for="view in ['powerx', 'disk']" :key="view" :color="historyView === view ? 'primary' : 'neutral'" :variant="historyView === view ? 'solid' : 'outline'" :aria-pressed="historyView === view" :data-testid="`storage-history-${view}`" @click="historyView = view">{{ t(`storageGov.usageViews.${view}`) }}</UButton>
    </div>
    <UFormField v-if="sourceOptions.length > 1" :label="t('storageGov.resource')"><select v-model="selectedSource" class="min-h-10 w-full rounded-md border border-default bg-default px-3" data-testid="storage-history-source"><option v-for="source in sourceOptions" :key="source.id" :value="source.id">{{ source.name }}</option></select></UFormField>
    <p class="text-sm text-muted" data-testid="storage-history-scope">{{ t(isResource ? 'storageGov.logs.historyHint' : historyView === 'powerx' ? 'storageGov.usageViews.databaseHistoryHint' : 'storageGov.usageViews.diskHint') }}</p>
    <p v-if="history?.sampling" class="text-sm" data-testid="storage-sampling-status">{{ t(history.sampling.configured ? (history.sampling.worker_enabled ? 'storageGov.samples.configured' : 'storageGov.samples.workerDisabled') : 'storageGov.samples.disabled') }}</p>
    <UAlert v-if="error" color="error" :title="t('storageGov.loadFailed')" :description="error" />
    <p v-if="loading && !history" role="status">{{ t('storageGov.loading') }}</p>
    <template v-if="history">
      <UAlert v-if="history.metric_changes || history.unavailable_samples" color="warning" variant="subtle" :title="t('storageGov.usageViews.historyWarning')" :description="`${t('storageGov.samples.changed')}：${history.metric_changes}；${t('storageGov.samples.unavailable')}：${history.unavailable_samples}`" />
      <p class="text-sm text-muted">{{ t('storageGov.usageViews.historyWindow', { count: history.sample_count }) }}</p>
      <p v-if="!history.points.length" class="text-muted">{{ t('storageGov.samples.empty') }}</p>
      <div v-else class="overflow-x-auto"><table class="w-full text-sm" :class="isResource || historyView === 'disk' ? 'min-w-[28rem]' : 'min-w-[15rem]'" data-testid="storage-history-table"><thead><tr><th class="p-2 text-left">{{ t('storageGov.usageViews.time') }}</th><template v-if="isResource"><th class="p-2">{{ t('storageGov.usageViews.measured') }}</th><th class="p-2">{{ t('storageGov.logs.netChange') }}</th><th class="p-2">{{ t('storageGov.state') }}</th></template><template v-else-if="historyView === 'disk'"><th class="p-2">{{ t('storageGov.metrics.used_bytes') }}</th><th class="p-2">{{ t('storageGov.metrics.available_bytes') }}</th></template><th v-else class="p-2">{{ t('storageGov.database.total') }}</th></tr></thead><tbody><tr v-for="p in history.points.slice(-12).reverse()" :key="p.observed_at"><td class="p-2">{{ new Date(p.observed_at).toLocaleString(locale) }}</td><template v-if="isResource"><td class="p-2 text-center">{{ formatStorageBytes(resourcePoint(p)?.bytes) }}</td><td class="p-2 text-center">{{ delta(resourcePoint(p)?.net_change_bytes) }}</td><td class="p-2">{{ resourcePoint(p) ? t(`storageGov.states.${resourcePoint(p)?.state}`) : t('storageGov.logs.notRecorded') }}<p v-if="resourcePoint(p)?.problem" class="text-xs text-muted">{{ t(`storageGov.problems.${resourcePoint(p)?.problem}`) }}</p></td></template><template v-else-if="historyView === 'disk'"><td class="p-2 text-center">{{ formatStorageBytes(p.used_bytes) }}</td><td class="p-2 text-center">{{ formatStorageBytes(p.available_bytes) }}</td></template><td v-else class="p-2 text-center">{{ formatStorageBytes(p.database_bytes) }}<p v-if="p.database_bytes === null" class="text-xs text-muted">{{ t('storageGov.logs.notRecorded') }}</p></td></tr></tbody></table></div>
      <details class="rounded border border-default p-3 text-sm" data-testid="storage-sampling-details"><summary class="cursor-pointer font-medium">{{ t('storageGov.usageViews.samplingDetails') }}</summary><p class="mt-3 text-muted">{{ t('storageGov.samples.hint') }}</p><dl class="mt-3 grid grid-cols-2 gap-3"><dt>{{ t('storageGov.samples.count') }}</dt><dd>{{ history.sample_count }} / {{ history.expected_samples }}</dd><dt>{{ t('storageGov.samples.missing') }}</dt><dd>{{ history.missing_samples }}</dd><dt>{{ t('storageGov.samples.changed') }}</dt><dd>{{ history.metric_changes }}</dd><dt>{{ t('storageGov.samples.unavailable') }}</dt><dd>{{ history.unavailable_samples }}</dd></dl></details>
    </template>
    <UButton to="/ops/backup" color="neutral" variant="outline">{{ t('storageGov.backupCenter') }}</UButton>
  </div>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useStorageGovernanceService, type StorageHistory } from '~/composables/api/services/storageGovernanceService';
import { formatStorageBytes } from '~/utils/storage-capacity';
const { t, locale } = useI18n(); const api = useStorageGovernanceService();
const historyView = ref('powerx');
const powerxSource = ref('database'), diskSource = ref('filesystem');
const selectedSource = computed({ get: () => historyView.value === 'powerx' ? powerxSource.value : diskSource.value, set: value => { if (historyView.value === 'powerx') powerxSource.value = value; else diskSource.value = value; } });
const isResource = computed(() => selectedSource.value.startsWith('resource:'));
const history = ref<StorageHistory>(); const loading = ref(false); const error = ref(''); let controller: AbortController | undefined;
const sourceOptions = computed(() => [{ id: historyView.value === 'powerx' ? 'database' : 'filesystem', name: t(historyView.value === 'powerx' ? 'storageGov.database.total' : 'storageGov.usageViews.disk') }, ...(history.value?.sources || []).filter(s => historyView.value === 'powerx' ? s.scope !== 'shared' : s.scope === 'shared').map(s => ({ id: `resource:${s.id}`, name: `${s.name} · ${t(`storageGov.scopes.${s.scope}`)}` }))]);
const resourcePoint = (p: StorageHistory['points'][number]) => p.resources?.find(r => r.id === selectedSource.value.slice('resource:'.length));
const delta = (value: number | null | undefined) => value == null ? '—' : `${value > 0 ? '+' : value < 0 ? '−' : ''}${formatStorageBytes(Math.abs(value))}`;
async function load() {
  controller?.abort(); const request = controller = new AbortController(); loading.value = true; error.value = '';
  try { const value = await api.history(request.signal); if (!request.signal.aborted) history.value = value; }
  catch (e) { if (!request.signal.aborted) error.value = e instanceof Error ? e.message : t('storageGov.requestFailed'); }
  finally { if (!request.signal.aborted) loading.value = false; }
}
onMounted(load); onBeforeUnmount(() => controller?.abort());
</script>
