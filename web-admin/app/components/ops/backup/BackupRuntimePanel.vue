<template>
  <UCard>
    <template #header><h2 class="text-lg font-semibold">{{ t('backupCenter.runtimeTitle') }}</h2></template>
    <p v-if="!runtime" class="text-amber-600">{{ t('backupCenter.runtimeMissing') }}</p>
    <template v-else>
      <dl class="grid gap-5 text-sm sm:grid-cols-2">
        <div><dt class="text-gray-500 dark:text-gray-400">{{ t('backupCenter.source') }}</dt><dd class="mt-1 break-all font-mono">{{ runtime.source_host }}:{{ runtime.source_port }} / {{ runtime.source_database }}</dd></div>
        <div><dt class="text-gray-500 dark:text-gray-400">{{ t('backupCenter.directory') }}</dt><dd class="mt-1 break-all font-mono">{{ runtime.artifact_directory }}</dd></div>
        <div class="sm:col-span-2"><dt class="text-gray-500 dark:text-gray-400">{{ t('backupCenter.pathTemplate') }}</dt><dd class="mt-1 break-all font-mono">{{ runtime.path_template }}</dd></div>
        <div><dt class="text-gray-500 dark:text-gray-400">{{ t('backupCenter.format') }}</dt><dd class="mt-1">{{ runtime.format }}</dd></div>
        <div><dt class="text-gray-500 dark:text-gray-400">{{ t('backupCenter.scope') }}</dt><dd class="mt-1">{{ t('backupCenter.scopeHint') }}</dd></div>
      </dl>
      <UAlert v-if="!runtime.ready" class="mt-4" color="warning" :title="t('backupCenter.notReady')" :description="runtime.problems.join(' · ')" />
      <p class="mt-4 text-sm text-gray-500 dark:text-gray-400">{{ t('backupCenter.directoryHint') }}</p>
    </template>
  </UCard>
</template>
<script setup lang="ts">
import type { BackupRuntimeSettings } from '~/composables/api/services/backupOpsService';
defineProps<{ runtime?: BackupRuntimeSettings; enabledCount?: number }>();
const { t } = useI18n();
</script>
