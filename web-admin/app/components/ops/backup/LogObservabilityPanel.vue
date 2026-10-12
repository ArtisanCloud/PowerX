<template>
  <UCard>
    <template #header><h2 class="text-lg font-semibold">{{ t('backupCenter.workspace.troubleshooting') }}</h2></template>
    <ul class="list-disc space-y-3 pl-5 text-sm leading-6 text-gray-600 dark:text-gray-300">
      <li>{{ t('backupCenter.workspace.troubleshootJob') }}</li>
      <li>{{ t('backupCenter.workspace.troubleshootRuntime') }}</li>
      <li>{{ t('backupCenter.workspace.troubleshootTrace') }}</li>
    </ul>
  </UCard>
</template>
<script setup lang="ts">
const { t } = useI18n();
</script>
