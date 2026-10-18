<template>
  <div class="space-y-4">
    <UAlert v-if="runtime && !runtime.ready" color="warning" :title="t('backupCenter.notReady')" :description="runtime.problems.join(' · ')" />
    <BackupSectionTabs v-model="section" :items="sections" :label="t('backupCenter.runtimeTitle')">
      <template #source>
        <UCard>
          <template #header><h2 class="text-lg font-semibold">{{ t('backupCenter.sections.databaseSource') }}</h2></template>
          <p v-if="!runtime" class="text-amber-600">{{ t('backupCenter.runtimeMissing') }}</p>
          <dl v-else class="space-y-5 text-sm">
            <div><dt class="text-muted">{{ t('backupCenter.source') }}</dt><dd class="mt-1 break-all font-mono">{{ runtime.source_host }}:{{ runtime.source_port }} / {{ runtime.source_database }}</dd></div>
            <div><dt class="text-muted">{{ t('backupCenter.scope') }}</dt><dd class="mt-1">{{ t('backupCenter.scopeHint') }}</dd></div>
          </dl>
        </UCard>
      </template>
      <template #files>
        <UCard>
          <template #header><h2 class="text-lg font-semibold">{{ t('backupCenter.sections.fileStorage') }}</h2></template>
          <p v-if="!runtime" class="text-amber-600">{{ t('backupCenter.runtimeMissing') }}</p>
          <template v-else>
            <dl class="space-y-5 text-sm">
              <div><dt class="text-muted">{{ t('backupCenter.directory') }}</dt><dd class="mt-1 break-all font-mono">{{ runtime.artifact_directory }}</dd></div>
              <div><dt class="text-muted">{{ t('backupCenter.pathTemplate') }}</dt><dd class="mt-1 break-all font-mono">{{ runtime.path_template }}</dd></div>
              <div><dt class="text-muted">{{ t('backupCenter.format') }}</dt><dd class="mt-1">{{ runtime.format }}</dd></div>
            </dl>
            <p class="mt-5 text-sm text-muted">{{ t('backupCenter.directoryHint') }}</p>
          </template>
        </UCard>
      </template>
      <template #dependencies>
        <UCard>
          <template #header><h2 class="text-lg font-semibold">{{ t('backupCenter.sections.dependencies') }}</h2></template>
          <p v-if="!runtime" class="text-amber-600">{{ t('backupCenter.runtimeMissing') }}</p>
          <p v-else :class="runtime.ready ? 'text-emerald-700 dark:text-emerald-400' : 'text-amber-700 dark:text-amber-400'">{{ t(runtime.ready ? 'backupCenter.sections.ready' : 'backupCenter.notReady') }}</p>
          <ul v-if="runtime?.problems.length" class="mt-4 list-disc space-y-2 pl-5 text-sm"><li v-for="problem in runtime.problems" :key="problem">{{ problem }}</li></ul>
        </UCard>
      </template>
    </BackupSectionTabs>
  </div>
</template>
<script setup lang="ts">
import { computed, ref } from 'vue';
import BackupSectionTabs from './BackupSectionTabs.vue';
import type { BackupRuntimeSettings } from '~/composables/api/services/backupOpsService';
defineProps<{ runtime?: BackupRuntimeSettings; enabledCount?: number }>();
const { t } = useI18n();
const section = ref('source');
const sections = computed(() => [
  {value:'source',label:t('backupCenter.sections.databaseSource'),icon:'i-heroicons-circle-stack'},
  {value:'files',label:t('backupCenter.sections.fileStorage'),icon:'i-heroicons-folder'},
  {value:'dependencies',label:t('backupCenter.sections.dependencies'),icon:'i-heroicons-check-badge'},
]);
</script>
