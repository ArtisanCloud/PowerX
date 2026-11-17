<template>
  <HostOverviewWorkspace :allowed="user.isRoot" :identity-key="identityKey" />
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useUserStore } from '~/stores/user';
import HostOverviewWorkspace from '~/components/monitor/host/HostOverviewWorkspace.vue';

definePageMeta({ title: '主机概览', layout: 'default' });
const user = useUserStore();
const identityKey = computed(() => `${user.user?.id || ''}:${user.currentTenantUuid || ''}`);
</script>
