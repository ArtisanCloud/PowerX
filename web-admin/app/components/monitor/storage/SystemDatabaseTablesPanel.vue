<template>
  <section class="min-w-0 space-y-4" data-testid="system-database-tables">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div><h2 class="text-lg font-semibold">{{ t('storageGov.databaseTables') }}</h2><p class="mt-1 text-sm text-muted">{{ t('storageGov.tables.scope') }}</p></div>
      <UButton icon="i-heroicons-arrow-path" color="neutral" variant="outline" :loading="loading" @click="load">{{ t('storageGov.refresh') }}</UButton>
    </div>
    <UAlert v-if="error" color="error" :title="t('storageGov.loadFailed')" :description="error" data-testid="system-tables-error" />
    <p v-if="loading && !inventory" role="status">{{ t('storageGov.loading') }}</p>
    <template v-if="inventory">
      <TableMonitoringControl :status="inventory.monitoring" @saved="load" />
      <div class="hidden gap-3 sm:grid sm:grid-cols-3">
        <UCard><p class="text-sm text-muted">{{ t('storageGov.tables.measured') }}</p><p class="mt-1 text-xl font-semibold tabular-nums" data-testid="system-tables-total">{{ bytes(knownBytes) }}</p></UCard>
        <UCard><p class="text-sm text-muted">{{ t('storageGov.tables.coverage') }}</p><p class="mt-1 text-xl tabular-nums">{{ measuredCount }} / {{ inventory.items.length }}</p></UCard>
        <UCard><p class="text-sm text-muted">{{ t('storageGov.tables.protectedCount') }}</p><p class="mt-1 text-xl tabular-nums">{{ inventory.items.filter(r => r.category === 'protected').length }}</p></UCard>
      </div>
      <div class="space-y-2 rounded-lg border border-default p-3 text-sm sm:hidden" data-testid="system-tables-mobile-summary">
        <p class="text-xs text-muted">{{ t('storageGov.tables.measured') }}</p><p class="text-lg font-semibold tabular-nums">{{ bytes(knownBytes) }}</p>
        <p class="text-xs text-muted">{{ t('storageGov.tables.coverage') }} {{ measuredCount }} / {{ inventory.items.length }} · {{ t('storageGov.tables.protectedCount') }} {{ inventory.items.filter(r => r.category === 'protected').length }}</p>
      </div>
      <p class="text-xs text-muted">{{ t('storageGov.observedAt') }} {{ date(inventory.observed_at) }} · {{ t('storageGov.tables.cache') }}</p>
      <div class="space-y-3 rounded-xl border border-slate-200/80 bg-slate-50/70 p-3 dark:border-slate-700/60 dark:bg-slate-900/25 sm:p-4">
        <div class="grid min-w-0 gap-3 sm:grid-cols-2 lg:grid-cols-[minmax(0,1fr)_12rem_13rem]">
          <label class="min-w-0 space-y-1.5 text-xs font-medium text-slate-600 dark:text-slate-300 sm:col-span-2 lg:col-span-1"><span>{{ t('storageGov.tables.search') }}</span><UInput v-model="search" size="lg" :aria-label="t('storageGov.tables.search')" :placeholder="t('storageGov.tables.search')" class="w-full min-w-0" data-testid="system-tables-search" /></label>
          <label class="min-w-0 space-y-1.5 text-xs font-medium text-slate-600 dark:text-slate-300"><span>{{ t('storageGov.tables.category') }}</span><select v-model="category" class="min-h-10 w-full rounded-lg border border-default bg-default px-3 text-sm font-normal" data-testid="system-tables-category"><option v-for="value in categories" :key="value" :value="value">{{ t(`storageGov.tables.categories.${value}`) }}</option></select></label>
          <label class="min-w-0 space-y-1.5 text-xs font-medium text-slate-600 dark:text-slate-300"><span>{{ t('storageGov.tables.listing.cleanupStatus') }}</span><select v-model="cleanupState" class="min-h-10 w-full rounded-lg border border-default bg-default px-3 text-sm font-normal" data-testid="system-tables-cleanup-filter"><option value="all">{{ t('storageGov.tables.listing.allStates') }}</option><option v-for="value in cleanupStates" :key="value" :value="value">{{ t(`storageGov.tables.listing.states.${value}`) }}</option></select></label>
        </div>
        <div class="flex min-w-0 flex-wrap items-end justify-between gap-3">
          <div class="flex min-w-0 flex-wrap items-end gap-2"><label class="min-w-0 space-y-1.5 text-xs font-medium text-slate-600 dark:text-slate-300"><span>{{ t('storageGov.tables.listing.sortBy') }}</span><select v-model="sortBy" class="block min-h-10 max-w-full rounded-lg border border-default bg-default px-3 text-sm font-normal" data-testid="system-tables-sort"><option v-for="value in sortFields" :key="value" :value="value">{{ t(`storageGov.tables.listing.sorts.${value}`) }}</option></select></label><UButton color="neutral" variant="soft" class="min-h-10" :aria-label="t('storageGov.tables.listing.reverseSort')" data-testid="system-tables-direction" @click="direction = direction === 'asc' ? 'desc' : 'asc'">{{ t(`storageGov.tables.listing.${direction}`) }}</UButton></div>
          <div class="flex flex-wrap items-center gap-2"><span role="status" aria-live="polite" class="text-sm text-muted">{{ t('storageGov.tables.results', { count: filtered.length }) }}</span><UButton color="neutral" variant="ghost" data-testid="system-tables-reset" @click="resetFilters">{{ t('storageGov.tables.listing.reset') }}</UButton></div>
        </div>
        <p v-if="sortBy === 'retention'" class="text-xs text-muted">{{ t('storageGov.tables.listing.retentionSortHint') }}</p>
        <p class="text-xs text-muted sm:hidden">{{ t('storageGov.tables.swipe') }}</p>
      </div>
      <div class="max-h-[36rem] overflow-auto rounded-xl border border-slate-200/90 bg-white shadow-sm dark:border-slate-700/60 dark:bg-[#101b2d]" data-testid="system-tables-scroll">
        <table class="w-full min-w-[46rem] table-fixed text-left text-sm" data-testid="system-tables-list">
          <colgroup><col class="w-[22%]" /><col class="w-[13%]" /><col class="w-[20%]" /><col class="w-[17%]" /><col class="w-[15%]" /><col class="w-[13%]" /></colgroup>
          <thead class="sticky top-0 z-10 border-b border-slate-200 bg-slate-50/95 text-xs font-semibold text-slate-600 backdrop-blur dark:border-slate-700/70 dark:bg-[#142036]/95 dark:text-slate-300"><tr><th class="px-4 py-3.5">{{ t('storageGov.tables.table') }}</th><th class="px-4 py-3.5">{{ t('storageGov.tables.capacity') }}</th><th class="px-4 py-3.5">{{ t('storageGov.tables.timeRange') }}</th><th class="px-3 py-3.5">{{ t('storageGov.tables.listing.cleanupStatus') }}</th><th class="px-4 py-3.5">{{ t('storageGov.tables.listing.retention') }}</th><th class="px-3 py-3.5 lg:sticky lg:right-0 lg:z-20 lg:border-l lg:border-slate-200 lg:bg-slate-50 dark:lg:border-slate-700/60 dark:lg:bg-[#142036]">{{ t('storageGov.tables.listing.actions') }}</th></tr></thead>
          <tbody class="divide-y divide-slate-100 dark:divide-slate-700/45"><tr v-for="row in pageRows" :key="row.id" class="group align-middle transition-colors hover:bg-slate-50/80 dark:hover:bg-slate-800/40" :data-table-id="row.id">
            <td class="px-3 py-4"><p class="font-semibold leading-5 text-slate-900 dark:text-slate-100">{{ purpose(row) }}</p><code class="mt-1.5 block truncate text-[11px] text-slate-500 dark:text-slate-400" :title="`${row.schema ? row.schema + '.' : ''}${row.id}`">{{ row.schema ? row.schema + '.' : '' }}{{ row.id }}</code><p class="mt-2 text-[11px] text-slate-500 dark:text-slate-400">{{ t(`storageGov.tables.categories.${row.category}`) }}</p></td>
            <td class="px-3 py-4 tabular-nums"><p class="whitespace-nowrap text-sm font-semibold tracking-tight text-slate-900 dark:text-slate-100">{{ bytes(row.total_bytes) }}</p><p v-if="health(row.id)?.net_change_bytes != null" class="mt-1.5 flex items-center gap-1 whitespace-nowrap text-[11px]" :class="health(row.id)?.level === 'warning' ? 'text-warning' : 'text-slate-500 dark:text-slate-400'" :aria-label="`${t('storageGov.tableMonitor.net')} ${delta(health(row.id)!.net_change_bytes!)}`" :title="t('storageGov.tableMonitor.net')"><span aria-hidden="true">{{ health(row.id)!.net_change_bytes! > 0 ? '↗' : health(row.id)!.net_change_bytes! < 0 ? '↘' : '—' }}</span><span>{{ delta(health(row.id)!.net_change_bytes!) }}</span></p><p v-if="row.state !== 'observed'" class="mt-1.5 text-xs text-warning">{{ t(`storageGov.tables.states.${row.state}`) }}</p></td>
            <td class="px-3 py-4 text-[11px]"><div v-if="row.time_state === 'observed'" class="space-y-2"><p class="flex items-center gap-2 whitespace-nowrap"><span aria-hidden="true" class="size-1.5 shrink-0 rounded-full bg-slate-300 dark:bg-slate-500" /><span class="sr-only">{{ t('storageGov.tables.oldest') }}</span><time :datetime="row.oldest_at || undefined" :title="t('storageGov.tables.oldest')" class="tabular-nums text-slate-500 dark:text-slate-400">{{ listDate(row.oldest_at) }}</time></p><p class="flex items-center gap-2 whitespace-nowrap"><span aria-hidden="true" class="size-1.5 shrink-0 rounded-full bg-primary-500" /><span class="sr-only">{{ t('storageGov.tables.newest') }}</span><time :datetime="row.newest_at || undefined" :title="t('storageGov.tables.newest')" class="tabular-nums text-slate-700 dark:text-slate-200">{{ listDate(row.newest_at) }}</time></p></div><p v-else class="text-slate-500 dark:text-slate-400">{{ t(`storageGov.tables.times.${row.time_state}`) }}</p><code class="mt-2 block truncate text-[10px] text-slate-500 dark:text-slate-400" :title="row.time_column">{{ row.time_column }}</code></td>
            <td class="px-3 py-4" data-cleanup-state><span class="inline-flex max-w-full items-center gap-1.5 whitespace-nowrap rounded-full border px-2.5 py-1 text-[11px] font-medium" :class="cleanupAppearance[governance(row).state]" :title="t(`storageGov.tables.listing.states.${governance(row).state}`) + (['blocked', 'unknown', 'managed'].includes(governance(row).state) ? ' · ' + t(`storageGov.tables.listing.hints.${governance(row).state}`) : '')"><span aria-hidden="true" class="size-1.5 shrink-0 rounded-full bg-current" /><span class="truncate">{{ t(`storageGov.tables.listing.states.${governance(row).state}`) }}</span></span></td>
            <td class="px-4 py-4 text-xs" data-retention-window><div v-if="governance(row).retention === 'invocationDays'" class="space-y-1.5"><p class="flex items-center gap-2 whitespace-nowrap"><span class="text-slate-500 dark:text-slate-400">{{ t('storageGov.tables.listing.successLabel') }}</span><span class="font-medium tabular-nums">{{ t('storageGov.tables.listing.days', { days: governance(row).values!.success }) }}</span></p><p class="flex items-center gap-2 whitespace-nowrap"><span class="text-slate-500 dark:text-slate-400">{{ t('storageGov.tables.listing.failureLabel') }}</span><span class="font-medium tabular-nums">{{ t('storageGov.tables.listing.days', { days: governance(row).values!.failure }) }}</span></p></div><p v-else class="font-medium leading-5">{{ t(`storageGov.tables.listing.${governance(row).retention}`, governance(row).values || {}) }}</p><p v-if="governance(row).inactive" class="mt-2 text-[11px] text-slate-500 dark:text-slate-400">{{ t('storageGov.tables.listing.inactive') }}</p></td>
            <td class="px-2 py-4 lg:sticky lg:right-0 lg:z-10 lg:border-l lg:border-slate-100 lg:bg-white lg:group-hover:bg-slate-50 dark:lg:border-slate-700/50 dark:lg:bg-[#101b2d] dark:lg:group-hover:bg-slate-800"><div class="flex flex-col items-stretch gap-1.5"><UButton v-if="governance(row).configurable" size="xs" color="primary" variant="soft" class="min-h-8 min-w-0 justify-center whitespace-nowrap px-1 text-[11px]" :title="t('storageGov.tables.listing.configure')" data-configure-cleanup @click="configure(row)"><span class="truncate">{{ t('storageGov.tables.listing.configure') }}</span></UButton><UButton size="xs" color="neutral" variant="ghost" class="min-h-8 min-w-0 justify-center whitespace-nowrap px-1 text-[11px]" :aria-label="t('storageGov.tables.detailsFor', { table: row.id })" :title="t(governance(row).state === 'protected' || governance(row).state === 'unsupported' ? 'storageGov.tables.listing.reason' : 'storageGov.tables.details')" data-table-details @click="open(row)"><span class="truncate">{{ t(governance(row).state === 'protected' || governance(row).state === 'unsupported' ? 'storageGov.tables.listing.reason' : 'storageGov.tables.details') }}</span></UButton></div></td>
          </tr></tbody>
        </table>
        <p v-if="!filtered.length" class="p-6 text-center text-muted" data-testid="system-tables-empty">{{ t('storageGov.tables.empty') }}</p>
      </div>
      <div class="flex flex-wrap items-center justify-between gap-2 text-sm"><span>{{ t('storageGov.tables.results', { count: filtered.length }) }} · {{ page }} / {{ pages }}</span><div class="flex gap-2"><UButton color="neutral" variant="outline" :disabled="page === 1" @click="page--">{{ t('storageGov.tables.previous') }}</UButton><UButton color="neutral" variant="outline" :disabled="page >= pages" @click="page++">{{ t('storageGov.tables.next') }}</UButton></div></div>
    </template>
    <UModal v-model:open="detailOpen" :title="selected ? purpose(selected) : t('storageGov.tables.details')" :description="t('storageGov.tables.detailHint')" :dismissible="!retentionUnconfirmed" :close="retentionUnconfirmed ? false : undefined" :ui="{ content: selected && supportsRetention(selected.id) ? 'max-w-5xl' : 'max-w-3xl' }">
      <template #body><div v-if="selected" class="min-w-0 space-y-4" data-testid="system-table-detail">
        <code class="block break-all text-sm">{{ selected.schema ? selected.schema + '.' : '' }}{{ selected.id }}</code>
        <UAlert v-if="detailError" color="error" :title="t('storageGov.loadFailed')" :description="detailError" />
        <BackupSectionTabs v-model="detailSection" :items="detailSections" :label="t('storageGov.tables.details')"><template #capacity><div class="space-y-4">
        <dl class="grid grid-cols-2 gap-3 text-sm"><dt>{{ t('storageGov.database.heap') }}</dt><dd>{{ bytes(selected.heap_bytes) }}</dd><dt>{{ t('storageGov.database.indexes') }}</dt><dd>{{ bytes(selected.indexes_bytes) }}</dd><dt>TOAST</dt><dd>{{ bytes(selected.toast_bytes) }}</dd><dt>{{ t('storageGov.tables.capacity') }}</dt><dd>{{ bytes(selected.total_bytes) }}</dd><dt>{{ t('storageGov.tables.rows') }}</dt><dd>{{ selected.estimated_rows === null ? '—' : selected.estimated_rows.toLocaleString() }}</dd><dt>{{ t('storageGov.tables.timeColumn') }}</dt><dd class="break-all">{{ selected.time_column }}</dd><dt>{{ t('storageGov.tables.oldest') }}</dt><dd>{{ date(selected.oldest_at) }}</dd><dt>{{ t('storageGov.tables.newest') }}</dt><dd>{{ date(selected.newest_at) }}</dd></dl>
        <p v-if="selected.problem || selected.time_problem" class="text-sm text-warning">{{ problem(selected.problem || selected.time_problem) }}</p>
        <div class="rounded-lg border border-default p-3 text-sm"><p class="font-medium">{{ mechanism(selected) }}</p><p class="mt-2 text-muted">{{ t(`storageGov.tables.protections.${selected.protection}`) }}</p><p v-if="selected.cleanup_mechanism === 'invocation_policy' && inventory?.invocation_policy" class="mt-2">{{ t('storageGov.tables.invocationDays', { success: inventory.invocation_policy.success_days, failure: inventory.invocation_policy.failure_days }) }}</p></div>
        <p class="text-xs text-muted">{{ t('storageGov.tables.accounting') }}</p>
        </div></template><template #history><div class="space-y-3" data-testid="table-history">
          <p class="text-sm text-muted">{{ t('storageGov.tableMonitor.metric') }}</p>
          <UButton color="neutral" variant="outline" :loading="historyLoading" @click="loadHistory">{{ t('storageGov.refresh') }}</UButton>
          <p v-if="historyError" role="alert" class="text-sm text-error">{{ historyError }}</p>
          <p v-if="history && !history.points.length" class="text-sm text-muted">{{ t('storageGov.tableMonitor.noHistory') }}</p>
          <div v-if="history?.points.length" class="max-h-72 overflow-auto"><table class="w-full text-left text-xs"><thead><tr><th class="p-2">{{ t('storageGov.observedAt') }}</th><th class="p-2">{{ t('storageGov.tables.capacity') }}</th></tr></thead><tbody><tr v-for="point in history.points.slice().reverse()" :key="point.observed_at"><td class="p-2">{{ date(point.observed_at) }}</td><td class="p-2">{{ point.state === 'observed' ? bytes(point.bytes) : '—' }}</td></tr></tbody></table></div>
        </div></template><template #cleanup><SystemTableRetentionControl v-if="cleanupVisited && selected && supportsRetention(selected.id)" :key="selected.id" :id="selected.id" :revision="revision" @saved="acceptRetention" @uncertain="retentionUnconfirmed = $event" /></template></BackupSectionTabs>
      </div></template>
      <template #footer><div class="flex min-w-0 flex-wrap gap-2"><p v-if="retentionUnconfirmed" role="alert" data-testid="typed-cleanup-unconfirmed" class="text-sm text-warning">{{ t('storageGov.tableRetention.unknownFooter') }}</p><UButton v-if="retentionUnconfirmed && detailSection !== 'cleanup'" @click="detailSection = 'cleanup'">{{ t('storageGov.systemControl.returnTask') }}</UButton><UButton v-if="detailSection === 'capacity'" color="neutral" variant="outline" :loading="detailLoading" @click="refreshDetail">{{ t('storageGov.tables.refreshDetail') }}</UButton><UButton v-if="selected?.cleanup_mechanism === 'invocation_policy'" @click="detailOpen = false; emit('open-retention')">{{ t('storageGov.tables.configure') }}</UButton></div></template>
    </UModal>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useStorageDatabaseTablesService, type DatabaseTableInventory, type SystemTableObservation } from '~/composables/api/services/storageDatabaseTablesService';
import type { TableHistory } from '~/composables/api/services/storageDatabaseTablesService';
import BackupSectionTabs from '~/components/ops/backup/BackupSectionTabs.vue';
import TableMonitoringControl from './TableMonitoringControl.vue';
import SystemTableRetentionControl from './SystemTableRetentionControl.vue';
import { formatStorageBytes as bytes } from '~/utils/storage-capacity';
import { tableGovernance, supportsTableRetention as supportsRetention, compareTableValues, type CleanupState, type TableSort, type SortDirection } from '~/utils/storage-table-list';
import type { SystemTableRetentionStatus } from '~/composables/api/services/storageDatabaseTablesService';
const props = defineProps<{ revision?: number }>();
const emit = defineEmits<{ 'open-retention': [] }>();
const { t, locale } = useI18n(), api = useStorageDatabaseTablesService();
const inventory = ref<DatabaseTableInventory>(), search = ref(''), category = ref('all'), page = ref(1);
const cleanupState = ref<CleanupState | 'all'>('all'), sortBy = ref<TableSort>('capacity'), direction = ref<SortDirection>('desc');
const typedPolicies = ref<Partial<Record<string, SystemTableRetentionStatus>>>({});
const loading = ref(false), error = ref(''), detailOpen = ref(false), detailLoading = ref(false), detailError = ref(''), selected = ref<SystemTableObservation>();
const detailSection = ref('capacity'), history = ref<TableHistory>(), historyLoading = ref(false), historyError = ref('');
const cleanupVisited = ref(false);
const retentionUnconfirmed = ref(false);
const detailSections = computed(() => [{ value: 'capacity', label: t('storageGov.tables.capacity') }, { value: 'history', label: t('storageGov.tableMonitor.history') }, ...(selected.value && supportsRetention(selected.value.id) ? [{ value: 'cleanup', label: t('storageGov.tableRetention.title') }] : [])]);
const categories = ['all', 'observability', 'runtime', 'security', 'usage', 'recovery', 'protected'];
const cleanupStates: CleanupState[] = ['enabled', 'disabled', 'blocked', 'managed', 'unsupported', 'protected', 'unknown'];
const cleanupAppearance: Record<CleanupState, string> = {
  enabled: 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-400/20 dark:bg-emerald-400/10 dark:text-emerald-300',
  disabled: 'border-slate-200 bg-slate-50 text-slate-600 dark:border-slate-600/70 dark:bg-slate-700/30 dark:text-slate-300',
  blocked: 'border-amber-200 bg-amber-50 text-amber-800 dark:border-amber-400/20 dark:bg-amber-400/10 dark:text-amber-300',
  managed: 'border-indigo-200 bg-indigo-50 text-indigo-700 dark:border-indigo-400/20 dark:bg-indigo-400/10 dark:text-indigo-300',
  unsupported: 'border-slate-200 bg-slate-50 text-slate-600 dark:border-slate-600/70 dark:bg-slate-700/30 dark:text-slate-300',
  protected: 'border-sky-200 bg-sky-50 text-sky-700 dark:border-sky-400/20 dark:bg-sky-400/10 dark:text-sky-300',
  unknown: 'border-amber-200 bg-amber-50 text-amber-800 dark:border-amber-400/20 dark:bg-amber-400/10 dark:text-amber-300',
};
const sortFields: TableSort[] = ['capacity', 'growth', 'oldest', 'newest', 'retention', 'name'];
let controller: AbortController | undefined, detailController: AbortController | undefined, historyController: AbortController | undefined, disposed = false;
const health = (id: string) => inventory.value?.monitoring.tables.find(r => r.id === id);
const delta = (value: number) => `${value >= 0 ? '+' : '−'}${bytes(Math.abs(value))}`;
const purpose = (r: SystemTableObservation) => t(`storageGov.tables.purposes.${r.id}`);
const date = (s: string | null) => s ? new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(s)) : '—';
const listDate = (s: string | null) => s ? new Intl.DateTimeFormat(locale.value, { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }).format(new Date(s)) : '—';
const measuredCount = computed(() => inventory.value?.items.filter(r => r.state === 'observed').length || 0);
const knownBytes = computed(() => measuredCount.value ? inventory.value!.items.reduce((n, r) => n + (r.total_bytes || 0), 0) : null);
const governance = (row: SystemTableObservation) => tableGovernance(row, inventory.value!, typedPolicies.value);
function sortValue(row: SystemTableObservation): number | null | undefined {
  if (sortBy.value === 'capacity') return row.total_bytes;
  if (sortBy.value === 'growth') return health(row.id)?.net_change_bytes;
  if (sortBy.value === 'retention') return governance(row).days;
  const timestamp = sortBy.value === 'oldest' ? row.oldest_at : row.newest_at;
  return row.time_state === 'observed' && timestamp ? Date.parse(timestamp) : null;
}
const filtered = computed(() => (inventory.value?.items || []).filter(r => (category.value === 'all' || r.category === category.value) && (cleanupState.value === 'all' || governance(r).state === cleanupState.value) && (r.id + ' ' + purpose(r)).toLocaleLowerCase(locale.value).includes(search.value.trim().toLocaleLowerCase(locale.value))).slice().sort((a, b) => {
  const comparison = sortBy.value === 'name' ? a.id.localeCompare(b.id) * (direction.value === 'asc' ? 1 : -1) : compareTableValues(sortValue(a), sortValue(b), direction.value);
  return comparison || a.id.localeCompare(b.id);
}));
const pages = computed(() => Math.max(1, Math.ceil(filtered.value.length / 20)));
const pageRows = computed(() => filtered.value.slice((page.value - 1) * 20, page.value * 20));
function mechanism(r: SystemTableObservation) {
  if (r.cleanup_mechanism === 'invocation_policy') return t(inventory.value?.invocation_policy ? inventory.value.invocation_policy.enabled ? 'storageGov.tables.policyEnabled' : 'storageGov.tables.policyDisabled' : 'storageGov.tables.policyUnknown');
  if (r.id === 'ops_storage_samples') return t('storageGov.tables.samplesWindow');
  if (r.id === 'ops_storage_cleanup_runs') return t('storageGov.tables.cleanupWindow');
  if (r.id === 'ops_storage_policy_revisions') return t('storageGov.tables.revisionWindow');
  return t(`storageGov.tables.mechanisms.${r.cleanup_mechanism}`);
}
function problem(code?: string) { return t(`storageGov.tables.problems.${code || 'relation_metrics_unreadable'}`); }
async function load() {
  controller?.abort(); const c = controller = new AbortController(); loading.value = true; error.value = '';
  try {
    const value = await api.inventory(c.signal);
    if (disposed || c.signal.aborted) return;
    inventory.value = value;
    const ids = value.items.filter(r => r.cleanup_mechanism === 'typed_policy' && supportsRetention(r.id)).map(r => r.id);
    const statuses: Partial<Record<string, SystemTableRetentionStatus>> = {};
    // 分别读取两类适配器的权威版本；失败项保持未知，不沿用旧启用许可。
    for (const id of ids) {
      if (disposed || c.signal.aborted) return;
      try { statuses[id] = await api.retention(c.signal, id); }
      catch { /* A failed read remains unknown; it must not become a disabled rule. */ }
    }
    if (!disposed && !c.signal.aborted) {
      typedPolicies.value = Object.fromEntries(Object.entries(statuses).map(([id, status]) => {
        const saved = typedPolicies.value[id];
        return [id, saved && saved.revision > status!.revision ? saved : status];
      }));
      page.value = Math.min(page.value, pages.value);
    }
  }
  catch (e) { if (!disposed && !c.signal.aborted) error.value = e instanceof Error ? e.message : String(e); }
  finally { if (!disposed && !c.signal.aborted) loading.value = false; }
}
function open(row: SystemTableObservation) { detailController?.abort(); historyController?.abort(); detailLoading.value = false; selected.value = row; detailError.value = ''; history.value = undefined; historyError.value = ''; detailSection.value = 'capacity'; cleanupVisited.value = false; retentionUnconfirmed.value = false; detailOpen.value = true; }
function configure(row: SystemTableObservation) { if (row.cleanup_mechanism === 'invocation_policy') { emit('open-retention'); return; } open(row); cleanupVisited.value = true; detailSection.value = 'cleanup'; }
function acceptRetention(status: SystemTableRetentionStatus) { typedPolicies.value = { ...typedPolicies.value, [status.id]: status }; }
function resetFilters() { search.value = ''; category.value = 'all'; cleanupState.value = 'all'; sortBy.value = 'capacity'; direction.value = 'desc'; page.value = 1; }
async function loadHistory() {
  if (!selected.value) return;
  historyController?.abort(); const c = historyController = new AbortController(), id = selected.value.id;
  historyLoading.value = true; historyError.value = '';
  try { const result = await api.history(c.signal, id); if (!disposed && !c.signal.aborted && selected.value?.id === id) history.value = result; }
  catch (e) { if (!disposed && !c.signal.aborted) historyError.value = e instanceof Error ? e.message : String(e); }
  finally { if (!disposed && !c.signal.aborted) historyLoading.value = false; }
}
async function refreshDetail() {
  if (!selected.value) return;
  detailController?.abort(); const c = detailController = new AbortController(), id = selected.value.id;
  detailLoading.value = true; detailError.value = '';
  try { const result = await api.inventory(c.signal, id); if (!disposed && !c.signal.aborted && selected.value?.id === id) { const row = result.items.find(r => r.id === id); if (row) { selected.value = row; if (inventory.value) inventory.value.items = inventory.value.items.map(r => r.id === id ? row : r); } } }
  catch (e) { if (!disposed && !c.signal.aborted) detailError.value = e instanceof Error ? e.message : String(e); }
  finally { if (!disposed && !c.signal.aborted) detailLoading.value = false; }
}
watch([search, category, cleanupState, sortBy, direction], () => { page.value = 1; });
watch(sortBy, value => { direction.value = ['oldest', 'retention', 'name'].includes(value) ? 'asc' : 'desc'; });
watch(detailOpen, value => { if (!value) { detailController?.abort(); historyController?.abort(); detailLoading.value = false; historyLoading.value = false; } });
watch(detailSection, value => { if (value === 'history' && detailOpen.value) loadHistory(); if (value === 'cleanup') cleanupVisited.value = true; });
watch(() => props.revision, () => { load(); if (detailOpen.value && detailSection.value === 'history') loadHistory(); });
onMounted(load);
onBeforeUnmount(() => { disposed = true; controller?.abort(); detailController?.abort(); historyController?.abort(); });
</script>
