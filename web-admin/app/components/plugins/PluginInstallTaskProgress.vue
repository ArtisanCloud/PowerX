<template>
  <section class="space-y-4" data-testid="install-task" aria-live="polite">
    <UAlert :color="task.status === 'succeeded' ? 'success' : ['failed','needs_attention'].includes(task.status) ? 'warning' : 'info'"
      :title="t(cancelling ? 'plugins.installTask.cancellingTitle' : uploading ? 'plugins.installTask.uploadingTitle' : `plugins.installTask.status.${task.status}`)" :description="t(cancelling ? 'plugins.installTask.cancellingDescription' : uploading ? 'plugins.installTask.uploadingDescription' : `plugins.installTask.description.${task.status}`)" />
    <dl class="grid grid-cols-1 gap-3 rounded-xl border border-[var(--border-color)] p-4 text-sm sm:grid-cols-2">
      <div class="min-w-0"><dt class="text-[var(--text-secondary)]">{{ t('plugins.installRecovery.plugin') }}</dt><dd class="mt-1 break-all">{{ task.plugin_id || '—' }}</dd></div>
      <div><dt class="text-[var(--text-secondary)]">{{ t('plugins.installRecovery.version') }}</dt><dd class="mt-1">{{ task.version || '—' }}</dd></div>
      <div><dt class="text-[var(--text-secondary)]">{{ t('plugins.installTask.phase') }}</dt><dd class="mt-1 flex items-center gap-2"><UIcon v-if="busy || task.status === 'queued' || task.status === 'running'" name="i-lucide-loader-circle" class="animate-spin" />{{ t(cancelling ? 'plugins.installTask.cancellingTitle' : `plugins.installTask.phases.${task.phase}`) }}</dd></div>
      <div><dt class="text-[var(--text-secondary)]">{{ t('plugins.installTask.connection') }}</dt><dd class="mt-1">{{ t(`plugins.installTask.connections.${connection}`) }}</dd></div>
    </dl>
    <p v-if="task.error_code" class="break-all text-sm text-amber-500">{{ t(`plugins.installTask.errors.${task.error_code}`, t('plugins.installTask.errors.default')) }} <code>{{ task.error_code }}</code></p>
    <p v-if="error" role="alert" class="break-words text-sm text-red-500">{{ error }}</p>
    <details class="text-xs text-[var(--text-secondary)]"><summary>{{ t('plugins.installTask.receipt') }}</summary><p class="mt-2 break-all">{{ task.task_id }}</p></details>
    <div class="flex flex-wrap gap-2">
      <UButton v-if="task.status === 'awaiting_upload'" :disabled="busy" @click="$emit('upload')">{{ t('plugins.installTask.upload') }}</UButton>
      <UButton v-if="task.status === 'awaiting_upload'" variant="outline" :disabled="cancelling" :loading="cancelling" @click="$emit('discard')">{{ t('plugins.installTask.discard') }}</UButton>
      <UButton v-if="task.status === 'needs_attention' && task.installed_revision" :disabled="busy" @click="$emit('recover')">{{ t('plugins.installRecovery.check') }}</UButton>
      <UButton v-if="task.status === 'failed' || task.status === 'cancelled'" @click="$emit('retry')">{{ t('plugins.installTask.review') }}</UButton>
      <UButton v-if="task.status === 'succeeded'" @click="$emit('done')">{{ t('plugins.installRecovery.done') }}</UButton>
      <UButton variant="outline" :loading="busy" @click="$emit('check')">{{ t('plugins.installTask.refresh') }}</UButton>
      <UButton variant="ghost" @click="$emit('close')">{{ t('plugins.installTask.close') }}</UButton>
    </div>
  </section>
</template>
<script setup lang="ts">
import type { PluginInstallTask } from '~/utils/plugin-install-task';
withDefaults(defineProps<{ task: PluginInstallTask; connection: string; busy: boolean; uploading?: boolean; cancelling?: boolean; error?: string }>(), { uploading: false, cancelling: false });
defineEmits<{ upload: []; discard: []; recover: []; retry: []; done: []; check: []; close: [] }>();
const { t } = useI18n();
</script>
