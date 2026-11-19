<template>
  <div class="space-y-4" data-testid="database-maintenance-run">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div><p class="font-semibold">{{ t('storageGov.dbMaintenance.modes.' + run.mode) }} · {{ t('storageGov.dbMaintenance.states.' + run.state) }}</p><p class="mt-1 text-xs text-muted">{{ new Date(run.at).toLocaleString() }}</p></div>
      <UButton v-if="['queued', 'running'].includes(run.state)" color="warning" variant="outline" :disabled="busy" data-testid="maintenance-cancel-run" @click="$emit('cancel', run)">{{ t('storageGov.dbMaintenance.cancelRun') }}</UButton>
      <UButton v-if="run.state === 'unconfirmed'" color="warning" :disabled="busy" data-testid="maintenance-review-run" @click="$emit('review', run)">{{ t('storageGov.dbMaintenance.review') }}</UButton>
    </div>
    <UAlert v-if="run.state === 'unconfirmed'" color="warning" :title="t('storageGov.dbMaintenance.unknownResult')" :description="t('storageGov.dbMaintenance.unknownHint')" />
    <p v-if="run.problem" role="alert" class="text-sm text-warning">{{ problemText(run.problem) }}</p>
    <p v-if="run.wal_stop" class="text-sm text-muted">
      {{ t('storageGov.dbMaintenance.walStop.detail', {
        phase: t('storageGov.dbMaintenance.walStop.' + run.wal_stop.phase),
        metric: t('storageGov.dbMaintenance.walStop.' + run.wal_stop.metric),
        observed: bytes(run.wal_stop.observed_bytes),
        limit: bytes(run.wal_stop.limit_bytes),
      }) }}
    </p>
    <ol v-if="run.steps.length" class="space-y-2" :aria-label="t('storageGov.dbMaintenance.progress')">
      <li v-for="step in run.steps" :key="step.table_id" class="rounded-lg border border-default p-3 text-sm">
        <div class="flex flex-wrap justify-between gap-2">
          <span>{{ tableLabel(step.table_id) }}</span><span>{{ stepLabel(step.state) }}</span>
        </div>
        <p v-if="step.incremental" class="mt-1 text-xs text-muted">
          {{ t('storageGov.dbMaintenance.incrementalProgress', { count: step.incremental.slices_completed, space: n(step.incremental.heap_reclaimed_bytes) }) }}
        </p>
      </li>
    </ol>
    <p v-else-if="['queued', 'running'].includes(run.state)" class="text-sm text-muted">{{ t('storageGov.dbMaintenance.' + (run.state === 'queued' ? 'waitingQueue' : 'preparing')) }}</p>
    <template v-if="['completed', 'cancelled', 'failed', 'unconfirmed', 'reviewed'].includes(run.state)">
      <div class="grid gap-3 sm:grid-cols-2">
        <div class="rounded-lg border border-default p-3"><p class="text-sm text-muted">{{ t('storageGov.dbMaintenance.tableChange') }}</p><p class="mt-1 font-semibold tabular-nums">{{ bytes(tableTotal(run.before)) }} → {{ bytes(run.after ? tableTotal(run.after) : undefined) }}</p><p v-if="run.mode === 'incremental' && run.after" class="mt-1 text-xs text-muted">{{ t('storageGov.dbMaintenance.tableNetChange', { bytes: netTableChange(run.before, run.after) }) }}</p></div>
        <div class="rounded-lg border border-default p-3"><p class="text-sm text-muted">{{ t('storageGov.dbMaintenance.databaseChange') }}</p><p class="mt-1 font-semibold tabular-nums">{{ bytes(run.before.database_bytes) }} → {{ bytes(run.after?.database_bytes) }}</p></div>
      </div>
      <p v-if="run.filesystem_available_before !== undefined" class="text-sm">{{ t('storageGov.dbMaintenance.diskChange') }}：{{ bytes(run.filesystem_available_before) }} → {{ bytes(run.filesystem_available_after) }}</p>
      <p class="text-xs text-muted">{{ t('storageGov.dbMaintenance.measuredHint') }}</p>
    </template>
    <details class="text-xs text-muted"><summary>{{ t('storageGov.dbMaintenance.receipt') }}</summary><p class="mt-2 break-all">{{ run.id }} · v{{ run.version }}</p></details>
  </div>
</template>
<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { DatabaseMaintenanceRun, MaintenanceSnapshot, MaintenanceTableID } from '~/composables/api/services/storageDatabaseMaintenanceService';
import { formatStorageBytes as bytes } from '~/utils/storage-capacity';
const props = defineProps<{ run: DatabaseMaintenanceRun; busy?: boolean }>();
defineEmits<{ cancel: [DatabaseMaintenanceRun]; review: [DatabaseMaintenanceRun] }>();
const { t, te, n } = useI18n();
const tableLabel = (id: MaintenanceTableID) => t('storageGov.dbMaintenance.tables.' + id);
const tableTotal = (s: MaintenanceSnapshot) => s.tables.filter(row => props.run.tables.includes(row.id)).reduce((total, row) => total + row.total_bytes, 0);
function stepLabel(state: DatabaseMaintenanceRun['steps'][number]['state']) {
  if (state === 'running' && !['running', 'cancel_requested'].includes(props.run.state)) {
    return t('storageGov.dbMaintenance.states.' + props.run.state);
  }
  return t('storageGov.dbMaintenance.stepStates.' + state);
}
function netTableChange(before: MaintenanceSnapshot, after: MaintenanceSnapshot) {
  const delta = tableTotal(after) - tableTotal(before);
  return (delta > 0 ? '+' : '') + n(delta);
}
const problemText = (code: string) => t(te('storageGov.dbMaintenance.problems.' + code) ? 'storageGov.dbMaintenance.problems.' + code : 'storageGov.dbMaintenance.requestFailed');
</script>
