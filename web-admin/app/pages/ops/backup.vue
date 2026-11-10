<template>
  <section class="mx-auto max-w-7xl space-y-6 p-4 text-gray-900 sm:p-6 dark:text-gray-100">
    <header class="flex flex-wrap items-center justify-between gap-4">
      <div>
        <h1 class="text-2xl font-semibold">{{ t('backupCenter.title') }}</h1>
        <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ t('backupCenter.onDemand.workflowHint') }}</p>
      </div>
      <UButton icon="i-heroicons-arrow-path" color="neutral" variant="outline" :loading="busy" :disabled="!isRoot" @click="perform(refreshCurrentView)">{{ t('backupCenter.reload') }}</UButton>
    </header>
    <UAlert v-if="permissionHint" color="warning" :title="permissionHint" />
    <UAlert v-if="error" color="error" :title="error" />
    <UAlert v-if="notice" color="success" :title="notice" />
    <UAlert v-if="isRoot && backupEventStatus === 'disconnected'" color="warning" :title="t('backupCenter.realtimeDisconnected')" />
    <UAlert v-if="isRoot && (cleanupAttention.problem || cleanupAttention.count)" color="warning" :title="t(cleanupAttention.problem ? 'backupReview.attentionFailed' : 'backupReview.attention', { count: cleanupAttention.count })" data-testid="cleanup-attention">
      <template #actions><UButton color="neutral" variant="outline" @click="activeView = 'operations'; databaseSection = 'cleanup_reviews'">{{ t('backupReview.view') }}</UButton></template>
    </UAlert>
    <UAlert v-if="isRoot && (inventoryAttention.problem || inventoryAttention.count)" color="warning" :title="t(inventoryAttention.problem?'quarantineInventory.attentionFailed':'quarantineInventory.coverageGap',{count:inventoryAttention.count})" data-testid="inventory-attention"><template #actions><UButton color="neutral" variant="outline" @click="activeView='operations';databaseSection='quarantine_inventory'">{{t('quarantineInventory.title')}}</UButton></template></UAlert>
    <UAlert v-if="linkedTask" :color="linkedError ? 'warning' : 'info'" :title="t(linkedLoading ? 'backupCenter.taskLink.loading' : linkedError || 'backupCenter.taskLink.focused')" data-testid="backup-linked-task">
      <template #actions><UButton color="neutral" variant="outline" @click="clearTaskLink">{{ t('backupCenter.taskLink.showAll') }}</UButton></template>
    </UAlert>

    <UTabs :unmount-on-hide="false" v-if="contextLoaded && isRoot" v-model="activeView" :items="viewItems" variant="link" :ui="{ list: 'overflow-x-auto justify-start border-b border-gray-200 dark:border-gray-700', trigger: 'shrink-0 flex-none', content: 'pt-6' }">
      <template #snapshots><SystemSnapshotsPanel v-if="isRoot && visitedViews.has('snapshots')" ref="systemSnapshots" /><UAlert v-else color="warning" :title="t('backupCenter.archives.rootOnly')" /></template>
      <template #operations>
        <div v-if="visitedViews.has('operations')" class="space-y-6" data-testid="backup-operations">
          <UAlert v-if="overview?.runtime && !overview.runtime.ready" color="warning" :title="t('backupCenter.notReady')" :description="overview.runtime.problems.join(' · ')" />
<UAlert v-if="enabledPolicyCount" color="info" :title="t('backupCenter.onDemand.activeSchedules', { count: enabledPolicyCount })"><template #actions><UButton size="sm" color="neutral" variant="outline" @click="databaseSection = 'policies'">{{ t('backupCenter.onDemand.advanced') }}</UButton></template></UAlert>
<BackupSectionTabs v-model="databaseSection" :items="databaseSections" :label="t('backupCenter.workspace.operations')">
<template #receipt_history><BackupRetentionHistory v-if="isRoot" /></template>
<template #quarantine_inventory><BackupQuarantineInventory @attention="inventoryAttention=$event" ref="quarantineInventory" v-if="isRoot && quarantineVisited" /></template>
<template #cleanup_reviews><BackupCleanupReviews v-if="isRoot" @attention="cleanupAttention = $event" @open-job="id => navigateTo({ path: route.path, query: { job_id: id } })" /></template>
<template #export>
  <UCard data-testid="database-on-demand">
    <template #header><h2 class="text-lg font-semibold">{{ t('backupCenter.onDemand.databaseTitle') }}</h2></template>
    <p class="text-sm text-muted">{{ t('backupCenter.onDemand.databaseHint') }}</p>
    <p class="mt-3 text-sm text-muted">{{ t('backupCenter.onDemand.retentionHint') }}</p>
    <p v-if="!isRoot" class="mt-3 text-sm text-warning">{{ t('backupCenter.archives.rootOnly') }}</p>
    <div class="mt-6 flex flex-wrap gap-3"><UButton icon="i-heroicons-circle-stack" :disabled="!canExecute || !isRoot || busy || !overview?.runtime?.ready" @click="confirmation = { kind: 'once' }">{{ t('backupCenter.onDemand.createDatabase') }}</UButton><UButton color="neutral" variant="outline" @click="databaseSection = 'files'">{{ t('backupCenter.onDemand.viewFiles') }}</UButton></div>
  </UCard>
</template>
<template #files>
<UCard>
            <template #header>
              <div class="flex flex-wrap items-center justify-between gap-3">
                <h2 class="text-lg font-semibold">{{ t('backupCenter.jobs') }}</h2>
                <UBadge color="neutral" variant="subtle">{{ linkedTask?.kind === 'job' ? displayedJobs.length : jobsTotal }}</UBadge>
              </div>
              <div v-if="linkedTask?.kind !== 'job'" class="mt-4 flex flex-wrap gap-2" role="group" :aria-label="t('backupCenter.workspace.jobViews')" data-testid="backup-job-views">
                <UButton v-for="view in jobViewItems" :key="view.value" size="sm" :variant="jobsView === view.value ? 'solid' : 'outline'" :color="jobsView === view.value ? 'primary' : 'neutral'" :aria-pressed="jobsView === view.value" :disabled="busy" @click="perform(() => changeJobView(view.value))">{{ view.label }}</UButton>
              </div>
              <p v-if="linkedTask?.kind !== 'job'" class="mt-3 text-sm text-gray-500 dark:text-gray-400">{{ t(jobsView === 'available' ? 'backupCenter.workspace.availableHint' : 'backupCenter.workspace.historyHint') }}</p>
            </template>
            <BackupJobTable v-if="!linkedLoading && !(linkedTask?.kind === 'job' && linkedError)" :items="displayedJobs" :can-execute="canExecute && !busy" :can-recover="isRoot" @restore-copy="openRestoreCopy" @restore-verify="job => confirmation = { kind: 'restore', job }" @protection="job => perform(() => protect(job))" @download="job => perform(() => backup.downloadJob(job.id))" />
            <template #footer>
              <div v-if="linkedTask?.kind !== 'job'" class="flex flex-wrap items-center justify-between gap-3 text-sm text-gray-500 dark:text-gray-400">
                <span>{{ t('backupCenter.page', { page: jobsPage, total: jobsTotal }) }}</span>
                <div class="flex gap-2"><UButton color="neutral" variant="outline" :disabled="jobsPage <= 1 || busy" @click="perform(() => changePage(-1))">{{ t('backupCenter.previous') }}</UButton><UButton color="neutral" variant="outline" :disabled="jobsPage * 20 >= jobsTotal || busy" @click="perform(() => changePage(1))">{{ t('backupCenter.next') }}</UButton></div>
              </div>
            </template>
          </UCard>
</template>
<template #policies><DatabaseBackupPolicies v-if="isRoot" @changed="perform(load)" /></template>
<template #copies>
<RestoreCopiesPanel v-if="isRoot" ref="restoreCopies" :jobs="jobs" />
</template>
<template #verification>
<RestoreDrillPanel v-if="!linkedLoading && !(linkedTask?.kind === 'drill' && linkedError)" :history="displayedDrills" :focused-id="linkedDrill?.id" :alerts="linkedTask?.kind === 'drill' ? [] : alerts" :can-execute="canExecute && !busy" @ack="id => perform(() => ackAlert(id))" />
</template>
<template #overview>
<div class="grid grid-cols-2 gap-4 sm:grid-cols-3">
            <div class="rounded-xl border border-gray-200 p-4 dark:border-gray-700">
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('backupCenter.workspace.automaticBackups') }}</p>
              <p class="mt-2 text-lg font-semibold" :class="enabledPolicyCount ? 'text-emerald-700 dark:text-emerald-400' : 'text-amber-700 dark:text-amber-400'">{{ overview ? (enabledPolicyCount ? t('backupCenter.workspace.enabledPolicies', { count: enabledPolicyCount }) : t('backupCenter.workspace.summaryScheduleOff')) : '—' }}</p>
            </div>
            <div class="rounded-xl border border-gray-200 p-4 dark:border-gray-700">
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('backupCenter.workspace.lastSuccessfulBackup') }}</p>
              <p class="mt-2 text-base font-semibold">{{ timeLabel(overview?.last_success_at) }}</p>
            </div>
            <div class="col-span-2 rounded-xl border border-gray-200 p-4 sm:col-span-1 dark:border-gray-700">
              <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('backupCenter.source') }}</p>
              <p class="mt-2 break-all font-mono text-base font-semibold">{{ overview?.runtime?.source_database || '—' }}</p>
            </div>
          </div>
</template>
<template #maintenance>
<UCard>
            <div class="flex flex-wrap items-center justify-between gap-4">
              <div><h2 class="font-semibold">{{ t('backupCenter.workspace.fileMaintenance') }}</h2><p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('backupCenter.workspace.cleanupBrief') }}</p></div>
              <UButton color="warning" variant="outline" :disabled="!canExecute || busy" @click="confirmation = { kind: 'cleanup' }">{{ t('backupCenter.cleanup') }}</UButton>
            </div>
          </UCard>
</template>
</BackupSectionTabs>
        </div>
      </template>
      <template #archives><ResourceArchivesPanel v-if="isRoot && visitedViews.has('archives')" /><UAlert v-else color="warning" :title="t('backupCenter.archives.rootOnly')" /></template>
      <template #configuration>
        <div class="space-y-6" data-testid="backup-configuration"><BackupRuntimePanel v-if="visitedViews.has('configuration')" :runtime="overview?.runtime" /></div>
      </template>
      <template #guide>
        <BackupGuidePanel v-if="visitedViews.has('guide')" />
      </template>
    </UTabs>

    <UModal :open="!!confirmation" :title="t(confirmation?.kind === 'once' ? 'backupCenter.onDemand.createDatabase' : confirmation?.kind === 'cleanup' ? 'backupCenter.cleanup' : 'backupCenter.verify')" :description="t(confirmation?.kind === 'once' ? 'backupCenter.onDemand.databaseHint' : confirmation?.kind === 'cleanup' ? 'backupCenter.cleanupConfirm' : 'backupCenter.verifyConfirm')" @update:open="value => { if (!value) confirmation = null }">
      <template #footer><div class="flex w-full justify-end gap-2"><UButton color="neutral" variant="ghost" @click="confirmation = null">{{ t('backupCenter.cancel') }}</UButton><UButton :loading="busy" @click="perform(confirmAction)">{{ t('backupCenter.confirm') }}</UButton></div></template>
    </UModal>
  </section>
</template>

<script setup lang="ts">
import DatabaseBackupPolicies from "~/components/ops/backup/DatabaseBackupPolicies.vue";
import { computed, nextTick, onMounted, ref, watch } from 'vue';
import { useBackupOpsService, type BackupPolicy, type BackupJob, type BackupOverview, type RestoreDrillRecord, type BackupAlert } from '~/composables/api/services/backupOpsService';
import { useOpsAccess } from '~/composables/useOpsAccess';
import BackupJobTable from '~/components/ops/backup/BackupJobTable.vue';
import BackupRuntimePanel from '~/components/ops/backup/BackupRuntimePanel.vue';
import RestoreDrillPanel from '~/components/ops/backup/RestoreDrillPanel.vue';
import BackupGuidePanel from '~/components/ops/backup/BackupGuidePanel.vue';
import { formatBackupTime } from '~/utils/backup-display';
import BackupSectionTabs from '~/components/ops/backup/BackupSectionTabs.vue';
import RestoreCopiesPanel from '~/components/ops/backup/RestoreCopiesPanel.vue';
import ResourceArchivesPanel from '~/components/ops/backup/ResourceArchivesPanel.vue';
import SystemSnapshotsPanel from '~/components/ops/backup/SystemSnapshotsPanel.vue';
import { useBackupEvents } from '~/composables/useBackupEvents';
import BackupRetentionHistory from '~/components/ops/backup/BackupRetentionHistory.vue';
import BackupCleanupReviews from '~/components/ops/backup/BackupCleanupReviews.vue';
import BackupQuarantineInventory from '~/components/ops/backup/BackupQuarantineInventory.vue';
const { status: backupEventStatus } = useBackupEvents();
const { t, locale } = useI18n();
const activeView = ref('operations');
const visitedViews = ref(new Set(['operations']));
watch(activeView,value=>visitedViews.value.add(value),{immediate:true});
const databaseSection = ref('export');
const quarantineInventory=ref<{refresh:()=>Promise<unknown>}|null>(null);
const quarantineVisited=ref(false);watch(databaseSection,v=>{if(v==='quarantine_inventory')quarantineVisited.value=true});
const inventoryAttention=ref({count:0,problem:false});
const cleanupAttention = ref<{ count: number | null; problem: boolean }>({ count: null, problem: false });
const databaseSections = computed(() => [
  { value: 'export', label: t('backupCenter.onDemand.tools'), icon: 'i-heroicons-arrow-down-tray' },
  { value: 'files', label: t('backupCenter.sections.files'), icon: 'i-heroicons-circle-stack' },
  { value: 'receipt_history', label: t('backupHistory.title'), icon: 'i-heroicons-document-check' },
  { value: 'cleanup_reviews', label: t('backupReview.title'), icon: 'i-heroicons-shield-exclamation', count: cleanupAttention.value.count || 0 },
  { value:'quarantine_inventory', label:t('quarantineInventory.title'),icon:'i-heroicons-archive-box-x-mark' },
  ...(isRoot?.value ? [{ value: 'copies', label: t('backupCenter.copies.title'), icon: 'i-heroicons-document-duplicate' }] : []),
  { value: 'verification', label: t('backupCenter.sections.verification'), icon: 'i-heroicons-check-badge' },
  { value: 'overview', label: t('backupCenter.sections.overview'), icon: 'i-heroicons-chart-bar' },
  { value: 'maintenance', label: t('backupCenter.workspace.fileMaintenance'), icon: 'i-heroicons-wrench-screwdriver' },
  { value: 'policies', label: t('backupCenter.onDemand.advanced'), icon: 'i-heroicons-adjustments-horizontal', count: enabledPolicyCount.value },
]);
const viewItems = computed(() => [
  ...(isRoot?.value ? [{ value: 'snapshots', slot: 'snapshots', label: t('backupCenter.snapshots.title'), icon: 'i-heroicons-camera' }] : []),
  { value: 'operations', slot: 'operations', label: t('backupCenter.workspace.operations'), icon: 'i-heroicons-command-line' },
  { value: 'archives', slot: 'archives', label: t('backupCenter.archives.tab'), icon: 'i-heroicons-cloud-arrow-up' },
  { value: 'configuration', slot: 'configuration', label: t('backupCenter.workspace.configuration'), icon: 'i-heroicons-adjustments-horizontal' },
  { value: 'guide', slot: 'guide', label: t('backupCenter.workspace.guide'), icon: 'i-heroicons-book-open' },
]);
const backup = useBackupOpsService();
const { isRoot, loadUserContext } = useOpsAccess();
const canExecute = computed(() => isRoot.value);
const permissionHint = computed(() => contextLoaded.value && !isRoot.value ? t('backupCenter.platformRootOnly') : '');
const restoreCopies = ref<InstanceType<typeof RestoreCopiesPanel>>();
const systemSnapshots = ref<InstanceType<typeof SystemSnapshotsPanel>>();
const overview = ref<BackupOverview>();
const policies = ref<BackupPolicy[]>([]);
const jobs = ref<BackupJob[]>([]);
const route = useRoute();
const linkedTask = computed(() => {
  const query = route.query || {};
  if (query.job_id) return { kind: 'job' as const, id: query.job_id };
  if (query.drill_id) return { kind: 'drill' as const, id: query.drill_id };
  return null;
});
const contextLoaded = ref(false);
let accessGeneration = 0;
watch(isRoot, root => {
  accessGeneration++;
  if (!root) { cleanupAttention.value = { count: null, problem: false }; overview.value = undefined; policies.value = []; jobs.value = []; drills.value = []; alerts.value = []; linkedJob.value = undefined; linkedDrill.value = undefined; confirmation.value = null; error.value = ''; notice.value = ''; }
  else if (contextLoaded.value) void perform(load);
});
const linkedJob = ref<BackupJob>();
const linkedDrill = ref<RestoreDrillRecord>();
const linkedLoading = ref(false);
const linkedError = ref('');
const linkRetry = ref(0);
const displayedJobs = computed(() => linkedTask.value?.kind === 'job' ? (linkedJob.value ? [linkedJob.value] : []) : jobs.value);
const displayedDrills = computed(() => linkedTask.value?.kind === 'drill' ? (linkedDrill.value ? [linkedDrill.value] : []) : drills.value);
watch([linkedTask, contextLoaded, linkRetry, isRoot], async ([task, ready], _previous, onCleanup) => {
  let current = true;
  onCleanup(() => { current = false; });
  linkedJob.value = undefined; linkedDrill.value = undefined; linkedError.value = '';
  linkedLoading.value = Boolean(task);
  if (!task) return;
  activeView.value = 'operations';
  databaseSection.value = task.kind === 'job' ? 'files' : 'verification';
  if (!ready || !isRoot.value) { linkedLoading.value = false; return; }
  if (typeof task.id !== 'string' || !/^[1-9]\d*$/.test(task.id) || (route.query?.job_id && route.query?.drill_id)) {
    linkedError.value = 'backupCenter.taskLink.unavailable'; linkedLoading.value = false; return;
  }
  try {
    const item = task.kind === 'job' ? await backup.getJob(task.id) : await backup.getRestoreDrill(task.id);
    if (!current) return;
    if (!item || String(item.id) !== task.id) { linkedError.value = 'backupCenter.taskLink.unavailable'; return; }
    if (task.kind === 'job') linkedJob.value = item as BackupJob;
    else linkedDrill.value = item as RestoreDrillRecord;
  } catch (e: any) {
    if (!current) return;
    const status = e?.statusCode || e?.status || e?.response?.status;
    linkedError.value = status === 404 || status === 403 ? 'backupCenter.taskLink.unavailable' : 'backupCenter.taskLink.failed';
  } finally { if (current) linkedLoading.value = false; }
}, { immediate: true });
function clearTaskLink() {
  const { job_id: _job, drill_id: _drill, ...query } = route.query || {};
  return navigateTo({ path: route.path, query }, { replace: true });
}
const enabledPolicyCount = computed(() => policies.value.filter(policy => policy.enabled && policy.is_current).length);
const timeLabel = (value?: string) => formatBackupTime(value, String(locale.value || 'zh-CN'));
const drills = ref<RestoreDrillRecord[]>([]);
const alerts = ref<BackupAlert[]>([]);
const jobsPage = ref(1);
const jobsTotal = ref(0);
const jobsView = ref<'available' | 'history'>('available');
const jobViewItems = computed(() => [
  { value: 'available' as const, label: t('backupCenter.workspace.availableBackups') },
  { value: 'history' as const, label: t('backupCenter.workspace.executionHistory') },
]);
const busy = ref(false);
const error = ref('');
const notice = ref('');
const confirmation = ref<{ kind: 'once' } | { kind: 'cleanup' } | { kind: 'restore'; job: BackupJob } | null>(null);
async function perform(action: () => Promise<unknown>) {
  if (busy.value) return;
  const generation = accessGeneration;
  busy.value = true; error.value = ''; notice.value = '';
  try { await action(); } catch (e: any) {
    if (generation !== accessGeneration) return;
    const data = e?.response?._data || e?.data;
    error.value = [data?.message || e?.message || t('backupCenter.failed'), data?.error, data?.trace_id || data?.request_id].filter(Boolean).join(' · ');
  } finally { busy.value = false; }
}
async function refreshCurrentView() {
  if(activeView.value==='operations' && databaseSection.value==='quarantine_inventory'){await quarantineInventory.value?.refresh();return;}
  if (activeView.value === 'snapshots') { await systemSnapshots.value?.refresh(); return; }
  await load();
  if (linkedTask.value) linkRetry.value++;
}
async function openRestoreCopy(job: BackupJob) { databaseSection.value = 'copies'; await nextTick(); restoreCopies.value?.open(job); }
async function loadJobs() { if (!isRoot.value) return; const generation = accessGeneration; const result = await backup.listJobs({ view: jobsView.value, page: jobsPage.value, pageSize: 20 }); if (!isRoot.value || generation !== accessGeneration) return; jobs.value = result.items; jobsTotal.value = result.total; }
async function changeJobView(view: 'available' | 'history') {
  if (view === jobsView.value) return;
  jobsView.value = view; jobsPage.value = 1; jobs.value = []; jobsTotal.value = 0;
  await loadJobs();
}
async function load() {
  if (!isRoot.value) return;
  const generation = accessGeneration;
  const [o, p, d, a] = await Promise.all([backup.getOverview(), backup.listPolicies({ pageSize: 200 }), backup.listRestoreDrills({ pageSize: 20 }), backup.listAlerts({ pageSize: 20 })]);
  if (!isRoot.value || generation !== accessGeneration) return;
  overview.value = o; policies.value = p.items; drills.value = d.items; alerts.value = a.items;
  await loadJobs();
}
async function changePage(delta: number) { jobsPage.value += delta; await loadJobs(); }
async function protect(job: BackupJob) { if (!canExecute.value) return; await backup.setJobProtected(job.id, !job.protected); await loadJobs(); }
async function ackAlert(id: string | number) { if (!canExecute.value) return; await backup.ackAlert(id); await load(); }
async function confirmAction() {
  if (!canExecute.value || !confirmation.value) return;
  const generation = accessGeneration;
  const action = confirmation.value; confirmation.value = null;
  if (action.kind === 'once') {
    const job = await backup.createOneTime(); if (!isRoot.value || generation !== accessGeneration) return; databaseSection.value = 'files'; jobsView.value = job.status === 'success' ? 'available' : 'history'; jobsPage.value = 1; await load();
    if (!isRoot.value || generation !== accessGeneration) return;
    if (job.status !== 'success') throw new Error(job.error_message || t('backupCenter.failed'));
    notice.value = t('backupCenter.onDemand.databaseReady');
  }
  else if (action.kind === 'cleanup') { await backup.triggerCleanup(); await load(); if (!isRoot.value || generation !== accessGeneration) return; notice.value = t('backupCenter.cleanupSucceeded'); }
  else { const drill = await backup.triggerRestoreDrill(action.job.id); if (!isRoot.value || generation !== accessGeneration) return; databaseSection.value = 'verification'; await load(); if (!isRoot.value || generation !== accessGeneration) return; if (drill.status !== 'success') throw new Error(drill.report_uri || t('backupCenter.failed')); notice.value = t('backupCenter.verifySucceeded'); }
}
onMounted(() => perform(async () => { await loadUserContext(); contextLoaded.value = true; if (isRoot?.value && !linkedTask.value) activeView.value = 'snapshots'; await load(); }));
</script>
