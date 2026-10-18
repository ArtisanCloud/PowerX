<template>
  <UCard v-if="overview?.configured" data-testid="system-recovery-panel">
    <template #header>
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div><h2 class="text-lg font-semibold">{{ t('backupCenter.recovery.title') }}</h2><p class="mt-1 text-sm text-muted">{{ t('backupCenter.recovery.description') }}</p></div>
        <div class="flex flex-wrap gap-2"><UButton v-if="overview.recovery_entry === '/__powerx_recovery/'" href="/__powerx_recovery/" color="neutral" variant="outline">{{ t('backupCenter.recovery.console') }}</UButton><UButton :disabled="blocked || !readySnapshots.length" color="warning" @click="openPrepare()">{{ t('backupCenter.recovery.prepare') }}</UButton></div>
      </div>
    </template>
    <UAlert v-if="!overview.online" color="warning" :title="t('backupCenter.recovery.offline')" class="mb-4" />
    <UAlert v-else-if="!installationReady" color="warning" :title="overview.reason || t('backupCenter.recovery.installationRequired')" class="mb-4" />
    <UAlert v-if="overview.maintenance" color="warning" :title="t('backupCenter.recovery.maintenanceHint')" class="mb-4" />
    <UAlert v-if="switched" color="success" :title="t('backupCenter.recovery.switched')" class="mb-4"><template #actions><UButton href="/" target="_blank" rel="noopener" color="neutral">{{ t('backupCenter.recovery.openCurrentSite') }}</UButton></template></UAlert>
    <UAlert v-if="error" color="warning" :title="error" class="mb-4" />
    <p v-if="!orderedJobs.length" class="py-5 text-sm text-muted">{{ t('backupCenter.recovery.empty') }}</p>
    <div v-else class="overflow-x-auto">
      <table class="w-full text-left text-sm">
        <thead><tr class="border-b border-default"><th class="p-3">{{ t('backupCenter.recovery.snapshot') }}</th><th class="p-3">{{ t('backupCenter.recovery.result') }}</th><th class="p-3">{{ t('backupCenter.recovery.rollbackPoint') }}</th><th class="p-3">{{ t('backupCenter.recovery.actions') }}</th></tr></thead>
        <tbody><tr v-for="job in orderedJobs" :key="job.id" class="border-b border-default align-top">
          <td class="p-3"><p>{{ new Date(job.created_at).toLocaleString() }}</p><p class="mt-1 break-all text-xs text-muted">{{ job.snapshot_id }}</p></td>
          <td class="p-3"><UBadge :color="job.status === 'complete' ? 'success' : ['attention_required','failed'].includes(job.status) ? 'error' : 'neutral'">{{ status(job.status) }}</UBadge><p v-if="active(job)" class="mt-2">{{ t(`backupCenter.recovery.phases.${job.phase}`, job.phase) }}</p><p v-if="job.error" class="mt-2 text-error">{{ job.error }}</p></td>
          <td class="p-3"><p class="break-all text-xs">{{ job.rollback_snapshot_id || '—' }}</p><UButton v-if="job.rollback_snapshot_id && readySnapshots.some(item => item.id === job.rollback_snapshot_id)" class="mt-2" size="sm" color="neutral" :disabled="blocked" @click="openPrepare(job.rollback_snapshot_id)">{{ t('backupCenter.recovery.restoreProtection') }}</UButton></td>
          <td class="p-3"><UButton size="sm" color="neutral" @click="detail = job">{{ t('backupCenter.recovery.report') }}</UButton></td>
        </tr></tbody>
      </table>
    </div>
    <UModal :open="dialog !== null" :title="t('backupCenter.recovery.prepare')" :description="t('backupCenter.recovery.prepareHint')" @update:open="value => { if (!value && !busy) close() }">
      <template #body><div class="space-y-4">
        <UFormField :label="t('backupCenter.recovery.snapshot')"><USelect v-model="snapshotID" :items="snapshotItems" value-key="value" label-key="label" class="w-full" /></UFormField>
        <UAlert color="warning" :title="t('backupCenter.recovery.prepareHint')" />
        <UAlert v-if="isOnlinePackage" color="warning" :title="t('backupCenter.recovery.localOnlineHint')" />
        <UCheckbox v-model="confirmed" :label="t('backupCenter.recovery.prepareConsent')" />
        <p v-if="modalError" class="text-sm text-error">{{ modalError }}</p>
      </div></template>
      <template #footer><div class="flex w-full justify-end gap-2"><UButton color="neutral" variant="ghost" :disabled="busy" @click="close">{{ t('backupCenter.cancel') }}</UButton><UButton color="warning" :loading="busy" :disabled="!confirmed || !snapshotID || blocked" @click="submit">{{ t('backupCenter.confirm') }}</UButton></div></template>
    </UModal>
    <UModal :open="detail !== null" :title="t('backupCenter.recovery.verification')" @update:open="value => { if (!value) detail = null }">
      <template #body><div class="space-y-3"><p class="break-all font-mono text-xs">{{ detail?.id }}</p><div v-for="(check,index) in detail?.checks" :key="`${check.name}-${index}`" class="rounded-lg border border-default p-3"><p class="font-medium">{{ check.passed ? '✓' : '×' }} {{ check.name }}</p><p class="mt-1 text-sm text-muted">{{ check.detail }}</p></div><p v-if="detail?.rollback_snapshot_id" class="break-all text-sm">{{ t('backupCenter.recovery.rollbackPoint') }}: {{ detail.rollback_snapshot_id }}</p></div></template>
    </UModal>
  </UCard>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useSystemRecoveryService, type RecoveryAvailability, type RecoveryJob, type RecoveryOverview } from '~/composables/api/services/systemRecoveryService';
import type { SystemSnapshot } from '~/composables/api/services/systemSnapshotService';
import { useBackupEvents } from '~/composables/useBackupEvents';
import { AUTH_SESSION_EVENT, readAuthSession } from '~/utils/auth-session';
const props = defineProps<{ snapshots: SystemSnapshot[] }>();
const { t, te } = useI18n();
const emit = defineEmits<{ availability: [RecoveryAvailability] }>();
const service = useSystemRecoveryService();
const overview = ref<RecoveryOverview | null>(null);
const loadFailed = ref(false);
const switched=ref(false);
 const error = ref(''); const modalError = ref(''); const busy = ref(false);
const tokens = new Map<string, string>();
let disposed = false; let loading = false; let pendingRefresh = false;
const streams = new Map<string, { token: string; stop: () => void }>();
const disconnected = new Set<string>();
let lastAvailability = '';
const identity = () => {const session=readAuthSession();return `${session?.sessionId || ''}:${session?.tenantUuid || ''}:${session?.status || ''}`;};
let owner=identity();
function identityChanged() {
  const next=identity();if(next===owner)return;owner=next;
  for(const stream of streams.values())stream.stop();streams.clear();tokens.clear();disconnected.clear();
  overview.value=null;dialog.value=null;detail.value=null;confirmed.value=false;busy.value=false;switched.value=false;loadFailed.value=false;
  emitAvailability();
  if(readAuthSession()?.status==='active')void load();
}
const dialog = ref<{ key: string } | null>(null);
const snapshotID = ref('');
const confirmed = ref(false);
const detail = ref<RecoveryJob | null>(null);
const isOnlinePackage = computed(() => !!props.snapshots.find(item => item.id === snapshotID.value && !item.recovery_supported));
const installationReady = computed(() => overview.value?.targets.filter(target => target.allow_overwrite).length === 1);
const readySnapshots = computed(() => props.snapshots.filter(item => item.status === 'ready' && (item.recovery_supported || overview.value?.targets.some(target => target.allow_overwrite && target.environment === 'dev' && target.allow_online_data))));
const snapshotItems = computed(() => readySnapshots.value.map(item => ({ value: item.id, label: `${new Date(item.captured_at || item.created_at).toLocaleString()} · ${item.version}` })));
const orderedJobs = computed(() => [...(overview.value?.jobs || [])].filter(job => job.kind !== 'capture').sort((a,b) => b.created_at.localeCompare(a.created_at)));
const active = (job: RecoveryJob) => ['queued', 'running'].includes(job.status);
const availabilityStatus = computed<RecoveryAvailability['status']>(() => {
  if (loadFailed.value) return 'error';
  if (!overview.value) return 'loading';
  if (!overview.value.configured) return 'unconfigured';
  if (!overview.value.online) return 'offline';
  if (!installationReady.value) return 'unregistered';
  if (overview.value.maintenance) return 'maintenance';
  if (overview.value.jobs.some(job => active(job) || job.status === 'attention_required')) return 'busy';
  return 'ready';
});
const blocked = computed(() => availabilityStatus.value !== 'ready');
const status = (value: string) => te(`backupCenter.recovery.states.${value}`) ? t(`backupCenter.recovery.states.${value}`) : value;
function close() { dialog.value = null; confirmed.value = false; modalError.value = ''; }
function openPrepare(id?: string) {
  const selected = id || readySnapshots.value[0]?.id;
  if (blocked.value || !readySnapshots.value.some(item => item.id === selected)) return;
  close(); snapshotID.value = selected!; dialog.value = { key: crypto.randomUUID() };
}
function emitAvailability() {
  const value: RecoveryAvailability = {
    configured: !!overview.value?.configured,
	 reason: overview.value?.reason,
    online: !!overview.value?.online,
    activeCount: overview.value?.jobs.filter(active).length || 0,
    attentionCount: overview.value?.jobs.filter(job => job.status === 'attention_required' || job.status === 'failed').length || 0,
    status: availabilityStatus.value,
    readySnapshotIDs: readySnapshots.value.map(item => item.id),
  };
  const signature = JSON.stringify(value);
  if (signature === lastAvailability) return;
  lastAvailability = signature;
  emit('availability', value);
}
function upsert(job: RecoveryJob) {
  if (!overview.value || disposed) return;
  overview.value.jobs = [job, ...overview.value.jobs.filter(item => item.id !== job.id)];
  if (detail.value?.id === job.id) detail.value = job;
  if (job.kind === 'restore' && job.status === 'complete') { switched.value = true; overview.value.maintenance = false; void load(); }
  emitAvailability(); syncStreams();
}
function syncStreams() {
  const wanted = new Map((overview.value?.jobs || []).filter(active).map(job => [job.id, tokens.get(job.id)]));
  for (const [id, stream] of streams) if (!wanted.get(id) || wanted.get(id) !== stream.token) { stream.stop(); streams.delete(id); disconnected.delete(id); }
  for (const [id, token] of wanted) {
    if (!token || streams.has(id)) continue;
    const streamOwner=owner;
    const stop = service.watchProgress(id, token, job => { if (!disposed && owner===streamOwner) upsert(job); }, state => {
      if (disposed || owner!==streamOwner) return;
      if (state === 'disconnected') disconnected.add(id); else disconnected.delete(id);
      error.value = disconnected.size ? t('backupCenter.recovery.waiting') : '';
    });
    streams.set(id, { token, stop });
  }
}
async function load() {
  if (disposed) return;
  if (loading) { pendingRefresh = true; return; }
  loading = true;
  const requestOwner=owner;
  try {
    const result = await service.overview(); if (disposed || owner!==requestOwner) return;
    // 独立进度推送可能在概览请求期间到达，保留时间更新的那份任务。
    const current = new Map((overview.value?.jobs || []).map(job => [job.id, job]));
    result.jobs = result.jobs.map(job => { const pushed = current.get(job.id); return pushed && String(pushed.updated_at || '') > String(job.updated_at || '') ? pushed : job; });
    overview.value = result; loadFailed.value = false;
    for (const [id, token] of Object.entries(result.progress_tokens || {})) {
      if (!tokens.has(id) || Number(tokens.get(id)?.split('.')[0]) * 1000 < Date.now() + 60000) tokens.set(id, token);
    }
    emitAvailability(); syncStreams(); error.value = disconnected.size ? t('backupCenter.recovery.waiting') : '';
  }
  catch { if (!disposed && owner===requestOwner) { loadFailed.value = true; error.value = t('backupCenter.recovery.availability.error'); emitAvailability(); } }
  finally { loading = false; if (pendingRefresh && !disposed) { pendingRefresh = false; void load(); } }
}
async function submit() {
  if (!dialog.value || busy.value || blocked.value || !confirmed.value || !snapshotID.value) return;
  const action = dialog.value; const requestOwner=owner;busy.value = true; modalError.value = '';
  try { const result = await service.restore(snapshotID.value, overview.value!.generation, action.key); if (disposed || owner!==requestOwner) return; tokens.set(result.recovery.id, result.progress_token); upsert(result.recovery); close(); if (overview.value?.recovery_entry === '/__powerx_recovery/') await navigateTo('/__powerx_recovery/', { external: true }); }
  catch (cause: any) { if (!disposed && owner===requestOwner) modalError.value = cause?.message || t('backupCenter.recovery.failed'); }
  finally { if (!disposed && owner===requestOwner) busy.value = false; }
}
function track(receipt: {recovery:RecoveryJob;progress_token:string}) { tokens.set(receipt.recovery.id,receipt.progress_token);upsert(receipt.recovery); }
 watch(()=>props.snapshots, snapshots=>{for (const snapshot of snapshots) if (snapshot.coordinator) track(snapshot.coordinator);emitAvailability();},{deep:true});
 defineExpose({track,openForSnapshot:(id:string)=>openPrepare(id),refresh:load});
useBackupEvents('recovery', () => { void load(); });
onMounted(()=>{window.addEventListener(AUTH_SESSION_EVENT,identityChanged);window.addEventListener('storage',identityChanged);void load();});
onBeforeUnmount(() => { disposed = true; window.removeEventListener(AUTH_SESSION_EVENT,identityChanged);window.removeEventListener('storage',identityChanged);for (const stream of streams.values()) stream.stop(); streams.clear(); tokens.clear(); });
</script>
