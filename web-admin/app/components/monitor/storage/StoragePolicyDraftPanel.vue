<template>
  <div class="min-w-0 space-y-4" data-testid="storage-policy-panel">
    <div class="flex flex-wrap items-end justify-between gap-3 rounded-xl border border-default bg-elevated/30 p-4">
      <label class="min-w-0 flex-1 space-y-1.5 text-sm"><span class="font-medium">{{ t('storageGov.resourcePolicy.scope') }}</span><select :value="environment" :disabled="saving || loading || logBusy || configBusy" class="block min-h-11 w-full max-w-sm rounded-lg border border-default bg-default px-3" data-testid="storage-policy-environment" @change="chooseEnvironment(($event.target as HTMLSelectElement).value)"><option v-for="e in environments" :key="e.key" :value="e.key">{{ e.label }}</option></select></label>
      <UButton color="neutral" variant="outline" :disabled="saving || logBusy || configBusy" data-testid="storage-live-budget" @click="emit('open-runtime', { action: 'budget' })">{{ t('storageGov.resourcePolicy.hostBudget') }}</UButton>
    </div>
    <p v-if="loaded" class="text-xs text-muted">{{ t(environment.startsWith('shared:') ? 'storageGov.resourcePolicy.sharedScope' : 'storageGov.resourcePolicy.instanceScope') }}</p>
    <UAlert v-if="error" color="error" :title="t('storageGov.requestFailed')" :description="error" />
    <UAlert v-if="configAttentionNames" color="warning" :title="t('storageGov.resourceConfig.unconfirmed')" :description="configAttentionNames" />
    <UAlert v-if="attentionNames" color="warning" :title="t('storageGov.logs.unconfirmed')" :description="attentionNames" />
    <p v-if="loading && !loaded" role="status">{{ t('storageGov.loading') }}</p>
    <div v-if="loaded" class="grid min-w-0 items-start gap-4 lg:grid-cols-[10.5rem_minmax(0,1fr)]">
      <StorageResourceNavigation :model-value="selected" :items="navigation" :label="t('storageGov.resource')" :dirty-label="t('storageGov.resourcePolicy.unsaved')" :id-base="navId" :disabled="saving || loading || logBusy || configBusy" @update:model-value="chooseResource" />
      <div :id="`${navId}-panel`" role="tabpanel" :aria-labelledby="selected ? `${navId}-tab-${selected}` : undefined" class="min-w-0 space-y-4" data-testid="storage-resource-content">
        <p v-if="!option" class="py-8 text-center text-muted">{{ t('storageGov.noResources') }}</p>
        <template v-else>
          <header class="space-y-4 rounded-xl border border-default p-4">
            <div class="flex flex-wrap items-start justify-between gap-2"><div class="min-w-0"><h3 class="text-lg font-semibold">{{ resourceDisplayName(option.resource.name) }}</h3><p class="mt-1 break-all text-xs text-muted">{{ scopeLabel(option.resource.scope) }} · {{ option.resource.id }}</p></div><span class="rounded-full bg-elevated px-2.5 py-1 text-xs" :class="management.state === 'automatic' ? 'text-success' : management.state === 'blocked' || management.state === 'unknown' ? 'text-warning' : ''">{{ t(`storageGov.resourcePolicy.states.${management.state}`) }}</span></div>
            <div class="flex flex-wrap gap-x-8 gap-y-3 text-sm"><div><p class="text-xs text-muted">{{ t('storageGov.context.currentUsage') }}</p><p class="mt-1 text-xl font-semibold tabular-nums">{{ formatBytes(usage?.attributed_bytes) }}</p></div><div><p class="text-xs text-muted">{{ t('storageGov.resourcePolicy.owner') }}</p><p class="mt-1">{{ t(`storageGov.resourcePolicy.owners.${management.owner}`) }}</p></div><div v-if="management.run"><p class="text-xs text-muted">{{ t('storageGov.resourcePolicy.latestRun') }}</p><p class="mt-1">{{ t(`storageGov.runtime.states.${management.run.state}`) }} · {{ new Date(management.run.started_at).toLocaleString() }}</p></div></div>
          </header>
          <UAlert v-if="reviewSources.has(selected) || management.needsReview" color="warning" :title="t('storageGov.logs.unconfirmed')" />
          <div class="space-y-4 rounded-xl border border-default p-4">
            <KeepAlive><StorageResourceConfiguration :key="option.resource.id" :source="option" :instance-id="loaded.identity.instance_id || ''" :runtime="runtime" :revision="runtimeRevision" @observed="emit('runtime-observed', $event)" @busy="setConfigBusy" @attention="setConfigAttention" @changed="emit('refresh')" /></KeepAlive>
            <p v-if="option.cleanup?.native_managed" class="text-xs text-muted">{{ t('storageGov.runtime.nativeManaged') }}</p>
            <KeepAlive><StorageLogCleanupPanel v-if="option.cleanup?.supported" :key="option.resource.id" :source="option" :usage="usage" compact @busy="logBusy = $event" @review="setReview" @changed="emit('cleanupChanged')" /></KeepAlive>
          </div>
          <div class="flex flex-wrap items-center justify-between gap-2 text-xs text-muted"><span>{{ t('storageGov.resourcePolicy.planningHint') }}</span><UButton color="neutral" variant="ghost" size="sm" :disabled="saving || loading || logBusy || configBusy" data-testid="storage-advanced-draft" @click="draftOpen = true">{{ t('storageGov.resourcePolicy.openDraft') }}<span v-if="dirtySources.has(selected)" class="ml-1 size-1.5 rounded-full bg-warning" /></UButton></div>
        </template>
      </div>
    </div>
    <p v-if="loaded" class="text-xs text-muted">{{ t('storageGov.context.registeredOnly') }}</p>
    <UButton v-if="!loading" color="neutral" variant="ghost" :disabled="saving || logBusy || configBusy" @click="refreshResource">{{ t('storageGov.refresh') }}</UButton>
    <UModal v-model:open="draftOpen" :title="option ? resourceDisplayName(option.resource.name) + ' · ' + t('storageGov.resourcePolicy.openDraft') : t('storageGov.resourcePolicy.openDraft')" :description="t('storageGov.context.draftOnly')" :dismissible="!saving && !estimating" :close="!saving && !estimating" :ui="{ content: 'max-w-2xl' }"><template #body><div v-if="option" class="min-w-0 space-y-4" data-testid="storage-resource-draft">
      <UAlert color="info" :title="t('storageGov.resourcePolicy.draftOnly')" :description="t('storageGov.context.draftOnly')" />
      <UAlert v-if="error" color="error" :title="t('storageGov.requestFailed')" :description="error" />
      <UButton color="neutral" variant="outline" :disabled="saving" data-testid="storage-host-open" @click="budgetOpen = true">{{ t('storageGov.resourcePolicy.budgetDraft') }}</UButton>
        <div class="rounded-lg border border-default p-4" data-testid="storage-policy-context">
          <div class="flex flex-wrap items-center justify-between gap-2"><h3 class="font-semibold">{{ scopeLabel(option.resource.scope) }} · {{ option.resource.name }}</h3><span class="text-sm text-muted">{{ t(`storageGov.context.intents.${option.preset.intent}`) }}</span></div>
          <p class="mt-2 text-sm text-muted">{{ t('storageGov.context.currentUsage') }} {{ formatBytes(usage?.physical_bytes) }} · {{ t('storageGov.drafts.revision') }} {{ draftRevision }} <span v-if="draftRevision !== loaded.revision" class="text-warning">· {{ t('storageGov.resourcePolicy.newRevision', { revision: loaded.revision }) }}</span> · {{ t(hasSavedSource ? 'storageGov.context.savedSource' : 'storageGov.context.suggestedSource') }}</p>
          <div class="mt-3 flex flex-wrap items-center justify-between gap-2 text-sm"><span>{{ t('storageGov.context.suggestedCadence') }} {{ t(`storageGov.context.cadences.${option.preset.suggested_cadence}`) }}</span><UButton size="sm" color="neutral" variant="outline" :disabled="saving" @click="applyPreset">{{ t('storageGov.context.applyPreset') }}</UButton></div>
        </div>
        <UForm :schema="schema" :state="form" class="space-y-4" data-testid="storage-resource-form" @submit="saveResource">
          <UFormField :label="t('storageGov.context.retentionMode')" name="mode"><select :value="form.mode" :disabled="saving" class="min-h-10 w-full rounded-md border border-default bg-default px-3" data-testid="storage-policy-mode" @change="chooseMode(($event.target as HTMLSelectElement).value)"><option v-if="form.mode === 'legacy'" value="legacy">{{ t('storageGov.context.modes.legacy') }}</option><option v-for="mode in option.allowed_modes" :key="mode" :value="mode">{{ t(`storageGov.context.modes.${mode}`) }}</option></select></UFormField>
          <p v-if="form.mode === 'legacy'" class="text-sm text-warning">{{ t('storageGov.context.legacyHint') }}</p>
          <div class="grid gap-4 sm:grid-cols-2">
            <UFormField v-if="form.mode === 'days' || form.mode === 'legacy'" :label="t('storageGov.days')" name="days"><UInput v-model="form.days" :disabled="saving" type="number" class="w-full" data-testid="storage-policy-days" /></UFormField>
            <UFormField v-if="form.mode === 'count' || form.mode === 'legacy'" :label="t(option.resource.kind === 'snapshot_files' ? 'storageGov.context.restoreCount' : 'storageGov.context.successCount')" name="count"><UInput v-model="form.count" :disabled="saving" type="number" class="w-full" data-testid="storage-policy-count" /></UFormField>
            <UFormField :label="t('storageGov.context.resourceBudget')" name="resource_gib"><UInput v-model="form.resource_gib" :disabled="saving" type="number" step="0.001" class="w-full" data-testid="storage-policy-budget" /></UFormField>
            <UFormField v-if="form.mode === 'count' || form.mode === 'legacy'" :label="t('storageGov.drafts.verified')" name="verified"><UInput v-model="form.verified" :disabled="saving" type="number" class="w-full" /></UFormField>
          </div>
          <p v-if="form.mode === 'protected'" class="text-sm text-muted">{{ t('storageGov.context.protectedHint') }}</p>
          <div class="flex flex-wrap gap-5"><UCheckbox v-model="form.protect" :disabled="saving" :label="t('storageGov.drafts.protect')" /><UCheckbox v-model="form.archive" :disabled="saving" :label="t('storageGov.drafts.archive')" /></div>
          <details class="rounded border border-default p-3 text-sm"><summary class="cursor-pointer font-medium">{{ t('storageGov.context.protectionRules') }}</summary><ul class="mt-3 list-inside list-disc space-y-1 text-muted"><li v-for="rule in option.protection_rules" :key="rule">{{ t(`storageGov.context.rules.${rule}`) }}</li></ul><p class="mt-3 text-muted">{{ t('storageGov.context.draftOnly') }}</p></details>
          <p v-if="!identityReady" class="text-sm text-warning">{{ t('storageGov.drafts.identityRequired') }}</p>
          <p v-else-if="!budgetReady" class="text-sm text-warning">{{ t('storageGov.context.hostRequired') }}</p>
          <div class="flex flex-wrap gap-3"><UButton type="submit" :disabled="!identityReady || !budgetReady || loading" :loading="saving" data-testid="storage-policy-save">{{ t('storageGov.context.saveResource') }}</UButton><UButton type="button" color="neutral" variant="outline" :disabled="saving || form.mode === 'legacy'" data-testid="storage-calculate" @click="openEstimate">{{ t('storageGov.calculate') }}</UButton><UButton type="button" color="neutral" variant="ghost" :disabled="saving" :loading="loading" @click="load">{{ t('storageGov.drafts.reload') }}</UButton><UButton type="button" color="neutral" variant="ghost" :disabled="saving || loading" data-testid="storage-draft-discard" @click="discardDraft">{{ t('storageGov.resourcePolicy.discardDraft') }}</UButton></div>
        </UForm>
        <UAlert v-if="saved" color="success" :title="saved" />
        <UCard v-if="estimate" data-testid="storage-estimate" aria-live="polite"><h4 class="font-medium">{{ scopeLabel(option.resource.scope) }} · {{ option.resource.name }}</h4><p class="mt-2">{{ t(`storageGov.estimates.${estimate.state}`) }}</p><dl v-if="estimate.required_bytes !== null" class="mt-4 grid grid-cols-[1fr_auto] gap-3 text-sm"><dt>{{ t('storageGov.required') }}</dt><dd>{{ formatBytes(estimate.required_bytes) }}</dd><dt>{{ t('storageGov.deficit') }}</dt><dd>{{ formatBytes(estimate.deficit_bytes) }}</dd><dt>{{ t('storageGov.supported') }}</dt><dd>{{ estimate.supported_days }}</dd></dl><p class="mt-3 text-sm text-muted">{{ t('storageGov.notPolicy') }}</p></UCard>

    </div></template></UModal>
    <UModal v-model:open="budgetOpen" :title="t('storageGov.context.hostBudget')" :description="t('storageGov.context.hostHint')" :ui="{ content: 'max-w-2xl' }">
      <template #body><div class="space-y-4" data-testid="storage-host-panel">
        <dl class="grid grid-cols-2 gap-2 text-sm"><dt>{{ t('storageGov.metrics.total_bytes') }}</dt><dd>{{ formatBytes(filesystem?.total_bytes) }}</dd><dt>{{ t('storageGov.metrics.available_bytes') }}</dt><dd>{{ formatBytes(filesystem?.available_bytes) }}</dd></dl>
        <p class="text-sm text-muted">{{ t('storageGov.context.hostHint') }}</p>
        <UForm :schema="budgetSchema" :state="host" class="space-y-4" data-testid="storage-host-form" @submit="saveHost">
          <div class="grid gap-4 sm:grid-cols-3"><UFormField :label="t('storageGov.drafts.hostBudget')" name="host_gib"><UInput v-model="host.host_gib" :disabled="saving" type="number" step="0.001" class="w-full" data-testid="storage-host-budget" /></UFormField><UFormField :label="t('storageGov.drafts.freeBudget')" name="free_gib"><UInput v-model="host.free_gib" :disabled="saving" type="number" step="0.001" class="w-full" data-testid="storage-host-free" /></UFormField><UFormField :label="t('storageGov.drafts.criticalBudget')" name="critical_gib"><UInput v-model="host.critical_gib" :disabled="saving" type="number" step="0.001" class="w-full" /></UFormField></div>
          <p v-if="hostError" class="text-sm text-error">{{ hostError }}</p>
          <p v-if="hostSaved" class="text-sm text-success">{{ t('storageGov.context.hostSaved') }}</p>
          <div class="flex flex-wrap gap-3"><UButton type="submit" :loading="saving" :disabled="!identityReady || loading" data-testid="storage-host-save">{{ t('storageGov.context.saveHost') }}</UButton><UButton type="button" color="neutral" variant="outline" :disabled="saving || !filesystem?.total_bytes" data-testid="storage-host-suggest" @click="suggestHost">{{ t('storageGov.context.suggestHost') }}</UButton></div>
        </UForm>
      </div></template>
    </UModal>
    <UModal v-model:open="estimateOpen" :title="t('storageGov.calculate')" :description="option ? `${scopeLabel(option.resource.scope)} · ${option.resource.name}` : ''" :ui="{ content: 'max-w-xl' }">
      <template #body><div v-if="option" class="space-y-4" data-testid="storage-estimate-dialog"><p>{{ t(`storageGov.context.modes.${form.mode}`) }} · {{ t('storageGov.context.resourceBudget') }} {{ form.resource_gib }} GiB</p><p class="text-sm text-muted">{{ t(form.mode === 'days' ? 'storageGov.manualHint' : form.mode === 'count' ? 'storageGov.context.countEstimateHint' : 'storageGov.context.protectedHint') }}</p><UForm :schema="estimateSchema" :state="assumption" class="space-y-4" data-testid="storage-estimate-form" @submit="calculate"><template v-if="form.mode === 'days'"><UFormField :label="t('storageGov.daily')" name="daily_gib"><UInput v-model="assumption.daily_gib" type="number" step="0.01" class="w-full" data-testid="storage-daily" /></UFormField><UFormField :label="t('storageGov.protected')" name="protected_gib"><UInput v-model="assumption.protected_gib" type="number" step="0.01" class="w-full" /></UFormField></template><p v-if="estimateError" class="text-sm text-error">{{ estimateError }}</p><UButton type="submit" :loading="estimating" data-testid="storage-estimate-submit">{{ t('storageGov.calculate') }}</UButton></UForm></div></template>
    </UModal>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, useId, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import * as v from 'valibot';
import { useStorageGovernanceService, type StoragePolicyOption, type StoragePolicy, type StorageResourcePolicy, type StorageUsage, type StorageOverview, type StorageEstimate } from '~/composables/api/services/storageGovernanceService';
import { formatStorageBytes as formatBytes } from '~/utils/storage-capacity';
import { resourceManagement, resourceDisplayName, type RuntimeNavigationRequest } from '~/utils/storage-resource-management';
import type { StorageRuntime } from '~/composables/api/services/storageRuntimeService';
import StorageResourceConfiguration from './StorageResourceConfiguration.vue';
import StorageResourceNavigation from './StorageResourceNavigation.vue';
import StorageLogCleanupPanel from './StorageLogCleanupPanel.vue';
const props = defineProps<{ resources: StorageUsage[]; filesystem?: StorageOverview['filesystem']; runtime?: StorageRuntime; runtimeRevision?: number }>();
const emit = defineEmits<{ cleanupChanged: []; 'runtime-observed': [value: StorageRuntime]; 'open-runtime': [request: RuntimeNavigationRequest]; 'open-archives': []; 'open-records': []; refresh: [] }>();
const { t } = useI18n(); const api = useStorageGovernanceService(); const GiB = 1024 ** 3;
const loaded = ref<StoragePolicy>(); const environment = ref(''); const selected = ref('');
const loading = ref(false), saving = ref(false), estimating = ref(false), error = ref(''), saved = ref(''), estimateError = ref(''), hostError = ref(''), hostSaved = ref(false), budgetOpen = ref(false), estimateOpen = ref(false);
const estimate = ref<StorageEstimate>();
const busySources = ref(new Set<string>());
const configBusy = computed(() => busySources.value.size > 0);
function setConfigBusy(event: { sourceId: string; busy: boolean }) { if (event.busy) busySources.value.add(event.sourceId); else busySources.value.delete(event.sourceId); }
const logBusy = ref(false), draftOpen = ref(false);
const navId = `storage-resource-${useId()}`;
const dirtySources = ref(new Set<string>()), reviewSources = ref(new Set<string>()), configAttentionSources = ref(new Set<string>());
const draftRevision = ref(0); let hostRevision = 0, hostDirty = false;
const selectionByScope = new Map<string, string>();
function setConfigAttention(event: { sourceId: string; pending: boolean }) { if (event.pending) configAttentionSources.value.add(event.sourceId); else configAttentionSources.value.delete(event.sourceId); }
function setReview(event: { sourceId: string; pending: boolean }) { if (event.pending) reviewSources.value.add(event.sourceId); else reviewSources.value.delete(event.sourceId); }
const form = reactive({ mode: 'protected', days: 0 as string | number, resource_gib: '' as string | number, count: 0 as string | number, verified: 0 as string | number, protect: true, archive: true });
const host = reactive({ host_gib: '' as string | number, free_gib: '' as string | number, critical_gib: '' as string | number });
const assumption = reactive({ daily_gib: '' as string | number, protected_gib: 0 as string | number });
let controller: AbortController | undefined, estimateController: AbortController | undefined, applying = false;
type Draft = { form: typeof form; assumption: typeof assumption; estimate?: StorageEstimate; revision: number; dirty: boolean };
const drafts = new Map<string, Draft>();
const scopeKey = (r: { scope: string; instance_id?: string }) => `${r.scope}:${r.instance_id || ''}`;
const scopeLabel = (s: string) => t(`storageGov.scopes.${s}`);
const options = computed(() => loaded.value?.catalog || []);
const environments = computed(() => {
  const groups = new Map<string, { key: string; label: string }>();
  for (const { resource: r } of options.value) groups.set(scopeKey(r), { key: scopeKey(r), label: `${scopeLabel(r.scope)}${r.instance_id ? ` · ${r.instance_id === loaded.value?.identity.instance_id ? t('storageGov.context.currentInstance') : r.instance_id.slice(0, 10)}` : ''}` });
  return [...groups.values()];
});
const visibleOptions = computed(() => options.value.filter(o => scopeKey(o.resource) === environment.value));
const option = computed(() => options.value.find(o => o.resource.id === selected.value));
const management = computed(() => option.value ? resourceManagement(option.value, props.runtime) : { state: 'unknown' as const, owner: 'governance', source: undefined, policy: undefined, run: undefined, needsReview: false, manual: false, canConfigure: false });
const needsReview = (o: StoragePolicyOption) => reviewSources.value.has(o.resource.id) || resourceManagement(o, props.runtime).needsReview;
const attentionNames = computed(() => options.value.filter(needsReview).map(o => resourceDisplayName(o.resource.name)).join(' · '));
const configAttentionNames = computed(() => [...configAttentionSources.value].map(id => resourceDisplayName(options.value.find(o => o.resource.id === id)?.resource.name || id)).join(' · '));
const navigation = computed(() => visibleOptions.value.map(o => ({ id: o.resource.id, name: resourceDisplayName(o.resource.name), state: configAttentionSources.value.has(o.resource.id) ? t('storageGov.resourceConfig.unconfirmed') : needsReview(o) ? t('storageGov.logs.unconfirmed') : t(`storageGov.resourcePolicy.states.${resourceManagement(o, props.runtime).state}`), usage: formatBytes(props.resources.find(r => r.id === o.resource.id)?.attributed_bytes), dirty: dirtySources.value.has(o.resource.id) })));
const usage = computed(() => props.resources.find(r => r.id === selected.value));
const hasSavedSource = computed(() => loaded.value?.resources.some(r => r.resource_id === selected.value));
const identityReady = computed(() => !!loaded.value?.identity.host_id && !!loaded.value?.identity.filesystem_id && !!loaded.value?.identity.instance_id);
const budgetReady = computed(() => !!loaded.value && loaded.value.budget.max_managed_bytes > 0);
const numeric = (min: number, max: number, integer = false) => v.pipe(v.union([v.string(), v.number()]), v.transform(Number), v.check(n => Number.isFinite(n) && n >= min && n <= max && (!integer || Number.isInteger(n)), t('storageGov.drafts.invalid')));
const schema = computed(() => v.pipe(v.object({ mode: v.string(), days: numeric(0, 3650, true), resource_gib: numeric(1 / GiB, Number.MAX_SAFE_INTEGER / GiB), count: numeric(0, 10000, true), verified: numeric(0, 10000, true), protect: v.boolean(), archive: v.boolean() }), v.check(f => f.verified <= f.count && (f.mode === 'legacy' ? f.days >= 1 : f.mode === 'days' ? f.days >= 1 && f.count === 0 && f.verified === 0 : f.mode === 'count' ? f.days === 0 && f.count >= 1 : f.mode === 'protected' && f.days === 0 && f.count === 0 && f.verified === 0), t('storageGov.drafts.invalid'))));
const budgetSchema = computed(() => v.pipe(v.object({ host_gib: numeric(1 / GiB, Number.MAX_SAFE_INTEGER / GiB), free_gib: numeric(1 / GiB, Number.MAX_SAFE_INTEGER / GiB), critical_gib: numeric(1 / GiB, Number.MAX_SAFE_INTEGER / GiB) }), v.check(f => f.critical_gib < f.free_gib, t('storageGov.drafts.invalid'))));
const estimateSchema = computed(() => v.object({ daily_gib: v.union([v.literal(''), numeric(1 / GiB, Number.MAX_SAFE_INTEGER / GiB)]), protected_gib: numeric(0, Number.MAX_SAFE_INTEGER / GiB) }));
const message = (e: unknown) => e instanceof Error ? e.message.includes('CATALOG') ? t('storageGov.context.catalogUpgrade') : e.message : t('storageGov.requestFailed');
const canonical = (value: unknown) => JSON.stringify(value, (_key, item) => item && typeof item === 'object' && !Array.isArray(item) ? Object.fromEntries(Object.keys(item).sort().map(key => [key, item[key]])) : item);
const abortEstimate = () => { estimateController?.abort(); estimate.value = undefined; estimating.value = false; estimateError.value = ''; };
watch(form, () => { if (!applying) { saved.value = ''; if (selected.value) dirtySources.value.add(selected.value); abortEstimate(); } }, { flush: 'sync' });
watch(assumption, () => { if (!applying) abortEstimate(); }, { flush: 'sync' });
watch(host, () => { hostSaved.value = false; if (!applying) hostDirty = true; }, { flush: 'sync' });
function remember() { if (selected.value) drafts.set(selected.value, { form: { ...form }, assumption: { ...assumption }, estimate: estimate.value, revision: draftRevision.value, dirty: dirtySources.value.has(selected.value) }); if (selected.value) selectionByScope.set(environment.value, selected.value); }
function restoreSource(id: string) {
  applying = true; abortEstimate(); estimateOpen.value = false; draftOpen.value = false; selected.value = id; error.value = '';
  const cached = drafts.get(id), row = loaded.value?.resources.find(r => r.resource_id === id), preset = option.value?.preset;
  draftRevision.value = cached?.dirty ? cached.revision : loaded.value?.revision || 0;
  if (cached?.dirty) { Object.assign(form, cached.form); Object.assign(assumption, cached.assumption); estimate.value = cached.estimate; }
  else if (row || preset) {
    const r = row || preset!;
    Object.assign(form, { mode: row ? (row.retention_mode || 'legacy') : preset!.retention_mode, days: r.target_days, resource_gib: r.max_bytes === null ? '' : r.max_bytes / GiB, count: r.retention_count, verified: r.min_verified_points, protect: r.protect_latest, archive: r.require_verified_archive });
    Object.assign(assumption, cached?.assumption || { daily_gib: '', protected_gib: 0 });
  }
  saved.value = ''; applying = false;
}
function refreshResource() { emit('refresh'); void load(); }
function chooseResource(id: string) { if (saving.value || loading.value || logBusy.value || configBusy.value || !visibleOptions.value.some(o => o.resource.id === id)) return; remember(); restoreSource(id); }
function chooseEnvironment(key: string) { if (saving.value || loading.value || logBusy.value || configBusy.value || !environments.value.some(e => e.key === key)) return; remember(); environment.value = key; const previous = selectionByScope.get(key); restoreSource(visibleOptions.value.find(o => o.resource.id === previous)?.resource.id || visibleOptions.value[0]?.resource.id || ''); }
function discardDraft() { const id = selected.value; drafts.delete(id); dirtySources.value.delete(id); restoreSource(id); draftOpen.value = true; }
function chooseMode(mode: string) { form.mode = mode; form.days = mode === 'days' ? Math.max(1, Number(form.days)) : 0; form.count = mode === 'count' ? Math.max(1, Number(form.count)) : 0; if (mode !== 'count') form.verified = 0; }
function applyPreset() { if (!option.value) return; drafts.delete(selected.value); const p = option.value.preset; Object.assign(form, { mode: p.retention_mode, days: p.target_days, resource_gib: p.max_bytes === null ? '' : p.max_bytes / GiB, count: p.retention_count, verified: p.min_verified_points, protect: p.protect_latest, archive: p.require_verified_archive }); }
async function load() {
  controller?.abort(); const request = controller = new AbortController(); loading.value = true; error.value = ''; saved.value = '';
  try {
    const p = await api.policy(request.signal); if (request.signal.aborted) return;
    remember(); loaded.value = p;
    applying = true;
    if (!hostDirty) { hostRevision = p.revision; Object.assign(host, { host_gib: p.budget.max_managed_bytes ? p.budget.max_managed_bytes / GiB : '', free_gib: p.budget.min_free_bytes ? p.budget.min_free_bytes / GiB : '', critical_gib: p.budget.critical_free_bytes ? p.budget.critical_free_bytes / GiB : '' }); }
    applying = false;
    const current = p.catalog?.find(o => o.resource.id === selected.value) || p.catalog?.find(o => o.resource.instance_id === p.identity.instance_id && o.cleanup?.supported) || p.catalog?.find(o => o.resource.instance_id === p.identity.instance_id) || p.catalog?.[0];
    environment.value = current ? scopeKey(current.resource) : ''; restoreSource(current?.resource.id || '');
  } catch (e) { if (!request.signal.aborted) error.value = message(e); }
  finally { if (!request.signal.aborted) loading.value = false; }
}
function suggestHost() {
  if (!props.filesystem?.total_bytes) return;
  const total = props.filesystem.total_bytes / GiB, reserved = (props.filesystem.reserved_bytes || 0) / GiB;
  const production = options.value.some(o => ['prod', 'staging', 'shared'].includes(o.resource.scope));
  const free = Math.ceil(Math.max(production ? 20 : 10, total * (production ? .1 : .05)));
  Object.assign(host, { host_gib: Math.max(1, Math.floor((total - reserved - free) / 5) * 5), free_gib: free, critical_gib: Math.max(1, Math.floor(free / 2)) });
}
async function persist(budget: StoragePolicy['budget'], resources: StorageResourcePolicy[], expected = loaded.value?.revision) {
  if (!loaded.value || expected === undefined) throw new Error(t('storageGov.requestFailed'));
  controller?.abort(); const request = controller = new AbortController();
  const result = await api.savePolicy({ schema_version: 1, expected_revision: expected, state: 'draft', budget, resources }, request.signal);
  const readback = await api.policy(request.signal); if (request.signal.aborted) return false;
  if (readback.revision !== expected + 1 || result.revision !== readback.revision || canonical(readback.resources) !== canonical(resources) || canonical(readback.budget) !== canonical(budget)) throw new Error(t('storageGov.drafts.readbackFailed'));
  loaded.value = readback;
  for (const draft of drafts.values()) if (draft.revision === expected) draft.revision = readback.revision;
  if (draftRevision.value === expected) draftRevision.value = readback.revision;
  if (hostRevision === expected) hostRevision = readback.revision;
  return true;
}
async function saveResource() {
  if (saving.value || loading.value || !loaded.value || !option.value || !identityReady.value || !budgetReady.value) return;
  error.value = ''; if (!v.safeParse(schema.value, form).success) { error.value = t('storageGov.drafts.invalid'); return; }
  const bytes = Math.round(Number(form.resource_gib) * GiB);
  if (bytes > loaded.value.budget.max_managed_bytes) { error.value = t('storageGov.context.exceedsHost'); return; }
  const row: StorageResourcePolicy = { resource_id: selected.value, target_days: Number(form.days), max_bytes: bytes, retention_count: Number(form.count), min_verified_points: Number(form.verified), protect_latest: form.protect, require_verified_archive: form.archive };
  if (form.mode !== 'legacy') row.retention_mode = form.mode as StorageResourcePolicy['retention_mode'];
  const rows = new Map(loaded.value.resources.map(r => [r.resource_id, r])); rows.set(selected.value, row);
  saving.value = true; saved.value = '';
  try { if (await persist(loaded.value.budget, [...rows.values()], draftRevision.value)) { dirtySources.value.delete(selected.value); remember(); saved.value = t('storageGov.context.resourceSaved', { resource: option.value.resource.name }); } }
  catch (e) { error.value = message(e); }
  finally { saving.value = false; }
}
async function saveHost() {
  if (saving.value || loading.value || !loaded.value || !identityReady.value) return;
  hostError.value = ''; if (!v.safeParse(budgetSchema.value, host).success) { hostError.value = t('storageGov.drafts.invalid'); return; }
  const budget = { max_managed_bytes: Math.round(Number(host.host_gib) * GiB), min_free_bytes: Math.round(Number(host.free_gib) * GiB), critical_free_bytes: Math.round(Number(host.critical_gib) * GiB) };
  if (loaded.value.resources.some(r => r.max_bytes > budget.max_managed_bytes)) { hostError.value = t('storageGov.context.exceedsHost'); return; }
  saving.value = true;
  try { if (await persist(budget, loaded.value.resources, hostRevision)) { hostDirty = false; hostSaved.value = true; } }
  catch (e) { hostError.value = message(e); }
  finally { saving.value = false; }
}
function openEstimate() { if (!v.safeParse(schema.value, form).success) { error.value = t('storageGov.drafts.invalid'); return; } estimateOpen.value = true; }
async function calculate() {
  if (!option.value || estimating.value || form.mode === 'legacy' || !v.safeParse(schema.value, form).success || !v.safeParse(estimateSchema.value, assumption).success) return;
  estimateController?.abort(); const request = estimateController = new AbortController(); estimating.value = true; estimateError.value = ''; estimate.value = undefined;
  try {
    const result = await api.estimate({ resource_id: selected.value, retention_mode: form.mode as 'days' | 'count' | 'protected', retention_count: Number(form.count), target_days: Number(form.days), max_bytes: Math.round(Number(form.resource_gib) * GiB), daily_increment_bytes: form.mode === 'days' && assumption.daily_gib !== '' ? Math.ceil(Number(assumption.daily_gib) * GiB) : undefined, protected_bytes: form.mode === 'days' ? Math.ceil(Number(assumption.protected_gib) * GiB) : 0 }, request.signal);
    if (!request.signal.aborted) { estimate.value = result; estimateOpen.value = false; }
  } catch (e) { if (!request.signal.aborted) estimateError.value = message(e); }
  finally { if (!request.signal.aborted) estimating.value = false; }
}
onMounted(load); onBeforeUnmount(() => { controller?.abort(); estimateController?.abort(); });
</script>
