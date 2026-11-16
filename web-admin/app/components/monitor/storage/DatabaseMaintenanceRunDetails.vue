<template>
  <div class="space-y-4" data-testid="database-maintenance-run">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div><p class="font-semibold">{{ t('storageGov.dbMaintenance.modes.' + run.mode) }} · {{ t('storageGov.dbMaintenance.states.' + run.state) }}</p><p class="mt-1 text-xs text-muted">{{ new Date(run.at).toLocaleString() }}</p></div>
      <UButton v-if="['queued', 'running'].includes(run.state)" color="warning" variant="outline" :disabled="busy" data-testid="maintenance-cancel-run" @click="$emit('cancel', run)">{{ t('storageGov.dbMaintenance.cancelRun') }}</UButton>
      <UButton v-if="run.state === 'unconfirmed'" color="warning" :disabled="busy" data-testid="maintenance-review-run" @click="$emit('review', run)">{{ t('storageGov.dbMaintenance.review') }}</UButton>
    </div>
    <UAlert v-if="run.state === 'unconfirmed'" color="warning" :title="t('storageGov.dbMaintenance.unknownResult')" :description="t('storageGov.dbMaintenance.unknownHint')" />
    <p v-if="run.problem" role="alert" class="text-sm text-warning">{{ problemText(run.problem) }}</p>
    <ol v-if="run.steps.length" class="space-y-2" :aria-label="t('storageGov.dbMaintenance.progress')"><li v-for="step in run.steps" :key="step.table_id" class="flex flex-wrap justify-between gap-2 rounded-lg border border-default p-3 text-sm"><span>{{ tableLabel(step.table_id) }}</span><span>{{ t('storageGov.dbMaintenance.stepStates.' + step.state) }}</span></li></ol>
    <p v-else-if="['queued', 'running'].includes(run.state)" class="text-sm text-muted">{{ t('storageGov.dbMaintenance.' + (run.state === 'queued' ? 'waitingQueue' : 'preparing')) }}</p>
    <template v-if="['completed', 'cancelled', 'failed', 'unconfirmed', 'reviewed'].includes(run.state)">
      <div class="grid gap-3 sm:grid-cols-2">
        <div class="rounded-lg border border-default p-3"><p class="text-sm text-muted">{{ t('storageGov.dbMaintenance.tableChange') }}</p><p class="mt-1 font-semibold tabular-nums">{{ bytes(tableTotal(run.before)) }} → {{ bytes(run.after ? tableTotal(run.after) : undefined) }}</p></div>
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
const { t, te } = useI18n();
const tableLabel = (id: MaintenanceTableID) => t('storageGov.dbMaintenance.tables.' + id);
const tableTotal = (s: MaintenanceSnapshot) => s.tables.filter(row => props.run.tables.includes(row.id)).reduce((total, row) => total + row.total_bytes, 0);
const problemText = (code: string) => t(te('storageGov.dbMaintenance.problems.' + code) ? 'storageGov.dbMaintenance.problems.' + code : 'storageGov.dbMaintenance.requestFailed');
</script>
