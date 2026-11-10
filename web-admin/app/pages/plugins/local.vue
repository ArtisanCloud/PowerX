<template>
  <div v-if="isRoot" class="min-w-0 space-y-6 p-4 sm:p-6">
    <PluginSourceNav />
    <PluginPageHeader :title="t('plugins.sources.local')" :description="t('plugins.presentation.localHelp')" icon="i-heroicons-computer-desktop">
      <template #actions>
        <UButton icon="i-heroicons-plus" @click="installOpen = true">{{ t('plugins.sources.installLocal') }}</UButton>
        <UButton icon="i-heroicons-arrow-path" color="neutral" variant="outline" :loading="loading" @click="load">{{ t('plugins.presentation.refresh') }}</UButton>
      </template>
    </PluginPageHeader>
    <UAlert v-if="error" color="error" :title="t('plugins.remoteMarket.installedFailed')" :description="error" />
    <PluginViewLayout v-model="group" :items="groups" :aria-label="t('plugins.sources.localGroups')" :title="activeGroupLabel" :description="t('plugins.presentation.results', { count: visible.length })">
      <template #tools><UInput v-model="query" :placeholder="t('plugins.sources.search')" icon="i-heroicons-magnifying-glass" class="w-full sm:w-64" /></template>
      <div v-if="group === 'unknown'" class="mb-4 flex items-start gap-2.5 rounded-lg bg-amber-50 px-4 py-3 text-sm leading-6 text-amber-800 dark:bg-amber-400/10 dark:text-amber-200">
        <UIcon name="i-heroicons-information-circle" class="mt-1 size-4 shrink-0" /><p>{{ t('plugins.sources.unknownHelp') }}</p>
      </div>
      <USkeleton v-if="loading && !plugins.length" class="h-48 w-full" />
      <div v-else-if="!visible.length" class="flex min-h-56 flex-col items-center justify-center rounded-xl border border-dashed border-[var(--border-color)] px-6 py-10 text-center">
        <UIcon name="i-heroicons-archive-box" class="mb-3 size-8 text-[var(--text-secondary)]" />
        <h3 class="font-medium text-[var(--text-primary)]">{{ t('plugins.presentation.emptyFiltered') }}</h3>
        <p class="mt-2 max-w-sm text-sm leading-6 text-[var(--text-secondary)]">{{ t('plugins.presentation.localEmptyHelp') }}</p>
      </div>
      <div v-else class="grid grid-cols-1 gap-4 xl:grid-cols-2">
        <article v-for="plugin in visible" :key="plugin.id" class="min-w-0 overflow-hidden rounded-xl border border-[var(--border-color)] bg-[#ffffff] transition-shadow hover:shadow-md dark:bg-[#172337]">
          <div class="p-5">
            <div class="flex items-start gap-3">
              <div class="flex size-12 shrink-0 items-center justify-center rounded-xl bg-primary-50 text-primary-700 dark:bg-primary-400/10 dark:text-primary-200"><UIcon name="i-heroicons-puzzle-piece" class="size-7" /></div>
              <div class="min-w-0">
                <h3 class="break-words font-semibold leading-6 text-[var(--text-primary)]">{{ plugin.name || plugin.id }}</h3>
                <p class="mt-1 break-all font-mono text-xs leading-5 text-[var(--text-secondary)]">{{ plugin.id }}</p>
              </div>
            </div>
            <div class="mt-5 flex flex-wrap items-center gap-2"><PluginOriginBadge :origin="plugin.install_origin" /><UBadge color="neutral" variant="subtle" class="whitespace-nowrap">{{ t('plugins.presentation.version') }} {{ plugin.version }}</UBadge></div>
          </div>
          <div class="flex flex-wrap items-center justify-between gap-3 border-t border-[var(--border-color)] bg-[#f8fafc] px-5 py-3.5 dark:bg-[#131e30]">
            <span class="inline-flex items-center gap-1.5 text-xs text-[var(--text-secondary)]"><span class="size-1.5 rounded-full" :class="plugin.state === 'enabled' ? 'bg-primary-500' : 'bg-slate-400'" />{{ t(plugin.state === 'enabled' ? 'plugins.presentation.enabled' : 'plugins.presentation.notEnabled') }}</span>
            <UButton :to="`/plugins/${plugin.id}`" size="sm" color="neutral" variant="outline" trailing-icon="i-heroicons-chevron-right">{{ t('plugins.sources.manage') }}</UButton>
          </div>
        </article>
      </div>
    </PluginViewLayout>
    <InstallDialog v-model="installOpen" @installed="onInstalled" />
  </div>
  <UAlert v-else color="warning" :title="t('plugins.sources.rootOnly')" />
</template>

<script setup lang="ts">
import InstallDialog from '~/components/plugins/InstallDialog.vue';
import PluginPageHeader from '~/components/plugins/PluginPageHeader.vue';
import PluginViewLayout from '~/components/plugins/PluginViewLayout.vue';
import PluginOriginBadge from '~/components/plugins/PluginOriginBadge.vue';
import PluginSourceNav from '~/components/plugins/PluginSourceNav.vue';
import { useAdminPluginsService, type AdminPluginItem } from '~/composables/api/services/adminPluginsService';
import { useUserStore } from '~/stores/user';

definePageMeta({ layout: 'default', title: 'plugins.sources.local' });
defineOptions({ name: 'LocalPluginsPage' });
const { t } = useI18n();
const userStore = useUserStore();
const isRoot = computed(() => userStore.isRoot);
const api = useAdminPluginsService();
const plugins = ref<AdminPluginItem[]>([]);
const group = ref('all');
const query = ref('');
const loading = ref(false);
const error = ref('');
const installOpen = ref(false);
const source = (p: AdminPluginItem) => p.install_origin?.type || 'unknown';
const eligible = computed(() => plugins.value.filter(p => source(p) !== 'marketplace'));
const count = (value: string) => eligible.value.filter(p => value === 'all' || source(p) === value).length;
const groups = computed(() => [
  { value: 'all', label: t('plugins.presentation.all'), icon: 'i-heroicons-squares-2x2', count: count('all') },
  { value: 'local', label: t('plugins.presentation.origin.local'), icon: 'i-heroicons-computer-desktop', count: count('local') },
  { value: 'unknown', label: t('plugins.sources.unknown'), icon: 'i-heroicons-question-mark-circle', count: count('unknown') },
]);
const activeGroupLabel = computed(() => group.value === 'all' ? t('plugins.presentation.allLocal') : groups.value.find(item => item.value === group.value)?.label || '');
const visible = computed(() => eligible.value.filter(p => (group.value === 'all' || source(p) === group.value) && `${p.id} ${p.name || ''}`.toLowerCase().includes(query.value.trim().toLowerCase())));
let disposed = false;
async function load() {
  if (!isRoot.value) return;
  loading.value = true; error.value = '';
  try { const result = await api.list(); if (!disposed) plugins.value = result; }
  catch (e) { if (!disposed) error.value = e instanceof Error ? e.message : t('plugins.remoteMarket.unknownError'); }
  finally { if (!disposed) loading.value = false; }
}
async function onInstalled() { installOpen.value = false; await load(); }
onMounted(load);
onBeforeUnmount(() => { disposed = true; });
</script>
