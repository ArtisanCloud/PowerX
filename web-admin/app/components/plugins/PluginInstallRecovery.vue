<template>
  <section class="mt-4 space-y-4" aria-live="polite" data-testid="install-recovery">
    <UAlert :color="state.outcome === 'ready' ? 'success' : 'warning'" variant="soft" :title="t(`plugins.installRecovery.${state.outcome}Title`)" :description="t(`plugins.installRecovery.${state.outcome}Description`)" />
    <dl v-if="state.target" class="grid grid-cols-1 gap-3 rounded-lg border border-[var(--border-color)] p-3 text-sm sm:grid-cols-2">
      <div class="min-w-0"><dt class="text-[var(--text-secondary)]">{{ t('plugins.installRecovery.plugin') }}</dt><dd class="mt-1 break-all">{{ state.target.id }}</dd></div>
      <div><dt class="text-[var(--text-secondary)]">{{ t('plugins.installRecovery.version') }}</dt><dd class="mt-1">{{ state.target.version }}</dd></div>
    </dl>
    <p v-if="error" role="alert" class="break-words text-sm text-red-500">{{ error }}</p>
    <div class="flex flex-wrap gap-2">
      <UButton v-if="state.target && state.outcome !== 'ready'" :loading="checking" :disabled="busy" @click="$emit('check')">{{ t('plugins.installRecovery.check') }}</UButton>
      <UButton v-if="canResumeInstall(state)" :loading="busy" :disabled="checking" @click="$emit('resume')">{{ t('plugins.installRecovery.resume') }}</UButton>
      <UButton v-if="state.outcome === 'ready'" @click="$emit('done')">{{ t('plugins.installRecovery.done') }}</UButton>
      <UButton variant="outline" :disabled="busy" @click="$emit('view')">{{ t('plugins.installRecovery.view') }}</UButton>
    </div>
  </section>
</template>

<script setup lang="ts">
import { canResumeInstall, type InstallRecovery } from '~/utils/plugin-install-recovery';
defineProps<{ state: InstallRecovery; checking: boolean; busy: boolean; error?: string }>();
defineEmits<{ check: []; resume: []; done: []; view: [] }>();
const { t } = useI18n();
</script>
