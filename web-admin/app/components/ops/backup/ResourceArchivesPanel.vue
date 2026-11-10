<template>
  <div class="space-y-6" data-testid="resource-archives">
    <div class="flex flex-wrap items-center justify-between gap-3"><h2 class="text-xl font-semibold">{{ t('backupCenter.archives.title') }}</h2><UButton color="neutral" variant="outline" icon="i-heroicons-arrow-path" :loading="loading" @click="load">{{ t('backupCenter.reload') }}</UButton></div>
    <UAlert v-if="error" color="error" :title="error" />
    <UAlert v-if="notice" color="success" :title="notice" />
    <UAlert v-if="overview?.pending_transfers" color="warning" :title="t('backupCenter.archives.pending', { count: overview.pending_transfers })" />
    <UAlert v-if="enabledPolicyCount" color="info" :title="t('backupCenter.onDemand.activeSchedules', { count: enabledPolicyCount })"><template #actions><UButton size="sm" color="neutral" variant="outline" @click="section = 'policies'">{{ t('backupCenter.onDemand.advanced') }}</UButton></template></UAlert>
    <BackupSectionTabs v-model="section" :items="sections" :label="t('backupCenter.archives.title')">
<template #export>
  <UCard data-testid="media-on-demand"><template #header><h3 class="font-semibold">{{ t('backupCenter.onDemand.mediaTitle') }}</h3></template>
    <p class="text-sm text-muted">{{ t('backupCenter.onDemand.mediaHint') }}</p><p class="mt-3 text-sm text-muted">{{ t('backupCenter.onDemand.retentionHint') }}</p>
    <p v-if="!mediaOptions.length" class="mt-4 text-sm text-warning">{{ t('backupCenter.onDemand.noMediaSource') }}</p>
    <div class="mt-6 flex flex-wrap gap-3"><UButton icon="i-heroicons-archive-box-arrow-down" :disabled="busy || !mediaOptions.length" @click="openOneTime">{{ t('backupCenter.onDemand.createMedia') }}</UButton><UButton color="neutral" variant="outline" @click="section = 'jobs'">{{ t('backupCenter.onDemand.viewFiles') }}</UButton><UButton v-if="!mediaOptions.length" color="neutral" variant="ghost" @click="section = 'sources'">{{ t('backupCenter.archives.sources') }}</UButton></div>
  </UCard>
</template>
<template #jobs>
<UCard>
      <template #header><div class="flex flex-wrap items-center justify-between gap-3"><h3 class="font-semibold">{{ t('backupCenter.archives.jobs') }}</h3><UButton color="warning" variant="outline" :disabled="busy" @click="confirm = { kind: 'cleanup' }">{{ t('backupCenter.cleanup') }}</UButton></div></template>
      <p v-if="!overview?.jobs.length" class="py-6 text-center text-sm text-gray-500">{{ t('backupCenter.noJobs') }}</p>
      <div class="space-y-4"><article v-for="job in overview?.jobs" :key="job.id" class="rounded-xl border border-gray-200 p-4 dark:border-gray-700" data-testid="archive-job">
        <div class="flex flex-wrap items-center justify-between gap-3"><div><h4 class="font-medium">{{ sourceLabel(job.source_id) }}</h4><p class="mt-1 text-xs text-gray-500">{{ timeLabel(job.created_at) }}</p></div><div class="flex gap-2"><UBadge variant="subtle" :color="job.status === 'success' ? 'success' : job.status === 'failed' ? 'error' : 'neutral'">{{ stateLabel(job.status) }}</UBadge><UBadge variant="subtle" :color="job.remote_state === 'success' ? 'success' : job.remote_state === 'failed' ? 'error' : 'neutral'">{{ remoteLabel(job.remote_state) }}</UBadge></div></div>
        <p v-if="job.kind === 'media'" class="mt-3 text-sm text-muted">{{ t(job.database ? 'backupCenter.onDemand.mediaWithDatabase' : 'backupCenter.onDemand.mediaOnly') }}<span v-if="job.one_time"> · {{ t('backupCenter.onDemand.once') }}</span></p>
        <p v-if="job.status === 'running'" class="mt-3 text-sm" role="status">{{ t('backupCenter.archives.processing') }} · {{ job.file_count }}</p>
        <p v-if="job.status !== 'retained' && (job.error || job.remote_error)" class="mt-3 break-words text-sm text-red-600 dark:text-red-400">{{ job.error || job.remote_error }}</p>
        <dl class="mt-4 grid gap-3 text-sm sm:grid-cols-4"><div><dt class="text-xs text-gray-500">{{ t('backupCenter.archives.fileCount') }}</dt><dd class="mt-1">{{ job.file_count }}</dd></div><div><dt class="text-xs text-gray-500">{{ t('backupCenter.archives.totalSize') }}</dt><dd class="mt-1">{{ sizeLabel(job.size_bytes) }}</dd></div><div><dt class="text-xs text-gray-500">{{ t('backupCenter.archives.newSize') }}</dt><dd class="mt-1">{{ sizeLabel(job.new_bytes) }}</dd></div><div v-if="job.database"><dt class="text-xs text-gray-500">{{ t('backupCenter.source') }}</dt><dd class="mt-1 break-all font-mono">{{ job.database }}</dd></div></dl>
        <ArchiveCaptureRetentionDetails v-if="job.capture_retention" :value="job.capture_retention"/>
        <details class="mt-4 text-xs"><summary class="cursor-pointer text-primary-600 dark:text-primary-400">{{ t('backupCenter.archives.details') }}</summary><dl class="mt-3 space-y-3 rounded-lg bg-gray-50 p-3 dark:bg-gray-800/50"><div><dt>{{ t('backupCenter.archives.jobID') }}</dt><dd class="mt-1 break-all font-mono">{{ job.id }}</dd></div><div><dt>{{ t('backupCenter.archives.manifest') }}</dt><dd class="mt-1 break-all font-mono">{{ overview?.directory }}/jobs/{{ job.id }}.json</dd></div><div v-if="job.remote_path"><dt>{{ t('backupCenter.archives.remotePath') }}</dt><dd class="mt-1 break-all font-mono">{{ job.remote_path }}</dd></div><div v-if="job.remote_state === 'failed'"><dt>{{ t('backupCenter.archives.attempts') }}</dt><dd>{{ job.attempts }} · {{ timeLabel(job.retry_at) }}</dd></div><p v-for="warning in job.warnings" :key="warning" class="text-amber-700 dark:text-amber-400">{{ warning }}</p></dl></details>
        <div class="mt-4 flex flex-wrap justify-end gap-2"><UBadge v-if="job.protected" color="success" variant="subtle">{{ t('backupCenter.protected') }}</UBadge><UButton v-if="canDownload(job)" size="sm" color="neutral" variant="outline" :disabled="busy" @click="perform(() => service.download(job.id))">{{ t('backupCenter.archives.download') }}</UButton><UButton v-if="job.status === 'success'" size="sm" color="neutral" variant="ghost" :disabled="busy" @click="perform(() => protect(job))">{{ t(job.protected ? 'backupCenter.unprotect' : 'backupCenter.protect') }}</UButton><UButton v-if="job.status === 'success' && job.remote_state === 'failed'" size="sm" color="warning" variant="outline" :disabled="busy" @click="perform(() => retry(job))">{{ t('backupCenter.archives.retry') }}</UButton><UButton v-if="canDownload(job) && job.kind === 'media'" size="sm" :disabled="busy" @click="confirm = { kind: 'restore', job }">{{ t(job.database ? 'backupCenter.archives.restore' : 'backupCenter.onDemand.restoreMedia') }}</UButton></div>
      </article></div>
    </UCard>
</template>
<template #policies><ArchiveGovernanceBody v-if="advancedVisited" media-mode data-testid="media-policy-control" @changed="load" /></template>
<template #logArchives><ArchiveGovernancePanel v-if="logArchivesVisited" /></template>
<template #restores>
<UCard><template #header><h3 class="font-semibold">{{ t('backupCenter.archives.restores') }}</h3></template><p v-if="!overview?.restores.length" class="py-6 text-sm text-muted">{{ t('backupCenter.sections.emptyRestores') }}</p><div class="space-y-4"><article v-for="r in overview?.restores" :key="r.id" class="rounded-xl border border-gray-200 p-4 dark:border-gray-700"><div class="flex flex-wrap justify-between gap-3"><p class="break-all font-mono text-xs">{{ r.directory }}</p><UBadge :color="r.status === 'success' ? 'success' : r.status === 'failed' ? 'error' : 'neutral'" variant="subtle">{{ stateLabel(r.status) }}</UBadge></div><p v-if="r.database_copy_name" class="mt-3 break-all text-sm">{{ t('backupCenter.archives.databaseProgress') }}: {{ r.database_copy_name }}</p><p v-if="r.error" class="mt-3 text-sm text-red-600">{{ r.error }}</p><UButton v-if="r.status !== 'running' && r.status !== 'deleted'" class="mt-3" size="sm" color="error" variant="ghost" :disabled="busy" @click="deleteRestore = r; deleteID = ''">{{ t('backupCenter.archives.deleteRestore') }}</UButton></article></div></UCard>
</template>
<template #sources>
<UCard><template #header><h3 class="font-semibold">{{ t('backupCenter.archives.sources') }}</h3></template><p class="mb-4 break-all font-mono text-xs">{{ t('backupCenter.archives.storageRoot') }}: {{ overview?.directory || '—' }}</p><div class="space-y-3"><div v-for="source in overview?.sources" :key="source.id" class="border-t border-gray-100 pt-3 dark:border-gray-800"><div class="flex justify-between gap-2"><p class="text-sm font-medium">{{ source.name }}</p><UBadge :color="source.ready ? 'success' : 'warning'" variant="subtle">{{ t(source.ready ? 'backupCenter.archives.ready' : 'backupCenter.notReady') }}</UBadge></div><p class="mt-1 break-all font-mono text-xs text-gray-500">{{ source.directory }}</p><p v-if="source.problem" class="mt-1 text-xs text-amber-700 dark:text-amber-400">{{ source.problem }}</p></div></div></UCard>
</template>
</BackupSectionTabs>

    <UModal v-model:open="oneTimeOpen" :title="t('backupCenter.onDemand.createMedia')" :description="t('backupCenter.onDemand.mediaHint')"><template #body><UForm id="one-time-media-form" :state="oneTimeForm" :schema="oneTimeSchema" class="space-y-4" @submit="perform(createOneTime)"><UFormField name="source_id" :label="t('backupCenter.source')" required><USelect v-model="oneTimeForm.source_id" :items="mediaOptions" class="w-full" /></UFormField><p class="text-sm text-muted">{{ t('backupCenter.onDemand.retentionHint') }}</p><p v-if="modalError" class="text-sm text-error">{{ modalError }}</p></UForm></template><template #footer><div class="flex w-full justify-end gap-2"><UButton color="neutral" variant="ghost" :disabled="busy" @click="oneTimeOpen = false">{{ t('backupCenter.cancel') }}</UButton><UButton type="submit" form="one-time-media-form" :loading="busy" :disabled="!oneTimeForm.source_id">{{ t('backupCenter.confirm') }}</UButton></div></template></UModal>


    <UModal v-if="confirm" :open="!!confirm" :title="t(confirm?.kind === 'restore' ? 'backupCenter.archives.restore' : 'backupCenter.cleanup')" :description="t(confirm?.kind === 'restore' ? 'backupCenter.archives.restoreHint' : 'backupCenter.archives.cleanupHint')" @update:open="open => { if (!open) confirm = null }"><template #body><div class="space-y-4"><p v-if="confirm?.kind === 'restore'" class="break-all font-mono text-xs">{{ confirm.job.id }}</p><UCheckbox v-if="confirm?.kind === 'restore'" v-model="allowOlder" :label="t('backupCenter.archives.olderConsent')" /><p v-if="modalError" class="text-sm text-red-600">{{ modalError }}</p></div></template><template #footer><div class="flex w-full justify-end gap-2"><UButton color="neutral" variant="ghost" :disabled="busy" @click="confirm = null">{{ t('backupCenter.cancel') }}</UButton><UButton :loading="busy" @click="perform(confirmAction)">{{ t('backupCenter.confirm') }}</UButton></div></template></UModal>
    <UModal v-if="deleteRestore" :open="!!deleteRestore" :title="t('backupCenter.archives.deleteRestore')" :description="t('backupCenter.archives.deleteHint')" @update:open="open => { if (!open) deleteRestore = undefined }"><template #body><div class="space-y-4"><p class="break-all font-mono text-xs">{{ deleteRestore?.id }}</p><UFormField :label="t('backupCenter.archives.typeRestoreID')"><UInput v-model="deleteID" class="w-full" /></UFormField><p v-if="modalError" class="text-sm text-red-600">{{ modalError }}</p></div></template><template #footer><div class="flex w-full justify-end gap-2"><UButton color="neutral" variant="ghost" @click="deleteRestore = undefined">{{ t('backupCenter.cancel') }}</UButton><UButton color="error" :disabled="deleteID !== deleteRestore?.id" :loading="busy" @click="perform(removeRestore)">{{ t('backupCenter.archives.deleteRestore') }}</UButton></div></template></UModal>
  </div>
</template>
<script setup lang="ts">
import ArchiveCaptureRetentionDetails from './ArchiveCaptureRetentionDetails.vue';
import ArchiveGovernancePanel from './ArchiveGovernancePanel.vue';
import ArchiveGovernanceBody from './ArchiveGovernanceBody.vue';
import BackupSectionTabs from './BackupSectionTabs.vue';
import { computed, onMounted, onBeforeUnmount, reactive, ref, watch } from 'vue';
import * as v from 'valibot';
import { useArchiveOpsService, type ArchiveOverview, type ArchiveJob, type ArchiveRestore } from '~/composables/api/services/archiveOpsService';
import { formatBackupTime } from '~/utils/backup-display';
import { useBackupEvents } from '~/composables/useBackupEvents';
const { t, locale } = useI18n(); const service = useArchiveOpsService();
const section = ref('export');
const sections = computed(() => [
  {value:'export',label:t('backupCenter.onDemand.tools'),icon:'i-heroicons-arrow-down-tray'},
  {value:'jobs',label:t('backupCenter.archives.jobs'),icon:'i-heroicons-archive-box'},
  {value:'logArchives',label:t('storageGov.archive.title'),icon:'i-heroicons-document-magnifying-glass'},
  {value:'restores',label:t('backupCenter.archives.restores'),icon:'i-heroicons-arrow-uturn-left'},
  {value:'sources',label:t('backupCenter.archives.sources'),icon:'i-heroicons-cog-6-tooth'},
  {value:'policies',label:t('backupCenter.onDemand.advanced'),icon:'i-heroicons-adjustments-horizontal',count:enabledPolicyCount.value},
]);
const overview = ref<ArchiveOverview>(); const error = ref(''); const notice = ref(''); const modalError = ref(''); const loading = ref(false); const busy = ref(false);
const logArchivesVisited = ref(false),advancedVisited=ref(false);
watch(section, value => { if (value === 'logArchives') logArchivesVisited.value = true;if(value==='policies')advancedVisited.value=true; });
const enabledPolicyCount = computed(() => overview.value?.policies.filter(policy => policy.enabled && policy.execution_ready && overview.value?.sources.some(s=>s.id===policy.source_id&&s.kind==='media')).length || 0);
const mediaOptions = computed(() => (overview.value?.sources || []).filter(source => source.kind === 'media' && source.ready).map(source => ({value:source.id,label:source.name})));
const oneTimeOpen = ref(false); const oneTimeKey = ref(''); const oneTimeForm = reactive({source_id:''});
const oneTimeSchema = v.object({source_id:v.pipe(v.string(),v.minLength(1))});
const allowOlder = ref(false);
const confirm = ref<{ kind: 'restore'; job: ArchiveJob } | { kind: 'cleanup' } | null>(null);
const deleteRestore = ref<ArchiveRestore>(); const deleteID = ref('');
const sourceLabel = (id: string) => overview.value?.sources.find(source => source.id === id)?.name || id;

const sizeLabel = (size: number) => size < 1024 ? `${size} B` : size < 1048576 ? `${(size / 1024).toFixed(1)} KiB` : size < 1073741824 ? `${(size / 1048576).toFixed(1)} MiB` : `${(size / 1073741824).toFixed(1)} GiB`;
const timeLabel = (time?: string) => formatBackupTime(time, String(locale?.value || 'zh-CN'));
const stateLabel = (state: string) => ['running', 'success', 'failed', 'pruned', 'deleted', 'interrupted', 'retained'].includes(state) ? t(`backupCenter.archives.state.${state}`) : state;
const remoteLabel = (state: string) => ['pending', 'success', 'failed', 'not_requested'].includes(state) ? t(`backupCenter.archives.remoteState.${state}`) : state;
const canDownload = (job: ArchiveJob) => job.status === 'success' || (job.status === 'pruned' && job.remote_state === 'success');
const failure = (e: any) => { const data = e?.response?._data || e?.data; return [data?.message || e?.message || t('backupCenter.failed'), data?.error].filter(Boolean).join(' · '); };
let disposed = false; let revision = 0; let pendingRefresh = false;
async function load() { if (disposed) return; if (loading.value) { pendingRefresh = true; return; } loading.value = true; const requestedRevision = revision; try { const data = await service.overview(); if (!disposed && requestedRevision === revision) { overview.value = data; error.value = ''; } } catch (e) { if (!disposed && requestedRevision === revision) error.value = failure(e); } finally { loading.value = false; if (pendingRefresh && !disposed) { pendingRefresh = false; void load(); } } }
async function perform(action: () => Promise<unknown>) { if (busy.value) return; busy.value = true; error.value = ''; modalError.value = ''; notice.value = ''; try { await action(); } catch (e) { error.value = failure(e); modalError.value = error.value; } finally { busy.value = false; } }
function openOneTime() { if (!mediaOptions.value.some(source => source.value === oneTimeForm.source_id)) oneTimeForm.source_id = mediaOptions.value[0]?.value || ''; oneTimeKey.value = crypto.randomUUID(); modalError.value = ''; oneTimeOpen.value = true; }
async function createOneTime() { const row = await service.createOneTime(oneTimeForm.source_id, oneTimeKey.value); revision++; section.value = 'jobs'; if (overview.value) overview.value.jobs = [row,...overview.value.jobs.filter(job => job.id !== row.id)]; oneTimeOpen.value = false; notice.value = t('backupCenter.onDemand.mediaStarted'); await load(); }
async function retry(job: ArchiveJob) { await service.retry(job.id); revision++; await load(); }
async function protect(job: ArchiveJob) { await service.protect(job.id, !job.protected); revision++; await load(); }

async function confirmAction() { if (!confirm.value) return; const action = confirm.value; if (action.kind === 'restore') { section.value = 'restores'; const row = await service.restore(action.job.id, allowOlder.value); revision++; if (overview.value) overview.value.restores.unshift(row); } else await service.cleanup(); confirm.value = null; await load(); }
watch(confirm, current => { allowOlder.value = false; modalError.value = ''; });
async function removeRestore() { if (!deleteRestore.value || deleteID.value !== deleteRestore.value.id) return; await service.removeRestore(deleteRestore.value.id, deleteID.value); deleteRestore.value = undefined; await load(); }
useBackupEvents("archives", () => { void load(); });
onMounted(load); onBeforeUnmount(() => { disposed = true; });
</script>
