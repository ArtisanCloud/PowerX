<template>
  <UCard data-testid="restore-copies-panel">
    <template #header>
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div><h2 class="text-lg font-semibold">{{ t('backupCenter.copies.title') }}</h2><p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('backupCenter.copies.subtitle') }}</p></div>
        <div class="flex gap-2"><UButton color="neutral" variant="ghost" icon="i-heroicons-arrow-path" :loading="loading" @click="refresh">{{ t('backupCenter.reload') }}</UButton><UButton icon="i-heroicons-circle-stack" @click="open()">{{ t('backupCenter.copies.new') }}</UButton></div>
      </div>
    </template>
    <UAlert v-if="error" class="mb-4" color="error" :title="error" />
    <p v-if="!items.length" class="py-6 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('backupCenter.copies.empty') }}</p>
    <div class="space-y-4">
      <article v-for="item in items" :key="item.id" class="rounded-xl border border-gray-200 p-4 dark:border-gray-700" data-testid="restore-copy">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div class="min-w-0"><p class="break-all font-mono text-sm font-semibold">{{ item.database }}</p><p class="mt-1 break-all text-xs text-gray-500 dark:text-gray-400">{{ item.host }}:{{ item.port }} · {{ timeLabel(item.created_at) }}</p></div>
          <UBadge variant="subtle" :color="item.status === 'success' ? 'success' : item.status === 'failed' ? 'error' : 'neutral'">{{ t(`backupCenter.copies.status.${item.status}`) }}</UBadge>
        </div>
        <p v-if="isRunning(item)" class="mt-3 text-sm" role="status">{{ t('backupCenter.copies.processing') }} · {{ phaseLabel(item.phase) }}</p>
        <p v-if="item.error_message" class="mt-3 break-words text-sm text-red-600 dark:text-red-400">{{ item.error_code }} · {{ item.error_message }}</p>
        <dl class="mt-4 grid gap-4 text-sm sm:grid-cols-3">
          <div><dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('backupCenter.copies.original') }}</dt><dd class="mt-1 break-all font-mono">{{ item.current_database }}</dd></div>
          <div><dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('backupCenter.workspace.backupFile') }}</dt><dd class="mt-1 break-all">{{ item.source_name }}</dd></div>
          <div><dt class="text-xs text-gray-500 dark:text-gray-400">{{ t('backupCenter.copies.result') }}</dt><dd class="mt-1">{{ t('backupCenter.copies.tables', { count: item.table_count }) }} · {{ sizeLabel(item.size_bytes) }}</dd></div>
        </dl>
        <details v-if="item.status !== 'deleted'" class="mt-4 text-sm">
          <summary class="cursor-pointer text-primary-600 dark:text-primary-400">{{ t('backupCenter.copies.report') }}</summary>
          <div class="mt-3 space-y-3 rounded-lg bg-gray-50 p-3 dark:bg-gray-800/50">
            <p v-if="item.status === 'success'">{{ t('backupCenter.copies.acceptanceHint') }}</p>
            <div v-if="Object.keys(item.business_counts || {}).length" class="grid grid-cols-2 gap-2 sm:grid-cols-3"><p v-for="(count, key) in item.business_counts" :key="key">{{ businessLabel(String(key)) }}: <strong>{{ count }}</strong></p></div>
            <p v-for="warning in item.warnings" :key="warning" class="text-amber-700 dark:text-amber-400">{{ warning }}</p>
            <dl class="space-y-2 text-xs"><div><dt>SHA256</dt><dd class="break-all font-mono">{{ item.source_sha256 }}</dd></div><div v-if="item.trace_id"><dt>Trace ID</dt><dd class="break-all font-mono">{{ item.trace_id }}</dd></div><div v-if="item.schema_counts"><dt>{{ t('backupCenter.copies.schemas') }}</dt><dd class="break-all font-mono">{{ Object.entries(item.schema_counts).map(([name, count]) => `${name}: ${count}`).join(' · ') }}</dd></div></dl>
          </div>
        </details>
        <div v-if="item.status !== 'deleted'" class="mt-4 flex flex-wrap justify-end gap-2">
          <UButton v-if="item.status === 'success'" size="sm" color="neutral" variant="outline" :loading="connectionBusy === item.id" @click="showConnection(item)">{{ t('backupCenter.copies.connection') }}</UButton>
          <UButton v-if="!isRunning(item)" size="sm" color="error" variant="ghost" @click="openDelete(item)">{{ t('backupCenter.copies.delete') }}</UButton>
        </div>
      </article>
    </div>
  </UCard>

  <UModal v-model:open="wizardOpen" :dismissible="!busy" :title="t('backupCenter.copies.new')" :description="t('backupCenter.copies.wizardHint')" :ui="{ content: 'max-w-2xl' }">
    <template #body>
      <div class="space-y-5" data-testid="restore-copy-wizard">
        <UAlert v-if="wizardError" color="error" :title="wizardError" />
        <template v-if="!plan">
          <UFormField :label="t('backupCenter.copies.source')"><USelect v-model="sourceMode" :items="sourceModes" class="w-full" @update:model-value="resetPlan" /></UFormField>
          <UFormField v-if="sourceMode === 'job'" :label="t('backupCenter.copies.backupJob')"><USelect v-model="jobId" :items="jobOptions" class="w-full" /></UFormField>
          <UFormField v-else :label="t('backupCenter.copies.upload')" :help="t('backupCenter.copies.uploadHint')"><input type="file" accept=".dump" class="block w-full rounded-lg border border-gray-200 p-3 text-sm dark:border-gray-700" @change="selectFile" /></UFormField>
        </template>
        <template v-else>
          <div class="rounded-xl border border-gray-200 p-4 dark:border-gray-700">
            <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('backupCenter.copies.server') }}</p><p class="mt-1 break-all font-mono">{{ plan.host }}:{{ plan.port }}</p>
            <div class="mt-4 grid gap-4 sm:grid-cols-2"><div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('backupCenter.copies.original') }}</p><p class="mt-1 break-all font-mono">{{ plan.current_database }}</p><p class="mt-1 text-xs text-emerald-700 dark:text-emerald-400">{{ t('backupCenter.copies.originalContinues') }}</p></div><div><p class="text-xs text-gray-500 dark:text-gray-400">{{ t('backupCenter.copies.target') }}</p><p class="mt-1 break-all font-mono">{{ plan.target_pattern }}</p><p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('backupCenter.copies.generated') }}</p></div></div>
          </div>
          <dl class="grid gap-3 text-sm sm:grid-cols-2"><div><dt class="text-gray-500">{{ t('backupCenter.workspace.backupFile') }}</dt><dd class="break-all">{{ plan.archive_name }} · {{ sizeLabel(plan.archive_size) }}</dd></div><div><dt class="text-gray-500">PostgreSQL</dt><dd>{{ plan.archive_version }} → {{ plan.server_version }}</dd></div></dl>
          <p v-if="plan.extensions.length" class="text-xs text-gray-500">{{ t('backupCenter.copies.extensions') }}: {{ plan.extensions.join(', ') }}</p>
          <UAlert v-if="!plan.ready" color="error" :title="t('backupCenter.notReady')" :description="plan.problems.join(' · ')" />
          <UAlert color="info" :title="t('backupCenter.copies.executionTitle')" :description="t('backupCenter.copies.executionHint')" />
          <UCheckbox v-if="plan.requires_compatibility_consent" v-model="allowOlder" :label="t('backupCenter.copies.olderConsent')" data-testid="restore-older-consent" />
          <UCheckbox v-model="confirmed" :label="t('backupCenter.copies.confirmNew')" data-testid="restore-target-confirm" />
        </template>
      </div>
    </template>
    <template #footer><div class="flex w-full justify-between gap-2"><UButton color="neutral" variant="ghost" :disabled="busy" @click="plan ? resetPlan() : wizardOpen = false">{{ t(plan ? 'backupCenter.copies.back' : 'backupCenter.cancel') }}</UButton><UButton v-if="!plan" :loading="busy" :disabled="!canPlan" @click="prepare">{{ t('backupCenter.copies.check') }}</UButton><UButton v-else :loading="busy" :disabled="!canCreate" @click="create">{{ t('backupCenter.copies.start') }}</UButton></div></template>
  </UModal>

  <UModal v-model:open="connectionOpen" :title="t('backupCenter.copies.connection')" :description="t('backupCenter.copies.connectionHint')">
    <template #body>
      <div class="space-y-4" data-testid="restore-copy-connection">
        <p v-if="connectionBusy" role="status">{{ t('backupCenter.copies.connectionLoading') }}</p>
        <UAlert v-if="connectionError" color="error" :title="connectionError" />
        <UAlert v-if="connectionConfigNeeded" color="warning" :title="t('backupCenter.copies.accessConfigTitle')" :description="t('backupCenter.copies.accessConfigHint')" />
        <template v-if="connection">
          <UAlert v-if="connection.access_mode === 'ssh_tunnel' && connection.tunnel" color="info" data-testid="restore-copy-tunnel" :title="t('backupCenter.copies.sshTunnel')" :description="t('backupCenter.copies.sshTunnelHint', { host: connection.host, port: connection.port, targetHost: connection.tunnel.host, targetPort: connection.tunnel.port })" />
          <UAlert v-else-if="connection.access_mode === 'direct'" color="info" :title="t('backupCenter.copies.directConnection')" :description="t('backupCenter.copies.remoteHint')" />
          <UAlert v-if="!connectionCanCopy" color="error" :title="t('backupCenter.copies.invalidConnection')" />
          <dl class="grid grid-cols-2 gap-4 text-sm"><div v-for="field in connectionFields" :key="field.key"><dt class="text-gray-500">{{ field.label }}</dt><dd class="mt-1 break-all font-mono">{{ field.value }}</dd></div></dl>
          <UFormField :label="t('backupCenter.copies.password')"><SecretInput :model-value="connection.password" readonly class="w-full" /></UFormField>
          <p class="text-xs text-gray-500">{{ t('backupCenter.copies.expires', { time: timeLabel(connection.expires_at) }) }}</p>
          <div class="flex flex-wrap gap-2">
            <UButton color="neutral" variant="outline" :disabled="!connectionCanCopy" @click="copyPassword">{{ t(passwordCopied ? 'backupCenter.copies.copied' : 'backupCenter.copies.copyPassword') }}</UButton>
            <UButton color="neutral" variant="outline" :disabled="!connectionCanCopy" @click="copyConnection">{{ t(copied ? 'backupCenter.copies.copied' : 'backupCenter.copies.copyDSN') }}</UButton>
          </div>
        </template>
      </div>
    </template>
  </UModal>

  <UModal v-model:open="deleteOpen" :dismissible="!deleting" :title="t('backupCenter.copies.delete')" :description="t('backupCenter.copies.deleteHint')">
    <template #body><div class="space-y-4"><p class="break-all font-mono text-sm">{{ deletingItem?.database }}</p><UFormField :label="t('backupCenter.copies.typeName')"><UInput v-model="deleteConfirmation" class="w-full" autocomplete="off" /></UFormField><UAlert v-if="deleteError" color="error" :title="deleteError" /></div></template>
    <template #footer><div class="flex w-full justify-end gap-2"><UButton color="neutral" variant="ghost" :disabled="deleting" @click="deleteOpen = false">{{ t('backupCenter.cancel') }}</UButton><UButton color="error" :loading="deleting" :disabled="deleteConfirmation !== deletingItem?.database" @click="remove">{{ t('backupCenter.copies.delete') }}</UButton></div></template>
  </UModal>
</template>

<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref, watch } from 'vue';
import { useRestoreCopyService, type RestoreCopy, type RestoreCopyPlan, type RestoreCopySource, type RestoreCopyConnection } from '~/composables/api/services/restoreCopyService';
import type { BackupJob } from '~/composables/api/services/backupOpsService';
import { formatBackupTime } from '~/utils/backup-display';
import { useBackupEvents } from '~/composables/useBackupEvents';
const props = defineProps<{ jobs: BackupJob[] }>();
const { t, locale } = useI18n();
const service = useRestoreCopyService();
const items = ref<RestoreCopy[]>([]); const error = ref(''); const loading = ref(false);
const wizardOpen = ref(false); const wizardError = ref(''); const busy = ref(false);
const sourceMode = ref('job'); const jobId = ref(''); const file = ref<File>();
const source = ref<RestoreCopySource>(); const plan = ref<RestoreCopyPlan>(); const requestKey = ref('');
const allowOlder = ref(false); const confirmed = ref(false);
const connectionOpen = ref(false); const connection = ref<RestoreCopyConnection>(); const connectionBusy = ref(''); const connectionError = ref(''); const copied = ref(false); const passwordCopied = ref(false);
const connectionConfigNeeded = ref(false);
let connectionRevision = 0;
const deleteOpen = ref(false); const deletingItem = ref<RestoreCopy>(); const deleteConfirmation = ref(''); const deleteError = ref(''); const deleting = ref(false);
const timeLabel = (value: string) => formatBackupTime(value, String(locale?.value || 'zh-CN'));
const sizeLabel = (bytes: number) => bytes ? `${(bytes / 1048576).toFixed(1)} MiB` : '—';
const sourceModes = computed(() => ['job', 'upload'].map(value => ({ value, label: t(`backupCenter.copies.sourceMode.${value}`) })));
const jobOptions = computed(() => props.jobs.filter(job => job.status === 'success' && job.storage_uri).map(job => ({ value: String(job.id), label: `#${job.id} · ${timeLabel(job.ended_at || job.started_at || '')}` })));
const canPlan = computed(() => sourceMode.value === 'job' ? !!jobId.value : !!file.value);
const canCreate = computed(() => plan.value?.ready && confirmed.value && (!plan.value.requires_compatibility_consent || allowOlder.value));
const isRunning = (item: RestoreCopy) => item.status === 'queued' || item.status === 'running';
const phaseLabel = (phase: string) => ['queued', 'validating', 'creating', 'restoring', 'checking'].includes(phase) ? t(`backupCenter.copies.phase.${phase}`) : phase;
const businessLabel = (key: string) => ['tenants', 'users', 'design_tasks', 'orders', 'sample_rounds'].includes(key) ? t(`backupCenter.copies.business.${key}`) : key;
const failure = (e: any) => { const data = e?.response?._data || e?.data; return [...new Set([data?.message || e?.message || t('backupCenter.failed'), data?.error_code || data?.error, data?.trace_id || data?.request_id].filter(Boolean))].join(' · '); };
let disposed = false;
let listRevision = 0;
async function refresh(fromPush = false) {
  if (disposed) return;
  if (loading.value) { if (fromPush) pendingRefresh = true; return; }
  loading.value = true;
  const revision = listRevision;
  try { const result = await service.list(); if (!disposed && revision === listRevision) { items.value = result; error.value = ''; } }
  catch (e) { if (!disposed && revision === listRevision) error.value = failure(e); }
  finally { loading.value = false; if (pendingRefresh && !disposed) { pendingRefresh = false; void refresh(); } }
}
function resetPlan() { plan.value = undefined; source.value = undefined; confirmed.value = false; allowOlder.value = false; requestKey.value = ''; wizardError.value = ''; }
function open(job?: BackupJob) { resetPlan(); file.value = undefined; sourceMode.value = job || jobOptions.value.length ? 'job' : 'upload'; jobId.value = job ? String(job.id) : jobOptions.value[0]?.value || ''; wizardOpen.value = true; }
function selectFile(event: Event) { file.value = (event.target as HTMLInputElement).files?.[0]; resetPlan(); }
async function prepare() {
  if (busy.value || !canPlan.value) return;
  busy.value = true; wizardError.value = '';
  try {
    if (sourceMode.value === 'upload') {
      if (!file.value || !file.value.name.toLowerCase().endsWith('.dump') || file.value.size > 512 * 1048576) throw new Error(t('backupCenter.copies.invalidFile'));
      const upload = await service.importFile(file.value); source.value = { import_id: upload.id };
    } else source.value = { source_job_id: jobId.value };
    plan.value = await service.plan(source.value); requestKey.value = crypto.randomUUID();
  } catch (e) { wizardError.value = failure(e); } finally { busy.value = false; }
}
async function create() {
  if (busy.value || !canCreate.value || !source.value) return;
  busy.value = true; wizardError.value = '';
  try { const row = await service.create(source.value, allowOlder.value, requestKey.value); listRevision++; items.value = [row, ...items.value.filter(item => item.id !== row.id)]; wizardOpen.value = false; await refresh(); }
  catch (e) { wizardError.value = failure(e); } finally { busy.value = false; }
}
const connectionFields = computed(() => connection.value ? [
  { key: 'host', label: t('backupCenter.copies.host'), value: connection.value.host },
  { key: 'port', label: t('backupCenter.copies.port'), value: connection.value.port },
  { key: 'database', label: t('backupCenter.copies.target'), value: connection.value.database },
  { key: 'username', label: t('backupCenter.copies.username'), value: connection.value.username },
  { key: 'ssl', label: 'SSL', value: connection.value.ssl_mode },
] : []);
const connectionCanCopy = computed(() => {
  const c = connection.value;
  return !!c?.host && Number.isInteger(c.port) && c.port > 0 && c.port <= 65535 && !!c.database && !!c.username && !!c.password && c.read_only === true
    && ['disable', 'allow', 'prefer', 'require', 'verify-ca', 'verify-full'].includes(c.ssl_mode)
    && (c.access_mode === 'direct' || (c.access_mode === 'ssh_tunnel' && !!c.tunnel?.host && Number.isInteger(c.tunnel.port) && c.tunnel.port > 0 && c.tunnel.port <= 65535));
});
async function showConnection(item: RestoreCopy) {
  if (connectionBusy.value) return;
  const revision = ++connectionRevision;
  connectionBusy.value = item.id; connection.value = undefined; connectionError.value = ''; connectionConfigNeeded.value = false; copied.value = false; passwordCopied.value = false;
  connectionOpen.value = true;
  try { const result = await service.connection(item.id); if (!disposed && connectionOpen.value && revision === connectionRevision) connection.value = result; }
  catch (e: any) {
    if (!disposed && connectionOpen.value && revision === connectionRevision) {
      connectionError.value = failure(e);
      const data = e?.response?._data || e?.data;
      connectionConfigNeeded.value = ['RESTORE_CONNECTION_NOT_CONFIGURED', 'RESTORE_CONNECTION_CONFIG_INVALID'].includes(data?.error_code || data?.error || data?.message);
    }
  } finally { connectionBusy.value = ''; }
}
async function copyConnection() {
  if (!connection.value || !connectionCanCopy.value) return;
  const c = connection.value; const host = c.host.includes(':') ? `[${c.host}]` : c.host;
  try { await navigator.clipboard.writeText(`postgresql://${encodeURIComponent(c.username)}:${encodeURIComponent(c.password)}@${host}:${c.port}/${encodeURIComponent(c.database)}?sslmode=${encodeURIComponent(c.ssl_mode)}`); copied.value = true; }
  catch { connectionError.value = t('backupCenter.copies.clipboardFailed'); }
}
async function copyPassword() {
  if (!connection.value || !connectionCanCopy.value) return;
  try { await navigator.clipboard.writeText(connection.value.password); passwordCopied.value = true; }
  catch { connectionError.value = t('backupCenter.copies.clipboardFailed'); }
}
function openDelete(item: RestoreCopy) { deletingItem.value = item; deleteConfirmation.value = ''; deleteError.value = ''; deleteOpen.value = true; }
async function remove() {
  if (deleting.value || !deletingItem.value || deleteConfirmation.value !== deletingItem.value.database) return;
  deleting.value = true; deleteError.value = '';
  try { const id = deletingItem.value.id; await service.remove(id, deleteConfirmation.value); listRevision++; items.value = items.value.map<RestoreCopy>(item => item.id === id ? { ...item, status: 'deleted', phase: 'deleted' } : item); deleteOpen.value = false; await refresh(); }
  catch (e) { deleteError.value = failure(e); } finally { deleting.value = false; }
}
watch(connectionOpen, open => { if (!open) { connectionRevision++; connection.value = undefined; copied.value = false; passwordCopied.value = false; } });
let pendingRefresh = false;
useBackupEvents("restore_copies", () => { void refresh(true); });
onMounted(refresh);
onBeforeUnmount(() => { disposed = true; connection.value = undefined; });
defineExpose({ open });
</script>
