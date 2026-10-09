<template>
  <UCard>
    <template #header><h2 class="font-semibold">{{ t('backupCenter.restoreHistory') }}</h2></template>
    <p class="mb-4 text-sm text-gray-500 dark:text-gray-400">{{ t('backupCenter.restoreHistoryHint') }}</p>
    <p v-if="!history.length" class="text-sm text-gray-500">{{ t('backupCenter.noRestoreHistory') }}</p>
    <details v-for="item in history" :key="String(item.id)" class="mb-2 rounded-lg border border-gray-200 p-3 text-sm dark:border-gray-700">
      <summary class="flex cursor-pointer flex-wrap items-center gap-3"><span>#{{ item.id }} · {{ t('backupCenter.job') }} #{{ item.source_job_id }}</span><UBadge :color="item.status === 'success' ? 'success' : item.status === 'failed' ? 'error' : 'neutral'">{{ item.status }}</UBadge><span>{{ item.rto_seconds }} s</span></summary>
      <dl class="mt-3 grid gap-2 sm:grid-cols-2">
        <div><dt class="text-gray-500">{{ t('backupCenter.restoreTarget') }}</dt><dd class="break-all font-mono">{{ item.restore_target_db || '-' }}</dd></div>
        <div><dt class="text-gray-500">{{ t('backupCenter.tableCount') }}</dt><dd>{{ item.restored_table_count ?? '-' }}</dd></div>
      </dl>
      <p v-if="item.status === 'success'" class="mt-3 text-sm">{{ t(item.keep_probe_db ? 'backupCenter.probeKept' : 'backupCenter.probeRemoved') }}</p>
      <p class="mt-2 break-all text-xs text-gray-500">Trace: {{ item.trace_id || '-' }}</p>
      <pre class="mt-3 max-h-64 overflow-auto whitespace-pre-wrap break-all rounded bg-gray-100 p-3 text-xs dark:bg-gray-900">{{ item.result_summary || item.report_uri }}</pre>
    </details>
    <div v-if="alerts?.length" class="mt-4 space-y-2">
      <h3 class="font-medium">{{ t('backupCenter.alerts') }}</h3>
      <div v-for="alert in alerts" :key="String(alert.id)" class="rounded border border-amber-500/30 p-3 text-sm"><p class="break-all">{{ alert.message }}</p><p class="mt-1 break-all text-xs text-gray-500">{{ alert.alert_type }} · {{ alert.level }} · Trace: {{ alert.trace_id || '-' }}</p><UButton v-if="!alert.acknowledged" class="mt-2" color="neutral" variant="outline" size="xs" :disabled="!canExecute" @click="emit('ack', alert.id)">{{ t('backupCenter.ackAlert') }}</UButton></div>
    </div>
  </UCard>
</template>
<script setup lang="ts">
import type { BackupAlert, RestoreDrillRecord } from '~/composables/api/services/backupOpsService';
defineProps<{ history: RestoreDrillRecord[]; alerts?: BackupAlert[]; canExecute: boolean }>();
const emit = defineEmits<{ ack: [id: string | number] }>();
const { t } = useI18n();
</script>
