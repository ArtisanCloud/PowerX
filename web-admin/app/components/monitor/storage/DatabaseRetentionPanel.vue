<template>
  <section class="space-y-4" data-testid="database-retention">
    <div class="flex flex-wrap justify-between gap-3"><h2 class="text-lg font-semibold">{{ t('storageGov.dbRetention.title') }}</h2><UButton color="neutral" variant="outline" :loading="busy" @click="load">{{ t('storageGov.refresh') }}</UButton></div>
    <UAlert v-if="error" color="error" :title="error" />
    <UAlert v-if="unknown" color="warning" :title="t('storageGov.dbControl.unknown')" :description="t('storageGov.dbControl.unknownHint') + ' · ' + unknown.key" :ui="{description:'break-all'}" data-testid="db-retention-unknown"><template #actions><UButton :loading="busy" data-testid="db-retention-reconcile" @click="reconcile">{{ t('storageGov.tableMonitor.control.reconcile') }}</UButton></template></UAlert>
    <p v-if="configuration" class="text-sm text-muted" data-testid="db-retention-versions">{{ t('storageGov.dbControl.versions', {desired:configuration.desired_revision,applied:configuration.applied_revision,snapshot:configuration.snapshot_revision ?? '—'}) }} · {{ configuration.authority === 'legacy' ? t('storageGov.dbControl.legacy') : t('storageGov.tableMonitor.control.states.'+configuration.state) }}</p>
    <UAlert v-if="readinessIssues.length" color="warning" variant="soft" :title="t('storageGov.dbRetention.blocked')" :description="readinessIssues.map(issue => issue.title).join(' · ')">
      <template #actions><UButton color="neutral" variant="outline" size="xs" @click="readinessOpen = true">{{ t('storageGov.dbRetention.preparation') }}</UButton></template>
    </UAlert>
    <UAlert v-if="status?.relation_bytes != null && status.relation_bytes >= (capacityPolicy?.max_bytes ?? status.policy.max_bytes)" color="warning" :title="t('storageGov.dbRetention.overBudget')" />
    <template v-if="status">
      <UCard><p class="font-medium">{{ t(status.policy.enabled && configuration?.execution_ready && status.worker_enabled && status.enforcement_allowed ? 'storageGov.runtime.enabled' : 'storageGov.runtime.disabled') }} · {{ bytes(status.relation_bytes) }}</p><p class="mt-2 text-sm">{{ t((configuration?.effective_capacity?.reject_new_when_full ?? status.policy.reject_new_when_full) ? 'storageGov.dbRetention.guardOn' : 'storageGov.dbRetention.guardOff') }} · {{ bytes(capacityPolicy?.max_bytes) }}</p><p v-if="configuration && configuration.desired_revision > configuration.applied_revision && capacityPolicy?.reject_new_when_full" class="mt-2 text-sm text-warning">{{ t('storageGov.dbControl.pendingCapacity') }}</p><p class="mt-2 text-sm text-muted">{{ t('storageGov.dbRetention.scope') }}</p><p class="mt-2 text-sm">{{ t('storageGov.dbRetention.summary', { success: status.policy.success_days, failure: status.policy.failure_days, quarantine: status.policy.quarantine_days }) }}</p></UCard>
      <p class="text-sm text-muted">{{ t('storageGov.dbRetention.reclaim') }}</p>
      <div class="flex flex-wrap gap-3">
        <UButton v-if="configuration?.authority === 'legacy'" :disabled="!configuration?.ready || blocked" data-testid="db-retention-adopt" @click="openAction('adopt')">{{ t('storageGov.dbControl.adopt') }}</UButton>
        <UButton :disabled="status.state_configured === false || configuration?.authority !== 'database' || !configuration?.ready || blocked" data-testid="db-retention-edit" @click="edit">{{ t('storageGov.runtime.configure') }}</UButton>
        <UButton v-if="configuration?.authority === 'database'" color="neutral" variant="outline" :disabled="!configuration?.ready || blocked || (!status.policy.enabled && !status.ready)" data-testid="db-retention-toggle" @click="openAction('status')">{{ t(status.policy.enabled ? 'storageGov.dbControl.pause' : 'storageGov.dbControl.enable') }}</UButton>
        <UButton color="neutral" variant="outline" :disabled="!status.ready || blocked" data-testid="db-retention-preview" @click="previewBatch">{{ t('storageGov.dbRetention.preview') }}</UButton>
        <UButton color="neutral" variant="outline" @click="runsOpen = true">{{ t('storageGov.runtime.runs') }} · {{ status.runs.length }}</UButton>
      </div>
    </template>
    <UModal v-model:open="readinessOpen" :title="t('storageGov.dbRetention.preparation')" :description="t('storageGov.dbRetention.preparationHint')">
      <template #body><ul class="space-y-4"><li v-for="issue in readinessIssues" :key="issue.key"><h3 class="font-medium">{{ issue.title }}</h3><p class="mt-1 text-sm text-muted">{{ issue.description }}</p></li></ul></template>
    </UModal>
    <UModal v-model:open="editorOpen" :title="t('storageGov.dbRetention.title')" :description="t('storageGov.dbRetention.scope')" :dismissible="!blocked" :close="blocked ? false : undefined">
      <template #body><UForm id="database-retention-form" :schema="schema" :state="form" class="space-y-4" @submit="save">
        <div class="grid gap-3 sm:grid-cols-2"><UFormField v-for="field in fields" :key="field" :name="field" :label="t('storageGov.dbRetention.' + field)"><UInput v-model.number="form[field]" type="number" class="w-full" :disabled="blocked" /></UFormField></div>
        <UCheckbox v-model="form.adopt_legacy" :label="t('storageGov.dbRetention.adopt')" :disabled="blocked" />
        <UCheckbox v-model="form.reject_new_when_full" :label="t('storageGov.dbRetention.reject')" :disabled="blocked || status?.enforcement_allowed === false" />
        <p class="text-sm text-muted">{{ t('storageGov.dbControl.savePaused') }}</p>
        <p class="text-sm text-muted">{{ t('storageGov.dbRetention.pending') }}</p>
        <UCheckbox v-model="confirmed" :label="t('storageGov.dbRetention.confirm')" :disabled="blocked" />
        <p v-if="error" role="alert" class="text-error">{{ error }}</p><p v-if="unknown" class="break-all text-xs text-muted">{{ unknown.key }}</p><UButton v-if="unknown" type="button" :loading="busy" data-testid="db-retention-reconcile-editor" @click="reconcile">{{ t('storageGov.tableMonitor.control.reconcile') }}</UButton>
      </UForm></template>
      <template #footer><UButton type="submit" form="database-retention-form" :disabled="!confirmed || blocked" :loading="busy" data-testid="db-retention-save">{{ t('storageGov.drafts.save') }}</UButton><UButton color="neutral" variant="ghost" :disabled="blocked" data-testid="db-retention-latest" @click="loadLatest">{{ t('storageGov.tableMonitor.control.loadLatest') }}</UButton></template>
    </UModal>
    <UModal v-model:open="actionOpen" :title="t(action === 'adopt' ? 'storageGov.dbControl.adopt' : actionEnabled ? 'storageGov.dbControl.enable' : 'storageGov.dbControl.pause')" :description="t('storageGov.dbControl.adoptionHint')" :dismissible="!blocked" :close="blocked ? false : undefined">
      <template #body><p>{{ t(action === 'adopt' ? 'storageGov.dbControl.adoptionHint' : 'storageGov.dbControl.activationHint') }}</p><UCheckbox v-model="confirmed" :disabled="blocked" :label="t('storageGov.dbRetention.confirm')" data-testid="db-retention-action-confirm" /><p v-if="error" role="alert" class="text-error">{{ error }}</p><p v-if="unknown" class="break-all text-xs text-muted">{{ unknown.key }}</p><UButton v-if="unknown" :loading="busy" data-testid="db-retention-reconcile-action" @click="reconcile">{{ t('storageGov.tableMonitor.control.reconcile') }}</UButton></template>
      <template #footer><UButton :disabled="!confirmed || blocked" :loading="busy" data-testid="db-retention-action-save" @click="changeAction">{{ t('storageGov.runtime.save') }}</UButton></template>
    </UModal>
    <UModal v-model:open="previewOpen" :title="t('storageGov.dbRetention.preview')" :description="t('storageGov.dbRetention.batchHint')" :dismissible="!blocked" :close="blocked ? false : undefined">
      <template #body><dl v-if="preview" class="grid grid-cols-2 gap-3"><template v-for="key in ['traces', 'events', 'quarantined'] as const" :key="key"><dt>{{ t('storageGov.dbRetention.' + key) }}</dt><dd>{{ preview.counts[key] }}</dd></template></dl><p class="mt-3 text-sm text-muted">{{ t('storageGov.dbControl.previewHint') }}</p><p v-if="error" role="alert" class="mt-3 text-error">{{ error }}</p><p v-if="unknown" class="break-all text-xs text-muted">{{ unknown.key }}</p><UButton v-if="unknown" :loading="busy" data-testid="db-retention-reconcile-run" @click="reconcile">{{ t('storageGov.tableMonitor.control.reconcile') }}</UButton></template>
      <template #footer><UButton color="warning" :disabled="!status?.policy.enabled || !configuration?.execution_ready || preview?.revision !== status?.policy.revision || blocked" :loading="busy" data-testid="db-retention-run" @click="runBatch">{{ t('storageGov.dbRetention.run') }}</UButton></template>
    </UModal>
    <UModal v-model:open="runsOpen" :title="t('storageGov.runtime.runs')" :description="t('storageGov.dbRetention.batchHint')" :ui="{ content: 'max-w-3xl' }"><template #body><ul class="max-h-[60vh] space-y-3 overflow-auto"><li v-for="run in status?.runs || []" :key="run.id" class="rounded border border-default p-3"><p>{{ new Date(run.at).toLocaleString() }} · {{ t(run.state === 'failed' ? 'storageGov.dbControl.failed' : 'storageGov.runtime.states.'+run.state) }} · v{{ run.revision }} · {{ t('storageGov.dbControl.'+(run.trigger === 'scheduled' ? 'scheduled' : run.trigger === 'manual' ? 'manual' : 'historical')) }}</p><p v-if="run.state === 'completed'">{{ t('storageGov.dbRetention.traces') }} {{ run.counts.traces }} · {{ t('storageGov.dbRetention.events') }} {{ run.counts.events }} · {{ t('storageGov.dbRetention.quarantined') }} {{ run.counts.quarantined }}</p><p v-else class="text-warning">{{ run.problem || t('storageGov.dbRetention.unconfirmed') }}</p><details class="mt-2 text-xs text-muted"><summary>{{ t('storageGov.dbControl.receipt') }}</summary><p class="mt-1 break-all">{{ run.id }}</p></details></li></ul></template></UModal>
  </section>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import * as v from 'valibot';
import { useStorageRuntimeService, type DatabaseRetentionStatus, type DatabaseRetentionRun, type DatabaseRetentionSettings, type DatabaseRetentionMutation } from '~/composables/api/services/storageRuntimeService';
import { formatStorageBytes as bytes } from '~/utils/storage-capacity';
const props = defineProps<{ revision?: number }>();
const { t } = useI18n(), api = useStorageRuntimeService(), GiB = 1024 ** 3;
const status = ref<DatabaseRetentionStatus>(), preview = ref<DatabaseRetentionRun>();
const busy = ref(false), error = ref(''), editorOpen = ref(false), previewOpen = ref(false), runsOpen = ref(false), confirmed = ref(false);
const actionEnabled=ref(false);let actionRevision=0,adoptionDigest='';
const readinessOpen = ref(false), actionOpen = ref(false), action = ref<'adopt'|'status'>('adopt');
const configuration = computed(() => status.value?.configuration);
const capacityPolicy=computed(()=>configuration.value?.effective_capacity ?? status.value?.policy);
type Operation = {kind:'policy'|'run'; key:string; revision:number};
const unknown=ref<Operation>(), blocked=computed(()=>busy.value || !!unknown.value);
let initialized=false;
const readinessIssues = computed(() => {
  const value = status.value;
  if (!value) return [];
  const issues: { key: string; title: string; description: string }[] = [];
  const addProblem = (problem: string) => {
    const short = problem === 'database_retention_runtime_not_configured' ? 'runtimeMissing' : problem === 'database_retention_enforcement_not_allowed' ? 'enforcementMissing' : 'migrationBlocked';
    issues.push({ key: problem, title: t('storageGov.dbRetention.' + short), description: problemText(problem) });
  };
  if (!value.ready) addProblem(value.problem || 'readiness_unknown');
  if (value.migration_problem && value.migration_problem !== value.problem) addProblem(value.migration_problem);
  if (value.worker_enabled === false) issues.push({ key: 'worker', title: t('storageGov.dbRetention.workerInactive'), description: t('storageGov.dbRetention.workerOff') });
  if ((value.unconfirmed_runs || 0) > 0) issues.push({ key: 'unconfirmed', title: t('storageGov.dbRetention.resultsReview'), description: t('storageGov.dbRetention.unconfirmedLimit') });
  return issues;
});
const fields = ['success_days', 'failure_days', 'pending_hours', 'quarantine_days', 'batch_limit', 'gib'] as const;
const form = reactive({ success_days: 30, failure_days: 90, pending_hours: 24, quarantine_days: 30, batch_limit: 200, gib: 8, adopt_legacy: false, reject_new_when_full: false, enabled: false });
const integer = (max: number) => v.pipe(v.number(), v.integer(), v.minValue(1), v.maxValue(max));
const schema = v.pipe(v.object({ success_days: integer(3650), failure_days: integer(3650), pending_hours: integer(720), quarantine_days: integer(3650), batch_limit: integer(1000), gib: v.pipe(v.number(), v.minValue(0.0625), v.maxValue(1048576)), enabled: v.boolean(), adopt_legacy: v.boolean(), reject_new_when_full: v.boolean() }), v.check(x => x.failure_days >= x.success_days));
let controller: AbortController | undefined, readController: AbortController | undefined, disposed = false, editRevision = 0, requestID = '';
function problemText(problem?: string) {
  const reasons: Record<string, string> = {
    STORAGE_CONTROL_SCHEMA_NOT_READY:'controlRequired',
    DATABASE_RETENTION_ADOPTION_REQUIRED:'adoptionHint',
    DATABASE_RETENTION_PUBLICATION_PENDING:'publicationPending',
    database_retention_runtime_not_configured: 'runtimeNotConfigured',
    database_retention_enforcement_not_allowed: 'deploymentBlocked',
    database_retention_migration_required: 'columnsRequired',
    database_retention_index_required: 'indexesRequired',
    database_unavailable: 'databaseUnavailable',
    tenant_registry_unavailable: 'tenantRegistryRequired',
    database_external_references_require_review: 'referencesReview',
  };
  return t('storageGov.dbRetention.' + (reasons[problem || ''] || 'readinessUnknown'));
}
async function load() { readController?.abort(); const c = readController = new AbortController(); try { const value = await api.databaseStatus(c.signal); if (!disposed && !c.signal.aborted) { status.value = value; error.value = ''; } } catch (e) { if (!disposed && !c.signal.aborted) error.value = e instanceof Error ? e.message : String(e); } }
function edit() { if (!status.value || blocked.value) return; if (!initialized) fillDraft(); confirmed.value = false; editorOpen.value = true; }
function fillDraft(){if(!status.value)return;const p=status.value.policy;editRevision=p.revision;Object.assign(form,{success_days:p.success_days,failure_days:p.failure_days,pending_hours:p.pending_hours,quarantine_days:p.quarantine_days,batch_limit:p.batch_limit,gib:p.max_bytes/GiB,adopt_legacy:p.adopt_legacy,reject_new_when_full:p.reject_new_when_full,enabled:p.enabled});initialized=true;confirmed.value=false;}
async function loadLatest(){if(blocked.value)return;await load();fillDraft();}
function settings():DatabaseRetentionSettings {const {gib,enabled,...values}=form;return {...values,max_bytes:Math.round(gib*GiB)};}
async function acceptMutation(m:DatabaseRetentionMutation,op:Operation){if(m.request_id!==op.key || m.operation_revision!==op.revision+1)throw new Error('DATABASE_RETENTION_RESPONSE_INVALID');unknown.value=undefined;editorOpen.value=false;actionOpen.value=false;confirmed.value=false;initialized=false;await load();}
async function write(op:Operation,call:(signal:AbortSignal)=>Promise<DatabaseRetentionMutation|DatabaseRetentionRun>){if(blocked.value)return;const c=controller=new AbortController();busy.value=true;error.value='';try{const r=await call(c.signal);if(disposed||c.signal.aborted)return;if(op.kind==='policy')await acceptMutation(r as DatabaseRetentionMutation,op);else{const run=r as DatabaseRetentionRun;if(run.id!==op.key || run.revision!==op.revision || run.state!=='completed')throw new Error('DATABASE_RETENTION_RESPONSE_INVALID');previewOpen.value=false;runsOpen.value=true;unknown.value=undefined;preview.value=undefined;await load();}}catch(e:any){if(!disposed&&!c.signal.aborted){error.value=e?.message||String(e);const code=e?.status||e?.statusCode||e?.response?.status;if(![400,401,403,409,422,428].includes(code))unknown.value=op}}finally{if(!disposed)busy.value=false}}
async function reconcile(){const op=unknown.value;if(!op||busy.value)return;const c=controller=new AbortController();busy.value=true;error.value='';try{if(op.kind==='policy')await acceptMutation(await api.databasePolicyRequest(op.key,c.signal),op);else{const r=await api.databaseRunRequest(op.key,c.signal);if(r.id!==op.key||r.revision!==op.revision||r.state!=='completed')throw new Error('DATABASE_RETENTION_RESPONSE_INVALID');unknown.value=undefined;previewOpen.value=false;preview.value=undefined;runsOpen.value=true;await load();}}catch{if(!disposed&&!c.signal.aborted)error.value=t('storageGov.dbControl.unknownHint')}finally{if(!disposed)busy.value=false}}
async function save(){if(!confirmed.value||!v.safeParse(schema,form).success)return;const op:Operation={kind:'policy',key:crypto.randomUUID(),revision:editRevision};await write(op,signal=>api.changeDatabasePolicy({request_id:op.key,expected_revision:op.revision,action:'save',settings:settings(),confirm:true},signal));}
function openAction(kind:'adopt'|'status'){if(!configuration.value||blocked.value)return;action.value=kind;actionRevision=configuration.value.desired_revision;adoptionDigest=configuration.value.legacy_digest;actionEnabled.value=!status.value!.policy.enabled;confirmed.value=false;actionOpen.value=true;}
async function changeAction(){if(!confirmed.value||!configuration.value)return;const op:Operation={kind:'policy',key:crypto.randomUUID(),revision:actionRevision};const hash=adoptionDigest, enabled=actionEnabled.value;await write(op,signal=>action.value==='adopt'?api.adoptDatabasePolicy({request_id:op.key,legacy_digest:hash,confirm:true},signal):api.changeDatabasePolicy({request_id:op.key,expected_revision:op.revision,action:'status',enabled,confirm:true},signal));}
async function previewBatch(){if(!status.value||blocked.value)return;const c=controller=new AbortController();busy.value=true;error.value='';try{preview.value=await api.previewDatabase(status.value.policy.revision,c.signal);requestID=crypto.randomUUID();previewOpen.value=true}catch(e:any){if(!disposed&&!c.signal.aborted)error.value=e?.message||String(e)}finally{if(!disposed)busy.value=false}}
async function runBatch(){if(!preview.value)return;const p=preview.value;await write({kind:'run',key:requestID,revision:p.revision},signal=>api.runDatabase(p.revision,requestID,p.id,signal));}
watch(() => props.revision, load); onMounted(load); onBeforeUnmount(() => { disposed = true; controller?.abort(); readController?.abort(); });
</script>
