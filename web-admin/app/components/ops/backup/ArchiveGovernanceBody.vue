<template>
  <section class="min-w-0 space-y-4" data-testid="archive-governance">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <h2 class="text-lg font-semibold">{{ t('storageGov.archive.title') }}</h2>
      <div class="flex flex-wrap gap-2"><UButton color="neutral" variant="outline" @click="gcRunsOpen = true">{{ t('storageGov.archive.gcRuns') }}</UButton><UButton color="neutral" variant="outline" :loading="loading" @click="reload">{{ t('storageGov.refresh') }}</UButton></div>
    </div>
    <UAlert color="info" :title="t('storageGov.archive.optIn')" :description="t('storageGov.archive.scopeHint')" />
    <UAlert v-if="error" color="error" :title="t('storageGov.requestFailed')" :description="error" />
    <UAlert v-if="notice" color="success" :title="notice" />
    <UAlert v-if="events.status.value === 'disconnected'" color="warning" :title="t('storageGov.archive.disconnected')" />
    <p v-if="overview?.custody" class="text-sm text-muted">{{ t('storageGov.archive.executor') }} · {{ overview.custody.node_id }}</p>
    <UAlert v-if="overview?.custody?.legacy_detected && !overview.custody.managed" color="warning" :title="t('storageGov.archive.handoffBlocked')" />
    <UAlert v-if="overview?.scheduler_started === false" color="warning" :title="t('storageGov.archive.workerOff')" />
    <div v-if="overview" class="flex flex-wrap gap-4 rounded border border-default p-3 text-sm"><span>{{ t('storageGov.archive.activeCount') }} {{ policies.filter(p => p.enabled).length }}</span><span>{{ t('storageGov.archive.pendingCount') }} {{ overview.log_pending_transfers ?? '—' }}</span><span>{{ t('storageGov.archive.lastSuccess') }} {{ time(lastSuccess) }}</span></div>
    <BackupSectionTabs v-model="section" :items="sections" :label="t('storageGov.archive.title')">
      <template #catalog>
        <form class="mb-4 flex flex-wrap gap-2" @submit.prevent="page = 1; loadCatalog()">
          <UInput :disabled="busy" v-model="query" :placeholder="t('storageGov.archive.searchHint')" :aria-label="t('storageGov.archive.searchHint')" class="min-w-0 flex-1" data-testid="archive-search" />
          <UInput :disabled="busy" v-model="from" type="date" :aria-label="t('storageGov.archive.from')" />
          <UInput :disabled="busy" v-model="to" type="date" :aria-label="t('storageGov.archive.to')" />
          <UButton type="submit" :loading="catalogLoading">{{ t('storageGov.archive.search') }}</UButton>
        </form>
        <p v-if="!catalogLoading && !catalog?.items.length" class="py-6 text-muted">{{ t('storageGov.archive.empty') }}</p>
        <div class="space-y-3">
          <article v-for="job in catalog?.items" :key="job.id" class="rounded-lg border border-default p-4" data-testid="archive-catalog-item">
            <div class="flex flex-wrap items-start justify-between gap-3">
              <div class="min-w-0"><p class="break-words font-medium">{{ job.source_file_name || sourceName(job.source_id) }}</p><p class="text-sm text-muted">{{ time(job.created_at) }} · {{ bytes(job.size_bytes) }} · {{ job.file_count }} {{ t('storageGov.archive.files') }}</p></div>
              <UBadge :color="job.remote_state === 'success' ? 'success' : 'warning'" variant="subtle">{{ t(job.remote_state === 'success' ? 'storageGov.archive.available' : 'storageGov.archive.attention') }}</UBadge>
            </div>
            <p v-if="(job.imported || job.recovered || job.migrated_from) && !job.provenance_verified" class="mt-2 text-sm text-warning">{{ t('storageGov.archive.originUnconfirmed') }}</p>
            <p v-if="job.error || job.remote_error" class="mt-2 text-sm text-error">{{ job.error || job.remote_error }}</p>
            <div class="mt-3 flex flex-wrap gap-2">
              <UButton color="neutral" variant="outline" :disabled="busy || !readable(job)" @click="inspect(job)">{{ t('storageGov.archive.preview') }}</UButton>
              <UButton color="neutral" variant="outline" :disabled="busy || !readable(job)" @click="perform(() => api.download(job.id, operationController?.signal))">{{ t('storageGov.archive.download') }}</UButton>
              <UButton color="neutral" variant="outline" :disabled="busy || !readable(job)" @click="prepareMigration(job)">{{ t('storageGov.archive.migrate') }}</UButton>
              <UButton color="neutral" variant="ghost" :disabled="busy || job.remote_state === 'expired' || job.imported || job.recovered" @click="perform(() => api.protect(job.id, !job.protected, operationController?.signal))">{{ t(job.imported || job.recovered ? 'storageGov.archive.legacyProtected' : job.protected ? 'storageGov.archive.protected' : 'storageGov.archive.protect') }}</UButton>
            </div>
          </article>
        </div>
        <div v-if="catalog" class="mt-4 flex flex-wrap items-center gap-3 text-sm">
          <UButton color="neutral" variant="outline" :disabled="page <= 1 || catalogLoading" @click="page--; loadCatalog()">{{ t('storageGov.archive.previous') }}</UButton>
          <span>{{ page }} / {{ Math.max(1, catalog.pagination.pages) }} · {{ catalog.pagination.total }}</span>
          <UButton color="neutral" variant="outline" :disabled="page >= catalog.pagination.pages || catalogLoading" @click="page++; loadCatalog()">{{ t('storageGov.archive.next') }}</UButton>
        </div>
      </template>
      <template #policies>
        <div class="mb-4 flex justify-end"><UButton :disabled="busy || !sources.length" @click="editPolicy()">{{ t('storageGov.archive.newPolicy') }}</UButton></div>
        <p v-if="!policies.length" class="py-6 text-muted">{{ t('storageGov.archive.noPolicies') }}</p>
        <article v-for="policy in policies" :key="policy.id" class="mb-3 rounded-lg border border-default p-4">
          <div class="flex flex-wrap items-center justify-between gap-3"><p class="font-medium">{{ policy.name }}</p><UBadge :color="policy.enabled ? 'success' : 'neutral'">{{ t(policy.enabled ? 'storageGov.archive.enabled' : 'storageGov.archive.disabled') }}</UBadge></div>
          <p class="mt-2 text-sm">{{ sourceName(policy.source_id) }} → {{ targetName(policy.target_id) }}</p>
          <p class="mt-1 text-sm text-muted">{{ policy.remote_retention_days }} {{ t('storageGov.runtime.dayUnit') }} · {{ bytes(policy.remote_max_bytes) }}</p>
          <div class="mt-3 flex flex-wrap gap-2"><UButton color="neutral" variant="outline" :disabled="busy" @click="editPolicy(policy)">{{ t('storageGov.runtime.configure') }}</UButton><UButton :disabled="busy" :color="policy.enabled ? 'warning' : 'primary'" @click="enableCandidate = policy">{{ t(policy.enabled ? 'storageGov.archive.disable' : 'storageGov.archive.enable') }}</UButton><UButton color="neutral" variant="outline" :disabled="busy || !policy.enabled" @click="runPolicy(policy)">{{ t('storageGov.archive.run') }}</UButton><UButton color="neutral" variant="outline" :disabled="busy" @click="gcPolicyCandidate = policy">{{ t(policy.remote_cleanup_enabled ? 'storageGov.archive.gcDisable' : 'storageGov.archive.gcEnable') }}</UButton><UButton color="neutral" variant="outline" :disabled="busy" @click="previewGC(policy)">{{ t('storageGov.archive.gcPreview') }}</UButton></div>
        </article>
      </template>
      <template #targets>
        <div class="mb-4 flex flex-wrap gap-2"><UButton :disabled="busy" @click="editTarget()">{{ t('storageGov.archive.newTarget') }}</UButton><UButton color="neutral" variant="outline" :disabled="busy" @click="credentialOpen = true">{{ t('storageGov.archive.newCredential') }}</UButton></div>
        <p v-if="!targets.length" class="py-6 text-muted">{{ t('storageGov.archive.noTargets') }}</p>
        <article v-for="target in targets" :key="target.id" class="mb-3 rounded-lg border border-default p-4">
          <div class="flex flex-wrap justify-between gap-2"><h3 class="font-medium">{{ target.name }}</h3><UBadge :color="target.verification_valid ? 'success' : 'warning'">{{ t(target.verification_valid ? 'storageGov.archive.verified' : 'storageGov.archive.unverified') }}</UBadge></div>
          <p class="mt-2 break-all text-sm">{{ target.endpoint }} / {{ target.bucket }} / {{ target.prefix }}</p>
          <p class="mt-1 text-sm text-muted">{{ t('storageGov.archive.executor') }}: {{ target.executor || overview?.instance }} · {{ time(target.verified_at) }}</p>
          <div class="mt-3 flex flex-wrap gap-2"><UButton color="neutral" variant="outline" :disabled="busy" @click="editTarget(target)">{{ t('storageGov.runtime.configure') }}</UButton><UButton :disabled="busy" color="neutral" @click="perform(() => api.testTarget(target.id!, operationController?.signal))">{{ t('storageGov.archive.test') }}</UButton><UButton :disabled="busy || !target.verification_valid" color="neutral" variant="outline" @click="importCandidate = target">{{ t('storageGov.archive.import') }}</UButton></div>
        </article>
      </template>
    </BackupSectionTabs>

    <UModal :open="!!gcPolicyCandidate" :title="t('storageGov.archive.gcSchedule')" :description="t('storageGov.archive.gcScheduleHint')" @update:open="open => { if (!open && !busy) gcPolicyCandidate = undefined }">
      <template #body><p>{{ gcPolicyCandidate?.name }} · {{ gcPolicyCandidate?.remote_retention_days }} {{ t('storageGov.runtime.dayUnit') }}</p><p v-if="error" role="alert" class="text-error">{{ error }}</p></template><template #footer><UButton :loading="busy" @click="toggleGC">{{ t('backupCenter.confirm') }}</UButton></template>
    </UModal>
    <UModal :open="!!gcPreview" :title="t('storageGov.archive.gcPreview')" :description="t('storageGov.archive.gcHint')" @update:open="open => { if (!open && !busy) gcPreview = undefined }">
      <template #body><p>{{ gcPreview?.candidate_versions }} {{ t('storageGov.archive.versions') }} · {{ bytes(gcPreview?.candidate_bytes) }}</p><p v-if="error" role="alert" class="text-error">{{ error }}</p><details v-if="gcPreview?.protected"><summary>{{ t('storageGov.archive.protectionDetails') }}</summary><p v-for="(count, reason) in gcPreview.protected" :key="reason">{{ reason }} · {{ count }}</p></details></template><template #footer><UButton color="error" :loading="busy" :disabled="!gcPreview?.candidate_versions" @click="startGC">{{ t('storageGov.archive.gcConfirm') }}</UButton></template>
    </UModal>
    <UModal v-model:open="gcRunsOpen" :title="t('storageGov.archive.gcRuns')" :description="t('storageGov.archive.gcHint')">
      <template #body><p v-if="!overview?.remote_runs?.length" class="text-muted">{{ t('storageGov.archive.noRuns') }}</p><article v-for="run in overview?.remote_runs" :key="run.id" class="mb-3 rounded border border-default p-3"><p>{{ time(run.created_at) }} · {{ t('storageGov.archive.gcStates.' + run.state) }}</p><p>{{ ['unconfirmed', 'running', 'queued'].includes(run.state) ? '—' : bytes(run.confirmed_bytes) }}</p><p v-if="run.problem" class="text-warning">{{ run.problem }}</p><UButton v-if="['unconfirmed', 'running', 'queued'].includes(run.state)" color="neutral" variant="outline" :disabled="busy" @click="perform(() => api.reconcileRemoteGC(run.id, operationController?.signal))">{{ t('storageGov.archive.reconcile') }}</UButton></article></template>
    </UModal>
    <UModal v-model:open="targetOpen" :title="t('storageGov.archive.target')" :description="t('storageGov.archive.targetHint')" :dismissible="!busy" :close="!busy">
      <template #body><UForm id="s3-archive-target" :state="targetForm" :schema="targetSchema" class="space-y-4" @submit="saveTarget">
        <UFormField name="name" :label="t('storageGov.archive.name')"><UInput :disabled="busy" v-model="targetForm.name" class="w-full" /></UFormField>
        <UFormField name="endpoint" :label="t('storageGov.archive.endpoint')"><UInput :disabled="busy" v-model="targetForm.endpoint" placeholder="https://storage.example.com" class="w-full" /></UFormField>
        <UFormField name="bucket" :label="t('storageGov.archive.bucket')"><UInput :disabled="busy" v-model="targetForm.bucket" class="w-full" /></UFormField>
        <UFormField name="prefix" :label="t('storageGov.archive.prefix')"><UInput :disabled="busy" v-model="targetForm.prefix" class="w-full" /></UFormField>
        <UFormField name="region" :label="t('storageGov.archive.region')"><UInput :disabled="busy" v-model="targetForm.region" class="w-full" /></UFormField>
        <UFormField name="credential_ref" :label="t('storageGov.archive.credential')"><USelect :disabled="busy" v-model="targetForm.credential_ref" :items="overview?.s3_credentials || []" class="w-full" /></UFormField>
        <UCheckbox :disabled="busy" v-model="targetForm.force_path_style" :label="t('storageGov.archive.pathStyle')" />
        <p v-if="error" role="alert" class="text-sm text-error">{{ error }}</p>
      </UForm></template>
      <template #footer><UButton type="submit" form="s3-archive-target" :loading="busy">{{ t('storageGov.archive.saveDraft') }}</UButton></template>
    </UModal>
    <UModal v-model:open="credentialOpen" :title="t('storageGov.archive.newCredential')" :description="t('storageGov.archive.credentialHint')" :dismissible="!busy" :close="!busy">
      <template #body><UForm id="s3-archive-credential" :state="credentialForm" :schema="credentialSchema" class="space-y-4" @submit="saveCredential">
        <UFormField name="ref" :label="t('storageGov.archive.credential')"><UInput :disabled="busy" v-model="credentialForm.ref" autocomplete="off" class="w-full" /></UFormField>
        <UFormField name="access_key" label="Access Key"><UInput :disabled="busy" v-model="credentialForm.access_key" type="password" autocomplete="new-password" class="w-full" /></UFormField>
        <UFormField name="secret_key" label="Secret Key"><UInput :disabled="busy" v-model="credentialForm.secret_key" type="password" autocomplete="new-password" class="w-full" /></UFormField>
        <UFormField name="session_token" :label="t('storageGov.archive.sessionToken')"><UInput :disabled="busy" v-model="credentialForm.session_token" type="password" autocomplete="new-password" class="w-full" /></UFormField>
        <p v-if="error" role="alert" class="text-sm text-error">{{ error }}</p>
      </UForm></template><template #footer><UButton type="submit" form="s3-archive-credential" :loading="busy">{{ t('storageGov.archive.saveDraft') }}</UButton></template>
    </UModal>
    <UModal v-model:open="policyOpen" :title="t('storageGov.archive.newPolicy')" :description="t('storageGov.archive.policyHint')" :dismissible="!busy" :close="!busy">
      <template #body><UForm id="log-archive-policy" :state="policyForm" :schema="policySchema" class="space-y-4" @submit="savePolicy">
        <UFormField name="name" :label="t('storageGov.archive.name')"><UInput :disabled="busy" v-model="policyForm.name" class="w-full" /></UFormField>
        <UFormField name="source_id" :label="t('storageGov.archive.source')"><USelect :disabled="busy" v-model="policyForm.source_id" :items="sources.map(s => ({ value: s.id, label: s.name }))" class="w-full" /></UFormField>
        <UFormField name="target_id" :label="t('storageGov.archive.target')"><USelect :disabled="busy" v-model="policyForm.target_id" :items="targets.map(s => ({ value: s.id!, label: s.name }))" class="w-full" /></UFormField>
        <div class="grid gap-3 sm:grid-cols-2"><UFormField name="remote_retention_days" :label="t('storageGov.archive.remoteDays')"><UInput :disabled="busy" v-model.number="policyForm.remote_retention_days" type="number" class="w-full" /></UFormField><UFormField name="budgetGiB" :label="t('storageGov.archive.budget')"><UInput :disabled="busy" v-model.number="policyBudget" type="number" min="0.01" step="0.01" class="w-full" /></UFormField></div>
        <UFormField name="interval_minutes" :label="t('storageGov.archive.interval')"><UInput :disabled="busy" v-model.number="policyForm.interval_minutes" type="number" class="w-full" /></UFormField>
        <p class="text-sm text-muted">{{ t('storageGov.archive.cleanupSeparate') }}</p><p v-if="error" role="alert" class="text-sm text-error">{{ error }}</p>
      </UForm></template><template #footer><UButton type="submit" form="log-archive-policy" :loading="busy">{{ t('storageGov.archive.saveDraft') }}</UButton></template>
    </UModal>
    <UModal :open="!!enableCandidate" :title="t(enableCandidate?.enabled ? 'storageGov.archive.disable' : 'storageGov.archive.enable')" :description="t('storageGov.archive.enableHint')" @update:open="open => { if (!open && !busy) enableCandidate = undefined }">
      <template #body><p>{{ enableCandidate?.name }} → {{ targetName(enableCandidate?.target_id || '') }}</p><p v-if="error" role="alert" class="mt-3 text-error">{{ error }}</p></template>
      <template #footer><UButton :loading="busy" @click="togglePolicy">{{ t('backupCenter.confirm') }}</UButton></template>
    </UModal>
    <UModal :open="!!importCandidate" :title="t('storageGov.archive.import')" :description="t('storageGov.archive.importHint')" @update:open="open => { if (!open && !busy) importCandidate = undefined }">
      <template #body><p class="break-all">{{ importCandidate?.bucket }}/{{ importCandidate?.prefix }}</p><p v-if="error" role="alert" class="mt-3 text-error">{{ error }}</p></template>
      <template #footer><UButton :loading="busy" @click="importLegacy">{{ t('backupCenter.confirm') }}</UButton></template>
    </UModal>
    <UModal :open="!!migrationJob" :title="t('storageGov.archive.migrate')" :description="t('storageGov.archive.migrationHint')" @update:open="open => { if (!open && !busy) migrationJob = undefined }"><template #body><UFormField :label="t('storageGov.archive.migrationPolicy')"><USelect :disabled="busy" v-model="migrationPolicy" :items="policies.filter(p => p.enabled && sources.some(s => s.id === p.source_id && s.kind === migrationJob?.kind)).map(p => ({ label: p.name + ' → ' + targetName(p.target_id), value: p.id! }))" class="w-full" /></UFormField><p v-if="error" role="alert" class="mt-3 text-error">{{ error }}</p></template><template #footer><UButton :disabled="!migrationPolicy" :loading="busy" @click="migrate">{{ t('backupCenter.confirm') }}</UButton></template></UModal>
    <UModal v-model:open="previewOpen" :title="t('storageGov.archive.preview')" :description="t('storageGov.archive.previewHint')" :ui="{ content: 'max-w-4xl' }">
      <template #body><div class="space-y-3"><USelect :disabled="busy" v-model="previewEntry" :items="previewJob?.files?.map(f => ({ value: f.path, label: f.path })) || []" class="w-full" @update:model-value="previewText = ''; readPreview()" /><UButton color="neutral" :loading="busy" @click="readPreview">{{ t('storageGov.archive.readPreview') }}</UButton><p v-if="error" role="alert" class="text-error">{{ error }}</p><p v-if="previewTruncated" class="text-warning">{{ t('storageGov.archive.truncated') }}</p><pre class="max-h-[55vh] overflow-auto whitespace-pre-wrap break-words rounded border border-default p-3 text-xs" data-testid="archive-log-preview">{{ previewText }}</pre></div></template>
    </UModal>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import * as v from 'valibot';
import BackupSectionTabs from './BackupSectionTabs.vue';
import { useArchiveOpsService, type ArchiveCatalog, type ArchiveJob, type ArchiveOverview, type ArchivePolicy, type ArchiveTarget, type RemoteGCResult } from '~/composables/api/services/archiveOpsService';
import { useBackupEvents } from '~/composables/useBackupEvents';
import { formatStorageBytes as bytes } from '~/utils/storage-capacity';
const { t } = useI18n(), api = useArchiveOpsService(), GiB = 1024 ** 3;
const section = ref('catalog'), loading = ref(false), catalogLoading = ref(false), busy = ref(false), error = ref(''), notice = ref('');
const migrationJob = ref<ArchiveJob>(), migrationPolicy = ref(''); let migrationKey = '';
const gcRunsOpen = ref(false), gcPreview = ref<RemoteGCResult>(), gcPolicyCandidate = ref<ArchivePolicy>();
const overview = ref<ArchiveOverview>(), catalog = ref<ArchiveCatalog>();
const query = ref(''), from = ref(''), to = ref(''), page = ref(1);
const targetOpen = ref(false), credentialOpen = ref(false), policyOpen = ref(false), previewOpen = ref(false);
const enableCandidate = ref<ArchivePolicy>(), importCandidate = ref<ArchiveTarget>();
const previewJob = ref<ArchiveJob>(), previewEntry = ref(''), previewText = ref(''), previewTruncated = ref(false);
let operationController: AbortController | undefined;
let disposed = false, overviewController: AbortController | undefined, catalogController: AbortController | undefined;
const sections = computed(() => ['catalog', 'policies', 'targets'].map(value => ({ value, label: t('storageGov.archive.' + value) })));
const logKinds = new Set(['postgresql_logs', 'system_logs', 'audit_logs', 'application_logs', 'journal']);
const sources = computed(() => overview.value?.sources.filter(s => logKinds.has(s.kind)) || []);
const policies = computed(() => overview.value?.policies.filter(p => sources.value.some(s => s.id === p.source_id)) || []);
const targets = computed(() => overview.value?.targets.filter(s => s.driver === 's3') || []);
const lastSuccess = computed(() => overview.value?.jobs.filter(j => logKinds.has(j.kind) && j.remote_state === 'success').sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at))[0]?.created_at);
const sourceName = (id: string) => overview.value?.sources.find(s => s.id === id)?.name || id;
const targetName = (id: string) => overview.value?.targets.find(s => s.id === id)?.name || '—';
const time = (value?: string) => value ? new Date(value).toLocaleString() : '—';
const readable = (job: ArchiveJob) => job.status === 'success' && !['expired', 'expiry_pending'].includes(job.remote_state) || job.status === 'pruned' && job.remote_state === 'success';
const targetDefaults = (): ArchiveTarget => ({ name: '', driver: 's3', revision: 0, host: '', port: 0, user: '', directory: '', credential_ref: '', endpoint: '', bucket: '', prefix: '', region: 'us-east-1', force_path_style: true });
const targetForm = reactive(targetDefaults());
const credentialForm = reactive({ ref: '', expected_revision: 0, access_key: '', secret_key: '', session_token: '' });
const policyDefaults = (): ArchivePolicy => ({ name: '', revision: 0, remote_cleanup_enabled: false, source_id: '', target_id: '', enabled: false, include_database: false, interval_minutes: 5, retention_days: 7, retention_count: 168, remote_retention_days: 180, remote_retention_count: 10000, remote_max_bytes: GiB });
const policyForm = reactive(policyDefaults()), policyBudget = ref(1);
const required = v.pipe(v.string(), v.minLength(1));
const targetSchema = v.object({ name: required, endpoint: v.pipe(required, v.url()), bucket: required, prefix: required, region: v.string(), credential_ref: required, force_path_style: v.boolean() });
const credentialSchema = v.object({ ref: v.pipe(required, v.regex(/^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$/)), access_key: required, secret_key: required, session_token: v.string() });
const policySchema = v.object({ name: required, source_id: required, target_id: required, interval_minutes: v.pipe(v.number(), v.integer(), v.minValue(1)), remote_retention_days: v.pipe(v.number(), v.integer(), v.minValue(1), v.maxValue(3650)) });
async function loadOverview() { overviewController?.abort(); const c = overviewController = new AbortController(); loading.value = true; try { const value = await api.overview(c.signal); if (!disposed && !c.signal.aborted) overview.value = value; } catch (e) { if (!disposed && !c.signal.aborted) error.value = message(e); } finally { if (!disposed && !c.signal.aborted) loading.value = false; } }
async function loadCatalog() { catalogController?.abort(); const c = catalogController = new AbortController(); catalogLoading.value = true; try { const value = await api.catalog({ q: query.value, page: page.value, page_size: 20, from: from.value ? new Date(from.value + 'T00:00:00').toISOString() : undefined, to: to.value ? new Date(to.value + 'T23:59:59').toISOString() : undefined }, c.signal); if (!disposed && !c.signal.aborted) catalog.value = value; } catch (e) { if (!disposed && !c.signal.aborted) error.value = message(e); } finally { if (!disposed && !c.signal.aborted) catalogLoading.value = false; } }
async function reload() { error.value = ''; await Promise.all([loadOverview(), loadCatalog()]); }
const message = (e: unknown) => e instanceof Error ? e.message : t('storageGov.requestFailed');
async function perform(action: () => Promise<unknown>) { if (busy.value) return; operationController = new AbortController(); busy.value = true; error.value = ''; notice.value = ''; try { await action(); if (!disposed) await reload(); } catch (e) { if (!disposed) error.value = message(e); } finally { if (!disposed) busy.value = false; } }
function editTarget(target?: ArchiveTarget) { Object.keys(targetForm).forEach(key => { delete (targetForm as any)[key]; }); Object.assign(targetForm, targetDefaults(), target || {}); error.value = ''; targetOpen.value = true; }
async function saveTarget() { await perform(async () => { await api.saveTarget({ ...targetForm }, operationController?.signal); targetOpen.value = false; }); }
async function saveCredential() { await perform(async () => { await api.saveCredential({ ...credentialForm }, operationController?.signal); credentialOpen.value = false; }); }
watch(credentialOpen, open => { if (!open) Object.assign(credentialForm, { ref: '', expected_revision: 0, access_key: '', secret_key: '', session_token: '' }); });
function editPolicy(policy?: ArchivePolicy) { delete policyForm.id; Object.assign(policyForm, policyDefaults(), policy || {}); policyBudget.value = (policyForm.remote_max_bytes || GiB) / GiB; error.value = ''; policyOpen.value = true; }
async function savePolicy() { if (!Number.isFinite(policyBudget.value) || policyBudget.value <= 0) { error.value = t('storageGov.drafts.invalid'); return; } await perform(async () => { await api.savePolicy({ ...policyForm, remote_max_bytes: Math.round(policyBudget.value * GiB), enabled: false, remote_cleanup_enabled: false }, operationController?.signal); policyOpen.value = false; }); }
async function previewGC(policy: ArchivePolicy) { await perform(async () => { gcPreview.value = await api.previewRemoteGC(policy.id!, operationController?.signal); }); }
async function startGC() { const preview = gcPreview.value; if (!preview) return; await perform(async () => { await api.startRemoteGC(preview.id, operationController?.signal); gcPreview.value = undefined; gcRunsOpen.value = true; }); }
async function toggleGC() { const policy = gcPolicyCandidate.value; if (!policy) return; await perform(async () => { await api.savePolicy({ ...policy, remote_cleanup_enabled: !policy.remote_cleanup_enabled }, operationController?.signal); gcPolicyCandidate.value = undefined; }); }
async function togglePolicy() { const p = enableCandidate.value; if (!p) return; await perform(async () => { await api.savePolicy({ ...p, enabled: !p.enabled }, operationController?.signal); enableCandidate.value = undefined; }); }
async function runPolicy(policy: ArchivePolicy) { await perform(() => api.run(policy.id!, crypto.randomUUID(), operationController?.signal)); }
function prepareMigration(job: ArchiveJob) { migrationJob.value = job; migrationPolicy.value = ''; migrationKey = crypto.randomUUID(); }
async function importLegacy() { const target = importCandidate.value; if (!target) return; await perform(async () => { const result = await api.importLegacy(target.id!, operationController?.signal); notice.value = t('storageGov.archive.imported', result); importCandidate.value = undefined; section.value = 'catalog'; }); }
async function migrate() { const job = migrationJob.value; if (!job || !migrationPolicy.value) return; await perform(async () => { await api.migrate(job.id, migrationPolicy.value, migrationKey, operationController?.signal); migrationJob.value = undefined; section.value = 'catalog'; }); }
async function inspect(job: ArchiveJob) { await perform(async () => { const detail = await api.job(job.id, operationController?.signal); previewJob.value = detail; previewEntry.value = detail.files?.[0]?.path || ''; previewText.value = ''; previewOpen.value = true; }); }
async function readPreview() { if (!previewJob.value || !previewEntry.value) return; await perform(async () => { const result = await api.preview(previewJob.value!.id, previewEntry.value, operationController?.signal); previewText.value = result.text; previewTruncated.value = result.truncated; }); }
const events = useBackupEvents('archives', () => { void reload(); });
onMounted(reload);
onBeforeUnmount(() => { disposed = true; operationController?.abort(); overviewController?.abort(); catalogController?.abort(); Object.assign(credentialForm, { access_key: '', secret_key: '', session_token: '' }); });
</script>
