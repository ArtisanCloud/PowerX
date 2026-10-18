<script setup lang="ts">
import { computed, ref } from 'vue';
import BackupSectionTabs from './BackupSectionTabs.vue';
import LogObservabilityPanel from './LogObservabilityPanel.vue';
const { t } = useI18n();
const section = ref('start');
const sections = computed(() => [
 {value:'start',label:t('backupCenter.workspace.guideStart'),icon:'i-heroicons-play'},
 {value:'files',label:t('backupCenter.workspace.filesAndRetention'),icon:'i-heroicons-archive-box'},
 {value:'verification',label:t('backupCenter.workspace.verificationHelpTitle'),icon:'i-heroicons-check-badge'},
 {value:'archives',label:t('backupCenter.archives.guideTitle'),icon:'i-heroicons-cloud-arrow-up'},
 {value:'recovery',label:t('backupCenter.recoveryTitle'),icon:'i-heroicons-arrow-uturn-left'},
 {value:'troubleshooting',label:t('backupCenter.workspace.troubleshooting'),icon:'i-heroicons-wrench-screwdriver'},
]);
const setupSteps = [
 {title:'backupCenter.snapshots.title',description:'backupCenter.onDemand.workflowHint'},
 {title:'backupCenter.onDemand.tools',description:'backupCenter.onDemand.guideOnce'},
 {title:'backupCenter.verify',description:'backupCenter.onDemand.guideVerify'},
 {title:'backupCenter.onDemand.advanced',description:'backupCenter.onDemand.advancedHint'},
];
</script>

<template>
<div data-testid="backup-guide"><BackupSectionTabs v-model="section" :items="sections" :label="t('backupCenter.workspace.guide')"><template #start><UCard>
      <template #header>
        <h2 class="text-lg font-semibold">{{ t('backupCenter.workspace.guideStart') }}</h2>
      </template>
      <ol class="grid gap-5 md:grid-cols-2">
        <li v-for="(step, index) in setupSteps" :key="step.title" class="flex gap-3">
          <span class="flex size-8 shrink-0 items-center justify-center rounded-full bg-primary-500/10 font-semibold text-primary-600 dark:text-primary-400">{{ index + 1 }}</span>
          <div>
            <h3 class="font-medium">{{ t(step.title) }}</h3>
            <p class="mt-1 text-sm leading-6 text-gray-600 dark:text-gray-300">{{ t(step.description) }}</p>
          </div>
        </li>
      </ol>
    </UCard></template>
<template #files><UCard>
        <template #header><h2 class="text-lg font-semibold">{{ t('backupCenter.workspace.filesAndRetention') }}</h2></template>
        <div class="space-y-5 text-sm leading-6">
          <div><h3 class="font-medium">{{ t('backupCenter.format') }}</h3><p class="mt-1 text-gray-600 dark:text-gray-300">{{ t('backupCenter.workspace.formatHelp') }}</p></div>
          <div><h3 class="font-medium">{{ t('backupCenter.scope') }}</h3><p class="mt-1 text-gray-600 dark:text-gray-300">{{ t('backupCenter.scopeHint') }}</p></div>
          <div><h3 class="font-medium">{{ t('backupCenter.retentionMode') }}</h3><p class="mt-1 text-gray-600 dark:text-gray-300">{{ t('backupCenter.retentionExplanation') }}</p></div>
        </div>
      </UCard></template>
<template #verification><UCard>
        <template #header><h2 class="text-lg font-semibold">{{ t('backupCenter.workspace.verificationHelpTitle') }}</h2></template>
        <div class="space-y-4 text-sm leading-6 text-gray-600 dark:text-gray-300">
          <p>{{ t('backupCenter.workspace.verificationScope') }}</p>
          <p>{{ t('backupCenter.workspace.scheduleHelp') }}</p>
          <p>{{ t('backupCenter.workspace.verificationHelp') }}</p>
        </div>
      </UCard></template>
<template #archives><UCard><template #header><h2 class="text-lg font-semibold">{{ t('backupCenter.archives.guideTitle') }}</h2></template><div class="space-y-3 text-sm leading-6 text-gray-600 dark:text-gray-300"><p>{{ t('backupCenter.archives.guideFormat') }}</p><p>{{ t('backupCenter.archives.guideSchedule') }}</p><p>{{ t('backupCenter.archives.guideRestore') }}</p></div></UCard></template>
<template #recovery><UCard>
      <template #header><h2 class="text-lg font-semibold">{{ t('backupCenter.recoveryTitle') }}</h2></template>
      <ol class="space-y-4">
        <li v-for="i in 5" :key="i" class="flex gap-3 text-sm leading-6">
          <span class="flex size-7 shrink-0 items-center justify-center rounded-full bg-gray-100 font-medium dark:bg-gray-800">{{ i }}</span>
          <p class="text-gray-600 dark:text-gray-300">{{ t(`backupCenter.recoveryStep${i}`) }}</p>
        </li>
      </ol>
      <p class="mt-5 rounded-lg bg-amber-500/10 p-3 text-sm leading-6 text-amber-800 dark:text-amber-200">{{ t('backupCenter.recoveryBoundary') }}</p>
    </UCard></template>
<template #troubleshooting><LogObservabilityPanel /></template></BackupSectionTabs></div>
</template>
