<template>
  <section class="mx-auto max-w-7xl space-y-6 p-4 text-gray-900 sm:p-6 dark:text-gray-100">
    <header class="flex flex-wrap items-center justify-between gap-4">
      <div>
        <h1 class="text-2xl font-semibold">{{ t('backupCenter.title') }}</h1>
        <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ t('backupCenter.onDemand.workflowHint') }}</p>
      </div>
      <UButton icon="i-heroicons-arrow-path" color="neutral" variant="outline" :loading="busy" @click="perform(refreshCurrentView)">{{ t('backupCenter.reload') }}</UButton>
    </header>
    <UAlert v-if="permissionHint" color="warning" :title="permissionHint" />
    <UAlert v-if="error" color="error" :title="error" />
    <UAlert v-if="notice" color="success" :title="notice" />
    <UAlert v-if="isRoot && backupEventStatus === 'disconnected'" color="warning" :title="t('backupCenter.realtimeDisconnected')" />

    <UTabs v-model="activeView" :items="viewItems" variant="link" :ui="{ list: 'overflow-x-auto justify-start border-b border-gray-200 dark:border-gray-700', trigger: 'shrink-0 flex-none', content: 'pt-6' }">
      <template #snapshots><SystemSnapshotsPanel v-if="isRoot" ref="systemSnapshots" /><UAlert v-else color="warning" :title="t('backupCenter.archives.rootOnly')" /></template>
      <template #operations>
        <div class="space-y-6" data-testid="backup-operations">
          <UAlert v-if="overview?.runtime && !overview.runtime.ready" color="warning" :title="t('backupCenter.notReady')" :description="overview.runtime.problems.join(' · ')" />
<UAlert v-if="enabledPolicyCount" color="info" :title="t('backupCenter.onDemand.activeSchedules', { count: enabledPolicyCount })"><template #actions><UButton size="sm" color="neutral" variant="outline" @click="databaseSection = 'policies'">{{ t('backupCenter.onDemand.advanced') }}</UButton></template></UAlert>
<BackupSectionTabs v-model="databaseSection" :items="databaseSections" :label="t('backupCenter.workspace.operations')">
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
                <UBadge color="neutral" variant="subtle">{{ jobsTotal }}</UBadge>
              </div>
              <div class="mt-4 flex flex-wrap gap-2" role="group" :aria-label="t('backupCenter.workspace.jobViews')" data-testid="backup-job-views">
                <UButton v-for="view in jobViewItems" :key="view.value" size="sm" :variant="jobsView === view.value ? 'solid' : 'outline'" :color="jobsView === view.value ? 'primary' : 'neutral'" :aria-pressed="jobsView === view.value" :disabled="busy" @click="perform(() => changeJobView(view.value))">{{ view.label }}</UButton>
              </div>
              <p class="mt-3 text-sm text-gray-500 dark:text-gray-400">{{ t(jobsView === 'available' ? 'backupCenter.workspace.availableHint' : 'backupCenter.workspace.historyHint') }}</p>
            </template>
            <BackupJobTable :items="jobs" :can-execute="canExecute && !busy" :can-recover="isRoot" @restore-copy="openRestoreCopy" @restore-verify="job => confirmation = { kind: 'restore', job }" @protection="job => perform(() => protect(job))" @download="job => perform(() => backup.downloadJob(job.id))" />
            <template #footer>
              <div class="flex flex-wrap items-center justify-between gap-3 text-sm text-gray-500 dark:text-gray-400">
                <span>{{ t('backupCenter.page', { page: jobsPage, total: jobsTotal }) }}</span>
                <div class="flex gap-2"><UButton color="neutral" variant="outline" :disabled="jobsPage <= 1 || busy" @click="perform(() => changePage(-1))">{{ t('backupCenter.previous') }}</UButton><UButton color="neutral" variant="outline" :disabled="jobsPage * 20 >= jobsTotal || busy" @click="perform(() => changePage(1))">{{ t('backupCenter.next') }}</UButton></div>
              </div>
            </template>
          </UCard>
</template>
<template #policies>
<UCard>
            <template #header>
              <div class="flex flex-wrap items-center justify-between gap-3">
                <h2 class="text-lg font-semibold">{{ t('backupCenter.onDemand.databaseSchedules') }}</h2>
                <UButton icon="i-heroicons-plus" :disabled="!canExecute || busy" @click="openPolicy()">{{ t('backupCenter.newPolicy') }}</UButton>
              </div>
            </template>
            <p class="mb-5 text-sm text-muted">{{ t('backupCenter.onDemand.advancedHint') }}</p>
            <p v-if="!policies.length" class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('backupCenter.noPolicies') }}</p>
            <div class="space-y-4">
              <article v-for="policy in policies" :key="String(policy.id)" class="rounded-xl border border-gray-200 p-4 sm:p-5 dark:border-gray-700" data-testid="backup-policy">
                <div class="flex flex-wrap items-center justify-between gap-3">
                  <h3 class="font-semibold">{{ policy.name }}</h3>
                  <UBadge variant="subtle" :color="policy.enabled ? 'success' : 'warning'">{{ t(policy.enabled ? 'backupCenter.workspace.scheduleOn' : 'backupCenter.workspace.scheduleOff') }}</UBadge>
                </div>
                <dl class="mt-5 grid gap-4 text-sm sm:grid-cols-3">
                  <div><dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('backupCenter.workspace.frequency') }}</dt><dd class="mt-1 font-medium">{{ intervalLabel(policy) }}</dd></div>
                  <div><dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('backupCenter.workspace.retention') }}</dt><dd class="mt-1 font-medium">{{ retentionText(policy) }}</dd></div>
                  <div><dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('backupCenter.autoVerify') }}</dt><dd class="mt-1 font-medium">{{ policy.drill_enabled ? t('backupCenter.everyDays', { days: policy.drill_interval_days }) : t('backupCenter.workspace.verificationOff') }}</dd></div>
                </dl>
                <div class="mt-5 flex flex-wrap items-center justify-between gap-3 border-t border-gray-100 pt-4 dark:border-gray-800">
                  <UButton size="sm" color="neutral" variant="ghost" icon="i-heroicons-pencil-square" :disabled="!canExecute || busy" @click="openPolicy(policy)">{{ t('backupCenter.edit') }}</UButton>
                  <div class="flex flex-wrap gap-2">
                    <UButton size="sm" :color="policy.enabled ? 'warning' : 'primary'" :variant="policy.enabled ? 'outline' : 'solid'" :disabled="!canExecute || busy" @click="perform(() => togglePolicy(policy))">{{ t(policy.enabled ? 'backupCenter.workspace.disableSchedule' : 'backupCenter.workspace.enableSchedule') }}</UButton>
                    <UButton size="sm" icon="i-heroicons-arrow-down-tray" :color="policy.enabled ? 'primary' : 'neutral'" :variant="policy.enabled ? 'solid' : 'outline'" :disabled="!canExecute || busy" @click="perform(() => runBackup(policy))">{{ t('backupCenter.backupNow') }}</UButton>
                  </div>
                </div>
              </article>
            </div>
          </UCard>
</template>
<template #copies>
<RestoreCopiesPanel v-if="isRoot" ref="restoreCopies" :jobs="jobs" />
</template>
<template #verification>
<RestoreDrillPanel :history="drills" :alerts="alerts" :can-execute="canExecute && !busy" @ack="id => perform(() => ackAlert(id))" />
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
      <template #archives><ResourceArchivesPanel v-if="isRoot" /><UAlert v-else color="warning" :title="t('backupCenter.archives.rootOnly')" /></template>
      <template #configuration>
        <div class="space-y-6" data-testid="backup-configuration"><BackupRuntimePanel :runtime="overview?.runtime" /></div>
      </template>
      <template #guide>
        <BackupGuidePanel />
      </template>
    </UTabs>

    <UModal v-model:open="modalOpen" :title="t(editingId ? 'backupCenter.editPolicy' : 'backupCenter.newPolicy')" :description="t('backupCenter.formHint')" :ui="{ content: 'max-w-2xl' }">
      <template #body>
        <UForm id="backup-policy-form" :schema="policySchema" :state="form" class="space-y-4" @submit="perform(savePolicy)">
          <UFormField name="name" :label="t('backupCenter.policyName')" required><UInput v-model="form.name" class="w-full" /></UFormField>
          <div class="grid gap-4 sm:grid-cols-2">
            <UFormField name="intervalValue" :label="t('backupCenter.interval')" required><div class="flex gap-2"><UInput v-model.number="form.intervalValue" type="number" min="1" class="w-full" /><USelect v-model="form.intervalUnit" :items="intervalOptions" class="w-32" /></div></UFormField>
            <UFormField name="retentionCount" :label="t('backupCenter.retentionCount')" required><UInput v-model.number="form.retentionCount" type="number" min="1" max="10000" class="w-full" /></UFormField>
            <UFormField name="retentionMode" :label="t('backupCenter.retentionMode')"><USelect v-model="form.retentionMode" :items="retentionOptions" class="w-full" /></UFormField>
            <UFormField v-if="form.retentionMode === 'age_and_count'" name="retentionDays" :label="t('backupCenter.retentionDays')" required><UInput v-model.number="form.retentionDays" type="number" min="1" max="3650" class="w-full" /></UFormField>
            <UFormField name="timezone" :label="t('backupCenter.timezone')" required><UInput v-model="form.timezone" class="w-full" /></UFormField>
          </div>
          <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('backupCenter.retentionExplanation') }}</p>
          <UCheckbox v-model="form.drillEnabled" :label="t('backupCenter.autoVerify')" />
          <UFormField v-if="form.drillEnabled" name="drillIntervalDays" :label="t('backupCenter.verifyInterval')"><UInput v-model.number="form.drillIntervalDays" type="number" min="1" max="3650" /></UFormField>
          <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('backupCenter.verifyHint') }}</p>
          <p v-if="error" class="text-sm text-red-600">{{ error }}</p>
        </UForm>
      </template>
      <template #footer><div class="flex w-full justify-end gap-2"><UButton color="neutral" variant="ghost" @click="modalOpen = false">{{ t('backupCenter.cancel') }}</UButton><UButton type="submit" form="backup-policy-form" :loading="busy" :disabled="!canExecute">{{ t('backupCenter.save') }}</UButton></div></template>
    </UModal>
    <UModal :open="!!confirmation" :title="t(confirmation?.kind === 'once' ? 'backupCenter.onDemand.createDatabase' : confirmation?.kind === 'cleanup' ? 'backupCenter.cleanup' : 'backupCenter.verify')" :description="t(confirmation?.kind === 'once' ? 'backupCenter.onDemand.databaseHint' : confirmation?.kind === 'cleanup' ? 'backupCenter.cleanupConfirm' : 'backupCenter.verifyConfirm')" @update:open="value => { if (!value) confirmation = null }">
      <template #footer><div class="flex w-full justify-end gap-2"><UButton color="neutral" variant="ghost" @click="confirmation = null">{{ t('backupCenter.cancel') }}</UButton><UButton :loading="busy" @click="perform(confirmAction)">{{ t('backupCenter.confirm') }}</UButton></div></template>
    </UModal>
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref } from 'vue';
import * as v from 'valibot';
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
const { status: backupEventStatus } = useBackupEvents();
const { t, locale } = useI18n();
const activeView = ref('operations');
const databaseSection = ref('export');
const databaseSections = computed(() => [
  { value: 'export', label: t('backupCenter.onDemand.tools'), icon: 'i-heroicons-arrow-down-tray' },
  { value: 'files', label: t('backupCenter.sections.files'), icon: 'i-heroicons-circle-stack' },
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
const { canExecute, isRoot, permissionHint, loadUserContext } = useOpsAccess();
const restoreCopies = ref<InstanceType<typeof RestoreCopiesPanel>>();
const systemSnapshots = ref<InstanceType<typeof SystemSnapshotsPanel>>();
const overview = ref<BackupOverview>();
const policies = ref<BackupPolicy[]>([]);
const jobs = ref<BackupJob[]>([]);
const enabledPolicyCount = computed(() => policies.value.filter(policy => policy.enabled).length);
const timeLabel = (value?: string) => formatBackupTime(value, String(locale.value || 'zh-CN'));
const intervalLabel = (policy: BackupPolicy) => {
  const match = policy.schedule?.match(/^(\d+)([mhd])$/);
  const count = match ? Number(match[1]) : policy.interval_hours;
  const unit = match?.[2] || 'h';
  return t(`backupCenter.workspace.every${unit === 'm' ? 'Minutes' : unit === 'd' ? 'Days' : 'Hours'}`, { count });
};
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
const modalOpen = ref(false);
const editingId = ref<string | number>('');
const confirmation = ref<{ kind: 'once' } | { kind: 'cleanup' } | { kind: 'restore'; job: BackupJob } | null>(null);
const defaults = () => ({ name: '', intervalValue: 1, intervalUnit: 'hour' as 'minute' | 'hour' | 'day', retentionCount: 168, retentionDays: 7, retentionMode: 'age_and_count' as 'count' | 'age_and_count', timezone: 'Asia/Shanghai', drillEnabled: false, drillIntervalDays: 7 });
const form = reactive(defaults());
const policySchema = computed(() => v.object({
  name: v.pipe(v.string(), v.trim(), v.minLength(1), v.maxLength(128)),
  intervalValue: v.pipe(v.number(), v.integer(), v.minValue(1), v.maxValue(10000)),
  intervalUnit: v.picklist(['minute', 'hour', 'day']),
  retentionCount: v.pipe(v.number(), v.integer(), v.minValue(1), v.maxValue(10000)),
  retentionDays: v.pipe(v.number(), v.integer(), v.minValue(1), v.maxValue(3650)),
  retentionMode: v.picklist(['count', 'age_and_count']),
  timezone: v.pipe(v.string(), v.check(value => { try { new Intl.DateTimeFormat('en', { timeZone: value }); return !!value; } catch { return false; } }, t('backupCenter.invalidTimezone'))),
  drillEnabled: v.boolean(),
  drillIntervalDays: v.pipe(v.number(), v.integer(), v.minValue(1), v.maxValue(3650)),
}));
const intervalOptions = computed(() => ['minute', 'hour', 'day'].map(value => ({ value, label: t(`backupCenter.${value}`) })));
const retentionOptions = computed(() => ['age_and_count', 'count'].map(value => ({ value, label: t(`backupCenter.${value}`) })));
const retentionText = (p: BackupPolicy) => p.retention_mode === 'age_and_count' ? t('backupCenter.keepDaysCount', { days: p.retention_days, count: p.retention_count }) : t('backupCenter.keepCount', { count: p.retention_count });
async function perform(action: () => Promise<unknown>) {
  if (busy.value) return;
  busy.value = true; error.value = ''; notice.value = '';
  try { await action(); } catch (e: any) {
    const data = e?.response?._data || e?.data;
    error.value = [data?.message || e?.message || t('backupCenter.failed'), data?.error, data?.trace_id || data?.request_id].filter(Boolean).join(' · ');
  } finally { busy.value = false; }
}
async function refreshCurrentView() {
  if (activeView.value === 'snapshots') { await systemSnapshots.value?.refresh(); return; }
  await load();
}
async function openRestoreCopy(job: BackupJob) { databaseSection.value = 'copies'; await nextTick(); restoreCopies.value?.open(job); }
async function loadJobs() { const result = await backup.listJobs({ view: jobsView.value, page: jobsPage.value, pageSize: 20 }); jobs.value = result.items; jobsTotal.value = result.total; }
async function changeJobView(view: 'available' | 'history') {
  if (view === jobsView.value) return;
  jobsView.value = view; jobsPage.value = 1; jobs.value = []; jobsTotal.value = 0;
  await loadJobs();
}
async function load() {
  const [o, p, d, a] = await Promise.all([backup.getOverview(), backup.listPolicies({ pageSize: 200 }), backup.listRestoreDrills({ pageSize: 20 }), backup.listAlerts({ pageSize: 20 })]);
  overview.value = o; policies.value = p.items; drills.value = d.items; alerts.value = a.items;
  await loadJobs();
}
function openPolicy(p?: BackupPolicy) {
  error.value = ''; Object.assign(form, defaults()); editingId.value = p?.id || '';
  if (p) { const match = p.schedule?.match(/^(\d+)([mhd])$/); Object.assign(form, { name: p.name, intervalValue: match ? Number(match[1]) : p.interval_hours, intervalUnit: match ? ({ m: 'minute', h: 'hour', d: 'day' } as const)[match[2] as 'm' | 'h' | 'd'] : 'hour', retentionCount: p.retention_count, retentionDays: p.retention_mode === 'age_and_count' ? (p.retention_days || 7) : 7, retentionMode: p.retention_mode || 'count', timezone: p.timezone, drillEnabled: p.drill_enabled, drillIntervalDays: p.drill_interval_days }); }
  modalOpen.value = true;
}
async function savePolicy() {
  if (!canExecute.value) return;
  const payload = { name: form.name.trim(), interval_value: form.intervalValue, interval_unit: form.intervalUnit, retention_count: form.retentionCount, retention_days: form.retentionDays, retention_mode: form.retentionMode, timezone: form.timezone, drill_enabled: form.drillEnabled, drill_interval_days: form.drillIntervalDays, target_ref: 'local_dump' };
  if (editingId.value) await backup.updatePolicy(editingId.value, payload); else await backup.createPolicy(payload);
  modalOpen.value = false; await load(); notice.value = t('backupCenter.saved');
}
async function togglePolicy(p: BackupPolicy) { if (!canExecute.value) return; if (p.enabled) await backup.disablePolicy(p.id); else await backup.enablePolicy(p.id); await load(); }
async function runBackup(p: BackupPolicy) { if (!canExecute.value) return; const job = await backup.triggerJob(p.id); databaseSection.value = 'files'; jobsView.value = job.status === 'success' ? 'available' : 'history'; jobsPage.value = 1; await load(); if (job.status !== 'success') throw new Error(job.error_message || t('backupCenter.failed')); notice.value = t('backupCenter.backupSucceeded'); }
async function changePage(delta: number) { jobsPage.value += delta; await loadJobs(); }
async function protect(job: BackupJob) { if (!canExecute.value) return; await backup.setJobProtected(job.id, !job.protected); await loadJobs(); }
async function ackAlert(id: string | number) { if (!canExecute.value) return; await backup.ackAlert(id); await load(); }
async function confirmAction() {
  if (!canExecute.value || !confirmation.value) return;
  const action = confirmation.value; confirmation.value = null;
  if (action.kind === 'once') {
    const job = await backup.createOneTime(); databaseSection.value = 'files'; jobsView.value = job.status === 'success' ? 'available' : 'history'; jobsPage.value = 1; await load();
    if (job.status !== 'success') throw new Error(job.error_message || t('backupCenter.failed'));
    notice.value = t('backupCenter.onDemand.databaseReady');
  }
  else if (action.kind === 'cleanup') { await backup.triggerCleanup(); await load(); notice.value = t('backupCenter.cleanupSucceeded'); }
  else { const drill = await backup.triggerRestoreDrill(action.job.id); databaseSection.value = 'verification'; await load(); if (drill.status !== 'success') throw new Error(drill.report_uri || t('backupCenter.failed')); notice.value = t('backupCenter.verifySucceeded'); }
}
onMounted(() => perform(async () => { await loadUserContext(); if (isRoot?.value) activeView.value = 'snapshots'; await load(); }));
</script>
