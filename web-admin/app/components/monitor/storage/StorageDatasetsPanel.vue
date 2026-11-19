<template>
  <section class="space-y-4" data-testid="storage-datasets">
    <div class="flex flex-wrap items-end justify-between gap-3">
      <div>
        <h2 class="text-lg font-semibold">{{ t('storageGov.datasets.title') }}</h2>
        <p v-if="page" class="mt-1 text-sm text-muted">{{ t('storageGov.datasets.pageSize') }} {{ formatStorageBytes(page.page_physical_bytes) }}</p>
      </div>
      <UFormField :label="t('storageGov.datasets.owner')">
        <USelect v-model="owner" :items="ownerOptions" class="min-w-36" />
      </UFormField>
    </div>
    <UAlert v-if="listError" color="error" :title="t('storageGov.loadFailed')" :description="listError" />
    <UButton v-if="intent" variant="outline" color="neutral" @click="dialogOpen = true">
      {{ selected?.name }} · {{ labelState(receipt?.state) }}
    </UButton>
    <div class="divide-y divide-default rounded-lg border border-default px-4">
      <div v-for="item in page?.items" :key="item.dataset_uuid" class="grid gap-2 py-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center">
        <div class="min-w-0">
          <p class="break-words font-medium">{{ item.name }}</p>
          <p class="break-all text-sm text-muted">
            {{ item.plugin_id || t('storageGov.datasets.system') }} · {{ formatStorageBytes(item.observation.total_bytes) }}
            <span v-if="item.source_state !== 'ready'"> · {{ t(`storageGov.datasets.source.${item.source_state}`) }}</span>
          </p>
        </div>
        <div class="flex flex-wrap gap-2">
          <UButton v-if="item.available_actions.includes('preview')" color="neutral" variant="outline"
            :disabled="!!intent && selected?.dataset_uuid !== item.dataset_uuid && !finished"
            @click="selectDataset(item)">
            {{ t('storageGov.datasets.preview') }}
          </UButton>
          <UButton v-else-if="item.available_actions.includes('open_existing_policy')" color="neutral" variant="outline"
            @click="emit('open-system-tables')">
            {{ t('storageGov.datasets.systemPolicy') }}
          </UButton>
        </div>
      </div>
      <p v-if="loading && !page" class="py-4 text-sm text-muted" role="status">{{ t('storageGov.loading') }}</p>
      <p v-else-if="page && page.items.length === 0" class="py-4 text-sm text-muted">{{ t('storageGov.datasets.empty') }}</p>
    </div>
    <div v-if="previous.length || page?.next_cursor" class="flex flex-wrap gap-2">
      <UButton :disabled="!previous.length || loading" color="neutral" variant="outline" @click="back">{{ t('storageGov.datasets.previous') }}</UButton>
      <UButton :disabled="!page?.next_cursor || loading" color="neutral" variant="outline" @click="next">{{ t('storageGov.datasets.next') }}</UButton>
    </div>

    <UModal v-model:open="dialogOpen" :title="selected?.name || t('storageGov.datasets.preview')" :description="t('storageGov.datasets.previewDescription')">
      <template #body>
        <div v-if="selected?.retention_bounds" class="space-y-4" data-testid="dataset-preview-dialog">
          <UAlert v-if="previewError" color="error" :title="t('storageGov.requestFailed')" :description="previewError" />
          <div v-if="!intent" class="space-y-3">
            <UFormField :label="t('storageGov.datasets.tenantSearch')">
              <form class="flex gap-2" @submit.prevent="loadTenants">
                <UInput v-model="tenantQuery" class="min-w-0 flex-1" />
                <UButton color="neutral" variant="outline" :loading="tenantLoading" type="submit">{{ t('storageGov.datasets.search') }}</UButton>
              </form>
            </UFormField>
            <UFormField :label="t('storageGov.datasets.tenant')" required>
              <USelect v-model="tenant" :items="tenants" :placeholder="t('storageGov.datasets.chooseTenant')" class="w-full" />
            </UFormField>
            <UFormField :label="t('storageGov.datasets.days')" :hint="t('storageGov.datasets.bounds', { min: selected.retention_bounds.minimum_days, max: selected.retention_bounds.maximum_days })" required>
              <UInput v-model.number="days" type="number" :min="selected.retention_bounds.minimum_days" :max="selected.retention_bounds.maximum_days" />
            </UFormField>
          </div>
          <template v-else>
            <p role="status" class="font-medium">{{ labelState(receipt?.state) }}</p>
            <p v-if="receipt" class="text-sm text-muted">{{ t('storageGov.datasets.cutoff') }} {{ new Date(receipt.cutoff_at).toLocaleString() }}</p>
            <template v-if="receipt?.result">
              <div class="grid gap-2 sm:grid-cols-2" data-testid="dataset-preview-counts">
                <p class="rounded-md border border-default p-3">{{ t('storageGov.datasets.examined') }} <strong>{{ receipt.result.examined_count }}</strong></p>
                <p class="rounded-md border border-default p-3">{{ t('storageGov.datasets.candidates') }} <strong>{{ receipt.result.candidate_count }}</strong></p>
              </div>
              <p v-if="receipt.result.coverage === 'bounded'" class="text-sm text-muted">{{ t('storageGov.datasets.bounded') }}</p>
              <ul v-if="receipt.result.protected_by_reason.length" class="divide-y divide-default">
                <li v-for="item in receipt.result.protected_by_reason" :key="item.reason" class="flex justify-between gap-2 py-2 text-sm">
                  <span>{{ t(`storageGov.datasets.protection.${item.reason}`) }}</span><span>{{ item.count }}</span>
                </li>
              </ul>
            </template>
          </template>
        </div>
      </template>
      <template #footer>
        <div class="flex flex-wrap gap-2">
          <UButton v-if="!intent" :disabled="!canStart" :loading="posting" @click="start">{{ t('storageGov.datasets.preview') }}</UButton>
          <UButton v-else-if="!receipt" :loading="posting" @click="submit">{{ t('storageGov.datasets.retry') }}</UButton>
          <UButton v-else-if="finished" color="neutral" variant="outline" @click="resetPreview">{{ t('storageGov.datasets.newPreview') }}</UButton>
          <UButton v-else color="neutral" variant="outline" @click="refreshPreview">{{ t('storageGov.refresh') }}</UButton>
          <UButton v-if="showExecution" @click="dialogOpen = false; executionOpen = true">{{ t('storageGov.datasets.execution.title') }}</UButton>
        </div>
      </template>
    </UModal>
    <StorageDatasetBatchPanel v-if="selected && intent"
      :key="`${selected.dataset_uuid}:${intent.tenant_uuid}`"
      v-model:open="executionOpen" :dataset="selected" :preview="intent"
      :preview-ready="previewReady" :tenant-name="tenants.find(item => item.value === intent?.tenant_uuid)?.label"
      :revision="revision" />
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useStorageDatasetService, type StorageDataset, type DatasetPreview, type DatasetPreviewInput } from '~/composables/api/services/storageDatasetService';
import { useTenantService } from '~/composables/api/services/tenantService';
import { formatStorageBytes } from '~/utils/storage-capacity';
import StorageDatasetBatchPanel from './StorageDatasetBatchPanel.vue';

const props = defineProps<{ revision: number; refresh: number }>();
const emit = defineEmits<{ 'open-system-tables': [] }>();
const { t } = useI18n();
const api = useStorageDatasetService();
const tenantApi = useTenantService();
const owner = ref('');
const ownerOptions = computed(() => [
  { label: t('storageGov.datasets.all'), value: '' },
  { label: t('storageGov.datasets.system'), value: 'system' },
  { label: t('storageGov.datasets.plugin'), value: 'plugin' },
]);
const cursor = ref('');
const previous = ref<string[]>([]);
const page = ref<Awaited<ReturnType<typeof api.list>>>();
const listError = ref('');
const loading = ref(false);
const dialogOpen = ref(false);
const executionOpen = ref(false);
const selected = ref<StorageDataset>();
const tenant = ref('');
const tenants = ref<{ value: string; label: string }[]>([]);
const tenantQuery = ref('');
const tenantLoading = ref(false);
const days = ref<number>();
const previewError = ref('');
const posting = ref(false);
const intent = ref<DatasetPreviewInput>();
const receipt = ref<DatasetPreview>();
let listRequest: AbortController | undefined;
let refreshRequest: AbortController | undefined;
let postRequest: AbortController | undefined;
let tenantGeneration = 0;
const showExecution = computed(() => !!receipt.value && selected.value?.available_actions.some(action => action === 'delete' || action === 'archive'));
const previewReady = computed(() => receipt.value?.state === 'completed' && (receipt.value.result?.candidate_count ?? 0) > 0);
const finished = computed(() => !!receipt.value && !['queued', 'running'].includes(receipt.value.state));
const labelState = (state = 'pending') => t(`storageGov.datasets.states.${state}`);
const errorText = (error: unknown) => error instanceof Error ? error.message : t('storageGov.requestFailed');
const canStart = computed(() => {
  const bounds = selected.value?.retention_bounds;
  return !!tenant.value && !!bounds && Number.isInteger(days.value) &&
    days.value! >= bounds.minimum_days && days.value! <= bounds.maximum_days;
});

async function load() {
  listRequest?.abort();
  const request = new AbortController();
  listRequest = request;
  loading.value = true;
  listError.value = '';
  try {
    const result = await api.list(owner.value, cursor.value, request.signal);
    if (!request.signal.aborted) page.value = result;
  } catch (error) {
    if (!request.signal.aborted) listError.value = errorText(error);
  } finally {
    if (!request.signal.aborted) loading.value = false;
  }
}
function next() {
  if (!page.value?.next_cursor) return;
  previous.value.push(cursor.value);
  cursor.value = page.value.next_cursor;
  void load();
}
function back() {
  cursor.value = previous.value.pop() ?? '';
  void load();
}
watch(owner, () => {
  cursor.value = '';
  previous.value = [];
  page.value = undefined;
  void load();
});
watch(() => props.refresh, () => { void load(); void refreshPreview(); });
watch(() => props.revision, () => { if (intent.value) void refreshPreview(); });

function selectDataset(item: StorageDataset) {
  if (selected.value?.dataset_uuid !== item.dataset_uuid) {
    selected.value = item;
    resetPreview();
  }
  dialogOpen.value = true;
}
function resetPreview() {
  refreshRequest?.abort();
  receipt.value = undefined;
  intent.value = undefined;
  previewError.value = '';
  days.value = selected.value?.retention_bounds?.suggested_days;
  if (!tenants.value.length && !tenantLoading.value) void loadTenants();
}
async function loadTenants() {
  const generation = ++tenantGeneration;
  previewError.value = '';
  tenantLoading.value = true;
  try {
    const result = await tenantApi.getTenants({ q: tenantQuery.value, status: 'active', page_size: 50 });
    if (generation !== tenantGeneration) return;
    const current = tenants.value.find(item => item.value === tenant.value);
    tenants.value = (result.data?.items ?? []).map(item => ({ value: item.uuid, label: item.name }));
    if (current && !tenants.value.some(item => item.value === current.value)) tenants.value.unshift(current);
  } catch (error) {
    if (generation === tenantGeneration) previewError.value = errorText(error);
  } finally {
    if (generation === tenantGeneration) tenantLoading.value = false;
  }
}

function start() {
  if (!canStart.value || !selected.value?.acceptance_revision) return;
  intent.value = {
    request_uuid: crypto.randomUUID(), tenant_uuid: tenant.value,
    expected_acceptance_revision: selected.value.acceptance_revision, retention_days: days.value!,
  };
  void submit();
}
async function submit() {
  if (!selected.value || !intent.value || posting.value) return;
  const request = new AbortController();
  postRequest = request;
  previewError.value = '';
  posting.value = true;
  try {
    const result = await api.createPreview(selected.value.dataset_uuid, intent.value, request.signal);
    if (!request.signal.aborted && !receipt.value) receipt.value = result;
  } catch (error) {
    if (!request.signal.aborted && !receipt.value) previewError.value = errorText(error);
  } finally {
    if (!request.signal.aborted) posting.value = false;
  }
}

async function refreshPreview() {
  if (!selected.value || !intent.value) return;
  refreshRequest?.abort();
  const request = new AbortController();
  refreshRequest = request;
  try {
    const result = await api.preview(selected.value.dataset_uuid, intent.value.request_uuid, request.signal);
    if (!request.signal.aborted) {
      receipt.value = result;
      previewError.value = '';
    }
  } catch (error) {
    if (!request.signal.aborted && !posting.value) previewError.value = errorText(error);
  }
}

onMounted(load);
onBeforeUnmount(() => {
  listRequest?.abort();
  refreshRequest?.abort();
  postRequest?.abort();
  tenantGeneration++;
});
</script>
