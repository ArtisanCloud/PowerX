<template>
  <div class="min-w-0 space-y-6 p-4 sm:p-6">
    <PluginSourceNav v-if="isRoot" />
    <PluginPageHeader :title="t(isRoot ? 'plugins.presentation.installedTitle' : 'plugins.presentation.subscriptions')" :description="t(isRoot ? 'plugins.presentation.installedHelp' : 'plugins.presentation.subscriptionsHelp')" icon="i-heroicons-squares-2x2">
      <template #actions>
        <UButton v-if="isRoot" to="/plugins/local" icon="i-heroicons-plus">{{ t('plugins.sources.installLocal') }}</UButton>
        <UButton icon="i-heroicons-arrow-path" color="neutral" variant="outline" :loading="loading" @click="load">{{ t('plugins.presentation.refresh') }}</UButton>
      </template>
    </PluginPageHeader>
    <UAlert v-if="error" color="error" :title="t('plugins.presentation.operationFailed')" :description="error" />
    <PluginViewLayout v-model="sourceFilter" :items="sourceOptions" :aria-label="t('plugins.sources.sourceFilter')" :title="activeSourceLabel" :description="t('plugins.presentation.results', { count: filtered.length })">
      <template #tools><UInput v-model="query" :placeholder="t('plugins.sources.search')" icon="i-heroicons-magnifying-glass" class="w-full sm:w-64" /></template>
      <USkeleton v-if="loading && !rows.length" class="h-48 w-full" />
      <div v-else-if="filtered.length" class="installed-results">
        <div class="installed-table overflow-hidden rounded-lg border border-[var(--border-color)]">
          <table class="w-full table-fixed text-left text-sm">
            <colgroup v-if="isRoot"><col class="w-[31%]"><col class="w-[9%]"><col class="w-[16%]"><col class="w-[10%]"><col class="w-[12%]"><col class="w-[22%]"></colgroup>
            <caption class="sr-only">{{ t('plugins.presentation.installedTitle') }}</caption>
            <thead class="bg-[#f5f7fa] text-xs text-[var(--text-secondary)] dark:bg-[#0e1727]">
              <tr class="border-b border-[var(--border-color)]">
                <th class="px-4 py-3 font-medium">{{ t('plugins.presentation.plugin') }}</th>
                <th class="whitespace-nowrap px-4 py-3 font-medium">{{ t('plugins.presentation.version') }}</th>
                <th v-if="isRoot" class="whitespace-nowrap px-4 py-3 font-medium">{{ t('plugins.sources.source') }}</th>
                <th v-if="isRoot" class="whitespace-nowrap px-4 py-3 font-medium">{{ t('plugins.presentation.systemState') }}</th>
                <th class="whitespace-nowrap px-4 py-3 font-medium">{{ t('plugins.presentation.tenantState') }}</th>
                <th class="whitespace-nowrap px-4 py-3 font-medium">{{ t('plugins.presentation.actions') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-[var(--border-color)]">
              <tr v-for="it in filtered" :key="it.id" class="bg-[#ffffff] dark:bg-[#172337]">
                <td class="px-4 py-5">
                  <NuxtLink :to="`/plugins/${it.id}`" class="font-medium leading-6 text-[var(--text-primary)] hover:text-primary-500">{{ it.name }}</NuxtLink>
                  <p class="mt-1 truncate font-mono text-xs text-[var(--text-secondary)]" :title="it.id">{{ it.id }}</p>
                </td>
                <td class="whitespace-nowrap px-4 py-5 font-mono text-xs text-[var(--text-secondary)]">{{ it.version }}</td>
                <td v-if="isRoot" class="px-4 py-5"><PluginOriginBadge :origin="it.installOrigin" /><p v-if="it.installOrigin?.source_id && it.installOrigin.type === 'marketplace'" class="mt-2 max-w-40 truncate text-xs text-[var(--text-secondary)]" :title="it.installOrigin.source_id">{{ it.installOrigin.source_id }}</p></td>
                <td v-if="isRoot" class="px-4 py-5"><UBadge :color="it.isSystemEnabled ? 'success' : 'neutral'" variant="subtle" class="whitespace-nowrap">{{ t(it.isSystemEnabled ? 'plugins.presentation.enabled' : 'plugins.presentation.notEnabled') }}</UBadge></td>
                <td class="px-4 py-5"><UBadge :color="it.tenantEnabled ? 'success' : 'neutral'" variant="subtle" class="whitespace-nowrap">{{ tenantLabel(it) }}</UBadge><p v-if="it.clientId" :title="`${t('plugins.presentation.clientId')}: ${it.clientId}`" class="mt-2 max-w-36 truncate font-mono text-xs text-[var(--text-secondary)]">{{ it.clientId }}</p></td>
                <td class="px-4 py-5"><PluginRuntimeActions :plugin-id="it.id" :enabled="isRoot ? it.isSystemEnabled : !!it.tenantEnabled" :mode="actionMode(it)" :busy="busyId === it.id" @toggle="toggle(it)" /></td>
              </tr>
            </tbody>
          </table>
        </div>
        <div class="installed-compact">
          <article v-for="it in filtered" :key="it.id" class="min-w-0 rounded-xl border border-[var(--border-color)] bg-[#ffffff] p-4 dark:bg-[#172337]">
            <h3 class="break-words font-semibold text-[var(--text-primary)]">{{ it.name }}</h3>
            <p class="mt-1 break-all font-mono text-xs leading-5 text-[var(--text-secondary)]">{{ it.id }}</p>
            <div class="mt-4 flex flex-wrap items-center gap-2"><PluginOriginBadge v-if="isRoot" :origin="it.installOrigin" /><UBadge color="neutral" variant="subtle">{{ t('plugins.presentation.version') }} {{ it.version }}</UBadge><UBadge v-if="isRoot" :color="it.isSystemEnabled ? 'success' : 'neutral'" variant="subtle">{{ t(it.isSystemEnabled ? 'plugins.presentation.enabled' : 'plugins.presentation.notEnabled') }}</UBadge></div>
            <div class="mt-4 flex flex-wrap items-center justify-between gap-3 border-t border-[var(--border-color)] pt-4"><span class="text-xs text-[var(--text-secondary)]">{{ tenantLabel(it) }}</span><PluginRuntimeActions :plugin-id="it.id" :enabled="isRoot ? it.isSystemEnabled : !!it.tenantEnabled" :mode="actionMode(it)" :busy="busyId === it.id" @toggle="toggle(it)" /></div>
          </article>
        </div>
      </div>
      <div v-else-if="!error" class="flex min-h-56 flex-col items-center justify-center rounded-xl border border-dashed border-[var(--border-color)] px-6 py-10 text-center">
        <UIcon name="i-heroicons-archive-box" class="mb-3 size-8 text-[var(--text-secondary)]" /><h3 class="font-medium text-[var(--text-primary)]">{{ t('plugins.presentation.emptyFiltered') }}</h3><p class="mt-2 text-sm text-[var(--text-secondary)]">{{ t('plugins.presentation.installedEmptyHelp') }}</p>
      </div>
    </PluginViewLayout>
  </div>
</template>

<script setup lang="ts">
import { useUserStore } from '~/stores/user';
import { useAdminPluginsService, type PluginInstallOrigin } from '~/composables/api/services/adminPluginsService';
import PluginSourceNav from '~/components/plugins/PluginSourceNav.vue';
import PluginPageHeader from '~/components/plugins/PluginPageHeader.vue';
import PluginViewLayout from '~/components/plugins/PluginViewLayout.vue';
import PluginOriginBadge from '~/components/plugins/PluginOriginBadge.vue';
import PluginRuntimeActions from '~/components/plugins/PluginRuntimeActions.vue';

definePageMeta({ layout: 'default', title: 'plugins.sources.installed' });
defineOptions({ name: 'InstalledPluginsPage' });
interface Row {
  id: string; name: string; version: string; description: string;
  installOrigin?: PluginInstallOrigin;
  isSystemEnabled: boolean; tenantEnabled?: boolean; clientId: string;
}
const { t } = useI18n();
const userStore = useUserStore();
const isRoot = computed(() => userStore.isRoot);
const api = useAdminPluginsService();
const sourceFilter = ref('all');
const query = ref('');
const rows = ref<Row[]>([]);
const loading = ref(false);
const busyId = ref('');
const error = ref('');
const menuRefreshToken = useState<number>('px-menu-refresh-token', () => 0);
const source = (r: Row) => r.installOrigin?.type || 'unknown';
const sourceOptions = computed(() => [
  { value: 'all', label: t('plugins.presentation.all'), icon: 'i-heroicons-squares-2x2', count: rows.value.length },
  ...(isRoot.value ? ['marketplace', 'local', 'unknown'].map(value => ({ value, label: t(`plugins.presentation.origin.${value}`), icon: value === 'marketplace' ? 'i-heroicons-building-storefront' : value === 'local' ? 'i-heroicons-computer-desktop' : 'i-heroicons-question-mark-circle', count: rows.value.filter(r => source(r) === value).length })) : []),
]);
const activeSourceLabel = computed(() => sourceFilter.value === 'all' ? t('plugins.presentation.allInstalled') : sourceOptions.value.find(item => item.value === sourceFilter.value)?.label || '');
const filtered = computed(() => rows.value.filter(r => (!isRoot.value || sourceFilter.value === 'all' || source(r) === sourceFilter.value) && `${r.name} ${r.id} ${r.description}`.toLowerCase().includes(query.value.trim().toLowerCase())));
const tenantLabel = (r: Row) => t(r.tenantEnabled === undefined ? 'plugins.presentation.pending' : r.tenantEnabled ? 'plugins.presentation.subscribed' : 'plugins.presentation.notSubscribed');
const actionMode = (r: Row): 'system' | 'tenant' | 'read' => isRoot.value ? 'system' : userStore.isCurrentTenantAdmin && r.tenantEnabled ? 'tenant' : 'read';
const record = (value: unknown): Record<string, unknown> => value && typeof value === 'object' ? value as Record<string, unknown> : {};
let disposed = false;
let loadSequence = 0;
async function load() {
  const sequence = ++loadSequence;
  loading.value = true; error.value = '';
  try {
    const list = isRoot.value ? await api.list() : await api.getMarketplace();
    const mapped: Row[] = list.map(p => ({
      id: p.id, name: p.menus?.[0]?.title || p.name || p.id, version: p.version || '—', description: p.description || '', installOrigin: p.install_origin,
      isSystemEnabled: isRoot.value ? p.state === 'enabled' : !!p.isSystemEnabled,
      tenantEnabled: isRoot.value ? undefined : !!(p.tenantInstance?.enabled ?? p.tenantEnabled),
      clientId: String(p.tenantInstance?.config?.client_id || p.tenantInstance?.config?.clientId || ''),
    }));
    if (isRoot.value) await Promise.all(mapped.map(async r => {
      try {
        const conf = record(await api.getTenantConfig(r.id));
        r.tenantEnabled = !!(conf.enabled ?? conf.isEnabled);
        r.clientId = String(conf.client_id || conf.clientId || record(conf.config).client_id || '');
      } catch { /* 保留“待确认”，不能把读取失败解释为未订阅。 */ }
    }));
    if (!disposed && sequence === loadSequence) rows.value = isRoot.value ? mapped : mapped.filter(r => r.isSystemEnabled && r.tenantEnabled);
  } catch (e) { if (!disposed && sequence === loadSequence) error.value = e instanceof Error ? e.message : t('plugins.remoteMarket.unknownError'); }
  finally { if (!disposed && sequence === loadSequence) loading.value = false; }
}
async function toggle(r: Row) {
  if (busyId.value) return;
  if (!isRoot.value) {
    const { confirm } = useConfirm();
    if (!await confirm({ title: t('plugins.presentation.unsubscribe'), description: t('plugins.presentation.unsubscribeHelp'), message: t('plugins.presentation.unsubscribeConfirm', { name: r.name }), confirmLabel: t('plugins.presentation.unsubscribe'), tone: 'warning' })) return;
  }
  busyId.value = r.id; error.value = '';
  try {
    if (isRoot.value) { if (r.isSystemEnabled) await api.disable(r.id); else await api.enable(r.id); }
    else await api.setTenantEnabled(r.id, false);
    menuRefreshToken.value += 1;
    await load();
  } catch (e) { error.value = e instanceof Error ? e.message : t('plugins.remoteMarket.unknownError'); }
  finally { busyId.value = ''; }
}
onMounted(load);
onBeforeUnmount(() => { disposed = true; ++loadSequence; });
</script>

<style scoped>
.installed-results { container-type: inline-size; }
.installed-table { display: none; }
.installed-compact { display: grid; gap: 0.75rem; }
@container (min-width: 560px) and (max-width: 799px) {
  .installed-compact { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}
@container (min-width: 800px) {
  .installed-table { display: block; }
  .installed-compact { display: none; }
}
</style>
