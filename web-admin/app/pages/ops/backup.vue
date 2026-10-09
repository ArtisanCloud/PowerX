<template>
  <section class="mx-auto max-w-7xl space-y-6 p-6 text-gray-900 dark:text-gray-100">
    <header class="flex flex-wrap items-start justify-between gap-3">
      <div><h1 class="text-2xl font-semibold">{{ t('backupCenter.title') }}</h1><p class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ t('backupCenter.subtitle') }}</p></div>
      <UButton color="neutral" variant="outline" :loading="busy" @click="perform(load)">{{ t('backupCenter.reload') }}</UButton>
    </header>
    <UAlert v-if="permissionHint" color="warning" :title="permissionHint" />
    <UAlert v-if="error" color="error" :title="error" />
    <UAlert v-if="notice" color="success" :title="notice" />
    <BackupRuntimePanel :runtime="overview?.runtime" :enabled-count="overview?.policies_enabled || 0" />

    <UCard>
      <template #header><div class="flex items-center justify-between"><h2 class="font-semibold">{{ t('backupCenter.policies') }}</h2><UButton :disabled="!canExecute || busy" @click="openPolicy()">{{ t('backupCenter.newPolicy') }}</UButton></div></template>
      <p class="mb-4 text-sm text-gray-500 dark:text-gray-400">{{ t('backupCenter.policySteps') }}</p>
      <p v-if="!policies.length" class="py-4 text-gray-500">{{ t('backupCenter.noPolicies') }}</p>
      <div v-for="policy in policies" :key="String(policy.id)" class="mb-3 rounded-lg border border-gray-200 p-4 dark:border-gray-700">
        <div class="flex flex-wrap justify-between gap-2"><strong>{{ policy.name }}</strong><UBadge :color="policy.enabled ? 'success' : 'neutral'">{{ t(policy.enabled ? 'backupCenter.enabled' : 'backupCenter.disabled') }}</UBadge></div>
        <p class="mt-2 text-sm">{{ policy.schedule }} · {{ retentionText(policy) }}</p>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('backupCenter.autoVerify') }}: {{ policy.drill_enabled ? t('backupCenter.everyDays', { days: policy.drill_interval_days }) : t('backupCenter.disabled') }}</p>
        <div class="mt-3 flex flex-wrap gap-2">
          <UButton size="sm" color="neutral" variant="outline" :disabled="!canExecute || busy" @click="openPolicy(policy)">{{ t('backupCenter.edit') }}</UButton>
          <UButton size="sm" color="neutral" variant="outline" :disabled="!canExecute || busy" @click="perform(() => togglePolicy(policy))">{{ t(policy.enabled ? 'backupCenter.disable' : 'backupCenter.enable') }}</UButton>
          <UButton size="sm" :disabled="!canExecute || busy" @click="perform(() => runBackup(policy))">{{ t('backupCenter.backupNow') }}</UButton>
        </div>
      </div>
      <template #footer><div class="flex flex-wrap items-center justify-between gap-3"><p class="text-sm text-gray-500 dark:text-gray-400">{{ t('backupCenter.cleanupScope') }}</p><UButton color="warning" variant="outline" :disabled="!canExecute || busy" @click="confirmation = { kind: 'cleanup' }">{{ t('backupCenter.cleanup') }}</UButton></div></template>
    </UCard>

    <UCard>
      <template #header><h2 class="font-semibold">{{ t('backupCenter.jobs') }}</h2></template>
      <BackupJobTable :items="jobs" :can-execute="canExecute && !busy" @restore-verify="job => confirmation = { kind: 'restore', job }" @protection="job => perform(() => protect(job))" />
      <div class="mt-4 flex items-center justify-between text-sm"><span>{{ t('backupCenter.page', { page: jobsPage, total: jobsTotal }) }}</span><div class="flex gap-2"><UButton color="neutral" variant="outline" :disabled="jobsPage <= 1 || busy" @click="perform(() => changePage(-1))">{{ t('backupCenter.previous') }}</UButton><UButton color="neutral" variant="outline" :disabled="jobsPage * 20 >= jobsTotal || busy" @click="perform(() => changePage(1))">{{ t('backupCenter.next') }}</UButton></div></div>
    </UCard>
    <RestoreDrillPanel :history="drills" :alerts="alerts" :can-execute="canExecute && !busy" @ack="id => perform(() => ackAlert(id))" />
    <UCard>
      <template #header><h2 class="font-semibold">{{ t('backupCenter.recoveryTitle') }}</h2></template>
      <ol class="list-decimal space-y-2 pl-5 text-sm"><li v-for="i in 5" :key="i">{{ t(`backupCenter.recoveryStep${i}`) }}</li></ol>
      <p class="mt-3 text-sm text-amber-700 dark:text-amber-400">{{ t('backupCenter.recoveryBoundary') }}</p>
    </UCard>
    <LogObservabilityPanel />

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
    <UModal :open="!!confirmation" :title="t(confirmation?.kind === 'cleanup' ? 'backupCenter.cleanup' : 'backupCenter.verify')" :description="t(confirmation?.kind === 'cleanup' ? 'backupCenter.cleanupConfirm' : 'backupCenter.verifyConfirm')" @update:open="value => { if (!value) confirmation = null }">
      <template #footer><div class="flex w-full justify-end gap-2"><UButton color="neutral" variant="ghost" @click="confirmation = null">{{ t('backupCenter.cancel') }}</UButton><UButton :loading="busy" @click="perform(confirmAction)">{{ t('backupCenter.confirm') }}</UButton></div></template>
    </UModal>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import * as v from 'valibot';
import { useBackupOpsService, type BackupPolicy, type BackupJob, type BackupOverview, type RestoreDrillRecord, type BackupAlert } from '~/composables/api/services/backupOpsService';
import { useOpsAccess } from '~/composables/useOpsAccess';
import BackupJobTable from '~/components/ops/backup/BackupJobTable.vue';
import BackupRuntimePanel from '~/components/ops/backup/BackupRuntimePanel.vue';
import RestoreDrillPanel from '~/components/ops/backup/RestoreDrillPanel.vue';
import LogObservabilityPanel from '~/components/ops/backup/LogObservabilityPanel.vue';
const { t } = useI18n();
const backup = useBackupOpsService();
const { canExecute, permissionHint, loadUserContext } = useOpsAccess();
const overview = ref<BackupOverview>();
const policies = ref<BackupPolicy[]>([]);
const jobs = ref<BackupJob[]>([]);
const drills = ref<RestoreDrillRecord[]>([]);
const alerts = ref<BackupAlert[]>([]);
const jobsPage = ref(1);
const jobsTotal = ref(0);
const busy = ref(false);
const error = ref('');
const notice = ref('');
const modalOpen = ref(false);
const editingId = ref<string | number>('');
const confirmation = ref<{ kind: 'cleanup' } | { kind: 'restore'; job: BackupJob } | null>(null);
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
async function loadJobs() { const result = await backup.listJobs({ page: jobsPage.value, pageSize: 20 }); jobs.value = result.items; jobsTotal.value = result.total; }
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
async function runBackup(p: BackupPolicy) { if (!canExecute.value) return; const job = await backup.triggerJob(p.id); jobsPage.value = 1; await load(); if (job.status !== 'success') throw new Error(job.error_message || t('backupCenter.failed')); notice.value = t('backupCenter.backupSucceeded'); }
async function changePage(delta: number) { jobsPage.value += delta; await loadJobs(); }
async function protect(job: BackupJob) { if (!canExecute.value) return; await backup.setJobProtected(job.id, !job.protected); await loadJobs(); }
async function ackAlert(id: string | number) { if (!canExecute.value) return; await backup.ackAlert(id); await load(); }
async function confirmAction() {
  if (!canExecute.value || !confirmation.value) return;
  const action = confirmation.value; confirmation.value = null;
  if (action.kind === 'cleanup') { await backup.triggerCleanup(); await load(); notice.value = t('backupCenter.cleanupSucceeded'); }
  else { const drill = await backup.triggerRestoreDrill(action.job.id); await load(); if (drill.status !== 'success') throw new Error(drill.report_uri || t('backupCenter.failed')); notice.value = t('backupCenter.verifySucceeded'); }
}
onMounted(() => perform(async () => { await loadUserContext(); await load(); }));
</script>
