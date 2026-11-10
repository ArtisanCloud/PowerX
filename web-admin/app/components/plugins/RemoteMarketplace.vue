<template>
  <div class="min-w-0 space-y-6" :aria-busy="loading">
    <PluginPageHeader :title="t('plugins.presentation.market')" :description="t('plugins.presentation.marketHelp')" icon="i-heroicons-building-storefront">
      <template v-if="catalog" #meta>
        <span class="inline-flex items-center gap-1.5"><UIcon name="i-heroicons-computer-desktop" class="size-3.5" />{{ catalog.connection.os }}/{{ catalog.connection.arch }}</span>
        <span class="text-[var(--border-color)]">·</span><span>PowerX {{ catalog.connection.core_version || '—' }}</span>
      </template>
      <template #actions><UButton icon="i-heroicons-arrow-path" color="neutral" variant="outline" :loading="loading" :disabled="installing" @click="loadCatalog">{{ t('plugins.presentation.refresh') }}</UButton></template>
    </PluginPageHeader>

    <UAlert v-if="error" color="error" icon="i-heroicons-exclamation-circle" :title="t('plugins.remoteMarket.loadFailed')" :description="error" />
    <UAlert v-if="catalog && !catalog.connection.configured" color="warning" variant="soft" icon="i-heroicons-link" :title="t('plugins.remoteMarket.notConnected')" :description="t('plugins.remoteMarket.connectHelp')" />
    <UAlert v-else-if="catalog && !catalog.connection.download_ready" :color="registrationNotice.color" variant="soft" icon="i-heroicons-link" :title="registrationNotice.title" :description="registrationNotice.description" data-testid="marketplace-registration-notice" />
    <UAlert v-if="catalog && !catalog.inventory_ready && catalog.connection.configured" color="warning" variant="soft" :title="t('plugins.remoteMarket.installedFailed')" :description="t('plugins.sources.inventoryHelp')" />
    <UAlert v-if="unconfirmedResults > 0" data-testid="marketplace-result-unconfirmed" color="warning" variant="subtle" icon="i-heroicons-exclamation-triangle" :title="t('plugins.remoteMarket.resultUnconfirmed')" :description="t('plugins.remoteMarket.resultUnconfirmedHelp', { count: unconfirmedResults })" />
    <UAlert v-if="success" color="success" icon="i-heroicons-check-circle" :title="success">
      <template #actions>
        <UButton to="/plugins/installed" size="sm" color="success" variant="outline">{{ t('plugins.remoteMarket.manageInstalled') }}</UButton>
      </template>
    </UAlert>

    <div v-if="loading && !catalog" class="grid grid-cols-1 gap-4 xl:grid-cols-2">
      <USkeleton v-for="n in 3" :key="n" class="h-48 rounded-lg" />
    </div>
    <PluginViewLayout v-if="catalog?.connection.configured" :model-value="installationFilter" :items="installationFilters" :aria-label="t('plugins.sources.installationFilter')" :title="activeFilterLabel" :description="t('plugins.remoteMarket.count', { count: catalog.total })" :disabled="loading || installing" @update:model-value="changeFilter">
      <template #tools><span class="text-xs tabular-nums text-[var(--text-secondary)]">{{ t('plugins.remoteMarket.page', { page: catalog.page }) }}</span></template>
      <div class="grid grid-cols-1 gap-4 xl:grid-cols-2">
        <article v-for="plugin in catalog.items" :key="plugin.id" class="flex min-w-0 flex-col overflow-hidden rounded-xl border border-[var(--border-color)] bg-[#ffffff] transition-shadow hover:shadow-md dark:bg-[#172337]">
          <div class="flex-1 p-5">
            <div class="flex items-start gap-3">
              <div class="flex size-12 shrink-0 items-center justify-center rounded-xl bg-primary-50 text-primary-700 dark:bg-primary-400/10 dark:text-primary-200"><UIcon name="i-heroicons-puzzle-piece" class="size-7" /></div>
              <div class="min-w-0 flex-1">
                <h3 class="break-words font-semibold leading-6 text-[var(--text-primary)]">{{ plugin.name }}</h3>
                <p class="mt-1 break-all font-mono text-xs leading-5 text-[var(--text-secondary)]">{{ plugin.id }}</p>
              </div>
            </div>
            <p class="mt-4 line-clamp-2 min-h-10 text-sm leading-5 text-[var(--text-secondary)]">{{ plugin.summary || t('plugins.remoteMarket.noSummary') }}</p>
            <div v-if="plugin.categories.length" class="mt-3 flex flex-wrap gap-1.5"><UBadge v-for="tag in plugin.categories" :key="tag" color="neutral" variant="subtle" size="xs">{{ tag }}</UBadge></div>
          </div>
          <div class="flex flex-wrap items-center justify-between gap-3 border-t border-[var(--border-color)] bg-[#f8fafc] px-5 py-3.5 dark:bg-[#131e30]">
            <span class="inline-flex items-center gap-1.5 text-xs" :class="plugin.installation.status === 'source_conflict' ? 'text-amber-700 dark:text-amber-300' : 'text-[var(--text-secondary)]'">
              <span class="size-1.5 shrink-0 rounded-full" :class="plugin.installation.status === 'installed' ? 'bg-primary-500' : plugin.installation.status === 'source_conflict' ? 'bg-amber-500' : 'bg-slate-400'" />{{ installationLabel(plugin) }}
            </span>
            <UButton size="sm" color="neutral" variant="outline" trailing-icon="i-heroicons-chevron-right" :disabled="installing" @click="openVersions(plugin)">{{ t('plugins.remoteMarket.viewVersions') }}</UButton>
          </div>
        </article>
      </div>
      <div v-if="!catalog.items.length && !error" class="flex min-h-56 flex-col items-center justify-center rounded-xl border border-dashed border-[var(--border-color)] px-6 py-10 text-center">
        <UIcon name="i-heroicons-magnifying-glass" class="mb-3 size-8 text-[var(--text-secondary)]" />
        <p class="font-medium text-[var(--text-primary)]">{{ t('plugins.presentation.emptyFiltered') }}</p>
        <p class="mt-2 max-w-sm text-sm leading-6 text-[var(--text-secondary)]">{{ t('plugins.presentation.marketEmptyHelp') }}</p>
      </div>
      <UPagination v-if="catalog.total > 12" :page="page" :total="catalog.total" :items-per-page="12" :disabled="loading || installing" class="mt-5 justify-center" @update:page="changePage" />
    </PluginViewLayout>

    <UModal v-model:open="detailOpen" :title="selectedPlugin?.name || t('plugins.remoteMarket.viewVersions')" :description="t('plugins.remoteMarket.versionHelp')" :dismissible="!installing" :close="!installing" :ui="{ content: 'sm:max-w-xl' }">
      <template #body>
        <div class="space-y-4">
          <p class="break-all text-xs text-[var(--text-secondary)]">{{ selectedPlugin?.id }}</p>
          <UAlert v-if="catalog && !catalog.connection.download_ready" :color="registrationNotice.color" variant="soft" :title="registrationNotice.title" :description="registrationNotice.description" />
          <UAlert v-if="catalog && !catalog.inventory_ready" color="warning" variant="soft" :title="t('plugins.remoteMarket.installedFailed')" :description="t('plugins.sources.inventoryHelp')" />
          <UAlert v-if="selectedPlugin?.installation.status === 'source_conflict'" color="warning" variant="soft" :title="t('plugins.installFlow.identityBlocked')" :description="t(selectedPlugin.installation.reason === 'provider_mismatch' ? 'plugins.installFlow.providerMismatch' : 'plugins.installFlow.providerUnknown')" />
          <div v-if="isReplacement" class="rounded-lg border border-[var(--border-color)] p-4 text-sm">
            <p class="font-medium">{{ t('plugins.installFlow.currentInstallation') }}</p>
            <p class="mt-1 text-[var(--text-secondary)]">{{ selectedPlugin?.installation.version }} · {{ t(`plugins.presentation.origin.${selectedPlugin?.installation.origin.type}`) }}</p>
          </div>
          <p v-if="selectedPlugin?.installation.provider?.id" class="break-all text-xs text-[var(--text-secondary)]">{{ t('plugins.installFlow.provider') }}: {{ selectedPlugin.installation.provider.id }}<span class="mt-1 block">{{ selectedPlugin.installation.provider.authority }}</span></p>
          <UAlert v-if="versionError" color="error" :title="t('plugins.remoteMarket.versionsFailed')" :description="versionError">
            <template #actions><UButton size="sm" color="error" variant="outline" @click="loadVersions">{{ t('plugins.remoteMarket.retry') }}</UButton></template>
          </UAlert>
          <USkeleton v-if="versionsLoading" class="h-32 w-full" />
          <template v-else-if="versions?.items.length">
            <UFormField :label="t('plugins.remoteMarket.version')">
              <USelect v-model="selectedSemver" :items="versions.items.map(v => ({ label: `${v.semver} · ${v.channel || 'stable'}`, value: v.semver }))" class="w-full" :disabled="installing" />
            </UFormField>
            <dl v-if="selectedVersion" class="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-3 rounded-lg border border-[var(--border-color)] p-4 text-sm">
              <dt class="text-[var(--text-secondary)]">{{ t('plugins.remoteMarket.platform') }}</dt><dd>{{ selectedVersion.os }}/{{ selectedVersion.arch }}</dd>
              <dt class="text-[var(--text-secondary)]">{{ t('plugins.remoteMarket.size') }}</dt><dd>{{ (selectedVersion.size / 1048576).toFixed(2) }} MiB</dd>
              <dt class="text-[var(--text-secondary)]">{{ t('plugins.remoteMarket.minCore') }}</dt><dd>{{ selectedVersion.min_core || '—' }}</dd>
              <dt class="text-[var(--text-secondary)]">SHA256</dt><dd class="break-all font-mono text-xs">{{ selectedVersion.sha256 }}</dd>
            </dl>
            <UPagination v-if="versions.total > 20" :page="versionPage" :total="versions.total" :items-per-page="20" :disabled="installing" @update:page="changeVersionPage" />
            <UAlert v-if="installError" color="error" :title="t('plugins.remoteMarket.installFailed')" :description="installError">
              <template v-if="installRequiresDrain" #actions><UButton :to="`/plugins/${encodeURIComponent(selectedPlugin?.id || '')}`" color="error" variant="outline">{{ t('plugins.installDialog.drain.viewDetail') }}</UButton></template>
            </UAlert>
            <div class="space-y-3 border-t border-[var(--border-color)] pt-4">
              <UCheckbox v-model="enableAfterInstall" :disabled="installing" :label="t('plugins.installFlow.enableAfterInstall')" />
              <UCheckbox v-if="needsProviderBinding && isReplacement" v-model="providerBindingConfirmed" :disabled="installing" :label="t('plugins.installFlow.providerBindingConfirmation')" />
              <UCheckbox v-if="isReplacement" v-model="replacementConfirmed" :disabled="installing" :label="t('plugins.installFlow.replaceConfirmation')" />
            </div>
            <p class="text-sm text-[var(--text-secondary)]">{{ installing ? t('plugins.remoteMarket.installing') : t('plugins.remoteMarket.installHelp') }}</p>
          </template>
          <p v-else-if="!versionError" class="text-sm text-[var(--text-secondary)]">{{ t('plugins.remoteMarket.noVersions') }}</p>
        </div>
      </template>
      <template #footer>
        <div class="flex w-full flex-wrap justify-end gap-2">
          <UButton color="neutral" variant="ghost" :disabled="installing" @click="detailOpen = false">{{ t('plugins.remoteMarket.close') }}</UButton>
          <UButton v-if="selectedPlugin && selectedPlugin.installation.status === 'source_conflict' && !selectedPlugin.installation.can_replace" to="/plugins/installed">{{ t('plugins.remoteMarket.manageInstalled') }}</UButton>
          <UButton v-else icon="i-heroicons-arrow-down-tray" :loading="installing" :disabled="!selectedVersion || versionsLoading || !!versionError || (!['not_installed', 'installed'].includes(selectedPlugin?.installation.status || '') && !isReplacement) || (isReplacement && (!replacementConfirmed || !selectedPlugin?.installation.can_replace || (needsProviderBinding && !providerBindingConfirmed))) || !catalog?.inventory_ready || !catalog?.connection.download_ready" @click="installSelected">{{ t(isReplacement ? 'plugins.installFlow.replaceInstall' : 'plugins.remoteMarket.installVersion') }}</UButton>
        </div>
      </template>
    </UModal>
  </div>
</template>

<script setup lang="ts">
import PluginPageHeader from './PluginPageHeader.vue';
import PluginViewLayout from './PluginViewLayout.vue';
import { useMarketplaceService, type MarketCatalog, type MarketPage, type MarketPlugin, type MarketVersion } from '~/composables/api/services/marketplaceService';

const { t } = useI18n();
const api = useMarketplaceService();
const catalog = ref<MarketCatalog>();
const registrationNotice = computed(() => {
  const instance = catalog.value?.connection.instance;
  const key = instance?.state === 'credential_rejected' ? 'registrationRejected'
    : instance?.state === 'unavailable' ? 'registrationUnavailable'
    : instance?.error_code === 'MARKETPLACE_RECONCILING_INSTALLS' ? 'registrationReconciling' : 'auth';
  return { color: instance?.state === 'credential_rejected' ? 'error' as const : 'warning' as const,
    title: t(`plugins.remoteMarket.${key === 'auth' ? 'authRequired' : key}`),
    description: t(`plugins.remoteMarket.${key}Help`) };
});
const unconfirmedResults = computed(() => catalog.value?.connection.instance?.unconfirmed_installs ?? 0);
const installationFilter = ref('all');
const installationFilters = computed(() => [
  { value: 'all', label: t('plugins.presentation.all'), icon: 'i-heroicons-squares-2x2' },
  { value: 'not_installed', label: t('plugins.sources.filter.not_installed'), icon: 'i-heroicons-arrow-down-tray' },
  { value: 'installed', label: t('plugins.installFlow.installedFilter'), icon: 'i-heroicons-check-circle' },
  { value: 'source_conflict', label: t('plugins.installFlow.identityFilter'), icon: 'i-heroicons-exclamation-triangle' },
]);
const activeFilterLabel = computed(() => installationFilter.value === 'all' ? t('plugins.presentation.allMarket') : installationFilters.value.find(item => item.value === installationFilter.value)?.label || '');
function installationLabel(plugin: MarketPlugin) {
  if (plugin.installation.status === 'installed') return t('plugins.remoteMarket.installedVersion', { version: plugin.installation.version });
  if (plugin.installation.status === 'source_conflict') return t(plugin.installation.reason === 'provider_mismatch' ? 'plugins.installFlow.providerMismatchShort' : 'plugins.installFlow.providerUnknownShort');
  if (plugin.installation.status === 'unknown') return t('plugins.remoteMarket.installedUnknown');
  return t('plugins.remoteMarket.notInstalled');
}
async function changeFilter(value: string) { installationFilter.value = value; page.value = 1; await loadCatalog(); }
const page = ref(1);
const loading = ref(false);
const error = ref('');
const success = ref('');
const detailOpen = ref(false);
const selectedPlugin = ref<MarketPlugin>();
const versions = ref<MarketPage<MarketVersion>>();
const versionPage = ref(1);
const selectedSemver = ref('');
const selectedVersion = computed(() => versions.value?.items.find(v => v.semver === selectedSemver.value));
const versionsLoading = ref(false);
const versionError = ref('');
const installing = ref(false);
const installError = ref('');
const installRequiresDrain = ref(false);
const enableAfterInstall = ref(true);
const replacementConfirmed = ref(false);
const isReplacement = computed(() => selectedPlugin.value?.installation.status === 'installed' || (selectedPlugin.value?.installation.status === 'source_conflict' && selectedPlugin.value.installation.can_replace));
const needsProviderBinding = computed(() => selectedPlugin.value?.installation.reason === 'provider_unconfirmed');
const providerBindingConfirmed = ref(false);
let catalogSequence = 0;
let versionSequence = 0;
let disposed = false;
const message = (e: unknown) => e instanceof Error ? e.message : t('plugins.remoteMarket.unknownError');

async function loadCatalog() {
  const sequence = ++catalogSequence;
  loading.value = true;
  error.value = '';
  try {
    const result = await api.catalog(page.value, installationFilter.value);
    if (!disposed && sequence === catalogSequence) catalog.value = result;
  } catch (e) { if (!disposed && sequence === catalogSequence) error.value = message(e); }
  finally { if (!disposed && sequence === catalogSequence) loading.value = false; }
}
async function changePage(value: number) { page.value = value; await loadCatalog(); }
async function openVersions(plugin: MarketPlugin) {
  selectedPlugin.value = plugin;
  versionPage.value = 1;
  detailOpen.value = true;
  installError.value = '';
  installRequiresDrain.value = false;
  replacementConfirmed.value = false;
  providerBindingConfirmed.value = false;
  await loadVersions();
}
async function changeVersionPage(value: number) { versionPage.value = value; await loadVersions(); }
async function loadVersions() {
  if (!selectedPlugin.value) return;
  const sequence = ++versionSequence;
  versionsLoading.value = true;
  versionError.value = '';
  versions.value = undefined;
  selectedSemver.value = '';
  try {
    const result = await api.versions(selectedPlugin.value.id, versionPage.value);
    if (disposed || sequence !== versionSequence) return;
    versions.value = result;
    selectedSemver.value = result.items[0]?.semver || '';
  } catch (e) { if (!disposed && sequence === versionSequence) versionError.value = message(e); }
  finally { if (!disposed && sequence === versionSequence) versionsLoading.value = false; }
}
async function installSelected() {
  if (!selectedPlugin.value || !selectedVersion.value || installing.value) return;
  installing.value = true;
  installError.value = '';
  installRequiresDrain.value = false;
  success.value = '';
  try {
    if (isReplacement.value && (!replacementConfirmed.value || !selectedPlugin.value.installation.can_replace || (needsProviderBinding.value && !providerBindingConfirmed.value))) return;
    const options = { enable: enableAfterInstall.value, ...(isReplacement.value ? { replace: true, expected_revision: selectedPlugin.value.installation.revision, ...(needsProviderBinding.value ? { confirm_legacy_provider: providerBindingConfirmed.value } : {}) } : {}) };
    const result = await api.install(selectedPlugin.value.id, selectedVersion.value.semver, options);
    if (disposed) return;
    selectedPlugin.value.installation = { status: 'installed', version: result.installed.version, origin: result.installed.install_origin, provider: result.installed.provider, revision: result.installed.revision, can_replace: true };
    success.value = t(result.enabled ? 'plugins.installFlow.installedEnabled' : 'plugins.remoteMarket.installed', { name: selectedPlugin.value.name, version: result.installed.version });
    detailOpen.value = false;
    await loadCatalog();
  } catch (e) {
    if (!disposed) {
      const details = e && typeof e === 'object' && 'details' in e ? e.details as { code?: string } : undefined;
      installRequiresDrain.value = details?.code === 'PLUGIN_DRAIN_REQUIRED';
      installError.value = installRequiresDrain.value ? t('plugins.installDialog.drain.noticeRequired') : message(e);
    }
  }
  finally { if (!disposed) installing.value = false; }
}
defineExpose({ refresh: loadCatalog });
onMounted(loadCatalog);
onBeforeUnmount(() => { disposed = true; ++catalogSequence; ++versionSequence; });
</script>
