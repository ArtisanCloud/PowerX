<template>
  <div class="overflow-x-auto">
    <table class="min-w-full text-sm">
      <thead><tr class="border-b border-gray-200 text-left text-gray-500 dark:border-gray-700 dark:text-gray-400"><th class="p-2">{{ t('backupCenter.job') }}</th><th class="p-2">{{ t('backupCenter.status') }}</th><th class="p-2">{{ t('backupCenter.artifact') }}</th><th class="p-2">{{ t('backupCenter.size') }}</th><th class="p-2">{{ t('backupCenter.actions') }}</th></tr></thead>
      <tbody>
        <tr v-for="job in items" :key="String(job.id)" class="border-b border-gray-100 align-top dark:border-gray-800">
          <td class="p-2"><p>#{{ job.id }} · {{ t('backupCenter.policy') }} #{{ job.policy_id }}</p><p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ job.ended_at || job.started_at }}</p><p class="mt-1 max-w-64 break-all text-xs text-gray-500 dark:text-gray-400">Trace: {{ job.trace_id || '-' }}</p></td>
          <td class="p-2"><UBadge :color="job.status === 'success' ? 'success' : job.status === 'failed' ? 'error' : 'neutral'">{{ job.status }}</UBadge><p v-if="job.protected" class="mt-2 text-xs text-emerald-600">{{ t('backupCenter.protected') }}</p><p v-if="job.error_message || job.error_summary" class="mt-2 max-w-64 break-all text-xs text-red-600">{{ job.error_message || job.error_summary }}</p></td>
          <td class="max-w-xl p-2"><p class="break-all font-mono text-xs">{{ job.storage_uri || t(job.status === 'success' ? 'backupCenter.pruned' : 'backupCenter.noArtifact') }}</p><details v-if="job.checksum" class="mt-2 text-xs"><summary class="cursor-pointer">SHA256 · .dump</summary><p class="mt-1 break-all font-mono">{{ job.checksum }}</p></details></td>
          <td class="whitespace-nowrap p-2">{{ formatSize(job.size_bytes) }}</td>
          <td class="p-2"><div v-if="canRestore(job)" class="flex min-w-36 flex-col gap-2"><UButton size="xs" color="neutral" variant="outline" :disabled="!canExecute" @click="emit('restore-verify', job)">{{ t('backupCenter.verify') }}</UButton><UButton size="xs" color="neutral" variant="outline" :disabled="!canExecute" @click="emit('protection', job)">{{ t(job.protected ? 'backupCenter.unprotect' : 'backupCenter.protect') }}</UButton></div></td>
        </tr>
        <tr v-if="!items.length"><td colspan="5" class="p-6 text-center text-gray-500">{{ t('backupCenter.noJobs') }}</td></tr>
      </tbody>
    </table>
  </div>
</template>
<script setup lang="ts">
import type { BackupJob } from '~/composables/api/services/backupOpsService';
defineProps<{ items: BackupJob[]; canExecute: boolean }>();
const emit = defineEmits<{ 'restore-verify': [job: BackupJob]; protection: [job: BackupJob] }>();
const { t } = useI18n();
const canRestore = (job: BackupJob) => job.status === 'success' && !!job.storage_uri;
const formatSize = (bytes?: number) => !bytes ? '-' : bytes < 1024 ? `${bytes} B` : bytes < 1048576 ? `${(bytes / 1024).toFixed(1)} KiB` : `${(bytes / 1048576).toFixed(1)} MiB`;
</script>
