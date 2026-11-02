<template><ArchiveGovernanceBody v-if="active" :key="identity" /></template>
<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue';
import ArchiveGovernanceBody from './ArchiveGovernanceBody.vue';
import { AUTH_SESSION_EVENT, readAuthSession } from '~/utils/auth-session';
const initial = typeof window === 'undefined' ? null : readAuthSession();
const key = (s: ReturnType<typeof readAuthSession>) => `${s?.sessionId || ''}:${s?.tenantUuid || ''}:${s?.status || ''}`;
const identity = ref(key(initial)), active = ref(initial ? initial.status === 'active' : true);
function sync() { const s = readAuthSession(); identity.value = key(s); active.value = s?.status === 'active'; }
onMounted(() => { if (readAuthSession()) sync(); window.addEventListener(AUTH_SESSION_EVENT, sync); window.addEventListener('storage', sync); });
onBeforeUnmount(() => { window.removeEventListener(AUTH_SESSION_EVENT, sync); window.removeEventListener('storage', sync); });
</script>
