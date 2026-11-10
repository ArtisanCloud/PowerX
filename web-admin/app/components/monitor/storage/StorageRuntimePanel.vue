<template>
  <div class="space-y-4" data-testid="storage-runtime">
    <template v-if="sourceId">
      <div class="flex flex-wrap items-center justify-between gap-2"><h4 class="font-semibold">{{ t('storageGov.resourceConfig.current') }}</h4><UButton color="neutral" variant="ghost" :loading="loading" @click="load">{{ t('storageGov.refresh') }}</UButton></div>
      <UAlert v-if="error" color="error" :title="error" />
      <p v-if="loading && !state" role="status">{{ t('storageGov.loading') }}</p>
      <UAlert v-else-if="!localSource?.supported || !state?.configured || !state.lifecycle?.configured" color="info" :title="t('storageGov.resourceConfig.unsupported')" :description="t('storageGov.resourceConfig.observationOnly')" />
      <template v-else>
        <dl class="grid gap-3 text-sm sm:grid-cols-2" data-testid="resource-log-settings">
          <div><dt class="text-muted">{{ t('storageGov.state') }}</dt><dd class="mt-1">{{ t(localPolicy?.enabled ? 'storageGov.archive.enabled' : 'storageGov.archive.disabled') }}</dd></div>
          <div><dt class="text-muted">{{ t('storageGov.logs.days') }}</dt><dd class="mt-1">{{ localPolicy ? localPolicy.retention_days + ' ' + t('storageGov.runtime.dayUnit') : '—' }}</dd></div>
          <div><dt class="text-muted">{{ t('storageGov.context.resourceBudget') }}</dt><dd class="mt-1">{{ bytes(localPolicy?.max_bytes) }}</dd></div>
          <div><dt class="text-muted">{{ t('storageGov.archive.cleanupMode') }}</dt><dd class="mt-1">{{ localPolicy ? t('storageGov.archive.modes.' + (localPolicy.cleanup_mode || (localPolicy.require_archive ? 'archive_then_delete' : 'expire_delete'))) : '—' }}</dd></div>
        </dl>
        <p class="text-xs text-muted" data-testid="resource-log-versions">{{ configurationLabel(sourceId) }}</p>
      <p v-if="localPolicy?.enabled && (!state.worker_enabled || !state.lifecycle?.allow_enforce)" class="text-sm text-warning">{{ t('storageGov.resourcePolicy.descriptions.blocked') }}</p>
        <div class="flex flex-wrap gap-2"><UButton :disabled="busy || !!unknown" data-testid="resource-log-edit" @click="edit(localSource)">{{ t('storageGov.resourceConfig.edit') }}</UButton><UButton color="neutral" variant="outline" @click="runSource = sourceId; runsOpen = true">{{ t('storageGov.resourcePolicy.automaticRecords') }}</UButton></div>
      </template>
    </template>
    <template v-else>
    <div class="flex flex-wrap items-center justify-between gap-3"><h2 class="text-lg font-semibold">{{ t('storageGov.automation') }}</h2><UButton color="neutral" variant="outline" :loading="loading" @click="load">{{ t('storageGov.refresh') }}</UButton></div>
    <UAlert v-if="error" color="error" :title="t('storageGov.requestFailed')" :description="error" />
    <UAlert v-if="state && !state.configured" color="info" :title="t('storageGov.runtime.notConfigured')" :description="t('storageGov.runtime.setupHint')" />
    <template v-if="state?.configured">
      <UAlert v-if="state.problem || state.notification_problem" color="warning" :title="t('storageGov.runtime.needsAttention')" :description="reason(state.problem || state.notification_problem)" />
      <div class="flex flex-wrap items-center gap-3 text-sm"><span>{{ t(state.worker_enabled ? 'storageGov.runtime.workerReady' : 'storageGov.runtime.workerOff') }}</span><span>{{ t(state.admission_enabled ? 'storageGov.runtime.admissionOn' : 'storageGov.runtime.admissionOff') }}</span><span>{{ t(state.notifications_configured ? 'storageGov.runtime.notificationsOn' : 'storageGov.runtime.notificationsOff') }}</span></div>
      <div class="flex flex-wrap items-center gap-3 rounded-lg border border-default p-4"><p>{{ t('storageGov.runtime.available') }} {{ bytes(state.budget?.available_bytes) }}</p><p>{{ t('storageGov.runtime.reserved') }} {{ bytes(state.budget?.reserved_bytes) }}</p><UButton color="neutral" variant="outline" @click="openBudget">{{ t('storageGov.runtime.budget') }}</UButton><UButton color="neutral" variant="outline" @click="deliveryOpen = true">{{ t('storageGov.runtime.delivery') }}</UButton></div>
      <p class="text-sm text-muted">{{ t('storageGov.runtime.safetyHint') }}</p>
      <UAlert v-if="state.lifecycle?.runs.some(r => r.state === 'unconfirmed' && !r.acknowledged_at)" color="warning" :title="t('storageGov.runtime.needsAttention')"><template #actions><UButton color="neutral" @click="runSource = ''; runsOpen = true">{{ t('storageGov.runtime.runs') }}</UButton></template></UAlert>
      <div v-if="state.lifecycle?.sources.length" class="overflow-x-auto"><table class="w-full text-left text-sm"><thead><tr><th class="p-2">{{ t('storageGov.resource') }}</th><th class="p-2">{{ t('storageGov.state') }}</th><th class="p-2">{{ t('storageGov.runtime.retention') }}</th><th /></tr></thead><tbody><tr v-for="source in state.lifecycle.sources" :key="source.id"><td class="p-2">{{ source.name }}</td><td class="p-2">{{ source.supported ? t(policy(source.id)?.enabled ? 'storageGov.runtime.enabled' : 'storageGov.runtime.disabled') : t('storageGov.runtime.protectedSource') }}<p v-if="source.problem" class="text-warning">{{ reason(source.problem) }}</p></td><td class="p-2">{{ policy(source.id) ? `${policy(source.id)!.retention_days} ${t('storageGov.runtime.dayUnit')} / ${bytes(policy(source.id)!.max_bytes)}` : '—' }}</td><td class="p-2"><UButton :disabled="!source.supported || busy" color="neutral" variant="outline" @click="edit(source)">{{ t('storageGov.runtime.configure') }}</UButton></td></tr></tbody></table></div>
      <UButton color="neutral" variant="outline" @click="runSource = ''; runsOpen = true">{{ t('storageGov.runtime.runs') }} · {{ state.lifecycle?.runs.length || 0 }}</UButton>
    </template>
    </template>
    <UModal v-model:open="policyOpen" :title="selected?.name || t('storageGov.runtime.configure')" :description="t('storageGov.runtime.policyHint')" :dismissible="!busy && !unknown" :close="busy || unknown ? false : undefined" :ui="{ content: 'max-w-4xl' }"><template #body><div class="space-y-4" data-testid="storage-runtime-policy">
      <UAlert v-if="error" color="error" :title="t('storageGov.requestFailed')" :description="error" />
      <UAlert v-if="!state?.lifecycle?.allow_enforce" color="warning" :title="t('storageGov.runtime.enforceOff')" />
      <div class="rounded-lg border border-default bg-elevated/50 p-3 text-sm" data-testid="runtime-config-versions">
        <p>{{ selected ? configurationLabel(selected.id) : '' }}</p>
        <p v-if="configuration?.authority === 'database' && draftDirty" class="mt-2 text-warning">{{ t('storageGov.tableMonitor.control.dirty') }}</p>
        <p v-if="configuration?.authority === 'database' && editorRevision !== configuration.desired_revision" class="mt-2 text-warning">{{ t('storageGov.tableMonitor.control.staleDraft', {version: editorRevision}) }}</p>
        <p v-if="configuration?.problem" class="mt-2 text-warning">{{ t('storageGov.lifecycleControl.blocked') }}</p>
      </div>
      <div v-if="unknown" class="space-y-3" data-testid="runtime-config-unknown">
        <UAlert color="warning" :title="t('storageGov.tableMonitor.control.unknown')" :description="t('storageGov.tableMonitor.control.unknownHint')" />
        <UButton color="neutral" variant="outline" :loading="busy" data-testid="runtime-reconcile" @click="reconcile">{{ t('storageGov.tableMonitor.control.reconcile') }}</UButton>
      </div>
      <BackupSectionTabs v-model="policySection" :items="policySections" :label="t('storageGov.runtime.configure')">
        <template #settings>
          <div v-if="configuration?.authority === 'legacy'" class="space-y-4" data-testid="runtime-adoption">
            <UAlert color="warning" :title="t('storageGov.tableMonitor.control.legacy')" :description="t('storageGov.lifecycleControl.adoptionHint')" />
            <p class="text-sm">{{ t('storageGov.tableMonitor.control.legacyVersion', {version: configuration.legacy_revision}) }} · {{ t(configuration.legacy_enabled ? 'storageGov.runtime.enabled' : 'storageGov.runtime.disabled') }}</p>
            <UCheckbox v-model="adoptionConfirmed" :disabled="busy || !!unknown" :label="t('storageGov.lifecycleControl.adoptionConfirm')" data-testid="runtime-adoption-confirm" />
            <UButton :loading="busy" :disabled="!configuration.ready || !adoptionConfirmed || !!unknown" data-testid="runtime-adopt" @click="adopt">{{ t('storageGov.tableMonitor.control.adopt') }}</UButton>
          </div>
          <UAlert v-else-if="!configuration || configuration.authority !== 'database'" color="warning" :title="t('storageGov.tableMonitor.control.upgrade')" :description="t('storageGov.lifecycleControl.blocked')" />
      <UForm v-else :schema="policySchema" :state="form" class="space-y-4" @submit="save">
        <div class="grid gap-3 sm:grid-cols-2"><UFormField :label="t('storageGov.runtime.days')" name="days"><UInput v-model="form.days" type="number" :disabled="busy || !!unknown" class="w-full" data-testid="runtime-days" /></UFormField><UFormField :label="t('storageGov.runtime.cap')" name="gib"><UInput v-model="form.gib" type="number" step="any" :disabled="busy || !!unknown" class="w-full" data-testid="runtime-budget" /></UFormField></div>
        <UCheckbox v-model="form.enabled" :disabled="busy || !!unknown || ((!canEnable || form.mode === 'disabled') && !form.enabled)" :label="t('storageGov.runtime.enablePolicy')" data-testid="runtime-enabled" />
        <UFormField :label="t('storageGov.archive.cleanupMode')" name="mode"><USelect v-model="form.mode" :items="cleanupModes" :disabled="busy || !!unknown" class="w-full" data-testid="runtime-cleanup-mode" /></UFormField>
        <p class="text-sm text-muted">{{ t('storageGov.archive.modeHint') }}</p>
        <UCheckbox v-model="confirmed" :disabled="busy || !!unknown" :label="t('storageGov.runtime.confirm')" data-testid="runtime-confirm" />
        <div class="flex flex-wrap gap-3"><UButton type="submit" :loading="busy" :disabled="!confirmed || !!unknown || !configuration?.ready || (form.enabled && !canEnable)" data-testid="runtime-save">{{ t('storageGov.runtime.save') }}</UButton></div>
        <UButton type="button" color="neutral" variant="ghost" :disabled="busy || !!unknown" @click="loadLatest">{{ t('storageGov.tableMonitor.control.loadLatest') }}</UButton>
      </UForm>
      <p v-if="savedRevision" class="mt-4 text-sm text-success" role="status" data-testid="runtime-save-result">{{ t(configuration?.state === 'applied' && configuration.desired_revision === savedRevision ? 'storageGov.tableMonitor.control.applied' : 'storageGov.tableMonitor.control.savedPending') }}</p>
        </template>
        <template #preview>
          <div class="space-y-4">
            <p class="text-sm text-muted">{{ t('storageGov.lifecycleControl.previewHint') }}</p>
            <UButton color="neutral" variant="outline" :loading="busy" :disabled="!!unknown" @click="previewPolicy">{{ t('storageGov.logs.preview') }}</UButton>
      <div v-if="preview" class="rounded border border-default p-3 text-sm" data-testid="runtime-preview"><p>{{ t('storageGov.runtime.candidates', { count: preview.candidate_names.length, bytes: bytes(preview.candidate_bytes) }) }}</p><p v-if="preview.over_budget" class="text-warning">{{ t('storageGov.runtime.overBudget') }}</p><ul class="mt-2 max-h-40 overflow-auto"><li v-for="name in preview.candidate_names" :key="name" class="break-all">{{ name }}</li></ul><details class="mt-2"><summary>{{ t('storageGov.logs.skipped') }}</summary><p v-for="(count, code) in preview.protected" :key="code">{{ reason(String(code)) }} · {{ count }}</p></details></div>
      <UButton v-if="selected && policy(selected.id)?.enabled" color="error" variant="outline" :loading="busy" :disabled="attempted || !!unknown || !preview || isDirty || !applied" data-testid="runtime-run" @click="runOnce">{{ t('storageGov.runtime.runOnce') }}</UButton>
      <p v-if="result" :class="result.state === 'completed' ? 'text-success' : 'text-warning'">{{ t('storageGov.runtime.states.' + result.state) }} · {{ t('storageGov.logs.confirmedDeleted') }} {{ result.state === 'unconfirmed' || result.state === 'executing' ? '—' : result.deleted_files }}</p>
          </div>
        </template>
      </BackupSectionTabs>
    </div></template></UModal>
    <HostBudgetControl v-model:open="budgetOpen" :runtime="state" @saved="load" @busy="budgetBusy = $event" />
    <UModal v-model:open="deliveryOpen" :title="t('storageGov.runtime.delivery')" :description="t('storageGov.runtime.deliveryHint')"><template #body><dl v-if="state?.log_delivery" class="grid grid-cols-2 gap-3"><template v-for="key in ['delivered', 'failed', 'dropped', 'queued_entries', 'queued_bytes'] as const" :key="key"><dt>{{ t('storageGov.runtime.deliveryFields.' + key) }}</dt><dd>{{ key === 'queued_bytes' ? bytes(state.log_delivery[key]) : state.log_delivery[key] }}</dd></template></dl><p v-else>{{ t('storageGov.runtime.lokiOff') }}</p></template></UModal>
    <UModal v-model:open="runsOpen" :title="(runSource ? sourceName(runSource) + ' · ' : '') + t('storageGov.runtime.runs')" :description="t('storageGov.runtime.runHint')" :ui="{ content: 'max-w-4xl' }"><template #body><div class="max-h-[65vh] overflow-auto"><table class="w-full text-left text-sm"><thead><tr><th class="p-2">{{ t('storageGov.resource') }}</th><th>{{ t('storageGov.state') }}</th><th>{{ t('storageGov.logs.confirmedDeleted') }}</th></tr></thead><tbody><tr v-if="!visibleRuns.length"><td colspan="3" class="p-4 text-center text-muted">{{ t('storageGov.resourcePolicy.noRuns') }}</td></tr><tr v-for="run in visibleRuns" :key="run.id"><td class="p-2">{{ sourceName(run.source_id) }}<p class="text-muted">{{ new Date(run.started_at).toLocaleString() }}</p></td><td class="p-2">{{ t('storageGov.runtime.states.' + run.state) }}<p v-if="run.problem" class="text-warning">{{ reason(run.problem) }}</p></td><td>{{ run.state === 'unconfirmed' || run.state === 'executing' ? '—' : run.deleted_files }}<UButton v-if="run.state === 'unconfirmed' && !run.acknowledged_at" :disabled="busy || policy(run.source_id)?.enabled" color="neutral" variant="outline" @click="ackRun = run; ackConfirmed = false">{{ t('storageGov.runtime.acknowledge') }}</UButton></td></tr></tbody></table></div></template></UModal>
    <UModal :open="!!ackRun" :title="t('storageGov.runtime.acknowledge')" :description="t('storageGov.runtime.runHint')" :dismissible="!busy" :close="!busy" @update:open="value => { if (!value) ackRun = undefined }"><template #body><div class="space-y-4"><UCheckbox v-model="ackConfirmed" :label="t('storageGov.runtime.ackConfirm')" /><UAlert v-if="error" color="error" :title="error" /><UButton :disabled="!ackConfirmed" :loading="busy" @click="acknowledge">{{ t('storageGov.runtime.acknowledge') }}</UButton></div></template></UModal>
  </div>
</template>
<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import * as v from 'valibot';
import HostBudgetControl from './HostBudgetControl.vue';
import BackupSectionTabs from '~/components/ops/backup/BackupSectionTabs.vue';
import { useStorageRuntimeService, type LifecycleSettings, type LifecycleMutation, type StorageRuntime, type RuntimeSource, type RuntimePreview, type RuntimeRun } from '~/composables/api/services/storageRuntimeService';
import { formatStorageBytes as bytes } from '~/utils/storage-capacity';
import type { RuntimeNavigation } from '~/utils/storage-resource-management';
const props = defineProps<{ revision?: number; navigation?: RuntimeNavigation; sourceId?: string; initial?: StorageRuntime }>();
const emit = defineEmits<{ observed: [value: StorageRuntime]; busy: [value: boolean] }>(); const { t, te } = useI18n(); const api = useStorageRuntimeService(); const GiB = 1024 ** 3;
const state = ref<StorageRuntime | undefined>(props.initial), selected = ref<RuntimeSource>(), preview = ref<RuntimePreview>(), result = ref<RuntimeRun>();
const loading = ref(false), busy = ref(false), error = ref(''), saved = ref(false), confirmed = ref(false), attempted = ref(false);
const ackRun = ref<RuntimeRun>(), ackConfirmed = ref(false);
const editorRevision = ref(0), savedRevision = ref(0), adoptionConfirmed = ref(false), policySection = ref('settings');
const unknown = ref<{ source: string; key: string; revision: number; settings: LifecycleSettings; adopting?: boolean }>();
const configuration = computed(() => selected.value ? state.value?.lifecycle_configurations?.[selected.value.id] : undefined);
const policySections = computed(() => [{ value: 'settings', label: t('storageGov.lifecycleControl.settings') }, { value: 'preview', label: t('storageGov.lifecycleControl.preview') }]);
const canEnable = computed(() => !!configuration.value?.can_enable && !!state.value?.lifecycle?.allow_enforce);
const applied = computed(() => configuration.value?.authority === 'database' && configuration.value.state === 'applied' && configuration.value.applied_revision === editorRevision.value && configuration.value.snapshot_revision === editorRevision.value);
function configurationLabel(id: string) {
  const c = state.value?.lifecycle_configurations?.[id];
  if (!c) return t('storageGov.tableMonitor.control.upgrade');
  if (c.authority === 'legacy') return t('storageGov.tableMonitor.control.legacy');
  return t('storageGov.tableMonitor.control.versions', { desired: c.desired_revision, applied: c.applied_revision, snapshot: c.snapshot_revision ?? '—', state: t(`storageGov.tableMonitor.control.states.${c.state}`) });
}
const drafts = new Map<string, { revision: number; form: typeof form }>();
let consumedNavigation = 0;
const runSource = ref('');
const visibleRuns = computed(() => state.value?.lifecycle?.runs.filter(r => !runSource.value || r.source_id === runSource.value).slice().reverse() || []);
const policyOpen = ref(false), budgetOpen = ref(false), deliveryOpen = ref(false), runsOpen = ref(false);
const form = reactive({ days: 7 as string | number, gib: 1 as string | number, enabled: false, mode: 'disabled' as 'disabled' | 'expire_delete' | 'archive_then_delete' });
const budgetBusy = ref(false);
let controller: AbortController | undefined, readController: AbortController | undefined, requestID = '';
const number = (min: number, max: number, integer = false) => v.pipe(v.union([v.string(), v.number()]), v.transform(Number), v.check(n => Number.isFinite(n) && n >= min && n <= max && (!integer || Number.isInteger(n)), t('storageGov.drafts.invalid')));
const policySchema = computed(() => v.pipe(v.object({ days: number(1, 3650, true), gib: number(1 / GiB, 1048576), enabled: v.boolean(), mode: v.picklist(['disabled', 'expire_delete', 'archive_then_delete']) }), v.check(value => !value.enabled || value.mode !== 'disabled', t('storageGov.drafts.invalid'))));

const cleanupModes = computed(() => ['disabled', 'expire_delete', 'archive_then_delete'].map(value => ({ value, label: t('storageGov.archive.modes.' + value) })));
const localSource = computed(() => state.value?.lifecycle?.sources.find(s => s.id === props.sourceId));
const localPolicy = computed(() => props.sourceId ? policy(props.sourceId) : undefined);
const policy = (id: string) => state.value?.lifecycle?.policies.find(p => p.source_id === id);
const sourceName = (id: string) => state.value?.lifecycle?.sources.find(s => s.id === id)?.name || id;
const reason = (code?: string) => te('storageGov.runtime.reasons.' + code) ? t('storageGov.runtime.reasons.' + code) : t('storageGov.runtime.reasons.unknown');
const isDirty = computed(() => { const p = selected.value && policy(selected.value.id); return !p || p.retention_days !== Number(form.days) || p.max_bytes !== Math.round(Number(form.gib) * GiB) || p.enabled !== form.enabled || p.cleanup_mode !== form.mode; });
function acceptRuntime(value: StorageRuntime) {
  const current = state.value?.lifecycle_configurations;
  if (current && value.lifecycle_configurations) for (const [id, old] of Object.entries(current)) {
    const next = value.lifecycle_configurations[id];
    if (old.authority === 'database' && next && (next.authority === 'legacy' || (next.authority === 'database' && next.desired_revision < old.desired_revision && !['unconfirmed', 'unavailable'].includes(next.state)))) return;
  }
  state.value = value;
}
async function load() { readController?.abort(); const r = readController = new AbortController(); loading.value = true; try { const value = await api.status(r.signal); if (!r.signal.aborted) acceptRuntime(value); } catch (e) { if (!r.signal.aborted) error.value = e instanceof Error ? e.message : t('storageGov.requestFailed'); } finally { if (!r.signal.aborted) loading.value = false; } }
function fillDraft(source: RuntimeSource, keep = true) {
  const c = state.value?.lifecycle_configurations?.[source.id], p = c?.desired_policy || policy(source.id), draft = keep ? drafts.get(source.id) : undefined;
  editorRevision.value = draft?.revision ?? c?.desired_revision ?? 0;
  Object.assign(form, draft?.form || { days: p?.retention_days || 30, gib: (p?.max_bytes || GiB) / GiB, enabled: p?.enabled || false, mode: p?.cleanup_mode || (source.require_archive ? 'archive_then_delete' : 'expire_delete') });
}
function edit(source: RuntimeSource) {
  if (unknown.value || busy.value) return;
  selected.value = source; fillDraft(source); policySection.value = 'settings'; adoptionConfirmed.value = false;
  confirmed.value = false; preview.value = undefined; result.value = undefined; attempted.value = false; requestID = ''; savedRevision.value = 0; error.value = ''; policyOpen.value = true;
}
async function loadLatest() {
  if (!selected.value || busy.value || unknown.value) return;
  error.value = ''; await load(); if (!error.value) { drafts.delete(selected.value.id); fillDraft(selected.value, false); confirmed.value = false; preview.value = undefined; }
}
async function previewPolicy() { if (!selected.value || busy.value || !v.safeParse(policySchema.value, form).success) return; const r = controller = new AbortController(); busy.value = true; error.value = ''; try { const value = await api.preview(selected.value.id, Number(form.days), Math.round(Number(form.gib) * GiB), r.signal); if (!r.signal.aborted) preview.value = value; policySection.value = 'preview'; } catch (e) { if (!r.signal.aborted) error.value = e instanceof Error ? e.message : t('storageGov.requestFailed'); } finally { if (!r.signal.aborted) busy.value = false; } }
function settings(): LifecycleSettings { return { enabled: form.enabled, cleanup_mode: form.mode, retention_days: Number(form.days), max_bytes: Math.round(Number(form.gib) * GiB) }; }
const draftDirty = computed(() => !!configuration.value?.desired_policy && !same(settings(), configuration.value.desired_policy));
const same = (a: LifecycleSettings, b: LifecycleSettings) => a.enabled === b.enabled && a.cleanup_mode === b.cleanup_mode && a.retention_days === b.retention_days && a.max_bytes === b.max_bytes;
async function accepted(receipt: LifecycleMutation, operation: NonNullable<typeof unknown.value>) {
  if (receipt.source_id !== operation.source || receipt.request_id !== operation.key || receipt.operation_revision !== operation.revision + 1 || !same(receipt.operation_policy, operation.settings) || receipt.configuration.authority !== 'database') throw new Error('STORAGE_LIFECYCLE_RESPONSE_INVALID');
  if (state.value) state.value = { ...state.value, lifecycle_configurations: { ...state.value.lifecycle_configurations, [operation.source]: receipt.configuration } };
  unknown.value = undefined; editorRevision.value = receipt.operation_revision;
  if (operation.adopting) { adoptionConfirmed.value = false; if (selected.value) fillDraft(selected.value, false); }
  drafts.set(operation.source, { revision: editorRevision.value, form: { ...form } }); confirmed.value = false; preview.value = undefined;
  await nextTick(); savedRevision.value = receipt.operation_revision;
  // 保存后的单次状态同步；后续变化由已有 SSE 驱动，不周期查询。
  await load();
}
async function reconcile() {
  const operation = unknown.value; if (!operation || busy.value) return;
  const r = controller = new AbortController(); busy.value = true; error.value = '';
  try { await accepted(await api.policyRequest(operation.source, operation.key, r.signal), operation); }
  catch { if (!r.signal.aborted) error.value = t('storageGov.tableMonitor.control.unknownHint'); }
  finally { if (!r.signal.aborted) busy.value = false; }
}
async function mutate(adopting: boolean) {
  const source = selected.value, c = configuration.value;
  if (!source || !c?.ready || busy.value || unknown.value) return;
  const operation = { source: source.id, key: crypto.randomUUID(), revision: adopting ? 0 : editorRevision.value, settings: adopting ? { ...settings(), enabled: false } : settings(), adopting };
  // 接管复制旧值，不能把未保存草稿当作来源证据。
  if (adopting) { const p = policy(source.id); operation.settings = { enabled: false, retention_days: p?.retention_days || 30, max_bytes: p?.max_bytes || GiB, cleanup_mode: p?.cleanup_mode || (source.require_archive ? 'archive_then_delete' : 'expire_delete') }; }
  const r = controller = new AbortController(); busy.value = true; error.value = ''; savedRevision.value = 0;
  try {
    const receipt = adopting
      ? await api.adoptPolicy(source.id, { legacy_revision: c.legacy_revision, legacy_digest: c.legacy_digest, request_id: operation.key, confirm: true }, r.signal)
      : await api.savePolicy(source.id, { ...operation.settings, request_id: operation.key, expected_revision: operation.revision, require_archive: operation.settings.cleanup_mode === 'archive_then_delete', confirm: true }, r.signal);
    if (!r.signal.aborted) await accepted(receipt, operation);
  } catch (e: any) {
    if (!r.signal.aborted) {
      const status = e?.statusCode || e?.status || e?.response?.status;
      if ([400, 401, 403, 404, 409, 422, 428].includes(status)) { error.value = e instanceof Error ? e.message : t('storageGov.requestFailed'); return; }
      // 任何未知响应都只查询原请求回执，不重发写入或解除未知状态。
      unknown.value = operation;
      try { await accepted(await api.policyRequest(source.id, operation.key, r.signal), operation); }
      catch { error.value = e instanceof Error ? e.message : t('storageGov.requestFailed'); }
    }
  } finally { if (!r.signal.aborted) busy.value = false; }
}
async function adopt() { if (configuration.value?.authority === 'legacy' && adoptionConfirmed.value) await mutate(true); }
async function save() {
  if (configuration.value?.authority !== 'database' || !confirmed.value || !v.safeParse(policySchema.value, form).success || (form.enabled && !canEnable.value)) return;
  await mutate(false);
}
async function runOnce() {
  const source = selected.value, p = source && policy(source.id); if (!source || !p?.enabled || !preview.value || busy.value || unknown.value || attempted.value || isDirty.value || !applied.value) return;
  const r = controller = new AbortController(); busy.value = true; attempted.value = true; requestID ||= crypto.randomUUID(); error.value = '';
  try { await api.run(source.id, p.revision, requestID, r.signal); const value = await api.status(r.signal); const run = value.lifecycle?.runs.find(row => row.request_key === requestID); if (!run) throw new Error(t('storageGov.logs.checkRecord')); if (!r.signal.aborted) { state.value = value; result.value = run; } } catch (e) { if (!r.signal.aborted) { error.value = e instanceof Error ? e.message : t('storageGov.logs.checkRecord'); try { const value = await api.status(r.signal); const run = value.lifecycle?.runs.find(row => row.request_key === requestID); if (run && !r.signal.aborted) { state.value = value; result.value = run; } } catch { /* 只读回；不重发执行。 */ } } } finally { if (!r.signal.aborted) busy.value = false; }
}
function openBudget() { budgetOpen.value = true; }

async function acknowledge() {
  const run = ackRun.value; if (!run || busy.value || !ackConfirmed.value || policy(run.source_id)?.enabled) return;
  const r = controller = new AbortController(); busy.value = true; error.value = '';
  try { await api.acknowledge(run.id, r.signal); const value = await api.status(r.signal); if (!value.lifecycle?.runs.find(item => item.id === run.id)?.acknowledged_at) throw new Error(t('storageGov.logs.checkRecord')); if (!r.signal.aborted) { state.value = value; ackRun.value = undefined; } }
  catch (e) { if (!r.signal.aborted) error.value = e instanceof Error ? e.message : t('storageGov.requestFailed'); } finally { if (!r.signal.aborted) busy.value = false; }
}
watch(() => busy.value || !!unknown.value || budgetBusy.value, value => emit('busy', value));
watch(() => props.initial, value => { if (value && !busy.value) acceptRuntime(value); });
watch(form, () => { if (selected.value && policyOpen.value) { drafts.set(selected.value.id, { revision: editorRevision.value, form: { ...form } }); saved.value = false; savedRevision.value = 0; confirmed.value = false; preview.value = undefined; } }, { deep: true });
function applyNavigation() {
  const n = props.navigation;
  if (!n || n.request === consumedNavigation || !state.value || loading.value || busy.value || unknown.value) return;
  consumedNavigation = n.request;
  if (n.action === 'budget') { openBudget(); }
  else if (n.action === 'records') { runSource.value = n.sourceId || ''; runsOpen.value = true; }
  else { const source = state.value.lifecycle?.sources.find(s => s.id === n.sourceId && s.supported); if (source) edit(source); else error.value = t('storageGov.resourcePolicy.sourceUnavailable'); }
}
watch([() => props.navigation, state, loading, busy], applyNavigation, { immediate: true });
watch(state, value => {
  if (value) emit('observed', value);
  const latest = requestID && value?.lifecycle?.runs.find(row => row.request_key === requestID);
  if (latest && (!result.value || result.value.state === 'executing' || latest.state !== 'executing')) result.value = latest;
});
watch(() => props.revision, () => void load()); onMounted(load); onBeforeUnmount(() => { controller?.abort(); readController?.abort(); emit('busy', false); });
</script>
