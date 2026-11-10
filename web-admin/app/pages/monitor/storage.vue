<template>
  <StorageGovernanceWorkspace :allowed="user.isRoot" :identity-key="identityKey" />
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useUserStore } from '~/stores/user';
import StorageGovernanceWorkspace from '~/components/monitor/storage/StorageGovernanceWorkspace.vue';

definePageMeta({ title: '存储治理', layout: 'default' });
const user = useUserStore();
const identityKey = computed(() => `${user.user?.id || ''}:${user.currentTenantUuid || ''}`);
</script>
