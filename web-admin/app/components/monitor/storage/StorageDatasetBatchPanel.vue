<template>
  <UModal v-model:open="open" :title="t('storageGov.datasets.execution.title')" :description="dataset.name">
    <template #body>
      <div class="space-y-4" data-testid="dataset-execution-panel">
        <p class="text-sm text-muted">{{ tenantName || t('storageGov.datasets.tenant') }} · {{ t('storageGov.datasets.days') }} {{ preview.retention_days }}</p>
        <UAlert v-if="error" color="error" :title="t('storageGov.requestFailed')" :description="error" />
        <p v-if="!policy" role="status">{{ loading ? t('storageGov.loading') : t('storageGov.datasets.execution.unavailable') }}</p>
        <template v-else>
          <div class="flex flex-wrap items-center justify-between gap-2">
            <p role="status" class="font-medium">{{ t(`storageGov.datasets.execution.states.${policy.state}`) }}</p>
            <UButton v-if="policy.enabled" color="warning" variant="outline" :loading="saving" @click="save(false)">
              {{ t('storageGov.datasets.execution.pause') }}
            </UButton>
          </div>
          <div v-if="!policy.enabled && !policy.active_request_uuid && !intent" class="grid gap-3">
            <UFormField :label="t('storageGov.datasets.execution.action')">
              <USelect v-model="action" :items="actions" class="w-full" />
            </UFormField>
            <UFormField v-if="action === 'archive'" :label="t('storageGov.datasets.execution.target')">
              <USelect v-model="target" :items="targets" :placeholder="t('storageGov.datasets.execution.targetHint')" class="w-full" />
            </UFormField>
            <div>
              <UButton :disabled="!action || (action === 'archive' && !target)" :loading="saving" @click="save(true)">
                {{ t('storageGov.datasets.execution.enable') }}
              </UButton>
            </div>
          </div>
          <div v-if="run" class="space-y-2 rounded-lg border border-default p-3">
            <p role="status" class="font-medium">{{ t(`storageGov.datasets.execution.states.${run.state}`) }}</p>
            <p v-if="run.affected_count !== undefined" class="text-sm text-muted">{{ t('storageGov.datasets.execution.affected') }} {{ run.affected_count }}</p>
            <UButton v-if="run.archive_available" color="neutral" variant="outline" :loading="saving" @click="download">
              {{ t('storageGov.datasets.execution.download') }}
            </UButton>
            <UButton v-if="run.state === 'queued' && !policy.enabled" color="neutral" variant="outline" :loading="saving" @click="cancelRun">
              {{ t('storageGov.datasets.execution.cancel') }}
            </UButton>
          </div>
          <UButton v-if="intent && !run" color="neutral" variant="outline" :loading="saving" @click="submit">
            {{ t('storageGov.datasets.execution.retry') }}
          </UButton>
          <div v-if="canDispatch" class="space-y-2 border-t border-default pt-3">
            <UCheckbox v-model="confirmed" :label="t(`storageGov.datasets.execution.confirm.${policy.action}`)" />
            <UButton :disabled="!confirmed" :loading="saving" @click="start">{{ t('storageGov.datasets.execution.run') }}</UButton>
          </div>
        </template>
      </div>
    </template>
    <template #footer>
      <UButton color="neutral" variant="outline" :loading="loading" @click="load">{{ t('storageGov.refresh') }}</UButton>
    </template>
  </UModal>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useArchiveOpsService } from '~/composables/api/services/archiveOpsService';
import { useStorageDatasetService, type DatasetAction, type DatasetExecutionPolicy, type DatasetPreviewInput,
  type DatasetRun, type DatasetRunInput, type StorageDataset } from '~/composables/api/services/storageDatasetService';

const props = defineProps<{
  dataset: StorageDataset; preview: DatasetPreviewInput; previewReady: boolean; tenantName?: string; revision: number;
}>();
const open = defineModel<boolean>('open', { required: true });
const { t } = useI18n();
const api = useStorageDatasetService();
const archives = useArchiveOpsService();
const policy = ref<DatasetExecutionPolicy>();
const run = ref<DatasetRun>();
const intent = ref<DatasetRunInput>();
const action = ref<DatasetAction>();
const target = ref('');
const targets = ref<{ label: string; value: string }[]>([]);
const confirmed = ref(false);
const error = ref('');
const loading = ref(false);
const saving = ref(false);
let read: AbortController | undefined;
let mutation: AbortController | undefined;
const actions = computed(() => {
  const allowed = props.dataset.available_actions.filter((item): item is DatasetAction => item === 'delete' || item === 'archive');
  return allowed.map(value => ({ value, label: t(`storageGov.datasets.execution.actions.${value}`) }));
});
const canDispatch = computed(() => props.previewReady && !!policy.value?.enabled && !policy.value.active_request_uuid && !intent.value &&
  policy.value.retention_days === props.preview.retention_days && policy.value.acceptance_revision === props.preview.expected_acceptance_revision);
const errorText = (e: unknown) => e instanceof Error ? e.message : t('storageGov.requestFailed');

async function load() {
  read?.abort();
  const request = new AbortController();
  read = request;
  loading.value = true;
  try {
    const latest = await api.policy(props.dataset.dataset_uuid, props.preview.tenant_uuid, request.signal);
    if (request.signal.aborted) return;
    policy.value = latest;
    const id = intent.value?.request_uuid || latest.active_request_uuid || run.value?.request_uuid;
    if (id) {
      const receipt = await api.run(props.dataset.dataset_uuid, id, request.signal);
      if (!request.signal.aborted) run.value = receipt;
    }
  } catch (e) {
    if (!request.signal.aborted) error.value = errorText(e);
  } finally {
    if (!request.signal.aborted) loading.value = false;
  }
}
async function loadTargets() {
  if (action.value !== 'archive') return;
  try {
    const view = await archives.overview();
    targets.value = view.targets.flatMap(item => item.driver === 's3' && item.verification_valid && item.id
      ? [{ value: item.id, label: item.name }] : []);
  } catch (e) { error.value = errorText(e); }
}
watch(action, loadTargets);
watch(open, value => {
  if (!value) return;
  action.value ??= actions.value[0]?.value;
  void load();
});
watch(() => props.revision, () => { if (open.value) void load(); });
watch(() => props.preview.request_uuid, () => {
  confirmed.value = false;
  if (run.value && !['queued', 'authorized', 'needs_reconciliation'].includes(run.value.state)) {
    intent.value = undefined;
    run.value = undefined;
  }
});

async function operate(work: (signal: AbortSignal) => Promise<void>) {
  if (saving.value) return;
  const request = new AbortController();
  mutation = request;
  saving.value = true;
  error.value = '';
  try { await work(request.signal); }
  catch (e) { if (!request.signal.aborted) error.value = errorText(e); }
  finally { if (!request.signal.aborted) saving.value = false; }
}
async function save(enabled: boolean) {
  if (!policy.value) return;
  const revision = policy.value.revision;
  await operate(async signal => {
    const value = await api.changePolicy(props.dataset.dataset_uuid, props.preview.tenant_uuid, enabled ? {
      expected_revision: revision, enabled, action: action.value, archive_target_uuid: action.value === 'archive' ? target.value : undefined,
      acceptance_revision: props.preview.expected_acceptance_revision, retention_days: props.preview.retention_days,
    } : { expected_revision: revision, enabled }, signal);
    if (!signal.aborted) policy.value = value;
  });
}
function start() {
  if (!canDispatch.value || !confirmed.value || !policy.value) return;
  intent.value = { request_uuid: crypto.randomUUID(), tenant_uuid: props.preview.tenant_uuid,
    preview_request_uuid: props.preview.request_uuid, expected_policy_revision: policy.value.revision };
  void submit();
}
async function submit() {
  if (!intent.value) return;
  const input = intent.value;
  await operate(async signal => {
    const value = await api.startRun(props.dataset.dataset_uuid, input, signal);
    if (!signal.aborted) run.value = value;
  });
}
async function cancelRun() {
  if (!run.value) return;
  const id = run.value.request_uuid;
  await operate(async signal => {
    const value = await api.cancelRun(props.dataset.dataset_uuid, id, signal);
    if (!signal.aborted) { run.value = value; await load(); }
  });
}
async function download() {
  if (!run.value?.archive_available) return;
  const id = run.value.request_uuid;
  await operate(async signal => {
    const raw = await api.download(props.dataset.dataset_uuid, id, signal);
    if (signal.aborted) return;
    const url = URL.createObjectURL(raw);
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = `dataset-${id}.jsonl`;
    anchor.click();
    window.setTimeout(() => URL.revokeObjectURL(url), 1000);
  });
}
onBeforeUnmount(() => { read?.abort(); mutation?.abort(); });
</script>
