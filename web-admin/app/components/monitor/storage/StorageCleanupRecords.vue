<template>
  <div class="space-y-4" data-testid="storage-cleanup-records">
    <div class="flex flex-wrap justify-between gap-3"><h2 class="text-lg font-semibold">{{ t('storageGov.cleanup') }}</h2><UButton color="neutral" variant="outline" :loading="loading" @click="load">{{ t('storageGov.refresh') }}</UButton></div>
    <p class="text-sm text-muted">{{ t('storageGov.logs.recordsHint') }}</p>
    <UAlert v-if="error" color="error" :title="t('storageGov.loadFailed')" :description="error" />
    <p v-if="loading && !records" role="status">{{ t('storageGov.loading') }}</p>
    <p v-else-if="records?.length === 0" class="text-muted">{{ t('storageGov.logs.empty') }}</p>
    <div v-if="records?.length" class="overflow-x-auto"><table class="w-full text-left text-sm"><thead><tr><th class="p-2">{{ t('storageGov.resource') }}</th><th class="p-2">{{ t('storageGov.usageViews.time') }}</th><th class="p-2">{{ t('storageGov.state') }}</th><th class="p-2">{{ t('storageGov.logs.confirmedDeleted') }}</th><th /></tr></thead><tbody><tr v-for="record in records" :key="record.id"><td class="p-2">{{ record.resource.name }}</td><td class="p-2">{{ new Date(record.created_at).toLocaleString(locale) }}</td><td class="p-2">{{ t(`storageGov.logs.states.${record.state}`) }}</td><td class="p-2">{{ ['completed', 'partial'].includes(record.state) ? record.deleted_files : '—' }}</td><td class="p-2"><UButton color="neutral" variant="outline" @click="read(record.id)">{{ t('storageGov.logs.readback') }}</UButton></td></tr></tbody></table></div>
    <UModal v-model:open="open" :title="t('storageGov.logs.recordTitle')" :description="t('storageGov.logs.reclaimedHint')"><template #body><div v-if="selected" class="space-y-3" data-testid="storage-cleanup-detail"><p>{{ selected.resource.name }} · {{ t(`storageGov.logs.states.${selected.state}`) }}</p><p v-if="selected.state === 'unconfirmed'" class="text-warning">{{ t('storageGov.logs.unconfirmed') }}</p><p v-else-if="selected.state !== 'preview'">{{ t('storageGov.logs.result', { count: selected.deleted_files, bytes: formatBytes(selected.removed_file_bytes) }) }}</p><ul class="max-h-72 space-y-2 overflow-auto text-sm"><li v-for="item in selected.items" :key="item.name" class="break-all">{{ item.name }} · {{ t(`storageGov.logs.outcomes.${item.outcome}`) }}</li></ul></div></template></UModal>
  </div>
</template>
<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useStorageGovernanceService, type StorageCleanupRecord } from '~/composables/api/services/storageGovernanceService';
import { formatStorageBytes as formatBytes } from '~/utils/storage-capacity';
const { t, locale } = useI18n(); const api = useStorageGovernanceService();
const props = defineProps<{ revision?: number }>();
const records = ref<StorageCleanupRecord[]>(), selected = ref<StorageCleanupRecord>(); const open = ref(false), loading = ref(false), error = ref(''); let controller: AbortController | undefined;
async function load() { controller?.abort(); const r = controller = new AbortController(); loading.value = true; error.value = ''; try { const value = await api.cleanupRuns(r.signal); if (!r.signal.aborted) records.value = value; } catch (e) { if (!r.signal.aborted) error.value = e instanceof Error ? e.message : t('storageGov.requestFailed'); } finally { if (!r.signal.aborted) loading.value = false; } }
async function read(id: string) { controller?.abort(); loading.value = false; const r = controller = new AbortController(); error.value = ''; try { const value = await api.cleanupRun(id, r.signal); if (!r.signal.aborted) { selected.value = value; open.value = true; } } catch (e) { if (!r.signal.aborted) error.value = e instanceof Error ? e.message : t('storageGov.requestFailed'); } }
onMounted(load); onBeforeUnmount(() => controller?.abort());
watch(() => props.revision, () => void load());
</script>
