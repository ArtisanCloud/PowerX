<template>
  <section class="min-w-0 space-y-4" data-testid="cleanup-reviews">
    <header class="flex flex-wrap items-start justify-between gap-3">
      <div><h2 class="text-lg font-semibold">{{ t('backupReview.title') }}</h2><p class="mt-1 text-sm text-muted">{{ t('backupReview.hint') }}</p></div>
      <UButton color="neutral" variant="outline" :loading="loading" data-testid="review-refresh" @click="load()">{{ t('backupReview.refresh') }}</UButton>
    </header>
    <UAlert v-if="listError" color="warning" :title="listError" />
    <p v-if="!loaded && loading" role="status" class="text-sm text-muted">{{ t('backupReview.loading') }}</p>
    <template v-if="loaded">
      <p class="text-sm text-muted">{{ t('backupReview.observed', { time: time(observed), count: total }) }}</p>
      <div v-if="!items.length" class="rounded-xl border border-default p-8 text-center text-muted" data-testid="review-empty">{{ t('backupReview.empty') }}</div>
      <div v-else class="overflow-x-auto rounded-xl border border-default">
        <table class="hidden w-full text-left text-sm sm:table" data-testid="review-table">
          <thead class="bg-elevated"><tr><th class="p-4">{{ t('backupReview.artifact') }}</th><th class="p-4">{{ t('backupReview.intent') }}</th><th class="p-4">{{ t('backupReview.updated') }}</th><th class="p-4">{{ t('backupReview.actions') }}</th></tr></thead>
          <tbody class="divide-y divide-default"><tr v-for="row in items" :key="row.artifact_id">
            <td class="p-4 align-top"><p class="font-medium">#{{ row.artifact_id }} · {{ t('backupReview.job', { id: row.job_id }) }}</p><p class="mt-1 text-muted">{{ bytes(row.size_bytes) }} · {{ t(row.protected ? 'backupReview.protected' : 'backupReview.notProtected') }}</p></td>
            <td class="max-w-72 p-4 align-top"><UBadge color="warning" variant="subtle">{{ t('backupReview.states.' + row.state) }}</UBadge><p class="mt-2 break-all font-mono text-xs text-muted">{{ row.token || '—' }}</p><p v-if="row.problem" class="mt-1 text-warning">{{ t('backupReview.recordProblem') }}</p></td>
            <td class="p-4 align-top text-muted">{{ time(row.updated_at) }}</td>
            <td class="p-4 align-top"><UButton size="sm" color="neutral" variant="outline" :disabled="!row.inspectable" :data-review-id="row.artifact_id" @click="inspect(row)">{{ t('backupReview.inspect') }}</UButton></td>
          </tr></tbody>
        </table>
        <div class="divide-y divide-default sm:hidden" data-testid="review-cards"><article v-for="row in items" :key="row.artifact_id" class="space-y-3 p-4">
          <div class="flex flex-wrap justify-between gap-2"><strong>#{{ row.artifact_id }} · {{ t('backupReview.job', { id: row.job_id }) }}</strong><UBadge color="warning" variant="subtle">{{ t('backupReview.states.' + row.state) }}</UBadge></div>
          <p class="text-sm text-muted">{{ bytes(row.size_bytes) }} · {{ time(row.updated_at) }}</p><p class="break-all font-mono text-xs text-muted">{{ row.token || '—' }}</p>
          <p v-if="row.problem" class="text-sm text-warning">{{ t('backupReview.recordProblem') }}</p>
          <UButton color="neutral" variant="outline" :disabled="!row.inspectable" :data-review-card-id="row.artifact_id" @click="inspect(row)">{{ t('backupReview.inspect') }}</UButton>
        </article></div>
      </div>
      <footer class="flex flex-wrap items-center justify-between gap-3"><span class="text-sm text-muted">{{ t('backupReview.page', { page }) }}</span><div class="flex gap-2"><UButton color="neutral" variant="outline" :disabled="loading || page <= 1" @click="load(page - 1)">{{ t('backupCenter.previous') }}</UButton><UButton color="neutral" variant="outline" :disabled="loading || page * 20 >= total" @click="load(page + 1)">{{ t('backupCenter.next') }}</UButton></div></footer>
    </template>
    <UModal v-model:open="open" :dismissible="!taskLocked" :close="taskLocked ? false : true" :title="t('backupReview.inspectTitle')" :description="t(detailSection === 'quarantine' ? 'backupQuarantine.hint' : detailSection === 'retain' ? 'backupRetain.modalHint' : detailSection === 'resolution' ? 'backupResolution.hint' : 'backupReview.inspectHint')" :ui="{ content: 'w-[calc(100%-2rem)] max-w-4xl' }">
      <template #body>
        <div class="min-w-0 space-y-4" data-testid="review-inspection">
          <p v-if="selected" class="text-sm font-medium">#{{ selected.artifact_id }} · {{ t('backupReview.job', { id: selected.job_id }) }}</p>
          <p v-if="selected" class="break-all font-mono text-xs text-muted" data-testid="review-frozen-token">{{ selected.token }}</p>
          <UAlert v-if="stale" color="warning" :title="t(['retain','resolution','quarantine'].includes(detailSection) && ['retained','quarantined'].includes(retainOutcome) ? 'backupRetain.snapshotOld' : 'backupReview.stale')" />
          <UAlert v-if="detailError" color="warning" :title="detailError" />
          <p v-if="inspecting" role="status">{{ t('backupReview.inspecting') }}</p>
          <BackupSectionTabs v-if="inspection" v-model="detailSection" :items="detailSections" :label="t('backupReview.inspectTitle')">
          <template #metadata><div class="space-y-4">
            <UAlert :color="inspection.state === 'present_unverified' ? 'info' : 'warning'" :title="t('backupReview.results.' + inspection.state)" :description="t('backupReview.noProof')" />
            <dl class="divide-y divide-default rounded-lg border border-default"><div v-for="file in inspection.files" :key="file.role" class="flex flex-wrap justify-between gap-2 p-3 text-sm"><dt>{{ t('backupReview.files.' + file.role) }}</dt><dd>{{ t('backupReview.fileStates.' + file.state) }}<span v-if="file.size_bytes !== undefined" class="ml-2 text-muted">{{ bytes(file.size_bytes) }}</span></dd></div></dl>
            <p class="text-xs text-muted">{{ t('backupReview.checkTime', { time: time(inspection.observed_at) }) }}</p>
            <p class="text-sm text-muted">{{ t('backupReview.nextHint') }}</p>
            <UButton color="neutral" variant="outline" data-testid="review-open-verification" @click="detailSection = 'verification'">{{ t('backupReview.verify.title') }}</UButton>
          </div></template>
          <template #verification>
          <section class="space-y-3 rounded-xl border border-default p-4" data-testid="review-verification">
            <h3 class="font-medium">{{ t('backupReview.verify.title') }}</h3>
            <p class="text-sm text-muted">{{ t('backupReview.verify.hint') }}</p>
            <div v-if="verifying" role="status" class="space-y-2">
              <p class="text-sm">{{ t('backupReview.verify.running') }} · {{ bytes(verification?.bytes_read || 0) }} / {{ bytes(verification?.total_bytes || selected?.size_bytes || 0) }}</p>
              <progress class="h-2 w-full accent-primary" :value="verification?.bytes_read || 0" :max="Math.max(verification?.total_bytes || selected?.size_bytes || 1, 1)" :aria-label="t('backupReview.verify.running')" />
            </div>
            <UAlert v-if="verification && !verifying && !verificationError" :color="verification.state === 'verified' && !stale ? 'success' : 'warning'" variant="subtle" :title="t('backupReview.verify.states.' + verification.state)" :description="t('backupReview.verify.diagnosticOnly')" data-testid="verification-result" />
            <UAlert v-if="verificationError" color="warning" variant="subtle" :title="verificationError" />
            <div class="flex flex-wrap gap-2">
              <UButton :disabled="stale || inspecting || verifying" data-testid="review-verify" @click="verify">{{ t('backupReview.verify.start') }}</UButton>
              <UButton v-if="verifying" color="neutral" variant="outline" data-testid="review-stop-verify" @click="stopVerification">{{ t('backupReview.verify.stop') }}</UButton>
            </div>
          </section>
          </template>
          <template #retain><BackupRetainReview v-if="retainVisited" :item="selected" :disabled="resolutionLocked || quarantineLocked || stale || inspecting || verifying" @locked="retainLocked = $event" @settled="retainOutcome = $event" /></template>
          <template #resolution><BackupCleanupResolution v-if="resolutionVisited" :item="selected" :disabled="retainLocked || quarantineLocked || stale || inspecting || verifying" @locked="resolutionLocked = $event" @settled="retainOutcome = $event" /></template>
          <template #quarantine><BackupCleanupQuarantine v-if="quarantineVisited" :item="selected" :disabled="retainLocked || resolutionLocked || stale || inspecting || verifying" @locked="quarantineLocked = $event" @settled="retainOutcome = $event" /></template>
          </BackupSectionTabs>
        </div>
      </template>
      <template #footer><div class="flex w-full flex-wrap justify-end gap-2"><UButton v-if="selected?.inspectable" color="primary" variant="outline" :disabled="taskLocked" data-testid="review-open-job" @click="openJob">{{ t('backupReview.openJob') }}</UButton><UButton color="neutral" variant="outline" :disabled="taskLocked || inspecting || verifying || !selected" data-testid="review-reload-intent" @click="reloadIntent">{{ t('backupReview.reloadIntent') }}</UButton><UButton color="neutral" :disabled="taskLocked" @click="close">{{ t('backupReview.close') }}</UButton></div></template>
    </UModal>
  </section>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import BackupSectionTabs from './BackupSectionTabs.vue';
import BackupRetainReview from './BackupRetainReview.vue';
import BackupCleanupResolution from './BackupCleanupResolution.vue';
import BackupCleanupQuarantine from './BackupCleanupQuarantine.vue';
import { useBackupCleanupReviewService, type BackupCleanupReview, type BackupCleanupInspection, type BackupCleanupVerification } from '~/composables/api/services/backupCleanupReviewService';
import { useBackupEvents } from '~/composables/useBackupEvents';
import { AUTH_SESSION_EVENT, readAuthSession } from '~/utils/auth-session';
const emit = defineEmits<{ attention: [value: { count: number | null; problem: boolean }]; 'open-job': [id: string] }>();
const { t } = useI18n(); const api = useBackupCleanupReviewService();
const items = ref<BackupCleanupReview[]>([]), total = ref(0), page = ref(1), observed = ref('');
const loaded = ref(false), loading = ref(false), listError = ref(''), detailError = ref('');
const open = ref(false), inspecting = ref(false), stale = ref(false);
const selected = ref<BackupCleanupReview>(), inspection = ref<BackupCleanupInspection>();
const resolutionLocked=ref(false),resolutionVisited=ref(false);
const quarantineLocked=ref(false),quarantineVisited=ref(false);
const taskLocked=computed(()=>retainLocked.value||resolutionLocked.value||quarantineLocked.value);
const retainLocked=ref(false),retainVisited=ref(false),retainOutcome=ref('');
const detailSection = ref('metadata');
watch(detailSection,v=>{if(v==='retain')retainVisited.value=true;if(v==='resolution')resolutionVisited.value=true;if(v==='quarantine')quarantineVisited.value=true});
const detailSections = computed(() => [{ value: 'metadata', label: t('backupReview.metadataTab') }, { value: 'verification', label: t('backupReview.verify.title') }, {value:'retain',label:t('backupRetain.title')},{value:'resolution',label:t('backupResolution.title')},{value:'quarantine',label:t('backupQuarantine.title')}]);
const verification = ref<BackupCleanupVerification>(), verifying = ref(false), verificationError = ref('');
let verificationController: AbortController | undefined;
let listController: AbortController | undefined, detailController: AbortController | undefined, owner = '', disposed = false;
const sessionKey = () => { const s = readAuthSession(); return `${s?.sessionId || ''}:${s?.tenantUuid || ''}:${s?.status || ''}`; };
const time = (v: string) => v ? new Date(v).toLocaleString() : '—';
const bytes = (v: number) => { if (v < 0) return t('backupReview.unknownSize'); if (v < 1024) return `${v} B`; const units = ['KiB','MiB','GiB','TiB']; let n = v, i = -1; do { n /= 1024; i++; } while (n >= 1024 && i < units.length - 1); return `${n.toFixed(2)} ${units[i]}`; };
async function load(next = page.value) {
  listController?.abort(); const c = listController = new AbortController(); loading.value = true;
  try {
    const value = await api.list(next, c.signal);
    if (c.signal.aborted || disposed) return;
    items.value = value.items; total.value = value.total; page.value = value.page; observed.value = value.observed_at; loaded.value = true; listError.value = '';
    emit('attention', { count: value.total, problem: false });
    return true;
  } catch { if (!c.signal.aborted && !disposed) { listError.value = t('backupReview.loadFailed'); emit('attention', { count: null, problem: true }); return false; } }
  finally { if (!c.signal.aborted && !disposed) loading.value = false; }
}
async function inspect(row: BackupCleanupReview) {
  quarantineVisited.value=false;quarantineLocked.value=false;resolutionVisited.value=false;resolutionLocked.value=false;retainVisited.value=false;retainOutcome.value='';stopVerification(); verification.value = undefined; verificationError.value = ""; detailSection.value = 'metadata';
  detailController?.abort(); const c = detailController = new AbortController();
  selected.value = { ...row }; inspection.value = undefined; detailError.value = ''; stale.value = false; open.value = true; inspecting.value = true;
  try {
    const value = await api.inspect(row.artifact_id, row.token, c.signal);
    if (c.signal.aborted || disposed) return;
    if (value.item.artifact_id !== row.artifact_id || value.item.token !== row.token) throw new Error('changed');
    inspection.value = value;
  } catch { if (!c.signal.aborted && !disposed) { detailError.value = t('backupReview.inspectFailed'); stale.value = true; } }
  finally { if (!c.signal.aborted && !disposed) inspecting.value = false; }
}
async function reloadIntent() {
  if (!selected.value) return;
  await load();
  if (listError.value || disposed || !open.value) return;
  const current = items.value.find(v => v.artifact_id === selected.value?.artifact_id);
  if (current?.inspectable) await inspect(current); else { stale.value = true; detailError.value = t('backupReview.noLongerListed'); }
}
function stopVerification() {
  verificationController?.abort();
  if (verifying.value) { verificationError.value = t('backupReview.verify.incomplete'); if (verification.value) verification.value = { ...verification.value, state: 'interrupted', content_verified: false }; }
  verifying.value = false;
}
async function verify() {
  if (!selected.value?.inspectable || stale.value || inspecting.value || verifying.value || !inspection.value) return;
  const row = { ...selected.value }; const c = verificationController = new AbortController();
  verifying.value = true; verificationError.value = ''; verification.value = undefined;
  try {
    const result = await api.verify(row.artifact_id, row.token, c.signal, value => { if (!c.signal.aborted && !disposed) verification.value = value; });
    if (!c.signal.aborted && !disposed && open.value && !stale.value) verification.value = result;
  } catch { if (!c.signal.aborted && !disposed) verificationError.value = t('backupReview.verify.incomplete'); }
  finally { if (!c.signal.aborted && !disposed) verifying.value = false; }
}
function close() { if(taskLocked.value)return; (document.activeElement as HTMLElement | null)?.blur(); open.value = false; }
function openJob() { if (!selected.value?.inspectable) return; const id = selected.value.job_id; close(); emit('open-job', id); }
function identityChanged() {
  const next = sessionKey(); if (next === owner) return false; owner = next;
  quarantineVisited.value=false;quarantineLocked.value=false;retainLocked.value=false;resolutionVisited.value=false;resolutionLocked.value=false;retainVisited.value=false;retainOutcome.value='';stopVerification(); verification.value = undefined; verificationError.value = "";
  listController?.abort(); detailController?.abort(); items.value = []; total.value = 0; loaded.value = false; selected.value = undefined; inspection.value = undefined; open.value = false; loading.value = inspecting.value = false; listError.value = detailError.value = '';
  emit('attention', { count: null, problem: false });
  if (readAuthSession()?.status === 'active') void load(1);
  return true;
}
async function synchronizeReview() {
  if (identityChanged()) return;
  const frozen = selected.value;
  const synchronized = await load();
  if (synchronized === undefined || !frozen || disposed || selected.value !== frozen) return;
  const current = items.value.find(value => value.artifact_id === frozen.artifact_id);
  const same = current && (['review_version','artifact_id','job_id','policy_id','state','token','size_bytes','updated_at','protected','inspectable','problem'] as const).every(key => current[key] === frozen[key]);
  if (listError.value || !same) { stale.value = true; stopVerification(); }
}
useBackupEvents('backup_cleanup', synchronizeReview);
watch(open, value => { if (!value) { if(taskLocked.value){open.value=true;return} stopVerification(); detailController?.abort(); inspecting.value = false; } });
onMounted(() => { owner = sessionKey(); window.addEventListener(AUTH_SESSION_EVENT, identityChanged); window.addEventListener('storage', identityChanged); void load(); });
onBeforeUnmount(() => { disposed = true; verificationController?.abort(); listController?.abort(); detailController?.abort(); window.removeEventListener(AUTH_SESSION_EVENT, identityChanged); window.removeEventListener('storage', identityChanged); });
</script>
