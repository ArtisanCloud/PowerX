<template>
  <div class="min-w-0" data-testid="resource-configuration">
    <UAlert v-if="!local" color="warning" :title="t('storageGov.resourceConfig.notConnected')" />
    <StorageRuntimePanel v-else-if="management.owner === 'governance'" :source-id="source.resource.id" :initial="runtime" :revision="revision" @observed="emit('observed', $event)" @busy="emit('busy', { sourceId: source.resource.id, busy: $event })" />
    <ArchiveGovernanceBody v-else-if="management.owner === 'archive'" configuration-only @attention="emit('attention', { sourceId: source.resource.id, pending: $event })" @busy="emit('busy', { sourceId: source.resource.id, busy: $event })" @changed="emit('changed')" />
    <StorageBackupConfiguration v-else-if="management.owner === 'backup' || management.owner === 'snapshot'" :kind="management.owner" :revision="revision" @busy="emit('busy', { sourceId: source.resource.id, busy: $event })" @attention="emit('attention', { sourceId: source.resource.id, pending: $event })" @changed="emit('changed')" />
    <UAlert v-else color="info" :title="t('storageGov.resourceConfig.unsupported')" :description="t('storageGov.resourceConfig.mediaProtected')" />
  </div>
</template>
<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { StoragePolicyOption } from '~/composables/api/services/storageGovernanceService';
import type { StorageRuntime } from '~/composables/api/services/storageRuntimeService';
import { resourceManagement } from '~/utils/storage-resource-management';
import StorageRuntimePanel from './StorageRuntimePanel.vue';
import StorageBackupConfiguration from './StorageBackupConfiguration.vue';
import ArchiveGovernanceBody from '~/components/ops/backup/ArchiveGovernanceBody.vue';
const props = defineProps<{ source: StoragePolicyOption; instanceId: string; runtime?: StorageRuntime; revision?: number }>();
const emit = defineEmits<{ observed: [value: StorageRuntime]; busy: [value: { sourceId: string; busy: boolean }]; attention: [value: { sourceId: string; pending: boolean }]; changed: [] }>();
const { t } = useI18n();
const local = computed(() => props.source.resource.instance_id ? props.source.resource.instance_id === props.instanceId : props.source.resource.scope === 'shared');
const management = computed(() => resourceManagement(props.source, props.runtime));
</script>
