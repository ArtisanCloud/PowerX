<template>
  <section class="mx-auto max-w-7xl space-y-5 p-4 text-gray-900 sm:p-6 dark:text-gray-100" data-testid="storage-workspace">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div><h1 class="text-2xl font-semibold">{{ t('storageGov.title') }}</h1><p class="mt-2 text-sm text-muted">{{ t('storageGov.subtitle') }}</p></div>
      <UButton v-if="allowed" icon="i-heroicons-arrow-path" color="neutral" variant="outline" :loading="loading" @click="load">{{ t('storageGov.refresh') }}</UButton>
    </div>
    <UAlert v-if="!allowed" color="warning" :title="t('storageGov.rootOnly')" />
    <template v-else>
      <div class="flex flex-wrap items-center gap-3 text-sm"><span class="rounded-md border border-default px-3 py-1.5">{{ t(overview?.cleanup_enabled ? 'storageGov.runtime.activeBadge' : 'storageGov.observeOnly') }}</span><span v-if="overview?.identity.environment" class="text-muted">{{ t('storageGov.context.currentInstall') }} {{ scopeLabel(overview.identity.environment) }} · {{ overview.identity.instance_id?.slice(0, 10) }}</span><UButton size="sm" color="neutral" variant="link" @click="scopeInfoOpen = true">{{ t('storageGov.context.observationScope') }}</UButton></div>
      <UAlert v-if="stream.status.value === 'disconnected'" color="warning" :title="t('storageGov.runtime.disconnected')" />
      <UAlert v-if="error" color="error" :title="t('storageGov.loadFailed')" :description="error" data-testid="storage-error" />
      <UAlert v-if="stale" color="warning" :title="t('storageGov.stale')" />
      <UAlert v-if="overview?.filesystem.state === 'unavailable'" color="warning" :title="t('storageGov.filesystemUnavailable')" />
      <UAlert v-if="overview?.sampling?.configured && !overview.sampling.worker_enabled" color="warning" :title="t('storageGov.samples.workerDisabled')" />
      <UAlert v-if="capacityRisk" color="warning" variant="subtle" :title="t('storageGov.risks.title')"><template #actions><UButton color="neutral" variant="outline" @click="section = 'alerts'">{{ t('storageGov.alerts') }}</UButton></template></UAlert>
      <p v-if="overview" class="text-sm text-muted">{{ t('storageGov.observedAt') }} {{ new Date(overview.observed_at).toLocaleString() }} · {{ t('storageGov.cacheHint') }}</p>
      <BackupSectionTabs v-model="section" :items="sections" :label="t('storageGov.sections')">
        <template #overview>
          <div class="space-y-5" data-testid="storage-overview">
            <h2 class="text-lg font-semibold">{{ t('storageGov.overview') }}</h2>
            <div class="flex flex-wrap gap-2" role="group" :aria-label="t('storageGov.usageViews.choose')">
              <UButton v-for="view in ['powerx', 'disk']" :key="view" :color="overviewView === view ? 'primary' : 'neutral'" :variant="overviewView === view ? 'solid' : 'outline'" :aria-pressed="overviewView === view" :data-testid="`storage-overview-${view}`" @click="overviewView = view">{{ t(`storageGov.usageViews.${view}`) }}</UButton>
            </div>
            <p v-if="loading && !overview" role="status">{{ t('storageGov.loading') }}</p>
            <div v-if="overviewView === 'powerx'" class="space-y-4" data-testid="storage-powerx-summary">
              <div class="rounded-lg border border-default p-4"><h3 class="font-medium">{{ t('storageGov.usageViews.powerxTotal') }}：{{ t('storageGov.usageViews.incomplete') }}</h3><p class="mt-2 text-sm text-muted">{{ t('storageGov.usageViews.powerxHint') }}</p></div>
              <UButton v-if="overview?.database" color="neutral" variant="outline" data-testid="storage-database-open" @click="databaseOpen = true">{{ t('storageGov.database.total') }} · {{ formatBytes(overview.database.database_bytes) }} · {{ t('storageGov.database.open') }}</UButton>
            </div>
            <p v-else class="text-sm text-muted" data-testid="storage-disk-scope">{{ t('storageGov.usageViews.diskHint') }}</p>
            <div v-if="overviewView === 'disk'" class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4" data-testid="storage-disk-metrics">
              <UCard v-for="metric in metrics" :key="metric.key"><p class="text-sm text-muted">{{ metric.label }}</p><p class="mt-2 text-xl font-semibold tabular-nums">{{ formatBytes(metric.value) }}</p></UCard>
            </div>
            <div class="flex flex-wrap items-center justify-between gap-3"><h3 class="font-medium">{{ t(overviewView === 'powerx' ? 'storageGov.usageViews.directories' : 'storageGov.usageViews.sharedSources') }}</h3><label v-if="overviewView === 'powerx'" class="flex items-center gap-2 text-sm">{{ t('storageGov.scope') }}<select v-model="scope" class="min-h-10 rounded-md border border-gray-300 bg-white px-3 dark:border-gray-700 dark:bg-gray-900" data-testid="storage-scope"><option v-for="item in scopeOptions" :key="item.value" :value="item.value">{{ item.label }}</option></select></label></div>
            <div class="overflow-x-auto">
              <table class="w-full text-left text-sm" data-testid="storage-resources">
                <thead><tr class="border-b border-gray-200 text-muted dark:border-gray-700"><th class="p-2">{{ t('storageGov.resource') }}</th><th class="p-2">{{ t('storageGov.scope') }}</th><th class="p-2">{{ t('storageGov.usageViews.measured') }}</th><th class="p-2">{{ t('storageGov.state') }}</th></tr></thead>
                <tbody><tr v-for="resource in visibleResources" :key="resource.id" class="border-b border-gray-200 dark:border-gray-700">
                  <td class="max-w-52 break-words p-2">{{ resource.name }}</td><td class="p-2">{{ scopeLabel(resource.scope) }}</td>
                  <td class="whitespace-nowrap p-2 tabular-nums">
                    <template v-if="resource.state === 'partial' && (resource.partial_bytes || 0) > 0"><span>{{ t('storageGov.usageViews.scanned', { bytes: formatBytes(resource.partial_bytes) }) }}</span><p class="mt-1 text-xs text-muted">{{ t('storageGov.usageViews.partialHint') }}</p></template>
                    <template v-else>{{ formatBytes(resource.attributed_bytes) }}</template>
                  </td>
                  <td class="p-2"><span>{{ t(`storageGov.states.${resource.state}`) }}</span><p v-if="resource.problem" class="mt-1 text-xs text-muted">{{ t(`storageGov.problems.${resource.problem}`) }}</p></td>
                </tr></tbody>
              </table>
            </div>
            <p v-if="!visibleResources.length" class="py-6 text-center text-muted">{{ t('storageGov.noResources') }}</p>
            <p v-if="overviewView === 'disk'" class="text-sm text-muted">{{ t('storageGov.unattributed') }}：{{ formatBytes(overview?.unattributed_bytes) }}。{{ t('storageGov.usageViews.unclassifiedHint') }}</p>
            <details class="text-sm text-muted"><summary class="cursor-pointer">{{ t('storageGov.usageViews.measurementDetails') }}</summary><p class="mt-2">{{ t('storageGov.allocationHint') }}</p><p class="mt-2">{{ t('storageGov.processView') }}</p></details>
          </div>
        </template>
        <template #policies>
          <div class="space-y-4" data-testid="storage-policies">
            <h2 class="text-lg font-semibold">{{ t('storageGov.policies') }}</h2>
            <StoragePolicyDraftPanel v-if="draftsVisited" :key="identityKey" :resources="overview?.resources || []" :filesystem="overview?.filesystem" @cleanup-changed="cleanupRevision++; load()" />
          </div>
        </template>
        <template #databaseRetention><DatabaseRetentionPanel v-if="databaseRetentionVisited" :key="identityKey" :revision="runtimeRevision" /></template>
        <template #archives><ArchiveGovernancePanel v-if="archivesVisited" :key="identityKey" /></template>
        <template #automation><StorageRuntimePanel v-if="runtimeVisited" :key="identityKey" :revision="runtimeRevision" /></template>
        <template #history><StorageHistoryPanel v-if="historyVisited" :key="identityKey" /></template>
        <template #cleanup><StorageCleanupRecords v-if="cleanupVisited" :key="identityKey" :revision="cleanupRevision" /></template>
        <template #alerts><div class="space-y-4"><h2 class="text-lg font-semibold">{{ t('storageGov.alerts') }}</h2><UAlert color="info" :title="t('storageGov.risks.currentOnly')" /><ul class="space-y-3"><li v-for="(alert, i) in overview?.alerts || []" :key="i" class="rounded border border-default p-3"><p>{{ t(`storageGov.risks.${alert.code}`) }} <span v-if="alert.resource_id">({{ alert.resource_id }})</span></p></li></ul><p class="text-sm text-muted">{{ t('storageGov.unknownHint') }}</p></div></template>
      </BackupSectionTabs>
      <UModal v-model:open="scopeInfoOpen" :title="t('storageGov.context.observationScope')" :description="t('storageGov.processView')"><template #body><p class="text-sm">{{ t('storageGov.partialCoverage') }}</p><ul v-if="overview?.inventory?.length" class="mt-4 space-y-3" data-testid="storage-inventory"><li v-for="item in overview.inventory" :key="item.kind" class="flex justify-between gap-4 border-b border-default pb-2"><span>{{ t(`storageGov.kinds.${item.kind}`) }}</span><span :class="item.state === 'not_connected' ? 'text-warning' : 'text-muted'">{{ t(`storageGov.inventory.${item.state}`) }}</span></li></ul></template></UModal>
      <UModal v-model:open="databaseOpen" :title="t('storageGov.database.open')" :description="t('storageGov.database.hint')" :ui="{ content: 'max-w-3xl' }">
        <template #body><div v-if="overview?.database" class="space-y-4" data-testid="storage-database">
          <dl class="grid grid-cols-2 gap-3 text-sm"><dt>{{ t('storageGov.database.total') }}</dt><dd>{{ formatBytes(overview.database.database_bytes) }}</dd><dt>{{ t('storageGov.database.wal') }}</dt><dd>{{ formatBytes(overview.database.wal_bytes) }}</dd><dt>{{ t('storageGov.database.reusable') }}</dt><dd>{{ formatBytes(overview.database.reusable_bytes) }}</dd></dl>
          <div class="overflow-x-auto"><table class="w-full min-w-[30rem] text-left text-sm"><thead><tr><th class="p-2">{{ t('storageGov.resource') }}</th><th>{{ t('storageGov.database.heap') }}</th><th>{{ t('storageGov.database.indexes') }}</th><th>TOAST</th><th>{{ t('storageGov.usageViews.relationTotal') }}</th></tr></thead><tbody><tr v-for="r in overview.database.relations" :key="r.id"><td class="p-2">{{ r.id }}<p v-if="r.state !== 'observed'" class="text-warning">{{ t('storageGov.states.unavailable') }}</p></td><td>{{ formatBytes(r.heap_bytes) }}</td><td>{{ formatBytes(r.indexes_bytes) }}</td><td>{{ formatBytes(r.toast_bytes) }}</td><td>{{ formatBytes(r.total_bytes) }}</td></tr></tbody></table></div>
        </div></template>
      </UModal>
    </template>
  </section>
</template>

<script setup lang="ts">
import StorageRuntimePanel from './StorageRuntimePanel.vue';
import DatabaseRetentionPanel from './DatabaseRetentionPanel.vue';
import ArchiveGovernancePanel from '~/components/ops/backup/ArchiveGovernancePanel.vue';
import { useStorageGovernanceEvents } from '~/composables/useStorageGovernanceEvents';
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import BackupSectionTabs from '~/components/ops/backup/BackupSectionTabs.vue';
import StoragePolicyDraftPanel from './StoragePolicyDraftPanel.vue';
import StorageHistoryPanel from './StorageHistoryPanel.vue';
import StorageCleanupRecords from './StorageCleanupRecords.vue';
import { formatStorageBytes as formatBytes } from '~/utils/storage-capacity';
import { useStorageGovernanceService, type StorageOverview } from '~/composables/api/services/storageGovernanceService';

const props = defineProps<{ allowed: boolean; identityKey: string }>();
const { t } = useI18n();
const api = useStorageGovernanceService();
const section = ref('overview'), scope = ref('all'), overviewView = ref('powerx');
const archivesVisited = ref(false), databaseRetentionVisited = ref(false);
const draftsVisited = ref(false), historyVisited = ref(false), databaseOpen = ref(false), scopeInfoOpen = ref(false);
const cleanupVisited = ref(false), cleanupRevision = ref(0), runtimeVisited = ref(false), runtimeRevision = ref(0);
const overview = ref<StorageOverview | null>(null);
const loading = ref(false), error = ref('');
const stale = ref(false);
let controller: AbortController | null = null, generation = 0;
let expiryTimer: ReturnType<typeof setTimeout> | undefined;
const sections = computed(() => ['overview', 'policies', 'automation', 'archives', 'databaseRetention', 'history', 'cleanup', 'alerts'].map(value => ({ value, label: t(`storageGov.${value}`) })));
const scopeLabel = (value: string) => ['dev', 'test', 'staging', 'prod', 'shared'].includes(value) ? t(`storageGov.scopes.${value}`) : value;
const scopeOptions = computed(() => [{ value: 'all', label: t('storageGov.allResources') }, ...Array.from(new Set(overview.value?.resources.filter(r => r.scope !== 'shared').map(r => r.scope) || [])).map(value => ({ value, label: scopeLabel(value) }))]);
const visibleResources = computed(() => (overview.value?.resources || []).filter(r => overviewView.value === 'disk' ? r.scope === 'shared' : r.scope !== 'shared' && (scope.value === 'all' || r.scope === scope.value)));
const capacityRisk = computed(() => overview.value?.alerts?.some(a => a.level === 'warning' || a.level === 'critical'));
watch(section, value => { if (value === 'databaseRetention') databaseRetentionVisited.value = true; if (value === 'archives') archivesVisited.value = true; if (value === 'history') historyVisited.value = true; if (value === 'policies') draftsVisited.value = true; if (value === 'cleanup') cleanupVisited.value = true; if (value === 'automation') runtimeVisited.value = true; });
const metrics = computed(() => ['total_bytes', 'used_bytes', 'available_bytes', 'reserved_bytes'].map(key => ({ key, label: t(`storageGov.metrics.${key}`), value: overview.value?.filesystem[key as keyof StorageOverview['filesystem']] as number | null | undefined })));
const stream = useStorageGovernanceEvents(computed(() => props.allowed), signal => api.runtimeEvents(signal), () => { runtimeRevision.value++; void load(); }, computed(() => props.identityKey));
const message = (e: unknown) => e instanceof Error ? e.message : t('storageGov.requestFailed');

async function load() {
  if (!props.allowed) return;
  controller?.abort(); controller = new AbortController();
  const signal = controller.signal, stamp = generation;
  loading.value = true; error.value = '';
  try { const result = await api.overview(signal); if (!signal.aborted && stamp === generation) overview.value = result; }
  catch (e) { if (!signal.aborted && stamp === generation) error.value = message(e); }
  finally { if (!signal.aborted && stamp === generation) loading.value = false; }
}
watch([() => props.allowed, () => props.identityKey], ([allowed, identity], [previousAllowed, previousIdentity]) => {
  if (allowed === previousAllowed && identity === previousIdentity) return;
  generation++; controller?.abort();
  overview.value = null; error.value = ''; loading.value = false; scope.value = 'all'; overviewView.value = 'powerx';
  archivesVisited.value = false; databaseRetentionVisited.value = false; draftsVisited.value = false; historyVisited.value = false; cleanupVisited.value = false; runtimeVisited.value = false; cleanupRevision.value = 0; databaseOpen.value = false; scopeInfoOpen.value = false; section.value = 'overview';
  if (props.allowed) void load();
}, { immediate: true });
onBeforeUnmount(() => { generation++; controller?.abort(); clearTimeout(expiryTimer); });
watch(overview, value => {
  clearTimeout(expiryTimer); stale.value = false;
  if (!value) return;
  const remaining = Date.parse(value.expires_at) - Date.now();
  if (remaining <= 0) stale.value = true;
  else expiryTimer = setTimeout(() => { stale.value = true; }, Math.min(remaining, 2147483647));
});
</script>
