<template>
  <div class="p-6 space-y-5">
    <div class="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
      <div>
        <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">
          {{ t("settings.metadataGovernance.title") }}
        </h1>
        <p class="text-sm text-gray-600 dark:text-gray-400">
          {{ t("settings.metadataGovernance.description") }}
        </p>
      </div>
      <div class="flex flex-wrap gap-2">
        <UButton icon="i-lucide-rotate-cw" :loading="store.loading" @click="refresh">
          {{ t("common.refresh") }}
        </UButton>
      </div>
    </div>

    <UTabs v-model="activeTab" :items="tabItems" class="w-full">
      <template #content="{ item }">
        <div class="pt-4">
          <UCard class="mb-4">
            <div class="grid grid-cols-1 gap-3 lg:grid-cols-[180px_180px_1fr_auto]">
              <USelect v-model="primaryFilterValue" :items="primaryFilterItems" class="w-full" />
              <USelect v-model="filters.status" :items="statusItems" class="w-full" />
              <UInput
                v-model="filters.q"
                icon="i-lucide-search"
                :placeholder="searchPlaceholder"
              />
              <UButton variant="ghost" icon="i-lucide-filter-x" @click="resetFilters">
                {{ t("common.reset") }}
              </UButton>
            </div>
          </UCard>

          <UAlert
            class="mb-4"
            variant="subtle"
            color="neutral"
            :icon="tabIntent(item.value).icon"
            :title="tabIntent(item.value).title"
            :description="tabIntent(item.value).description"
          />

          <MetadataStatePanel
            v-if="isBlockingState(stateFor(item.value))"
            :state="stateFor(item.value)"
            :title="stateTitle(stateFor(item.value))"
            :description="stateDescription(stateFor(item.value))"
          />

          <div v-else-if="item.value === 'dictionaries'" class="grid grid-cols-1 gap-4 xl:grid-cols-[360px_1fr]">
            <UCard>
              <template #header>
                <div class="flex items-start justify-between gap-3">
                  <div>
                    <div class="text-sm font-medium">{{ t("settings.metadataGovernance.dictionary.namespaces") }}</div>
                    <div class="text-xs text-gray-500">{{ t("settings.metadataGovernance.dictionary.namespaceShape") }}</div>
                  </div>
                  <UButton
                    v-if="store.hasManagePermission"
                    size="xs"
                    variant="subtle"
                    icon="i-lucide-plus"
                    @click="openCreate('dictionaryNamespace')"
                  >
                    {{ t("settings.metadataGovernance.create.dictionaryNamespace.button") }}
                  </UButton>
                </div>
              </template>
              <MetadataStatePanel
                v-if="store.dictionaries.length === 0"
                state="empty"
                :title="t('settings.metadataGovernance.empty.title')"
                :description="t('settings.metadataGovernance.empty.description')"
              />
              <div v-else class="space-y-2">
                <button
                  v-for="namespace in store.dictionaries"
                  :key="namespace.uuid"
                  type="button"
                  class="w-full rounded-md border px-3 py-2 text-left text-sm transition"
                  :class="namespace.uuid === store.selectedDictionaryUuid ? 'border-primary-500 bg-primary-50 dark:bg-primary-950/30' : 'border-gray-200 hover:bg-gray-50 dark:border-gray-800 dark:hover:bg-gray-900'"
                  @click="selectDictionary(namespace.uuid)"
                >
                  <div class="font-medium text-gray-900 dark:text-white">{{ namespace.display_name }}</div>
                  <div class="mt-1 flex flex-wrap gap-2 text-xs text-gray-500">
                    <span>{{ namespace.namespace }}</span>
                    <span>{{ moduleLabel(namespace.module) }}</span>
                    <span v-if="namespace.display_locale_missing">{{ t("settings.metadataGovernance.localeMissing") }}</span>
                  </div>
                </button>
              </div>
              <template #footer>
                <PaginationBar
                  v-if="mainPagination.total > 0"
                  compact
                  :pagination="mainPagination"
                  :page-size="mainPager.pageSize"
                  :page-size-items="pageSizeItems"
                  :summary="mainPaginationSummary"
                  @update:page="changeMainPage"
                  @update:page-size="changeMainPageSize"
                />
              </template>
            </UCard>
            <UCard>
              <template #header>
                <div class="flex items-start justify-between gap-3">
                  <div>
                    <div class="text-sm font-medium">{{ t("settings.metadataGovernance.dictionary.items") }}</div>
                    <div class="text-xs text-gray-500">{{ t("settings.metadataGovernance.dictionary.itemShape") }}</div>
                  </div>
                  <UButton
                    v-if="store.hasManagePermission"
                    size="xs"
                    variant="subtle"
                    icon="i-lucide-plus"
                    :disabled="!store.selectedDictionaryUuid"
                    @click="openCreate('dictionaryItem')"
                  >
                    {{ t("settings.metadataGovernance.create.dictionaryItem.button") }}
                  </UButton>
                </div>
              </template>
              <MetadataStatePanel
                v-if="store.dictionaryItems.length === 0"
                state="empty"
                :title="t('settings.metadataGovernance.empty.title')"
                :description="t('settings.metadataGovernance.empty.description')"
              />
              <div v-else class="overflow-x-auto">
                <table class="min-w-full text-sm">
                  <thead class="text-left text-xs uppercase text-gray-500">
                    <tr>
                      <th class="px-3 py-2">{{ t("settings.metadataGovernance.table.name") }}</th>
                      <th class="px-3 py-2">{{ t("settings.metadataGovernance.table.code") }}</th>
                      <th class="px-3 py-2">{{ t("settings.metadataGovernance.table.status") }}</th>
                      <th class="px-3 py-2">{{ t("settings.metadataGovernance.table.references") }}</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="row in store.dictionaryItems" :key="row.uuid" class="border-t border-gray-100 dark:border-gray-800">
                      <td class="px-3 py-2">{{ row.display_name }}</td>
                      <td class="px-3 py-2 text-gray-500">{{ row.code }}</td>
                      <td class="px-3 py-2"><StatusBadge :status="row.status" /></td>
                      <td class="px-3 py-2">{{ row.reference_count }}</td>
                    </tr>
                  </tbody>
                </table>
              </div>
              <template #footer>
                <PaginationBar
                  v-if="detailPagination.total > 0"
                  :pagination="detailPagination"
                  :page-size="detailPager.pageSize"
                  :page-size-items="pageSizeItems"
                  :summary="detailPaginationSummary"
                  @update:page="changeDetailPage"
                  @update:page-size="changeDetailPageSize"
                />
              </template>
            </UCard>
          </div>

          <div v-else-if="item.value === 'taxonomies'" class="grid grid-cols-1 gap-4 xl:grid-cols-[360px_1fr]">
            <UCard>
              <template #header>
                <div class="flex items-start justify-between gap-3">
                  <div>
                    <div class="text-sm font-medium">{{ t("settings.metadataGovernance.taxonomy.list") }}</div>
                    <div class="text-xs text-gray-500">{{ t("settings.metadataGovernance.taxonomy.taxonomyShape") }}</div>
                  </div>
                  <UButton
                    v-if="store.hasManagePermission"
                    size="xs"
                    variant="subtle"
                    icon="i-lucide-plus"
                    @click="openCreate('taxonomy')"
                  >
                    {{ t("settings.metadataGovernance.create.taxonomy.button") }}
                  </UButton>
                </div>
              </template>
              <MetadataStatePanel
                v-if="store.taxonomies.length === 0"
                state="empty"
                :title="t('settings.metadataGovernance.empty.title')"
                :description="t('settings.metadataGovernance.empty.description')"
              />
              <div v-else class="space-y-2">
                <button
                  v-for="taxonomy in store.taxonomies"
                  :key="taxonomy.uuid"
                  type="button"
                  class="w-full rounded-md border px-3 py-2 text-left text-sm transition"
                  :class="taxonomy.uuid === store.selectedTaxonomyUuid ? 'border-primary-500 bg-primary-50 dark:bg-primary-950/30' : 'border-gray-200 hover:bg-gray-50 dark:border-gray-800 dark:hover:bg-gray-900'"
                  @click="selectTaxonomy(taxonomy.uuid)"
                >
                  <div class="font-medium text-gray-900 dark:text-white">{{ taxonomy.display_name }}</div>
                  <div class="mt-1 text-xs text-gray-500">{{ taxonomy.namespace }} · {{ moduleLabel(taxonomy.module) }}</div>
                </button>
              </div>
              <template #footer>
                <PaginationBar
                  v-if="mainPagination.total > 0"
                  compact
                  :pagination="mainPagination"
                  :page-size="mainPager.pageSize"
                  :page-size-items="pageSizeItems"
                  :summary="mainPaginationSummary"
                  @update:page="changeMainPage"
                  @update:page-size="changeMainPageSize"
                />
              </template>
            </UCard>
            <UCard>
              <template #header>
                <div class="flex items-start justify-between gap-3">
                  <div>
                    <div class="text-sm font-medium">{{ t("settings.metadataGovernance.taxonomy.nodes") }}</div>
                    <div class="text-xs text-gray-500">{{ t("settings.metadataGovernance.taxonomy.nodeShape") }}</div>
                  </div>
                  <UButton
                    v-if="store.hasManagePermission"
                    size="xs"
                    variant="subtle"
                    icon="i-lucide-plus"
                    :disabled="!store.selectedTaxonomyUuid"
                    @click="openCreate('taxonomyNode')"
                  >
                    {{ t("settings.metadataGovernance.create.taxonomyNode.button") }}
                  </UButton>
                </div>
              </template>
              <TaxonomyTreeBody
                :rows="store.taxonomyNodes"
                :empty-title="t('settings.metadataGovernance.empty.title')"
                :empty-description="t('settings.metadataGovernance.empty.description')"
              />
              <template #footer>
                <PaginationBar
                  v-if="detailPagination.total > 0"
                  :pagination="detailPagination"
                  :page-size="detailPager.pageSize"
                  :page-size-items="pageSizeItems"
                  :summary="detailPaginationSummary"
                  @update:page="changeDetailPage"
                  @update:page-size="changeDetailPageSize"
                />
              </template>
            </UCard>
          </div>

          <UCard v-else-if="item.value === 'tags'">
            <template #header>
              <div class="flex items-start justify-between gap-3">
                <div>
                  <div class="text-sm font-medium">{{ t("settings.metadataGovernance.tabs.tags") }}</div>
                  <div class="text-xs text-gray-500">{{ t("settings.metadataGovernance.intent.tags.description") }}</div>
                </div>
                <UButton
                  v-if="store.hasManagePermission"
                  size="xs"
                  variant="subtle"
                  icon="i-lucide-plus"
                  @click="openCreate('tag')"
                >
                  {{ t("settings.metadataGovernance.create.tag.button") }}
                </UButton>
              </div>
            </template>
            <SimpleTableBody
              :rows="store.tags"
              code-key="code"
              count-key="usage_count"
              :empty-title="t('settings.metadataGovernance.empty.title')"
              :empty-description="t('settings.metadataGovernance.empty.description')"
            />
          </UCard>

          <UCard v-else>
            <template #header>
              <div class="flex items-start justify-between gap-3">
                <div>
                  <div class="text-sm font-medium">{{ t("settings.metadataGovernance.tabs.resourceTypes") }}</div>
                  <div class="text-xs text-gray-500">{{ t("settings.metadataGovernance.intent.resourceTypes.description") }}</div>
                </div>
                <UButton
                  v-if="store.hasManagePermission"
                  size="xs"
                  variant="subtle"
                  icon="i-lucide-plus"
                  @click="openCreate('resourceType')"
                >
                  {{ t("settings.metadataGovernance.create.resourceType.button") }}
                </UButton>
              </div>
            </template>
            <MetadataStatePanel
              v-if="store.resourceTypes.length === 0"
              state="empty"
              :title="t('settings.metadataGovernance.empty.title')"
              :description="t('settings.metadataGovernance.empty.description')"
            />
            <div v-else class="overflow-x-auto">
              <table class="min-w-full text-sm">
                <thead class="text-left text-xs uppercase text-gray-500">
                  <tr>
                    <th class="px-3 py-2">{{ t("settings.metadataGovernance.table.name") }}</th>
                    <th class="px-3 py-2">{{ t("settings.metadataGovernance.table.resourceType") }}</th>
                    <th class="px-3 py-2">{{ t("settings.metadataGovernance.table.module") }}</th>
                    <th class="px-3 py-2">{{ t("settings.metadataGovernance.table.validator") }}</th>
                    <th class="px-3 py-2">{{ t("settings.metadataGovernance.table.status") }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="row in store.resourceTypes" :key="row.uuid" class="border-t border-gray-100 dark:border-gray-800">
                    <td class="px-3 py-2">{{ row.display_name }}</td>
                    <td class="px-3 py-2 text-gray-500">{{ row.resource_type }}</td>
                    <td class="px-3 py-2">{{ moduleLabel(row.module) }}</td>
                    <td class="px-3 py-2">
                      <UBadge variant="subtle" :color="row.validator_status === 'available' ? 'success' : 'warning'">
                        {{ t(`settings.metadataGovernance.validator.${row.validator_status}`) }}
                      </UBadge>
                    </td>
                    <td class="px-3 py-2"><StatusBadge :status="row.status" /></td>
                  </tr>
                </tbody>
              </table>
            </div>
          </UCard>

          <div
            v-if="stateFor(item.value) === 'ready' && showSingleTablePagination"
            class="mt-4 flex flex-col gap-3 rounded-lg border border-gray-200 bg-white px-4 py-3 text-sm dark:border-gray-800 dark:bg-gray-900 md:flex-row md:items-center md:justify-between"
          >
            <div class="text-gray-600 dark:text-gray-400">
              {{ t("settings.metadataGovernance.pagination.summary", mainPaginationSummary) }}
            </div>
            <div class="flex flex-wrap items-center gap-3">
              <div class="flex items-center gap-2">
                <span class="text-gray-500">{{ t("settings.metadataGovernance.pagination.pageSize") }}</span>
                <USelect
                  :model-value="mainPager.pageSize"
                  :items="pageSizeItems"
                  class="w-24"
                  @update:model-value="changeMainPageSize"
                />
              </div>
              <UPagination
                :page="mainPager.page"
                :total="mainPagination.total"
                :items-per-page="mainPager.pageSize"
                @update:page="changeMainPage"
              />
            </div>
          </div>
        </div>
      </template>
    </UTabs>

    <MetadataCreateModal
      v-model:open="createOpen"
      :target="createTarget"
      :taxonomy-nodes="store.taxonomyNodes"
      :module-items="createModuleItems"
      :resource-type-items="resourceTypeItems"
      :default-module="defaultCreateModule"
      :context-title="createContextTitle"
      :context-description="createContextDescription"
      :active-locale="currentLocale"
      :submitting="createSubmitting"
      :error-message="createError"
      @submit="submitCreate"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, reactive, ref, resolveComponent, watch, type PropType } from "vue";
import MetadataCreateModal from "~/components/settings/metadata-governance/MetadataCreateModal.vue";
import MetadataStatePanel from "~/components/settings/metadata-governance/MetadataStatePanel.vue";
import TaxonomyTreeBody from "~/components/settings/metadata-governance/TaxonomyTreeBody.vue";
import { useMetadataGovernanceStore } from "~/stores/metadata-governance";
import { useUserStore } from "~/stores/user";
import type {
  CreateDictionaryItemPayload,
  CreateDictionaryNamespacePayload,
  CreateMetadataTagPayload,
  CreateResourceTypePayload,
  CreateTaxonomyNodePayload,
  CreateTaxonomyPayload,
  MetadataCreateTarget,
  MetadataStatus,
  MetadataTabKey,
  MetadataViewState,
} from "~/types/metadata-governance";

const { t, te, locale } = useI18n();
const me = useMe();
const userStore = useUserStore();
const store = useMetadataGovernanceStore();
const FILTER_ALL = "__all";
const currentLocale = computed(() => String((locale as any)?.value ?? locale ?? "zh"));

const filters = reactive({
  module: FILTER_ALL,
  resourceType: FILTER_ALL,
  status: FILTER_ALL,
  q: "",
});
const mainPager = reactive({
  page: 1,
  pageSize: 20,
});
const createOpen = ref(false);
const createSubmitting = ref(false);
const createError = ref("");
const createTarget = ref<MetadataCreateTarget>("dictionaryNamespace");
const detailPager = reactive({
  page: 1,
  pageSize: 20,
});

const activeTab = computed({
  get: () => store.activeTab,
  set: (value) => {
    store.activeTab = value as MetadataTabKey;
    refresh();
  },
});

const queryParams = computed(() => ({
  module: store.activeTab !== "tags" && filters.module !== FILTER_ALL ? filters.module : undefined,
  resource_type: store.activeTab === "tags" && filters.resourceType !== FILTER_ALL ? filters.resourceType : undefined,
  status: filters.status === FILTER_ALL ? undefined : filters.status,
  q: filters.q || undefined,
  locale: locale.value,
  page: mainPager.page,
  page_size: mainPager.pageSize,
}));
const detailQueryParams = computed(() => ({
  status: filters.status === FILTER_ALL ? undefined : filters.status,
  q: filters.q || undefined,
  locale: locale.value,
  page: detailPager.page,
  page_size: detailPager.pageSize,
}));

const tabItems = computed(() => [
  { label: t("settings.metadataGovernance.tabs.dictionaries"), value: "dictionaries" },
  { label: t("settings.metadataGovernance.tabs.taxonomies"), value: "taxonomies" },
  { label: t("settings.metadataGovernance.tabs.tags"), value: "tags" },
  { label: t("settings.metadataGovernance.tabs.resourceTypes"), value: "resourceTypes" },
]);

const discoveredModuleItems = computed(() => {
  const values = new Set<string>();
  for (const item of store.dictionaries) {
    if (item.module) values.add(item.module);
  }
  for (const item of store.taxonomies) {
    if (item.module) values.add(item.module);
  }
  for (const item of store.resourceTypes) {
    if (item.module) values.add(item.module);
  }
  if (filters.module !== FILTER_ALL) values.add(filters.module);
  return Array.from(values)
    .sort((a, b) => a.localeCompare(b))
    .map((value) => ({ label: moduleLabel(value), value }));
});
const createModuleItems = computed(() => discoveredModuleItems.value);
const moduleItems = computed(() => [
  { label: t("settings.metadataGovernance.filters.allModules"), value: FILTER_ALL },
  ...createModuleItems.value,
]);
const resourceTypeFilterItems = computed(() => [
  { label: t("settings.metadataGovernance.filters.allResourceTypes"), value: FILTER_ALL },
  ...resourceTypeItems.value,
]);
const primaryFilterItems = computed(() => store.activeTab === "tags" ? resourceTypeFilterItems.value : moduleItems.value);
const primaryFilterValue = computed({
  get: () => store.activeTab === "tags" ? filters.resourceType : filters.module,
  set: (value: string) => {
    if (store.activeTab === "tags") {
      filters.resourceType = value;
      return;
    }
    filters.module = value;
  },
});
const searchPlaceholder = computed(() =>
  store.activeTab === "tags"
    ? t("settings.metadataGovernance.filters.searchTags")
    : t("settings.metadataGovernance.filters.search"),
);
const resourceTypeItems = computed(() =>
  store.resourceTypes.map((item) => ({
    label: `${item.display_name} · ${item.resource_type}`,
    value: item.resource_type,
    module: item.module,
  })),
);
const selectedDictionary = computed(() => store.dictionaries.find((item) => item.uuid === store.selectedDictionaryUuid));
const selectedTaxonomy = computed(() => store.taxonomies.find((item) => item.uuid === store.selectedTaxonomyUuid));
const defaultCreateModule = computed(() => {
  if (filters.module !== FILTER_ALL) return filters.module;
  if (createTarget.value === "dictionaryItem" && selectedDictionary.value?.module) return selectedDictionary.value.module;
  if (createTarget.value === "taxonomyNode" && selectedTaxonomy.value?.module) return selectedTaxonomy.value.module;
  if ((createTarget.value === "dictionaryNamespace" || createTarget.value === "taxonomy") && createModuleItems.value.length > 0) {
    return createModuleItems.value[0].value;
  }
  return createModuleItems.value[0]?.value || "";
});
const createContextTitle = computed(() => {
  if (createTarget.value === "dictionaryItem" && selectedDictionary.value) {
    return t("settings.metadataGovernance.create.context.dictionary", { name: selectedDictionary.value.display_name });
  }
  if (createTarget.value === "taxonomyNode" && selectedTaxonomy.value) {
    return t("settings.metadataGovernance.create.context.taxonomy", { name: selectedTaxonomy.value.display_name });
  }
  return "";
});
const createContextDescription = computed(() => {
  if (createTarget.value === "dictionaryItem" && selectedDictionary.value) {
    return `${selectedDictionary.value.namespace} · ${moduleLabel(selectedDictionary.value.module)}`;
  }
  if (createTarget.value === "taxonomyNode" && selectedTaxonomy.value) {
    return `${selectedTaxonomy.value.namespace} · ${moduleLabel(selectedTaxonomy.value.module)}`;
  }
  return "";
});

const moduleLabel = (value: string) => {
  const normalized = String(value || "").trim();
  if (!normalized) return "";
  const key = `settings.metadataGovernance.modules.${normalized.replace(/[.-]/g, "_")}`;
  return te(key) ? t(key) : normalized;
};

const statusItems = computed(() => [
  { label: t("settings.metadataGovernance.filters.allStatus"), value: FILTER_ALL },
  { label: t("settings.metadataGovernance.status.enabled"), value: "enabled" },
  { label: t("settings.metadataGovernance.status.disabled"), value: "disabled" },
  { label: t("settings.metadataGovernance.status.archived"), value: "archived" },
]);
const pageSizeItems = computed(() => [
  { label: "10", value: 10 },
  { label: "20", value: 20 },
  { label: "50", value: 50 },
  { label: "100", value: 100 },
]);
const mainPagination = computed(() => store.pagination[store.activeTab] ?? {
  page: mainPager.page,
  page_size: mainPager.pageSize,
  total: 0,
});
const detailPaginationKey = computed(() => store.activeTab === "dictionaries" ? "dictionaryItems" : "taxonomyNodes");
const detailPagination = computed(() => store.pagination[detailPaginationKey.value] ?? {
  page: detailPager.page,
  page_size: detailPager.pageSize,
  total: 0,
});
const summarizePagination = (pageInfo: { total?: number; page?: number; page_size?: number }, fallback: { page: number; pageSize: number }) => {
  const total = pageInfo.total || 0;
  const page = pageInfo.page || fallback.page;
  const pageSize = pageInfo.page_size || fallback.pageSize;
  const start = total === 0 ? 0 : (page - 1) * pageSize + 1;
  const end = Math.min(page * pageSize, total);
  return { start, end, total, page };
};
const mainPaginationSummary = computed(() => summarizePagination(mainPagination.value, mainPager));
const detailPaginationSummary = computed(() => summarizePagination(detailPagination.value, detailPager));
const showSingleTablePagination = computed(() =>
  (store.activeTab === "tags" || store.activeTab === "resourceTypes") && mainPagination.value.total > 0
);
const tabIntent = (tab: string) => ({
  dictionaries: {
    icon: "i-lucide-list",
    title: t("settings.metadataGovernance.intent.dictionaries.title"),
    description: t("settings.metadataGovernance.intent.dictionaries.description"),
  },
  taxonomies: {
    icon: "i-lucide-git-fork",
    title: t("settings.metadataGovernance.intent.taxonomies.title"),
    description: t("settings.metadataGovernance.intent.taxonomies.description"),
  },
  tags: {
    icon: "i-lucide-tags",
    title: t("settings.metadataGovernance.intent.tags.title"),
    description: t("settings.metadataGovernance.intent.tags.description"),
  },
  resourceTypes: {
    icon: "i-lucide-shapes",
    title: t("settings.metadataGovernance.intent.resourceTypes.title"),
    description: t("settings.metadataGovernance.intent.resourceTypes.description"),
  },
}[tab as MetadataTabKey] ?? {
  icon: "i-lucide-info",
  title: t("settings.metadataGovernance.title"),
  description: t("settings.metadataGovernance.description"),
});

watch(
  () => ({ ...filters, tab: store.activeTab, locale: locale.value }),
  () => {
    mainPager.page = 1;
    detailPager.page = 1;
    refresh();
  },
  { deep: true },
);
watch(
  () => [mainPager.page, mainPager.pageSize],
  () => refresh(),
);
watch(
  () => [detailPager.page, detailPager.pageSize],
  () => refreshDetail(),
);

onMounted(async () => {
  if (userStore.isRoot) {
    store.setPermissions(true, true);
    await refresh();
    return;
  }

  const readPermissions = [
    "metadata.dictionary:read",
    "metadata.taxonomy:read",
    "metadata.tag:read",
    "metadata.resource_type:read",
  ];
  const managePermissions = [
    "metadata.dictionary:manage",
    "metadata.taxonomy:manage",
    "metadata.tag:manage",
    "metadata.resource_type:manage",
  ];

  const readResults = await Promise.all(readPermissions.map((permission) => me.hasPermission(permission)));
  const manageResults = await Promise.all(managePermissions.map((permission) => me.hasPermission(permission)));
  const canRead = readResults.some(Boolean);
  const canManage = manageResults.some(Boolean);
  store.setPermissions(canRead, canManage);
  await refresh();
});

const refresh = async () => {
  if (!store.hasReadPermission) return;
  const params = queryParams.value;
  if (store.activeTab === "dictionaries") await store.fetchDictionaries(params, detailQueryParams.value);
  if (store.activeTab === "taxonomies") await store.fetchTaxonomies(params, detailQueryParams.value);
  if (store.activeTab === "tags") await store.fetchTags(params);
  if (store.activeTab === "resourceTypes") await store.fetchResourceTypes(params);
};
const selectDictionary = async (namespaceUuid: string) => {
  detailPager.page = 1;
  await store.selectDictionary(namespaceUuid, detailQueryParams.value);
};
const selectTaxonomy = async (taxonomyUuid: string) => {
  detailPager.page = 1;
  await store.selectTaxonomy(taxonomyUuid, detailQueryParams.value);
};
const refreshDetail = async () => {
  if (!store.hasReadPermission) return;
  if (store.activeTab === "dictionaries" && store.selectedDictionaryUuid) {
    await store.fetchDictionaryItems(store.selectedDictionaryUuid, detailQueryParams.value);
  }
  if (store.activeTab === "taxonomies" && store.selectedTaxonomyUuid) {
    await store.fetchTaxonomyNodes(store.selectedTaxonomyUuid, detailQueryParams.value);
  }
};

const resetFilters = () => {
  filters.module = FILTER_ALL;
  filters.resourceType = FILTER_ALL;
  filters.status = FILTER_ALL;
  filters.q = "";
  mainPager.page = 1;
  detailPager.page = 1;
};

const openCreate = async (target: MetadataCreateTarget) => {
  createTarget.value = target;
  createError.value = "";
  if (target === "tag" && store.resourceTypes.length === 0) {
    await store.fetchResourceTypes({
      page: 1,
      page_size: 100,
      status: "enabled",
      locale: locale.value,
    });
  }
  createOpen.value = true;
};
const submitCreate = async (payload: Record<string, unknown>) => {
  createSubmitting.value = true;
  createError.value = "";
  try {
    if (createTarget.value === "dictionaryNamespace") {
      await store.createDictionaryNamespace(payload as CreateDictionaryNamespacePayload);
      await refresh();
    }
    if (createTarget.value === "dictionaryItem") {
      await store.createDictionaryItem(store.selectedDictionaryUuid, payload as CreateDictionaryItemPayload);
      await refreshDetail();
    }
    if (createTarget.value === "taxonomy") {
      await store.createTaxonomy(payload as CreateTaxonomyPayload);
      await refresh();
    }
    if (createTarget.value === "taxonomyNode") {
      await store.createTaxonomyNode(store.selectedTaxonomyUuid, payload as CreateTaxonomyNodePayload);
      await refreshDetail();
    }
    if (createTarget.value === "tag") {
      await store.createTag(payload as CreateMetadataTagPayload);
      await refresh();
    }
    if (createTarget.value === "resourceType") {
      await store.createResourceType(payload as CreateResourceTypePayload);
      await refresh();
    }
    createOpen.value = false;
  } catch (err) {
    createError.value = err && typeof err === "object" && "message" in err ? String((err as any).message) : "metadata_governance.create_failed";
  } finally {
    createSubmitting.value = false;
  }
};

const changeMainPage = (value: number) => {
  mainPager.page = Number(value) || 1;
};
const changeMainPageSize = (value: string | number) => {
  mainPager.pageSize = Number(value) || 20;
  mainPager.page = 1;
};
const changeDetailPage = (value: number) => {
  detailPager.page = Number(value) || 1;
};
const changeDetailPageSize = (value: string | number) => {
  detailPager.pageSize = Number(value) || 20;
  detailPager.page = 1;
};

const stateFor = (tab: string) => store.tabState(tab as MetadataTabKey);
const isBlockingState = (state: MetadataViewState) => state === "loading" || state === "no_permission" || state === "error";
const stateTitle = (state: MetadataViewState) => t(`settings.metadataGovernance.states.${state}.title`);
const stateDescription = (state: MetadataViewState) =>
  state === "error" && store.error
    ? store.error
    : t(`settings.metadataGovernance.states.${state}.description`);

const StatusBadge = defineComponent({
  props: {
    status: { type: String as PropType<MetadataStatus>, required: true },
  },
  setup(props) {
    return () =>
      h(
        resolveComponent("UBadge"),
        {
          variant: "subtle",
          color: props.status === "enabled" ? "success" : props.status === "disabled" ? "warning" : "neutral",
        },
        () => t(`settings.metadataGovernance.status.${props.status}`),
      );
  },
});

const SimpleTable = defineComponent({
  props: {
    rows: { type: Array as PropType<any[]>, required: true },
    codeKey: { type: String, required: true },
    countKey: { type: String, required: true },
    emptyTitle: { type: String, required: true },
    emptyDescription: { type: String, required: true },
  },
  setup(props) {
    return () =>
      h(
        resolveComponent("UCard"),
        {},
        () => h(SimpleTableBody, props),
      );
  },
});

const SimpleTableBody = defineComponent({
  props: {
    rows: { type: Array as PropType<any[]>, required: true },
    codeKey: { type: String, required: true },
    countKey: { type: String, required: true },
    emptyTitle: { type: String, required: true },
    emptyDescription: { type: String, required: true },
  },
  setup(props) {
    return () =>
      props.rows.length === 0
        ? h(MetadataStatePanel, { state: "empty", title: props.emptyTitle, description: props.emptyDescription })
        : h("div", { class: "overflow-x-auto" }, [
                h("table", { class: "min-w-full text-sm" }, [
                  h("thead", { class: "text-left text-xs uppercase text-gray-500" }, [
                    h("tr", [
                      h("th", { class: "px-3 py-2" }, t("settings.metadataGovernance.table.name")),
                      h("th", { class: "px-3 py-2" }, t("settings.metadataGovernance.table.code")),
                      h("th", { class: "px-3 py-2" }, t("settings.metadataGovernance.table.status")),
                      h("th", { class: "px-3 py-2" }, t("settings.metadataGovernance.table.count")),
                    ]),
                  ]),
                  h(
                    "tbody",
                    props.rows.map((row) =>
                      h("tr", { key: row.uuid, class: "border-t border-gray-100 dark:border-gray-800" }, [
                        h("td", { class: "px-3 py-2" }, row.display_name),
                        h("td", { class: "px-3 py-2 text-gray-500" }, row[props.codeKey]),
                        h("td", { class: "px-3 py-2" }, h(StatusBadge, { status: row.status })),
                        h("td", { class: "px-3 py-2" }, String(row[props.countKey] ?? 0)),
                      ]),
                    ),
                  ),
                ]),
              ]);
  },
});

const PaginationBar = defineComponent({
  props: {
    compact: { type: Boolean, default: false },
    pagination: { type: Object as PropType<{ total: number }>, required: true },
    pageSize: { type: Number, required: true },
    pageSizeItems: { type: Array as PropType<Array<{ label: string; value: number }>>, required: true },
    summary: { type: Object as PropType<{ start: number; end: number; total: number; page: number }>, required: true },
  },
  emits: ["update:page", "update:page-size"],
  setup(props, { emit }) {
    return () =>
      h("div", { class: props.compact ? "flex justify-center" : "flex flex-col gap-3 text-sm lg:flex-row lg:items-center lg:justify-between" }, [
        props.compact
          ? null
          : h(
              "div",
              { class: "text-gray-600 dark:text-gray-400" },
              t("settings.metadataGovernance.pagination.summary", props.summary),
            ),
        h("div", { class: props.compact ? "flex justify-center" : "flex flex-wrap items-center gap-3" }, [
          props.compact
            ? null
            : h("div", { class: "flex items-center gap-2" }, [
            h("span", { class: "text-gray-500" }, t("settings.metadataGovernance.pagination.pageSize")),
            h(resolveComponent("USelect"), {
              modelValue: props.pageSize,
              items: props.pageSizeItems,
              class: "w-24 shrink-0",
              "onUpdate:modelValue": (value: string | number) => emit("update:page-size", value),
            }),
          ]),
          h(
            "div",
            { class: props.compact ? "flex justify-center" : "" },
            h(resolveComponent("UPagination"), {
              page: props.summary.page,
              total: props.pagination.total,
              itemsPerPage: props.pageSize,
              size: props.compact ? "sm" : undefined,
              "onUpdate:page": (value: number) => emit("update:page", value),
            }),
          ),
        ]),
      ]);
  },
});
</script>
