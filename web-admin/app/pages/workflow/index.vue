<template>
  <div class="workflow-page">
    <div class="workflow-header">
      <div>
        <h1 class="text-2xl font-bold text-highlighted flex items-center gap-2">
          <UIcon name="i-heroicons-squares-2x2" />
          {{ t("workflow.list.title") }}
        </h1>
        <p class="text-muted mt-1">{{ t("workflow.list.description") }}</p>
      </div>
      <div class="workflow-actions">
        <UInput
          v-model="searchQuery"
          :placeholder="t('workflow.list.search')"
          icon="i-heroicons-magnifying-glass"
          class="w-64"
        />
        <USelectMenu
          v-model="statusFilter"
          :items="statusOptions"
          value-key="value"
          label-key="label"
          class="w-40"
        />
        <USelectMenu
          v-model="sourceFilter"
          :items="sourceOptions"
          value-key="value"
          label-key="label"
          class="w-40"
        />
        <USelectMenu
          v-model="categoryFilter"
          :items="categoryOptions"
          value-key="value"
          label-key="label"
          :loading="workflowCategoryLoading"
          class="w-44"
        />
        <UButton
          v-if="canInitializeBuiltinWorkflows"
          icon="i-heroicons-sparkles"
          color="neutral"
          variant="subtle"
          :loading="initializingBuiltinWorkflows"
          @click="initializeBuiltinWorkflows"
        >
          {{ t("workflow.list.initializeBuiltin") }}
        </UButton>
        <UButton
          icon="i-heroicons-arrow-path"
          color="neutral"
          variant="ghost"
          :aria-label="t('common.refresh')"
          :loading="loading"
          @click="refreshPageData"
        />
        <UButton icon="i-heroicons-document-plus" color="primary" @click="openCreateModal">
          {{ t("workflow.list.create") }}
        </UButton>
      </div>
    </div>

    <UAlert
      v-if="workflowCategoryLoadFailed"
      icon="i-heroicons-exclamation-triangle"
      color="error"
      variant="soft"
      :title="t('workflow.category.loadFailed')"
      :description="t('workflow.category.loadFailedDescription')"
      class="mb-5"
    />

    <div class="grid grid-cols-1 md:grid-cols-3 gap-4 mb-5">
      <UCard>
        <div class="stat-item">
          <UIcon name="i-heroicons-squares-2x2" class="stat-icon text-primary" />
          <div>
            <p class="stat-label">{{ t("workflow.stats.total") }}</p>
            <p class="stat-value">{{ total }}</p>
          </div>
        </div>
      </UCard>
      <UCard>
        <div class="stat-item">
          <UIcon name="i-heroicons-check-badge" class="stat-icon text-success" />
          <div>
            <p class="stat-label">{{ t("workflow.stats.published") }}</p>
            <p class="stat-value">{{ publishedCount }}</p>
          </div>
        </div>
      </UCard>
      <UCard>
        <div class="stat-item">
          <UIcon name="i-heroicons-pencil-square" class="stat-icon text-warning" />
          <div>
            <p class="stat-label">{{ t("workflow.stats.draft") }}</p>
            <p class="stat-value">{{ draftCount }}</p>
          </div>
        </div>
      </UCard>
    </div>

    <UCard>
      <template #header>
        <div class="flex items-center justify-between">
          <h2 class="text-base font-semibold text-highlighted">{{ t("workflow.list.definitions") }}</h2>
          <UButton
            icon="i-heroicons-user-group"
            color="neutral"
            variant="subtle"
            to="/workflow/review-tasks"
          >
            {{ t("workflow.review.title") }}
          </UButton>
        </div>
      </template>

      <div v-if="!loading && definitions.length === 0" class="empty-state">
        <UIcon name="i-heroicons-squares-2x2" class="empty-icon" />
        <h3>{{ t("workflow.list.emptyTitle") }}</h3>
        <p>{{ t("workflow.list.emptyDescription") }}</p>
        <UButton icon="i-heroicons-document-plus" color="primary" @click="openCreateModal">
          {{ t("workflow.list.create") }}
        </UButton>
      </div>

      <div v-else class="definition-grid">
        <div
          v-for="definition in definitions"
          :key="definition.uuid"
          class="definition-card"
          @click="openWorkflowEditor(definition.uuid)"
        >
          <div class="definition-thumb">
            <div class="definition-icon">
              <UIcon name="i-heroicons-squares-2x2" />
            </div>
            <UBadge :color="statusColor(definition.status)" variant="soft" size="sm">
              {{ statusLabel(definition.status) }}
            </UBadge>
          </div>

          <div class="definition-card-body">
            <h3 class="definition-title">{{ definitionDisplayName(definition) }}</h3>
            <p class="definition-desc">
              {{ definitionDescription(definition) }}
            </p>
          </div>

          <div class="definition-card-footer">
            <div class="definition-meta">
              <span>{{ formatDate(definition.updated_at || definition.created_at) }}</span>
            </div>
            <div class="definition-footer-actions">
              <UBadge v-if="definition.workflow_pack_key" color="neutral" variant="subtle" size="sm">
                {{ t("workflow.list.builtinPack") }}
              </UBadge>
              <UBadge :color="categoryColor(definitionCategory(definition))" variant="soft" size="sm">
                {{ categoryLabel(definitionCategory(definition)) }}
              </UBadge>
              <UDropdownMenu :items="workflowActions(definition)" @click.stop>
                <UButton
                  icon="i-heroicons-ellipsis-vertical"
                  color="neutral"
                  variant="ghost"
                  size="sm"
                  :aria-label="t('workflow.list.actions')"
                />
              </UDropdownMenu>
            </div>
          </div>
        </div>

        <div class="pagination-row definition-pagination">
          <div class="text-sm text-muted">
            {{ t("workflow.pagination.total", { total }) }}
          </div>
          <div class="flex items-center gap-3">
            <USelectMenu
              v-model="pageSize"
              :items="pageSizeOptions"
              value-key="value"
              label-key="label"
              class="w-36"
            />
            <UPagination v-model:page="page" :total="total" :page-count="pageSize" :max="5" />
          </div>
        </div>
      </div>
    </UCard>

    <UModal
      v-model:open="showCreateModal"
      :title="t('workflow.create.title')"
      :description="t('workflow.create.description')"
    >
      <template #body>
        <div class="space-y-4">
          <UFormField :label="t('workflow.create.name')" required>
            <UInput v-model="newWorkflow.name" :placeholder="t('workflow.create.namePlaceholder')" />
          </UFormField>
          <UFormField :label="t('workflow.create.definitionDescription')">
            <UTextarea
              v-model="newWorkflow.description"
              :placeholder="t('workflow.create.descriptionPlaceholder')"
              :rows="3"
            />
          </UFormField>
        </div>
      </template>
      <template #footer>
        <div class="flex justify-end gap-2 w-full">
          <UButton color="neutral" variant="subtle" @click="showCreateModal = false">
            {{ t("common.cancel") }}
          </UButton>
          <UButton color="primary" :disabled="!newWorkflow.name.trim()" :loading="creating" @click="createWorkflow">
            {{ t("workflow.create.submit") }}
          </UButton>
        </div>
      </template>
    </UModal>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import { useI18n, useToast } from "#imports";
import { useMetadataGovernanceService } from "~/composables/api/services/metadataGovernanceService";
import { useWorkflowService, type WorkflowDefinition, type WorkflowDefinitionStatus } from "~/composables/api/services/workflowService";
import { useUserStore } from "~/stores/user";
import type { DictionaryItem } from "~/types/metadata-governance";

const WORKFLOW_CATEGORY_MODULE = "corex.workflow";
const WORKFLOW_CATEGORY_NAMESPACE = "corex.workflow.category";

const { t, te, locale } = useI18n();
const toast = useToast();
const metadataService = useMetadataGovernanceService();
const workflowService = useWorkflowService();
const userStore = useUserStore();

const loading = ref(false);
const creating = ref(false);
const initializingBuiltinWorkflows = ref(false);
const workflowCategoryLoading = ref(false);
const workflowCategoryLoadFailed = ref(false);
const workflowCategoryItems = ref<DictionaryItem[]>([]);
const searchQuery = ref("");
const statusFilter = ref("all");
const sourceFilter = ref("all");
const categoryFilter = ref("all");
const page = ref(1);
const pageSize = ref(12);
const total = ref(0);
const definitions = ref<WorkflowDefinition[]>([]);
const showCreateModal = ref(false);
const newWorkflow = reactive({ name: "", description: "" });

const statusOptions = computed(() => [
  { label: t("workflow.status.all"), value: "all" },
  { label: t("workflow.status.draft"), value: "draft" },
  { label: t("workflow.status.published"), value: "published" },
  { label: t("workflow.status.archived"), value: "archived" },
]);

const sourceOptions = computed(() => [
  { label: t("workflow.source.all"), value: "all" },
  { label: t("workflow.source.workflowPack"), value: "workflow_pack" },
  { label: t("workflow.source.manual"), value: "manual" },
]);

const categoryOptions = computed(() => [
  { label: t("workflow.category.all"), value: "all" },
  ...workflowCategoryItems.value.map((item) => ({
    label: dictionaryItemLabel(item),
    value: item.code,
  })),
]);

const pageSizeOptions = computed(() => [10, 12, 20, 50].map((value) => ({
  label: t("workflow.pagination.pageSize", { value }),
  value,
})));

const publishedCount = computed(() => definitions.value.filter((item) => item.status === "published").length);
const draftCount = computed(() => definitions.value.filter((item) => item.status === "draft").length);
const canInitializeBuiltinWorkflows = computed(() =>
  userStore.isRoot || userStore.isCurrentTenantOwner || userStore.isCurrentTenantAdmin
);

watch([searchQuery, statusFilter, sourceFilter, categoryFilter, pageSize], () => {
  page.value = 1;
  loadDefinitions();
});

watch(page, () => {
  loadDefinitions();
});

async function loadDefinitions() {
  loading.value = true;
  try {
    const result = await workflowService.listDefinitions({
      page: page.value,
      pageSize: pageSize.value,
      keyword: searchQuery.value.trim() || undefined,
      status: statusFilter.value === "all" ? undefined : (statusFilter.value as WorkflowDefinitionStatus),
      source_type: sourceFilter.value === "all" ? undefined : sourceFilter.value,
      category: categoryFilter.value === "all" ? undefined : categoryFilter.value,
    });
    definitions.value = result.items;
    total.value = result.total;
  } finally {
    loading.value = false;
  }
}

async function refreshPageData() {
  await Promise.all([loadWorkflowCategories(), loadDefinitions()]);
}

async function loadWorkflowCategories() {
  workflowCategoryLoading.value = true;
  workflowCategoryLoadFailed.value = false;
  try {
    const namespaces = await metadataService.listDictionaries({
      module: WORKFLOW_CATEGORY_MODULE,
      q: WORKFLOW_CATEGORY_NAMESPACE,
      status: "enabled",
      locale: metadataLocale(),
      page_size: 20,
    });
    const namespace = namespaces.items.find((item) => item.namespace === WORKFLOW_CATEGORY_NAMESPACE);
    if (!namespace) {
      throw new Error("workflow_category_dictionary_missing");
    }
    const items = await metadataService.listDictionaryItems(namespace.uuid, {
      status: "enabled",
      locale: metadataLocale(),
      page_size: 100,
    });
    workflowCategoryItems.value = items.items;
    if (categoryFilter.value !== "all" && !workflowCategoryItems.value.some((item) => item.code === categoryFilter.value)) {
      categoryFilter.value = "all";
    }
  } catch {
    workflowCategoryItems.value = [];
    workflowCategoryLoadFailed.value = true;
    if (categoryFilter.value !== "all") {
      categoryFilter.value = "all";
    }
    toast.add({
      title: t("workflow.category.loadFailed"),
      description: t("workflow.category.loadFailedDescription"),
      color: "error",
    });
  } finally {
    workflowCategoryLoading.value = false;
  }
}

async function initializeBuiltinWorkflows() {
  initializingBuiltinWorkflows.value = true;
  try {
    const result = await workflowService.seedWorkflowPacks([]);
    await loadDefinitions();
    toast.add({
      title: t("workflow.list.initializeBuiltinSuccess"),
      description: t("workflow.list.initializeBuiltinResult", {
        seeded: result.seeded.length,
        skipped: result.skipped.length,
      }),
      color: "success",
    });
  } catch (error: any) {
    toast.add({
      title: t("workflow.list.initializeBuiltinFailed"),
      description: error?.message || t("common.unknown"),
      color: "error",
    });
  } finally {
    initializingBuiltinWorkflows.value = false;
  }
}

function openCreateModal() {
  showCreateModal.value = true;
}

async function createWorkflow() {
  const name = newWorkflow.name.trim();
  if (!name) return;
  creating.value = true;
  try {
    const definition = await workflowService.createDefinition({
      name,
      description: newWorkflow.description.trim(),
      steps: [
        {
          id: "input",
          type: "system",
          node_kind: "input.capture",
          config: {
            input_schema_ref: "workflow.input.manual.v1",
            source_policy: { text: true, form: true },
            artifact_output_path: "$.artifacts.source",
          },
          next_step_ids: ["end"],
        },
        {
          id: "end",
          type: "system",
          node_kind: "workflow.end",
          config: {},
        },
      ],
    });
    showCreateModal.value = false;
    newWorkflow.name = "";
    newWorkflow.description = "";
    openWorkspaceWindow(definition.uuid);
  } finally {
    creating.value = false;
  }
}

function openWorkflowEditor(definitionUUID: string) {
  openWorkspaceWindow(definitionUUID);
}

function openWorkspaceWindow(definitionUUID: string) {
  if (!import.meta.client) return;
  window.open(`/workflow/workspace?id=${encodeURIComponent(definitionUUID)}`, "_blank", "noopener,noreferrer");
}

function workflowActions(definition: WorkflowDefinition) {
  return [[
    {
      label: t("workflow.actions.edit"),
      icon: "i-heroicons-pencil",
      onSelect: () => openWorkflowEditor(definition.uuid),
    },
    {
      label: t("workflow.actions.run"),
      icon: "i-heroicons-play",
      onSelect: () => startInstance(definition.uuid),
    },
  ]];
}

async function startInstance(definitionUUID: string) {
  const instance = await workflowService.startInstance(definitionUUID, {});
  await navigateTo(`/workflow/instances/${instance.uuid}`);
}

function statusColor(status: string) {
  if (status === "published") return "success";
  if (status === "archived") return "neutral";
  return "warning";
}

function statusLabel(status: string) {
  return t(`workflow.status.${status}`);
}

function definitionCategory(definition: WorkflowDefinition) {
  const raw = definition.category || definition.metadata?.category;
  return typeof raw === "string" && raw.trim() ? raw.trim() : "uncategorized";
}

function categoryLabel(category: string) {
  const item = workflowCategoryItems.value.find((candidate) => candidate.code === category);
  if (item) {
    return dictionaryItemLabel(item);
  }
  if (category === "uncategorized") {
    return t("workflow.category.uncategorized");
  }
  return t("workflow.category.unknown");
}

function categoryColor(category: string) {
  if (category === "knowledge_curation") return "primary";
  if (category === "governance") return "warning";
  if (category === "automation") return "success";
  return "neutral";
}

function definitionDisplayName(definition: WorkflowDefinition) {
  const packNameKey = workflowPackI18nKey(definition, "name");
  if (packNameKey && te(packNameKey)) {
    return t(packNameKey);
  }
  return definition.name;
}

function definitionDescription(definition: WorkflowDefinition) {
  const rawDescription = definition.description?.trim();
  if (rawDescription && te(rawDescription)) {
    return t(rawDescription);
  }
  const packDescriptionKey = workflowPackI18nKey(definition, "description");
  if (packDescriptionKey && te(packDescriptionKey)) {
    return t(packDescriptionKey);
  }
  return rawDescription || t("workflow.list.noDescription");
}

function workflowPackI18nKey(definition: WorkflowDefinition, field: "name" | "description") {
  const packKey = definition.workflow_pack_key?.trim();
  if (!packKey) return "";
  return `workflow.pack.${camelCase(packKey)}.${field}`;
}

function dictionaryItemLabel(item: DictionaryItem) {
  const labels = item.label_i18n || {};
  return labels[metadataLocale()] || item.display_name || labels["zh-CN"] || labels["en-US"] || t("workflow.category.unknown");
}

function metadataLocale() {
  const value = String(locale.value || "");
  if (value.toLowerCase().startsWith("zh")) return "zh-CN";
  if (value.toLowerCase().startsWith("en")) return "en-US";
  return value || "zh-CN";
}

function camelCase(value: string) {
  return value.replace(/_([a-z0-9])/g, (_, char: string) => char.toUpperCase());
}

function formatDate(value?: string | null) {
  if (!value) return t("common.unknown");
  return new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
}

onMounted(() => {
  void userStore.fetchUserContext().catch(() => undefined);
  void refreshPageData();
});
</script>

<style scoped>
.workflow-page {
  padding: 24px;
  max-width: 1280px;
  margin: 0 auto;
}

.workflow-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 20px;
}

.workflow-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.stat-item {
  display: flex;
  align-items: center;
  gap: 12px;
}

.stat-icon {
  width: 32px;
  height: 32px;
}

.stat-label {
  font-size: 13px;
  color: var(--ui-text-muted);
}

.stat-value {
  font-size: 24px;
  font-weight: 700;
  color: var(--ui-text-highlighted);
}

.empty-state {
  min-height: 280px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  text-align: center;
  gap: 12px;
  color: var(--ui-text-muted);
}

.empty-state h3 {
  color: var(--ui-text-highlighted);
  font-weight: 700;
}

.empty-icon {
  width: 56px;
  height: 56px;
}

.definition-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 12px;
}

.definition-card {
  min-height: 188px;
  border: 1px solid var(--ui-border);
  border-radius: 8px;
  padding: 12px;
  cursor: pointer;
  background: var(--ui-bg);
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  gap: 12px;
  transition:
    border-color 140ms ease,
    background 140ms ease,
    transform 140ms ease;
}

.definition-card:hover {
  background: var(--ui-bg-elevated);
  border-color: var(--ui-primary);
  transform: translateY(-1px);
}

.definition-thumb {
  height: 64px;
  border-radius: 8px;
  background:
    linear-gradient(135deg, color-mix(in srgb, var(--ui-primary) 18%, transparent), transparent 62%),
    var(--ui-bg-elevated);
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  padding: 10px;
}

.definition-icon {
  width: 36px;
  height: 36px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: color-mix(in srgb, var(--ui-bg) 82%, transparent);
  color: var(--ui-primary);
  flex: none;
}

.definition-card-body {
  min-width: 0;
}

.definition-title {
  font-size: 15px;
  line-height: 1.35;
  font-weight: 700;
  color: var(--ui-text-highlighted);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.definition-desc {
  min-height: 40px;
  margin-top: 6px;
  font-size: 13px;
  line-height: 1.45;
  color: var(--ui-text-muted);
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}

.definition-card-footer {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 10px;
}

.definition-meta {
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 12px;
  color: var(--ui-text-dimmed);
  min-width: 0;
}

.definition-footer-actions {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: none;
}

.pagination-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding-top: 12px;
}

@media (max-width: 900px) {
  .workflow-header,
  .pagination-row {
    flex-direction: column;
    align-items: stretch;
  }

  .workflow-actions {
    justify-content: flex-start;
  }

  .definition-grid {
    grid-template-columns: 1fr;
  }
}
</style>
