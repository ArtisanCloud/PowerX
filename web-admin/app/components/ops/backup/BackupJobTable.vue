<template>
  <div class="overflow-x-auto">
    <table class="w-full min-w-[760px] text-sm">
      <thead><tr class="border-b border-gray-200 text-left text-xs text-gray-500 dark:border-gray-700 dark:text-gray-400"><th class="py-3 pr-4">{{ t('backupCenter.job') }}</th><th class="py-3 pr-4">{{ t('backupCenter.workspace.executionResult') }}</th><th class="py-3 pr-4">{{ t('backupCenter.workspace.backupFile') }}</th><th class="py-3 pr-4">{{ t('backupCenter.size') }}</th><th class="py-3 text-right">{{ t('backupCenter.actions') }}</th></tr></thead>
      <tbody>
        <tr v-for="job in items" :key="String(job.id)" class="border-b border-gray-100 align-top last:border-0 dark:border-gray-800">
          <td class="py-4 pr-4"><p class="font-medium">#{{ job.id }} <span class="ml-2 font-normal text-gray-500 dark:text-gray-400">{{ Number(job.policy_id) === 0 ? t('backupCenter.onDemand.once') : `${t('backupCenter.policy')} #${job.policy_id}` }}</span></p><p class="mt-1 whitespace-nowrap text-xs text-gray-500 dark:text-gray-400">{{ timeLabel(job.ended_at || job.started_at) }}</p><p class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ triggerLabel(job.trigger_type) }} · {{ t('backupCenter.onDemand.databaseOnly') }}</p></td>
          <td class="py-4 pr-4"><UBadge variant="subtle" :color="job.status === 'success' ? 'success' : job.status === 'failed' ? 'error' : 'neutral'">{{ statusLabel(job.status) }}</UBadge><p v-if="job.protected" class="mt-2 max-w-32 text-xs text-emerald-700 dark:text-emerald-400">{{ t('backupCenter.protected') }}</p><p v-if="job.error_message || job.error_summary" class="mt-2 max-w-56 break-words text-xs text-red-600 dark:text-red-400">{{ job.error_message || job.error_summary }}</p></td>
          <td class="max-w-sm py-4 pr-4">
            <UBadge size="xs" variant="subtle" :color="job.storage_uri ? 'neutral' : job.status === 'success' ? 'warning' : 'neutral'" class="mb-2">{{ t(job.storage_uri ? 'backupCenter.workspace.fileRegistered' : 'backupCenter.workspace.fileUnregistered') }}</UBadge>
            <p class="break-all font-mono text-xs">{{ job.storage_uri ? backupFileName(job.storage_uri) : t(job.status === 'success' ? 'backupCenter.pruned' : 'backupCenter.noArtifact') }}</p>
            <details class="mt-2 text-xs" data-testid="backup-file-details">
              <summary class="cursor-pointer text-primary-600 dark:text-primary-400">{{ t('backupCenter.workspace.fileDetails') }}</summary>
              <dl class="mt-3 space-y-3 rounded-lg bg-gray-50 p-3 dark:bg-gray-800/50">
                <div v-if="job.storage_uri"><dt class="text-gray-500 dark:text-gray-400">{{ t('backupCenter.workspace.fullPath') }}</dt><dd class="mt-1 break-all font-mono">{{ job.storage_uri }}</dd></div>
                <div v-if="job.checksum"><dt class="text-gray-500 dark:text-gray-400">SHA256</dt><dd class="mt-1 break-all font-mono">{{ job.checksum }}</dd></div>
                <div><dt class="text-gray-500 dark:text-gray-400">Trace ID</dt><dd class="mt-1 break-all font-mono">{{ job.trace_id || '—' }}</dd></div>
              </dl>
            </details>
          </td>
          <td class="whitespace-nowrap py-4 pr-4">{{ formatSize(job.size_bytes) }}</td>
          <td class="py-4"><div v-if="canRestore(job)" class="flex min-w-40 flex-col items-end gap-2"><UButton v-if="canRecover" size="xs" :disabled="!canExecute" @click="emit('download', job)">{{ t('backupCenter.onDemand.downloadDatabase') }}</UButton><UButton v-if="canRecover" size="xs" color="primary" variant="outline" @click="emit('restore-copy', job)">{{ t('backupCenter.copies.new') }}</UButton><UButton size="xs" color="neutral" variant="outline" :disabled="!canExecute" @click="emit('restore-verify', job)">{{ t('backupCenter.verify') }}</UButton><UButton size="xs" color="neutral" variant="ghost" :disabled="!canExecute" @click="emit('protection', job)">{{ t(job.protected ? 'backupCenter.unprotect' : 'backupCenter.protect') }}</UButton></div></td>
        </tr>
        <tr v-if="!items.length"><td colspan="5" class="p-6 text-center text-gray-500">{{ t('backupCenter.noJobs') }}</td></tr>
      </tbody>
    </table>
  </div>
</template>
<script setup lang="ts">
import type { BackupJob } from '~/composables/api/services/backupOpsService';
import { backupFileName, backupStatusKey, formatBackupTime } from '~/utils/backup-display';
defineProps<{ items: BackupJob[]; canExecute: boolean; canRecover?: boolean }>();
const emit = defineEmits<{ 'restore-verify': [job: BackupJob]; 'restore-copy': [job: BackupJob]; protection: [job: BackupJob]; download: [job: BackupJob] }>();
const { t, locale } = useI18n();
const statusLabel = (status: string) => backupStatusKey(status) ? t(backupStatusKey(status)!) : status;
const triggerLabel = (type: string) => type === 'scheduled' ? t('backupCenter.workspace.scheduledTrigger') : type === 'manual' ? t('backupCenter.workspace.manualTrigger') : type || '—';
const timeLabel = (value?: string) => formatBackupTime(value, String(locale?.value || 'zh-CN'));
const canRestore = (job: BackupJob) => job.status === 'success' && !!job.storage_uri;
const formatSize = (bytes?: number) => !bytes ? '-' : bytes < 1024 ? `${bytes} B` : bytes < 1048576 ? `${(bytes / 1024).toFixed(1)} KiB` : `${(bytes / 1048576).toFixed(1)} MiB`;
</script>
