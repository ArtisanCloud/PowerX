<template>
  <section class="space-y-4" data-testid="database-retention">
    <div class="flex flex-wrap justify-between gap-3"><h2 class="text-lg font-semibold">{{ t('storageGov.dbRetention.title') }}</h2><UButton color="neutral" variant="outline" :loading="busy" @click="load">{{ t('storageGov.refresh') }}</UButton></div>
    <UAlert v-if="error" color="error" :title="error" />
    <UAlert v-if="readinessIssues.length" color="warning" variant="soft" :title="t('storageGov.dbRetention.blocked')" :description="readinessIssues.map(issue => issue.title).join(' · ')">
      <template #actions><UButton color="neutral" variant="outline" size="xs" @click="readinessOpen = true">{{ t('storageGov.dbRetention.preparation') }}</UButton></template>
    </UAlert>
    <UAlert v-if="status?.relation_bytes != null && status.relation_bytes >= status.policy.max_bytes" color="warning" :title="t('storageGov.dbRetention.overBudget')" />
    <template v-if="status">
      <UCard><p class="font-medium">{{ t(status.policy.enabled ? 'storageGov.runtime.enabled' : 'storageGov.runtime.disabled') }} · {{ bytes(status.relation_bytes) }}</p><p class="mt-2 text-sm">{{ t(status.policy.reject_new_when_full ? 'storageGov.dbRetention.guardOn' : 'storageGov.dbRetention.guardOff') }}</p><p class="mt-2 text-sm text-muted">{{ t('storageGov.dbRetention.scope') }}</p><p class="mt-2 text-sm">{{ t('storageGov.dbRetention.summary', { success: status.policy.success_days, failure: status.policy.failure_days, quarantine: status.policy.quarantine_days }) }}</p></UCard>
      <p class="text-sm text-muted">{{ t('storageGov.dbRetention.reclaim') }}</p>
      <div class="flex flex-wrap gap-3"><UButton :disabled="status.state_configured === false || busy" @click="edit">{{ t('storageGov.runtime.configure') }}</UButton><UButton color="neutral" variant="outline" :disabled="!status.ready || busy" @click="previewBatch">{{ t('storageGov.dbRetention.preview') }}</UButton><UButton color="neutral" variant="outline" @click="runsOpen = true">{{ t('storageGov.runtime.runs') }} · {{ status.runs.length }}</UButton></div>
    </template>
    <UModal v-model:open="readinessOpen" :title="t('storageGov.dbRetention.preparation')" :description="t('storageGov.dbRetention.preparationHint')">
      <template #body><ul class="space-y-4"><li v-for="issue in readinessIssues" :key="issue.key"><h3 class="font-medium">{{ issue.title }}</h3><p class="mt-1 text-sm text-muted">{{ issue.description }}</p></li></ul></template>
    </UModal>
    <UModal v-model:open="editorOpen" :title="t('storageGov.dbRetention.title')" :description="t('storageGov.dbRetention.scope')" :dismissible="!busy" :close="!busy">
      <template #body><UForm id="database-retention-form" :schema="schema" :state="form" class="space-y-4" @submit="save">
        <div class="grid gap-3 sm:grid-cols-2"><UFormField v-for="field in fields" :key="field" :name="field" :label="t('storageGov.dbRetention.' + field)"><UInput v-model.number="form[field]" type="number" class="w-full" :disabled="busy" /></UFormField></div>
        <UCheckbox v-model="form.adopt_legacy" :label="t('storageGov.dbRetention.adopt')" :disabled="busy" />
        <UCheckbox v-model="form.reject_new_when_full" :label="t('storageGov.dbRetention.reject')" :disabled="busy || status?.enforcement_allowed === false" />
        <UCheckbox v-model="form.enabled" :label="t('storageGov.dbRetention.enable')" :disabled="busy || !status?.ready" />
        <p class="text-sm text-muted">{{ t('storageGov.dbRetention.pending') }}</p>
        <UCheckbox v-model="confirmed" :label="t('storageGov.dbRetention.confirm')" :disabled="busy" />
        <p v-if="error" role="alert" class="text-error">{{ error }}</p>
      </UForm></template>
      <template #footer><UButton type="submit" form="database-retention-form" :disabled="!confirmed" :loading="busy">{{ t('storageGov.drafts.save') }}</UButton></template>
    </UModal>
    <UModal v-model:open="previewOpen" :title="t('storageGov.dbRetention.preview')" :description="t('storageGov.dbRetention.batchHint')" :dismissible="!busy" :close="!busy">
      <template #body><dl v-if="preview" class="grid grid-cols-2 gap-3"><template v-for="key in ['traces', 'events', 'quarantined'] as const" :key="key"><dt>{{ t('storageGov.dbRetention.' + key) }}</dt><dd>{{ preview.counts[key] }}</dd></template></dl><p v-if="error" role="alert" class="mt-3 text-error">{{ error }}</p></template>
      <template #footer><UButton color="warning" :disabled="!status?.policy.enabled" :loading="busy" @click="runBatch">{{ t('storageGov.dbRetention.run') }}</UButton></template>
    </UModal>
    <UModal v-model:open="runsOpen" :title="t('storageGov.runtime.runs')" :description="t('storageGov.dbRetention.batchHint')" :ui="{ content: 'max-w-3xl' }"><template #body><ul class="max-h-[60vh] space-y-3 overflow-auto"><li v-for="run in status?.runs || []" :key="run.id" class="rounded border border-default p-3"><p>{{ new Date(run.at).toLocaleString() }} · {{ run.state }}</p><p v-if="run.state === 'completed'">{{ t('storageGov.dbRetention.traces') }} {{ run.counts.traces }} · {{ t('storageGov.dbRetention.events') }} {{ run.counts.events }} · {{ t('storageGov.dbRetention.quarantined') }} {{ run.counts.quarantined }}</p><p v-else class="text-warning">{{ run.problem || t('storageGov.dbRetention.unconfirmed') }}</p></li></ul></template></UModal>
  </section>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import * as v from 'valibot';
import { useStorageRuntimeService, type DatabaseRetentionStatus, type DatabaseRetentionRun } from '~/composables/api/services/storageRuntimeService';
import { formatStorageBytes as bytes } from '~/utils/storage-capacity';
const props = defineProps<{ revision?: number }>();
const { t } = useI18n(), api = useStorageRuntimeService(), GiB = 1024 ** 3;
const status = ref<DatabaseRetentionStatus>(), preview = ref<DatabaseRetentionRun>();
const busy = ref(false), error = ref(''), editorOpen = ref(false), previewOpen = ref(false), runsOpen = ref(false), confirmed = ref(false);
const readinessOpen = ref(false);
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
  if ((value.unconfirmed_runs || 0) >= 100) issues.push({ key: 'unconfirmed', title: t('storageGov.dbRetention.resultsReview'), description: t('storageGov.dbRetention.unconfirmedLimit') });
  return issues;
});
const fields = ['success_days', 'failure_days', 'pending_hours', 'quarantine_days', 'batch_limit', 'gib'] as const;
const form = reactive({ success_days: 30, failure_days: 90, pending_hours: 24, quarantine_days: 30, batch_limit: 200, gib: 8, adopt_legacy: false, reject_new_when_full: false, enabled: false });
const integer = (max: number) => v.pipe(v.number(), v.integer(), v.minValue(1), v.maxValue(max));
const schema = v.pipe(v.object({ success_days: integer(3650), failure_days: integer(3650), pending_hours: integer(720), quarantine_days: integer(3650), batch_limit: integer(1000), gib: v.pipe(v.number(), v.minValue(0.0625), v.maxValue(1048576)), enabled: v.boolean(), adopt_legacy: v.boolean(), reject_new_when_full: v.boolean() }), v.check(x => x.failure_days >= x.success_days));
let controller: AbortController | undefined, readController: AbortController | undefined, disposed = false, editRevision = 0, requestID = '';
function problemText(problem?: string) {
  const reasons: Record<string, string> = {
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
async function perform(action: (signal: AbortSignal) => Promise<void>) { if (busy.value) return; const c = controller = new AbortController(); busy.value = true; error.value = ''; try { await action(c.signal); if (!disposed && !c.signal.aborted) await load(); } catch (e) { if (!disposed && !c.signal.aborted) error.value = e instanceof Error ? e.message : String(e); } finally { if (!disposed && !c.signal.aborted) busy.value = false; } }
function edit() { if (!status.value) return; const p = status.value.policy; editRevision = p.revision; Object.assign(form, p, { gib: p.max_bytes / GiB }); confirmed.value = false; editorOpen.value = true; }
async function save() { if (!status.value || !confirmed.value || !v.safeParse(schema, form).success) return; await perform(async signal => { const { gib, ...values } = form; await api.saveDatabasePolicy(editRevision, { ...status.value!.policy, ...values, max_bytes: Math.round(gib * GiB) }, signal); if (!signal.aborted) editorOpen.value = false; }); }
async function previewBatch() { if (!status.value) return; await perform(async signal => { const value = await api.previewDatabase(status.value!.policy.revision, signal); if (!signal.aborted) { preview.value = value; requestID = crypto.randomUUID(); previewOpen.value = true; } }); }
async function runBatch() { if (!preview.value) return; await perform(async signal => { await api.runDatabase(preview.value!.revision, requestID, signal); if (!signal.aborted) { previewOpen.value = false; runsOpen.value = true; } }); }
watch(() => props.revision, load); onMounted(load); onBeforeUnmount(() => { disposed = true; controller?.abort(); readController?.abort(); });
</script>
