<template>
  <section class="flex min-w-0 flex-wrap items-center justify-between gap-3 rounded-xl border border-default bg-elevated/40 px-4 py-3" data-testid="execution-control">
    <div class="min-w-0"><p class="text-sm font-semibold">{{ t('storageGov.execution.title') }} · <span :class="protectedMode ? 'text-warning' : 'text-muted'" role="status" data-testid="execution-status">{{ statusLabel }}</span></p><p class="mt-1 text-xs text-muted">{{ t('storageGov.execution.scopeShort') }}</p></div>
    <div class="flex flex-wrap gap-2"><UButton v-if="configuration?.recovery?.state === 'pending'" color="warning" size="sm" data-testid="execution-recovery-open" @click="open = true; section = 'recovery'">{{ t('storageGov.execution.recovery.open') }}</UButton><UButton color="error" variant="outline" size="sm" :disabled="!configuration?.can_pause || configuration.mode === 'handoff' || busy || !!error" data-testid="execution-emergency" @click="prepare('pause')">{{ t('storageGov.execution.pause') }}</UButton><UButton color="neutral" variant="outline" size="sm" data-testid="execution-manage" @click="open = true">{{ t('storageGov.execution.manage') }}</UButton></div>
    <UModal v-model:open="open" :title="t('storageGov.execution.title')" :description="t('storageGov.execution.scopeShort')" :dismissible="!busy" :close="!busy" :ui="{ content: 'max-w-4xl' }">
      <template #body><div class="min-w-0 space-y-4" data-testid="execution-modal">
        <UAlert v-if="error" color="error" :title="t('storageGov.requestFailed')" :description="error" data-testid="execution-error" />
        <UAlert v-if="unknown" color="warning" :title="t('storageGov.execution.unknown')" :description="t('storageGov.execution.unknownHint')" data-testid="execution-unknown"><template #actions><UButton color="neutral" variant="outline" :loading="saving" data-testid="execution-reconcile" @click="reconcile">{{ t('storageGov.execution.reconcile') }}</UButton></template></UAlert>
        <BackupSectionTabs v-model="section" :items="sections" :label="t('storageGov.execution.title')">
          <template #overview><div class="space-y-4">
            <div class="rounded-xl border border-default p-4"><p class="font-semibold">{{ statusLabel }}</p><p class="mt-2 text-sm text-muted">{{ t('storageGov.execution.modesHint.' + (configuration?.mode || 'unavailable')) }}</p><p v-if="configuration" class="mt-3 text-xs text-muted">{{ t('storageGov.execution.versions', { desired: configuration.desired_revision, applied: configuration.applied_revision }) }}</p></div>
            <UAlert v-if="configuration && !configuration.worker_ready" color="warning" :title="t('storageGov.execution.workerUnavailable')" :description="t('storageGov.execution.pauseStillAvailable')" />
            <UAlert color="info" :title="t('storageGov.execution.boundary')" :description="t('storageGov.execution.boundaryHint')" />
            <div class="flex flex-wrap gap-2"><UButton :disabled="!configuration?.can_resume || busy || !!error" data-testid="execution-resume" @click="prepare('resume')">{{ t('storageGov.execution.resume') }}</UButton><UButton color="neutral" variant="outline" :loading="loading" data-testid="execution-refresh" @click="refresh">{{ t('storageGov.refresh') }}</UButton></div>
          </div></template>
          <template #operation><div class="space-y-4">
            <p class="font-semibold">{{ t('storageGov.execution.actions.' + action) }}</p><p class="text-sm text-muted">{{ t('storageGov.execution.actionHints.' + action) }}</p>
            <p v-if="baseRevision !== configuration?.desired_revision || ['acknowledge_restore', 'prepare_restore'].includes(action) && baseEpoch !== configuration?.recovery?.epoch" class="text-sm text-warning" data-testid="execution-stale">{{ t('storageGov.execution.stale', { version: baseRevision }) }}</p>
            <UForm :state="form" :schema="schema" class="space-y-4" @submit="mutate">
              <UFormField name="reason" :label="t('storageGov.execution.reason')"><UTextarea v-model="form.reason" class="w-full" :maxlength="500" :disabled="busy" data-testid="execution-reason" /></UFormField>
              <template v-if="action === 'complete_handoff' || ['acknowledge_restore', 'prepare_restore'].includes(action)"><UCheckbox v-model="externalStopped" :disabled="busy" :label="t('storageGov.execution.externalAck')" data-testid="execution-external" /><UCheckbox v-model="admittedChecked" :disabled="busy" :label="t('storageGov.execution.tasksAck')" data-testid="execution-tasks" /><UCheckbox v-if="['acknowledge_restore', 'prepare_restore'].includes(action)" v-model="environmentChecked" :disabled="busy" :label="t('storageGov.execution.recovery.environmentAck')" data-testid="execution-environment" /><p class="text-xs text-muted">{{ t('storageGov.execution.operatorDeclaration') }}</p></template>
              <UCheckbox v-model="confirmed" :disabled="busy" :label="t('storageGov.execution.confirm')" data-testid="execution-confirm" />
              <div class="flex flex-wrap gap-2"><UButton type="submit" :color="action === 'pause' || action === 'begin_handoff' ? 'error' : 'primary'" :loading="saving" :disabled="!canSubmit" data-testid="execution-submit">{{ t('storageGov.execution.actions.' + action) }}</UButton><UButton color="neutral" variant="ghost" :disabled="busy" data-testid="execution-latest" @click="loadLatest">{{ t('storageGov.execution.loadLatest') }}</UButton></div>
            </UForm>
          </div></template>
          <template #handoff><div class="space-y-4">
            <UAlert v-if="configuration?.legacy_handoff_available === false" color="warning" :title="t('storageGov.release.handoffUnavailable')" :description="t('storageGov.release.handoffHint')" data-testid="execution-handoff-release-unavailable" />
            <UAlert color="warning" :title="t('storageGov.execution.handoffTitle')" :description="t('storageGov.execution.handoffHint')" />
            <p class="text-sm text-muted">{{ t('storageGov.execution.inventoryHint') }}</p>
            <ul class="space-y-2" data-testid="execution-policies"><li v-for="p in configuration?.policies || []" :key="p.domain" class="flex min-w-0 flex-wrap items-start justify-between gap-2 rounded-lg border border-default p-3 text-sm"><div class="min-w-0"><p class="font-medium">{{ t('storageGov.execution.policyKinds.' + p.kind) }}<span v-if="p.resource_name"> · {{ p.resource_name }}</span></p><details class="mt-1 text-xs text-muted"><summary class="cursor-pointer">{{ t('storageGov.execution.policyIdentifier') }}</summary><code class="mt-1 block break-all">{{ p.domain }}</code></details></div><span :class="p.paused && p.confirmed ? 'text-success' : 'text-warning'">{{ t(p.paused && p.confirmed ? 'storageGov.execution.policyPaused' : 'storageGov.execution.policyBlocker') }}</span></li></ul>
            <p v-if="!configuration?.policies.length" class="text-sm text-muted">{{ t('storageGov.execution.noPolicies') }}</p>
            <div class="flex flex-wrap gap-2"><UButton v-if="configuration?.mode !== 'handoff'" color="warning" :disabled="configuration?.legacy_handoff_available === false || busy || !configuration?.can_pause || !!error" data-testid="execution-begin" @click="prepare('begin_handoff')">{{ t('storageGov.execution.actions.begin_handoff') }}</UButton><UButton v-else :disabled="configuration?.legacy_handoff_available === false || busy || !configuration.confirmed || !configuration.worker_ready || blockers || !!error" data-testid="execution-complete" @click="prepare('complete_handoff')">{{ t('storageGov.execution.actions.complete_handoff') }}</UButton></div>
          </div></template>
          <template #recovery><div class="space-y-4" data-testid="execution-recovery">
            <UAlert :color="configuration?.recovery?.state === 'pending' ? 'warning' : 'info'" :title="t('storageGov.execution.recovery.title')" :description="t('storageGov.execution.recovery.states.' + (configuration?.recovery?.state || 'not_connected'))" />
            <p class="text-sm text-muted">{{ t('storageGov.execution.recovery.hint') }}</p>
            <template v-if="configuration?.recovery?.epoch"><p class="text-sm">{{ t('storageGov.execution.recovery.at') }} · {{ new Date(configuration.recovery.created_at || '').toLocaleString() }}</p><details class="text-xs text-muted"><summary class="cursor-pointer">{{ t('storageGov.execution.recovery.details') }}</summary><dl class="mt-2 space-y-2"><div v-for="key in ['job_id', 'snapshot_id', 'epoch'] as const" :key="key"><dt>{{ t('storageGov.execution.recovery.' + key) }}</dt><dd class="break-all font-mono">{{ configuration.recovery[key] }}</dd></div></dl></details></template>
            <p class="text-sm text-muted">{{ t('storageGov.execution.recovery.rebuildHint') }}</p>
            <ul class="space-y-2" data-testid="execution-recovery-resources"><li v-for="resource in configuration?.recovery?.resources || []" :key="resource.domain" class="rounded-lg border border-default p-3 text-sm"><p class="font-medium">{{ t('storageGov.execution.policyKinds.' + resource.kind) }} · #{{ resource.revision }}</p><p :class="resource.state === 'ready' ? 'text-success' : 'text-warning'">{{ resource.state === 'ready' ? t('storageGov.execution.recovery.resourceReady') : t('storageGov.execution.recovery.problems.' + (resource.problem || 'STORAGE_RECOVERY_REBUILD_REQUIRED')) }}</p></li></ul>
            <UAlert v-if="blockers" color="warning" :title="t('storageGov.execution.recovery.policiesBlocked')" />
            <UButton color="warning" variant="outline" :disabled="busy || !!error || configuration?.mode !== 'recovery' || !configuration?.recovery?.epoch || !configuration.worker_ready" data-testid="execution-recovery-prepare" @click="prepare('prepare_restore')">{{ t('storageGov.execution.actions.prepare_restore') }}</UButton>
            <div class="flex flex-wrap gap-2"><UButton :disabled="busy || !!error || configuration?.mode !== 'recovery' || !configuration?.recovery?.epoch || !configuration.worker_ready || blockers || recoveryBlockers" data-testid="execution-recovery-acknowledge" @click="prepare('acknowledge_restore')">{{ t('storageGov.execution.actions.acknowledge_restore') }}</UButton><UButton color="neutral" variant="outline" @click="section = 'handoff'">{{ t('storageGov.execution.recovery.policies') }}</UButton></div>
          </div></template>
          <template #receipt><div class="space-y-3" data-testid="execution-receipt"><template v-if="receipt"><p class="font-medium">{{ t('storageGov.execution.actions.' + receipt.operation.action) }} · #{{ receipt.operation_revision }}</p><p class="break-all font-mono text-xs">{{ receipt.request_id }}</p><p class="text-sm">{{ receipt.operation.reason }}</p><p class="text-sm text-muted">{{ t('storageGov.execution.receiptHint') }}</p></template><p v-else class="text-sm text-muted">{{ t('storageGov.execution.noReceipt') }}</p></div></template>
        </BackupSectionTabs>
      </div></template>
    </UModal>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import * as v from 'valibot';
import BackupSectionTabs from '~/components/ops/backup/BackupSectionTabs.vue';
import { sameExecutionOperation, useStorageExecutionService, type ExecutionAction, type ExecutionChange, type ExecutionConfiguration, type ExecutionMutation } from '~/composables/api/services/storageExecutionService';
const props = defineProps<{ revision: number }>();
const { t } = useI18n(), api = useStorageExecutionService();
const open = ref(false), section = ref('overview'), action = ref<ExecutionAction>('pause'), configuration = ref<ExecutionConfiguration>();
const loading = ref(false), saving = ref(false), error = ref(''), confirmed = ref(false), externalStopped = ref(false), admittedChecked = ref(false), baseRevision = ref(0), baseEpoch = ref(''), environmentChecked = ref(false);
const form = reactive({ reason: '' }), unknown = ref<ExecutionChange>(), receipt = ref<ExecutionMutation>();
const schema = computed(() => v.object({ reason: v.pipe(v.string(), v.trim(), v.check(s => Array.from(s).length >= 3 && Array.from(s).length <= 500, t('storageGov.execution.reasonInvalid'))) }));
const busy = computed(() => saving.value || !!unknown.value);
const sections = computed(() => ['overview', 'operation', 'recovery', 'handoff', 'receipt'].map(value => ({ value, label: t('storageGov.execution.sections.' + value) })));
const protectedMode = computed(() => ['paused', 'handoff', 'recovery'].includes(configuration.value?.mode || '') || configuration.value?.mode === 'open' && (!configuration.value.confirmed || !configuration.value.worker_ready));
const statusLabel = computed(() => error.value ? t('storageGov.execution.modes.unavailable') : configuration.value?.mode === 'open' && (!configuration.value.confirmed || !configuration.value.worker_ready) ? t('storageGov.execution.openPending') : t('storageGov.execution.modes.' + (configuration.value?.mode || 'unavailable')));
const blockers = computed(() => configuration.value?.policies.some(p => !p.paused || !p.confirmed) ?? true);
const recoveryBlockers = computed(() => configuration.value?.recovery?.prepared !== true || (configuration.value?.recovery?.resources ?? []).some(r => r.state !== 'ready'));
const actionAllowed = computed(() => {
  const c = configuration.value;
  if (!c || error.value) return false;
  if (c.legacy_handoff_available === false && ['begin_handoff','complete_handoff'].includes(action.value)) return false;
  if (action.value === 'prepare_restore') return c.mode === 'recovery' && !!c.recovery?.epoch && c.worker_ready && externalStopped.value && admittedChecked.value && environmentChecked.value;
  if (action.value === 'acknowledge_restore') return !recoveryBlockers.value && c.mode === 'recovery' && !!c.recovery?.epoch && c.worker_ready && !blockers.value && externalStopped.value && admittedChecked.value && environmentChecked.value;
  if (action.value === 'resume') return c.can_resume;
  if (action.value === 'complete_handoff') return c.mode === 'handoff' && c.confirmed && c.worker_ready && !blockers.value && externalStopped.value && admittedChecked.value;
  return c.can_pause && (action.value !== 'pause' || c.mode !== 'handoff');
});
const canSubmit = computed(() => !busy.value && !loading.value && confirmed.value && actionAllowed.value && v.safeParse(schema.value, form).success);
let read: AbortController | undefined, write: AbortController | undefined;
function accept(c: ExecutionConfiguration) {
  if (configuration.value && c.desired_revision < configuration.value.desired_revision && c.mode !== 'unavailable' && !(c.mode === 'recovery' && c.recovery?.epoch && c.recovery.epoch !== configuration.value.recovery?.epoch)) {
    error.value = t('storageGov.execution.rollback');
    return;
  }
  configuration.value = c;
}
async function refresh() {
  read?.abort(); const r = read = new AbortController(); loading.value = true;
  try { const c = await api.configuration(r.signal); if (!r.signal.aborted) { error.value = ''; accept(c); } return !r.signal.aborted; }
  catch (e: any) { if (!r.signal.aborted) error.value = e?.message || t('storageGov.requestFailed'); return false; }
  finally { if (!r.signal.aborted) loading.value = false; }
}
function prepare(value: ExecutionAction) {
  if (busy.value) return;
  action.value = value; baseRevision.value = configuration.value?.desired_revision || 0; baseEpoch.value = configuration.value?.recovery?.epoch || '';
  environmentChecked.value = false; confirmed.value = false; externalStopped.value = false; admittedChecked.value = false;
  open.value = true; section.value = 'operation';
}
async function loadLatest() { if (busy.value) return; if (await refresh()) { baseRevision.value = configuration.value?.desired_revision || 0; baseEpoch.value = configuration.value?.recovery?.epoch || ''; confirmed.value = false; externalStopped.value = false; admittedChecked.value = false; environmentChecked.value = false; } }
function accepted(m: ExecutionMutation, op: ExecutionChange) {
  if (m.request_id !== op.request_id || (op.action === 'prepare_restore' ? m.operation_revision <= op.expected_revision : m.operation_revision !== op.expected_revision + 1) || !sameExecutionOperation(m.operation, op)) throw new Error('STORAGE_EXECUTION_RESPONSE_INVALID');
  accept(m.configuration); receipt.value = m; unknown.value = undefined; confirmed.value = false; section.value = op.action === 'prepare_restore' ? 'recovery' : 'overview';
}
async function mutate() {
  if (!canSubmit.value) return;
  const op: ExecutionChange = { request_id: crypto.randomUUID(), expected_revision: baseRevision.value, action: action.value, reason: form.reason.trim(), confirm: true, external_stopped: externalStopped.value, admitted_tasks_checked: admittedChecked.value, ...(['acknowledge_restore', 'prepare_restore'].includes(action.value) ? { expected_recovery_epoch: baseEpoch.value, environment_checked: environmentChecked.value } : {}) };
  read?.abort(); loading.value = false; const r = write = new AbortController(); saving.value = true; error.value = '';
  try { const m = await api.change(op, r.signal); if (!r.signal.aborted) accepted(m, op); }
  catch (e: any) { if (!r.signal.aborted) { error.value = e?.message || t('storageGov.requestFailed'); const status = e?.status || e?.statusCode || e?.response?.status; if (![400, 401, 403, 409, 422, 428].includes(status)) unknown.value = op; } }
  finally { if (!r.signal.aborted) saving.value = false; }
}
async function reconcile() {
  const op = unknown.value; if (!op || saving.value) return;
  const r = write = new AbortController(); saving.value = true;
  try { const m = await api.request(op.request_id, r.signal); if (!r.signal.aborted) { error.value = ''; accepted(m, op); } }
  catch { if (!r.signal.aborted) error.value = t('storageGov.execution.unknownHint'); }
  finally { if (!r.signal.aborted) saving.value = false; }
}
watch(form, () => { confirmed.value = false; });
watch(() => props.revision, () => { void refresh(); }, { immediate: true });
onBeforeUnmount(() => { read?.abort(); write?.abort(); });
</script>
