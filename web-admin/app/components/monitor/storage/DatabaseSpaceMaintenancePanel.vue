<template>
  <section class="min-w-0 space-y-4" data-testid="database-space-maintenance">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <h2 class="text-lg font-semibold">{{ t('storageGov.dbMaintenance.title') }}</h2>
      <div class="flex flex-wrap gap-2"><UButton color="neutral" variant="outline" @click="$emit('openRetention')">{{ t('storageGov.dbMaintenance.retention') }}</UButton><UButton color="neutral" variant="outline" data-testid="maintenance-history" @click="recordsOpen = true">{{ t('storageGov.dbMaintenance.records') }}</UButton><UButton color="neutral" variant="ghost" :loading="loading" data-testid="maintenance-refresh" @click="load">{{ t('storageGov.refresh') }}</UButton></div>
    </div>
    <UAlert v-if="error" color="error" :title="error" data-testid="maintenance-error" />
    <UAlert v-if="status?.problem" color="warning" :title="problemText(status.problem)" data-testid="maintenance-readiness" />
    <UAlert v-if="pending || intentDamaged" color="warning" :title="t('storageGov.dbMaintenance.pendingTitle')" :description="t('storageGov.dbMaintenance.pendingHint')" data-testid="maintenance-pending">
      <template #actions><UButton v-if="pending" :disabled="busy" data-testid="maintenance-reconcile" @click="reconcile">{{ t('storageGov.dbMaintenance.reconcile') }}</UButton><UButton v-if="pending && pending.scopeId === status?.scope_id" :disabled="busy" color="neutral" variant="outline" data-testid="maintenance-cancel-pending" @click="cancelPending">{{ t('storageGov.dbMaintenance.cancelRequest') }}</UButton><UButton color="neutral" variant="ghost" @click="recordsOpen = true">{{ t('storageGov.dbMaintenance.records') }}</UButton></template>
    </UAlert>
    <UCard v-if="current"><DatabaseMaintenanceRunDetails :run="current" :busy="busy" @cancel="cancelRun" @review="reviewRun" /></UCard>
    <UCard>
      <div class="flex flex-wrap items-start justify-between gap-3"><div><p class="text-sm text-muted">{{ t('storageGov.dbMaintenance.databaseSize') }}</p><p class="mt-1 text-xl font-semibold tabular-nums">{{ bytes(status?.snapshot?.database_bytes) }}</p></div><p class="max-w-xl text-sm text-muted">{{ t('storageGov.dbMaintenance.scope') }}</p></div>
      <fieldset class="mt-5 space-y-3" :disabled="busy || !!pending || intentDamaged"><legend class="mb-2 text-sm font-medium">{{ t('storageGov.dbMaintenance.selectTables') }}</legend>
        <label v-for="row in availableTables" :key="row.id" class="flex min-w-0 items-start gap-3 rounded-lg border border-default p-3">
          <input v-if="incremental" v-model="incrementalTable" type="radio" name="incremental-maintenance-table" :value="row.id" :disabled="!row.can_maintain || !row.indexes_valid" class="mt-1 size-4 shrink-0 accent-emerald-500" :data-testid="'maintenance-table-' + row.id" />
          <input v-else v-model="selected" type="checkbox" :value="row.id" :disabled="!row.can_maintain || !row.indexes_valid" class="mt-1 size-4 shrink-0 accent-emerald-500" :data-testid="'maintenance-table-' + row.id" />
          <span class="min-w-0 flex-1"><span class="block font-medium">{{ tableLabel(row.id) }}</span><span class="mt-1 block text-xs text-muted">{{ t('storageGov.dbMaintenance.deadEstimate', { count: row.dead_rows_estimate }) }}</span><span v-if="!row.can_maintain || !row.indexes_valid" class="block text-xs text-warning">{{ problemText(!row.can_maintain ? 'DATABASE_MAINTENANCE_OWNER_REQUIRED' : 'DATABASE_MAINTENANCE_INDEXES_INVALID') }}</span></span>
          <span class="shrink-0 text-sm tabular-nums">{{ bytes(row.total_bytes) }}</span>
        </label>
        <p v-if="!status?.snapshot" class="text-sm text-muted">{{ t('storageGov.dbMaintenance.observationNeeded') }}</p>
      </fieldset>
      <UFormField class="mt-5" :label="t('storageGov.dbMaintenance.mode')"><USelect v-model="mode" :items="modes" value-key="value" class="w-full sm:w-80" :disabled="busy || !!pending || intentDamaged" data-testid="maintenance-mode" /></UFormField>
      <p class="mt-2 text-sm text-muted">{{ t('storageGov.dbMaintenance.modeHints.' + mode) }}</p>
      <div v-if="incremental" class="mt-4 grid max-w-2xl gap-4 sm:grid-cols-2">
        <UFormField :label="t('storageGov.dbMaintenance.maintenanceWindow')">
          <USelect v-model="windowMinutes" :items="windows" :disabled="planDisabled" class="w-full" data-testid="maintenance-window" />
        </UFormField>
        <UFormField :label="t('storageGov.dbMaintenance.targetSize')" :error="invalidTarget ? t('storageGov.dbMaintenance.invalidTargetSize') : undefined">
          <UInput v-model="targetGiB" type="number" min="1" :max="2 ** 20" step="1" :disabled="planDisabled" class="w-full" data-testid="maintenance-target" />
        </UFormField>
      </div>
      <p v-if="modeProblem" role="status" class="mt-3 text-sm text-warning">{{ problemText(modeProblem) }}</p>
      <div class="mt-5 flex flex-wrap items-center gap-3"><UButton :disabled="!canPreview" :loading="busy" data-testid="maintenance-preview" @click="check">{{ t('storageGov.dbMaintenance.check') }}</UButton><p class="text-xs text-muted">{{ t('storageGov.dbMaintenance.checkHint') }}</p></div>
    </UCard>

    <UModal v-model:open="previewOpen" :title="t('storageGov.dbMaintenance.previewTitle')" :description="t('storageGov.dbMaintenance.previewHint')" :ui="{ content: 'max-w-3xl' }">
      <template #body><div v-if="preview" class="space-y-4" data-testid="maintenance-preview-detail">
        <p class="font-medium">{{ t('storageGov.dbMaintenance.modes.' + preview.mode) }}</p><ul class="space-y-2"><li v-for="id in preview.tables" :key="id">{{ tableLabel(id) }}</li></ul>
        <dl class="grid grid-cols-2 gap-3 text-sm"><dt>{{ t('storageGov.dbMaintenance.workSpace') }}</dt><dd class="tabular-nums">{{ bytes(preview.peak_bytes) }}</dd><dt>{{ t('storageGov.dbMaintenance.expires') }}</dt><dd>{{ new Date(preview.expires_at!).toLocaleTimeString() }}</dd></dl>
        <dl v-if="preview.window_minutes" class="grid grid-cols-2 gap-3 text-sm">
          <dt>{{ t('storageGov.dbMaintenance.maintenanceWindow') }}</dt><dd>{{ t('storageGov.dbMaintenance.durationMinutes', { count: preview.window_minutes }) }}</dd>
          <template v-if="preview.target_database_bytes"><dt>{{ t('storageGov.dbMaintenance.targetSize') }}</dt><dd>{{ bytes(preview.target_database_bytes) }}</dd></template>
        </dl>
        <UAlert v-if="preview.mode !== 'reuse'" color="warning" :title="t('storageGov.dbMaintenance.' + (preview.mode === 'incremental' ? 'incrementalLockTitle' : 'lockTitle'))" :description="t('storageGov.dbMaintenance.' + (preview.mode === 'incremental' ? 'incrementalLockHint' : 'lockHint'))" />
        <UCheckbox v-model="confirmed" :disabled="busy || !!pending" :label="t('storageGov.dbMaintenance.confirm')" data-testid="maintenance-confirm" />
        <UCheckbox v-if="preview.mode !== 'reuse'" v-model="exclusiveConfirmed" :disabled="busy || !!pending" :label="t('storageGov.dbMaintenance.confirmLock')" data-testid="maintenance-confirm-lock" />
        <p v-if="expired" role="alert" class="text-warning">{{ t('storageGov.dbMaintenance.expired') }}</p><p v-if="error" role="alert" class="text-error">{{ error }}</p>
        <div v-if="pending" class="flex flex-wrap gap-2"><UButton :disabled="busy" @click="reconcile">{{ t('storageGov.dbMaintenance.reconcile') }}</UButton><UButton :disabled="busy || pending.scopeId !== status?.scope_id" color="neutral" @click="cancelPending">{{ t('storageGov.dbMaintenance.cancelRequest') }}</UButton></div>
      </div></template>
      <template #footer><UButton :disabled="!preview || !confirmed || expired || busy || !!pending || intentDamaged || preview.mode !== 'reuse' && !exclusiveConfirmed" :loading="busy" data-testid="maintenance-start" @click="start">{{ t('storageGov.dbMaintenance.start') }}</UButton><UButton color="neutral" variant="ghost" @click="previewOpen = false">{{ t('storageGov.dbMaintenance.close') }}</UButton></template>
    </UModal>
    <UModal v-model:open="recordsOpen" :title="t('storageGov.dbMaintenance.records')" :description="t('storageGov.dbMaintenance.recordsHint')" :ui="{ content: 'max-w-3xl' }">
      <template #body>
        <div v-if="detail" class="space-y-4"><UButton color="neutral" variant="ghost" @click="detailID = ''">{{ t('storageGov.dbMaintenance.back') }}</UButton><DatabaseMaintenanceRunDetails :run="detail" :busy="busy" @cancel="cancelRun" @review="reviewRun" /></div>
        <ul v-else class="max-h-[65vh] space-y-3 overflow-y-auto"><li v-for="row in history" :key="row.id"><button class="flex w-full flex-wrap justify-between gap-2 rounded-lg border border-default p-3 text-left hover:bg-elevated" @click="detailID = row.id"><span>{{ t('storageGov.dbMaintenance.modes.' + row.mode) }} · {{ new Date(row.at).toLocaleString() }}</span><span>{{ t('storageGov.dbMaintenance.states.' + row.state) }}</span></button></li><li v-if="!history.length" class="text-sm text-muted">{{ t('storageGov.dbMaintenance.noRecords') }}</li></ul>
      </template>
    </UModal>
  </section>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import DatabaseMaintenanceRunDetails from './DatabaseMaintenanceRunDetails.vue';
import { useStorageDatabaseMaintenanceService, type DatabaseMaintenanceRun, type DatabaseMaintenanceStatus, type MaintenanceMode, type MaintenanceTableID } from '~/composables/api/services/storageDatabaseMaintenanceService';
import { AUTH_SESSION_EVENT, readAuthSession } from '~/utils/auth-session';
import { readMaintenanceIntent, saveMaintenanceIntent, clearMaintenanceIntent, type MaintenanceIntent } from '~/utils/database-maintenance-intent';
import { formatStorageBytes as bytes } from '~/utils/storage-capacity';
const props = defineProps<{ revision?: number }>();
defineEmits<{ openRetention: [] }>();
const { t, te } = useI18n(), api = useStorageDatabaseMaintenanceService();
const status = ref<DatabaseMaintenanceStatus>(), runs = ref<Record<string, DatabaseMaintenanceRun>>({});
const selected = ref<MaintenanceTableID[]>([]), mode = ref<MaintenanceMode>('reuse');
const preview = ref<DatabaseMaintenanceRun>(), pending = ref<MaintenanceIntent>();
const previewOpen = ref(false), recordsOpen = ref(false), detailID = ref(''), trackedID = ref('');
const loading = ref(false), busy = ref(false), error = ref(''), confirmed = ref(false), exclusiveConfirmed = ref(false), expired = ref(false), intentDamaged = ref(false);
let readController: AbortController | undefined, actionController: AbortController | undefined, expiryTimer: ReturnType<typeof setTimeout> | undefined, disposed = false, initialized = false;
let owner: ReturnType<typeof identity> | undefined;
const modes = computed(() => (['reuse', 'incremental', 'compact'] as const).map(value => ({ value, label: t('storageGov.dbMaintenance.modes.' + value) })));
const tableLabel = (id: MaintenanceTableID) => t('storageGov.dbMaintenance.tables.' + id);
const current = computed(() => runs.value[status.value?.active?.id || trackedID.value]);
const detail = computed(() => runs.value[detailID.value]);
const history = computed(() => Object.values(runs.value).filter(x => !['preview', 'consumed'].includes(x.state)).sort((a, b) => Date.parse(b.at) - Date.parse(a.at)));
const notificationTable = 'capability-event-publications';
const incremental = computed(() => mode.value === 'incremental');
const incrementalTable = ref<MaintenanceTableID>(notificationTable);
const windowMinutes = ref(60);
const targetGiB = ref<string | number>('');
const windows = computed(() => [12, 60, 240, 480].map(value => ({
  value, label: t('storageGov.dbMaintenance.durationMinutes', { count: value }),
})));
const invalidTarget = computed(() => targetGiB.value !== '' &&
  (!Number.isInteger(Number(targetGiB.value)) || Number(targetGiB.value) < 1 || Number(targetGiB.value) > 2 ** 20));
const planDisabled = computed(() => busy.value || !!pending.value || intentDamaged.value);
const availableTables = computed(() => (status.value?.snapshot?.tables || []).filter(row =>
  !incremental.value || row.id === notificationTable || row.incremental_problem !== undefined,
));
const selectedForMode = computed(() => incremental.value
  ? availableTables.value.filter(row => row.id === incrementalTable.value).map(row => row.id)
  : selected.value,
);
const modeProblem = computed(() => {
  const snapshot = status.value?.snapshot;
  if (mode.value === 'reuse' || !snapshot) return '';
  if (!snapshot.local_disk_verified || availableTables.value.some(row => selectedForMode.value.includes(row.id) && !row.default_tablespace)) {
    return 'DATABASE_MAINTENANCE_DISK_UNVERIFIED';
  }
  if (incremental.value) {
    const target = availableTables.value.find(row => row.id === incrementalTable.value);
    if (target && !target.can_maintain) return 'DATABASE_MAINTENANCE_OWNER_REQUIRED';
    if (target && !target.indexes_valid) return 'DATABASE_MAINTENANCE_INDEXES_INVALID';
    return target?.incremental_problem ?? snapshot.incremental_problem ?? '';
  }
  return snapshot.rewrite_safe ? '' : 'DATABASE_MAINTENANCE_WAL_PROTECTED';
});
const canPreview = computed(() => !!status.value?.ready && selectedForMode.value.length > 0 && !busy.value && !pending.value && !intentDamaged.value && !status.value.active && !modeProblem.value && (!incremental.value || !invalidTarget.value));
function identity() { const s = readAuthSession(); if (!s || s.status !== 'active') throw new Error('AUTH_SESSION_CHANGED'); return { sessionId: s.sessionId, tenantUuid: s.tenantUuid || '' }; }
function sessionChanged() {
  const next = readAuthSession();
  if (next?.status === 'active' && next.sessionId === owner?.sessionId && (next.tenantUuid || '') === owner.tenantUuid) return;
  disposed = true; readController?.abort(); actionController?.abort(); clearTimeout(expiryTimer);
  previewOpen.value = false; recordsOpen.value = false; preview.value = undefined; pending.value = undefined;
  status.value = undefined; runs.value = {}; busy.value = false; loading.value = false; intentDamaged.value = true;
  error.value = problemText('AUTH_SESSION_CHANGED');
}
function problemText(value: unknown) { const text = value instanceof Error ? value.message : String(value || ''); const code = text.match(/[A-Z][A-Z_]{8,}/)?.[0] || ''; return t(te('storageGov.dbMaintenance.problems.' + code) ? 'storageGov.dbMaintenance.problems.' + code : 'storageGov.dbMaintenance.requestFailed'); }
function accept(run: DatabaseMaintenanceRun) {
  if (status.value?.scope_id && run.scope_id !== status.value.scope_id) throw new Error('DATABASE_MAINTENANCE_TARGET_CHANGED');
  if (!runs.value[run.id] || runs.value[run.id]!.version < run.version) runs.value = { ...runs.value, [run.id]: run };
  if (pending.value?.requestId === run.id && pending.value.scopeId === run.scope_id) {
    clearMaintenanceIntent(window.sessionStorage, identity(), run.id); pending.value = undefined; trackedID.value = run.id; error.value = ''; previewOpen.value = false;
  }
}
async function load() {
  if (disposed) return;
  readController?.abort(); readController = new AbortController(); const signal = readController.signal; loading.value = true;
  try {
    const next = await api.status(signal); if (disposed || signal.aborted) return;
    if (status.value?.scope_id && next.scope_id && status.value.scope_id !== next.scope_id) {
      runs.value = {}; trackedID.value = ''; detailID.value = ''; preview.value = undefined; previewOpen.value = false; clearTimeout(expiryTimer); error.value = problemText('DATABASE_MAINTENANCE_TARGET_CHANGED');
    }
    status.value = next; next.runs.forEach(accept); if (next.active) accept(next.active);
    if (!initialized) { initialized = true; selected.value = next.snapshot?.tables.filter(x => x.can_maintain && x.indexes_valid).map(x => x.id) || [];
      try { pending.value = readMaintenanceIntent(window.sessionStorage, identity()); } catch (e) { intentDamaged.value = true; error.value = problemText(e); }
      if (pending.value) { const known = runs.value[pending.value.requestId]; if (known) { if (known.mode === 'incremental') incrementalTable.value = known.tables[0]!; else selected.value = [...known.tables]; mode.value = known.mode; accept(known); } }
    }
  } catch (e) { if (!disposed && !signal.aborted) error.value = problemText(e); }
  finally { if (!disposed && !signal.aborted) loading.value = false; }
}
function action() { actionController?.abort(); actionController = new AbortController(); busy.value = true; error.value = ''; return actionController.signal; }
async function check() {
  if (!canPreview.value) return; const signal = action();
  try {
    const plan = incremental.value ? {
      window_minutes: windowMinutes.value,
      ...(targetGiB.value === '' ? {} : { target_database_bytes: Number(targetGiB.value) * 2 ** 30 }),
    } : undefined;
    const value = await api.preview(mode.value, [...selectedForMode.value], signal, plan);
    if (disposed || signal.aborted) return;
    preview.value = value; confirmed.value = false; exclusiveConfirmed.value = false; expired.value = Date.parse(value.expires_at!) <= Date.now(); previewOpen.value = true;
    clearTimeout(expiryTimer); expiryTimer = setTimeout(() => { expired.value = true; }, Math.max(0, Date.parse(value.expires_at!) - Date.now()));
  } catch (e) { if (!disposed && !signal.aborted) error.value = problemText(e); }
  finally { if (!disposed && !signal.aborted) busy.value = false; }
}
async function start() {
  if (!preview.value || !confirmed.value || expired.value || pending.value || intentDamaged.value || preview.value.mode !== 'reuse' && !exclusiveConfirmed.value) return;
  const signal = action(), value = preview.value; let attemptID = '';
  try {
    const intent: MaintenanceIntent = { ...identity(), version: 1, scopeId: value.scope_id, requestId: crypto.randomUUID(), previewId: value.id, exclusiveLockConfirmed: exclusiveConfirmed.value, at: Date.now() };
    attemptID = intent.requestId;
    saveMaintenanceIntent(window.sessionStorage, intent); pending.value = intent;
    const result = await api.start(intent.requestId, intent.previewId, intent.exclusiveLockConfirmed, signal);
    if (!disposed && !signal.aborted) { accept(result); trackedID.value = result.id; previewOpen.value = false; }
  } catch (e) { if (!disposed && !signal.aborted) error.value = problemText(e); }
  finally { if (!disposed && !signal.aborted) { busy.value = false; await load(); if (attemptID && runs.value[attemptID]) error.value = ''; } }
}
async function reconcile() {
  if (!pending.value || busy.value) return; const value = pending.value, signal = action();
  try { const result = await api.run(value.requestId, signal); if (!disposed && !signal.aborted) accept(result); }
  catch (e) { if (!disposed && !signal.aborted) error.value = problemText(e); }
  finally { if (!disposed && !signal.aborted) { busy.value = false; await load(); } }
}
async function cancelPending() {
  if (!pending.value || pending.value.scopeId !== status.value?.scope_id || busy.value) return; const value = pending.value, signal = action();
  try { const result = await api.cancel(value.requestId, value.previewId, signal); if (!disposed && !signal.aborted) accept(result); }
  catch (e) { if (!disposed && !signal.aborted) error.value = problemText(e); }
  finally { if (!disposed && !signal.aborted) { busy.value = false; await load(); } }
}
async function cancelRun(run: DatabaseMaintenanceRun) { await updateRun(run, false); }
async function reviewRun(run: DatabaseMaintenanceRun) { await updateRun(run, true); }
async function updateRun(run: DatabaseMaintenanceRun, review: boolean) {
  if (busy.value) return; const signal = action();
  try { const result = review ? await api.review(run.id, signal) : await api.cancel(run.id, undefined, signal); if (!disposed && !signal.aborted) accept(result); }
  catch (e) { if (!disposed && !signal.aborted) error.value = problemText(e); }
  finally { if (!disposed && !signal.aborted) { busy.value = false; await load(); } }
}
watch(() => props.revision, () => { void load(); });
watch([mode, () => selectedForMode.value.join(',')], () => { if (!previewOpen.value) { preview.value = undefined; clearTimeout(expiryTimer); } });
onMounted(() => { try { owner = identity(); } catch { sessionChanged(); } window.addEventListener(AUTH_SESSION_EVENT, sessionChanged); void load(); });
onBeforeUnmount(() => { disposed = true; readController?.abort(); actionController?.abort(); clearTimeout(expiryTimer); window.removeEventListener(AUTH_SESSION_EVENT, sessionChanged); });
</script>
