<template>
  <div class="rounded-lg border border-default p-3 text-sm" data-testid="table-monitoring-summary">
    <div class="flex flex-wrap items-center justify-between gap-2"><p><span class="font-medium">{{ t('storageGov.tableMonitor.title') }}</span> · {{ t(`storageGov.tableMonitor.states.${status.state}`) }}</p><UButton color="neutral" variant="outline" size="sm" @click="open = true">{{ t('storageGov.tableMonitor.manage') }}</UButton></div>
    <p class="mt-2 text-xs text-muted" data-testid="monitor-versions">{{ configurationLabel }}</p>
    <p v-if="configuration?.problem" class="mt-2 text-warning">{{ message(configuration.problem) }}</p>
    <p v-if="status.problem" class="mt-2 text-warning">{{ message(status.problem) }}</p>
    <ul v-if="status.alerts.length" class="mt-2 space-y-1 text-warning"><li v-for="a in status.alerts" :key="a.code + a.resource_id">{{ message(a.code) }} <code v-if="a.resource_id" class="break-all text-xs">{{ a.resource_id }}</code></li></ul>
    <UModal v-model:open="open" :dismissible="!saving && !unknown" :close="saving || unknown ? false : undefined" :title="t('storageGov.tableMonitor.title')" :description="t('storageGov.tableMonitor.metric')" :ui="{ content: 'max-w-4xl' }">
      <template #body>
        <div class="mb-4 rounded-lg border border-default bg-elevated/50 p-3 text-sm" data-testid="monitor-dialog-versions">
          <p>{{ configurationLabel }}</p>
          <p v-if="dirty" class="mt-2 font-medium text-warning">{{ t('storageGov.tableMonitor.control.dirty') }}</p>
          <p v-if="configuration?.authority === 'database' && baseRevision > 0 && configuration.desired_revision !== baseRevision" class="mt-2 text-warning">{{ t('storageGov.tableMonitor.control.staleDraft', { version:baseRevision }) }}</p>
        </div>
        <UAlert v-if="error" color="error" :title="t('storageGov.tableMonitor.saveFailed')" :description="error" class="mb-4" />
        <BackupSectionTabs v-model="section" :items="sections" :label="t('storageGov.tableMonitor.title')">
          <template #status><div class="space-y-4 text-sm" data-testid="table-monitoring-status">
            <p class="font-medium">{{ t(`storageGov.tableMonitor.states.${status.state}`) }}</p>
            <dl class="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)] gap-3"><dt>{{ t('storageGov.tableMonitor.sampled') }}</dt><dd>{{ date(status.sampled_at) }}</dd><dt>{{ t('storageGov.tableMonitor.worker') }}</dt><dd>{{ t(status.worker_enabled ? 'storageGov.tableMonitor.enabled' : 'storageGov.tableMonitor.disabled') }}</dd><dt>{{ t('storageGov.tableMonitor.cleanup') }}</dt><dd>{{ status.cleanup ? t(`storageGov.tableMonitor.cycles.${status.cleanup.state}`) : t('storageGov.tableMonitor.unknown') }}</dd><dt>{{ t('storageGov.tableMonitor.lastSuccess') }}</dt><dd>{{ date(status.cleanup?.last_success_at) }}</dd><dt>{{ t('storageGov.tableMonitor.failures') }}</dt><dd>{{ status.cleanup?.consecutive_failures ?? '—' }}</dd><dt>{{ t('storageGov.tableMonitor.backlog') }}</dt><dd>{{ backlogLabel }}</dd></dl>
            <p class="text-muted">{{ t('storageGov.tableMonitor.backlogHint') }}</p>
          </div></template>
          <template #settings>
            <div v-if="configuration?.authority === 'legacy'" class="space-y-4 text-sm" data-testid="table-monitoring-adoption">
              <UAlert color="warning" :title="t('storageGov.tableMonitor.control.legacy')" :description="t('storageGov.tableMonitor.control.adoptionHint')" />
              <p>{{ t('storageGov.tableMonitor.control.legacyVersion', { version: configuration.legacy_revision }) }} · {{ t(configuration.legacy_enabled ? 'storageGov.tableMonitor.enabled' : 'storageGov.tableMonitor.disabled') }}</p>
              <dl class="grid grid-cols-2 gap-3"><dt>{{ t('storageGov.tableMonitor.warningGiB') }}</dt><dd>{{ localStatus.policy.warning_bytes / 2 ** 30 }}</dd><dt>{{ t('storageGov.tableMonitor.growthMiB') }}</dt><dd>{{ localStatus.policy.growth_bytes / 2 ** 20 }}</dd></dl>
              <UCheckbox v-model="adoptionConfirmed" :label="t('storageGov.tableMonitor.control.adoptionConfirm')" data-testid="monitor-adoption-confirm" />
              <UButton :disabled="!configuration.ready || !adoptionConfirmed || !!unknown" :loading="saving" data-testid="monitor-adopt" @click="adopt">{{ t('storageGov.tableMonitor.control.adopt') }}</UButton>
            </div>
            <UAlert v-else-if="!configuration || configuration.authority !== 'database'" color="warning" :title="t('storageGov.tableMonitor.control.upgrade')" :description="t('storageGov.tableMonitor.control.unavailableHint')" />
            <UForm v-else :state="form" :schema="schema" class="space-y-4" data-testid="table-monitoring-form" @submit="save">
              <fieldset :disabled="saving || !!unknown" class="space-y-4">
                <UCheckbox v-model="form.enabled" :label="t('storageGov.tableMonitor.enable')" data-testid="table-monitoring-enable" />
                <p v-if="!canEnable" class="text-sm text-warning">{{ t('storageGov.tableMonitor.prerequisites') }}</p>
                <div class="grid min-w-0 gap-4 sm:grid-cols-2">
                  <UFormField name="warningGiB" :label="t('storageGov.tableMonitor.warningGiB')"><UInput v-model.number="form.warningGiB" type="number" min="0.0625" max="1048576" step="0.0625" class="w-full" data-testid="table-warning-gib" /></UFormField>
                  <UFormField name="growthMiB" :label="t('storageGov.tableMonitor.growthMiB')"><UInput v-model.number="form.growthMiB" type="number" min="1" max="1073741824" step="1" class="w-full" data-testid="table-growth-mib" /></UFormField>
                  <UFormField name="growth_window_minutes" :label="t('storageGov.tableMonitor.window')"><UInput v-model.number="form.growth_window_minutes" type="number" min="5" max="1440" class="w-full" /></UFormField>
                  <UFormField name="stale_after_minutes" :label="t('storageGov.tableMonitor.stale')"><UInput v-model.number="form.stale_after_minutes" type="number" min="15" max="1440" class="w-full" /></UFormField>
                  <UFormField name="failure_limit" :label="t('storageGov.tableMonitor.failureLimit')"><UInput v-model.number="form.failure_limit" type="number" min="1" max="20" class="w-full" /></UFormField>
                </div>
                <p class="text-xs text-muted">{{ t('storageGov.tableMonitor.notice') }}</p>
                <UButton type="submit" :loading="saving" :disabled="!configuration.ready || (form.enabled && !canEnable) || !!unknown">{{ t('storageGov.tableMonitor.save') }}</UButton>
                <UButton type="button" color="neutral" variant="ghost" @click="loadLatest">{{ t('storageGov.tableMonitor.control.loadLatest') }}</UButton>
              </fieldset>
            </UForm>
            <UAlert v-if="notice" :color="configuration?.state === 'applied' && configuration.desired_revision === baseRevision ? 'success' : 'info'" :title="publicationNotice" class="mt-4" />
            <div v-if="unknown" class="mt-4 space-y-3" data-testid="monitor-unknown">
              <UAlert color="warning" :title="t('storageGov.tableMonitor.control.unknown')" :description="t('storageGov.tableMonitor.control.unknownHint')" />
              <UButton color="neutral" variant="outline" :loading="saving" data-testid="monitor-reconcile" @click="reconcile">{{ t('storageGov.tableMonitor.control.reconcile') }}</UButton>
            </div>
          </template>
        </BackupSectionTabs>
      </template>
    </UModal>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import * as v from 'valibot';
import BackupSectionTabs from '~/components/ops/backup/BackupSectionTabs.vue';
import { monitoringSettings, useStorageDatabaseTablesService, type TableMonitoringSettings, type TableMonitoringStatus, type TableMonitoringMutation } from '~/composables/api/services/storageDatabaseTablesService';
const props = defineProps<{ status: TableMonitoringStatus }>(), emit = defineEmits<{ saved: [] }>();
const { t, te, locale } = useI18n(), api = useStorageDatabaseTablesService();
const localStatus = ref(props.status), configuration = computed(() => localStatus.value.configuration);
const open = ref(false), section = ref('status'), saving = ref(false), error = ref(''), notice = ref(''), adoptionConfirmed = ref(false);
const form = reactive({ enabled: false, warningGiB: 4, growthMiB: 256, growth_window_minutes: 15, stale_after_minutes: 45, failure_limit: 3 });
const baseRevision = ref(0), unknown = ref<{ key: string; settings: TableMonitoringSettings; adopting?:boolean }>();
const draftOriginal = ref<TableMonitoringSettings>();
const dirty = computed(() => !!draftOriginal.value && !same(settings(),draftOriginal.value));
const publicationNotice = computed(() => configuration.value?.state === 'applied' && configuration.value.desired_revision === baseRevision.value ? t('storageGov.tableMonitor.control.applied') : notice.value);
let controller: AbortController | undefined, initialized = false;
const sections = computed(() => [{ value: 'status', label: t('storageGov.tableMonitor.status') }, { value: 'settings', label: t('storageGov.tableMonitor.settings') }]);
const canEnable = computed(() => !!configuration.value?.ready && localStatus.value.sampling_configured && localStatus.value.worker_enabled);
const configurationLabel = computed(() => {
  const c = configuration.value;
  if (!c) return t('storageGov.tableMonitor.control.upgrade');
  if (c.authority === 'legacy') return t('storageGov.tableMonitor.control.legacy');
  return t('storageGov.tableMonitor.control.versions', { desired: c.desired_revision, applied: c.applied_revision, snapshot: c.snapshot_revision ?? '—', state: t(`storageGov.tableMonitor.control.states.${c.state}`) });
});
const schema = v.object({ enabled:v.boolean(), warningGiB:v.pipe(v.number(),v.minValue(0.0625),v.maxValue(1048576)), growthMiB:v.pipe(v.number(),v.integer(),v.minValue(1),v.maxValue(1073741824)), growth_window_minutes:v.pipe(v.number(),v.integer(),v.minValue(5),v.maxValue(1440)), stale_after_minutes:v.pipe(v.number(),v.integer(),v.minValue(15),v.maxValue(1440)), failure_limit:v.pipe(v.number(),v.integer(),v.minValue(1),v.maxValue(20)) });
const backlogLabel = computed(() => {
  const backlog = props.status.backlog;
  const key = backlog?.state === 'disabled' ? 'disabled' : backlog?.state === 'observed' ? (backlog.eligible_present ? 'present' : 'absent') : 'unknown';
  return t(`storageGov.tableMonitor.${key}`);
});
const date = (value?: string | null) => value ? new Intl.DateTimeFormat(locale.value, { dateStyle: 'short', timeStyle: 'short' }).format(new Date(value)) : '—';
function message(code: string) {
  if (code === 'STORAGE_CONTROL_SCHEMA_NOT_READY' || code === 'STORAGE_CONTROL_UNAVAILABLE' || code === 'STORAGE_CONTROL_WORKER_UNAVAILABLE') return t('storageGov.tableMonitor.control.unavailableHint');
  if (code.includes('SNAPSHOT_') || code.includes('RESTORE_CONFIRMATION')) return t('storageGov.tableMonitor.control.snapshotProblem');
  if (code === 'STORAGE_TABLE_MONITORING_LEGACY_CHANGED') return t('storageGov.tableMonitor.control.legacyChanged');
  if (code === 'STORAGE_CONTROL_APPLY_FAILED') return t('storageGov.tableMonitor.control.publicationFailed');
  const key = `storageGov.tableMonitor.problems.${code}`; return te(key) ? t(key) : code;
}
watch(() => props.status, value => {
  const previous=configuration.value,next=value.configuration;
  if(previous?.authority==='database'&&next?.authority==='legacy')return;
  if(previous?.authority==='database'&&next?.authority==='database'&&next.desired_revision<previous.desired_revision&&!['unconfirmed','unavailable'].includes(next.state))return;
  localStatus.value = value;
});
function fillDraft() {
  const p = configuration.value?.desired_policy || localStatus.value.policy;
  Object.assign(form, { enabled:p.enabled, growth_window_minutes:p.growth_window_minutes, stale_after_minutes:p.stale_after_minutes, failure_limit:p.failure_limit, warningGiB: p.warning_bytes / 2 ** 30, growthMiB: p.growth_bytes / 2 ** 20 });
  baseRevision.value = configuration.value?.desired_revision || 0; initialized = true;
  draftOriginal.value = monitoringSettings(p);
}
watch(open, value => { if (value && !initialized) fillDraft(); });
function settings(): TableMonitoringSettings { return { enabled:form.enabled, warning_bytes:Math.round(form.warningGiB * 2 ** 30), growth_bytes:form.growthMiB * 2 ** 20, growth_window_minutes:form.growth_window_minutes, stale_after_minutes:form.stale_after_minutes, failure_limit:form.failure_limit }; }
const same = (a:TableMonitoringSettings,b:TableMonitoringSettings) => JSON.stringify(monitoringSettings(a)) === JSON.stringify(monitoringSettings(b));
function accepted(result:TableMonitoringMutation, operation:{key:string;settings:TableMonitoringSettings;adopting?:boolean}) {
  if (result.request_id !== operation.key || !same(result.operation_policy, operation.settings)) throw new Error('STORAGE_TABLE_MUTATION_RESPONSE_INVALID');
  if (!configuration.value || result.configuration.authority !== 'database' || result.configuration.desired_revision >= configuration.value.desired_revision) localStatus.value = { ...localStatus.value, configuration:result.configuration };
  baseRevision.value = result.operation_revision; unknown.value = undefined;
  draftOriginal.value = operation.settings;
  if (operation.adopting) { initialized=false;fillDraft();adoptionConfirmed.value=false; }
  notice.value = t(result.configuration.state === 'applied' ? 'storageGov.tableMonitor.control.applied' : 'storageGov.tableMonitor.control.savedPending');
  emit('saved');
}
async function write(adopting:boolean) {
  if (saving.value || unknown.value || !configuration.value?.ready || adopting && (!adoptionConfirmed.value || configuration.value.authority !== 'legacy')) return;
  if (!adopting && (!v.safeParse(schema, form).success || form.enabled && !canEnable.value)) return;
  const operation = { key:crypto.randomUUID(), adopting, settings:adopting ? {...monitoringSettings(localStatus.value.policy), enabled:false} : settings() };
  const c = controller = new AbortController(); saving.value=true;error.value='';notice.value='';
  try {
    const result = adopting ? await api.adoptMonitoring(c.signal,configuration.value,operation.key) : await api.saveMonitoring(c.signal,operation.settings,baseRevision.value,operation.key);
    if (!c.signal.aborted) { accepted(result,operation); if (adopting) { initialized=false;fillDraft();adoptionConfirmed.value=false; } }
  } catch(e:any) {
    if (!c.signal.aborted) {
      const status=e?.statusCode || e?.status || e?.response?.status;
      if (![400,401,403,404,409,422,428].includes(status)) unknown.value=operation;
      error.value=e instanceof Error ? e.message : String(e);
    }
  } finally { if (!c.signal.aborted) saving.value=false; }
}
const save = () => write(false), adopt = () => write(true);
async function reconcile() {
  if (!unknown.value || saving.value) return;
  const operation=unknown.value,c=controller=new AbortController();saving.value=true;error.value='';
  try { const result=await api.monitoringRequest(c.signal,operation.key);if(!c.signal.aborted)accepted(result,operation); }
  catch(e) { if(!c.signal.aborted)error.value=e instanceof Error ? e.message : String(e); }
  finally { if(!c.signal.aborted)saving.value=false; }
}
async function loadLatest() {
  if (saving.value || unknown.value) return;
  const c=controller=new AbortController();saving.value=true;error.value='';
  try { const status=await api.monitoring(c.signal);if(!c.signal.aborted){localStatus.value=status;fillDraft();notice.value='';emit('saved');} }
  catch(e) { if(!c.signal.aborted)error.value=e instanceof Error ? e.message : String(e); }
  finally { if(!c.signal.aborted)saving.value=false; }
}
onBeforeUnmount(() => controller?.abort());
</script>
