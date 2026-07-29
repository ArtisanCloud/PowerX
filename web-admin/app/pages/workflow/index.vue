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
        <UButton
          icon="i-heroicons-arrow-path"
          color="neutral"
          variant="ghost"
          :aria-label="t('common.refresh')"
          :loading="loading"
          @click="loadDefinitions"
        />
        <UButton icon="i-heroicons-document-plus" color="primary" @click="openCreateModal">
          {{ t("workflow.list.create") }}
        </UButton>
      </div>
    </div>

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
              <span>{{ t("workflow.list.version", { version: definition.version }) }}</span>
              <span>{{ formatDate(definition.updated_at || definition.created_at) }}</span>
            </div>
            <div class="definition-footer-actions">
              <UBadge v-if="definition.workflow_pack_key" color="neutral" variant="subtle" size="sm">
                {{ t("workflow.list.builtinPack") }}
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
import { useI18n } from "#imports";
import { useWorkflowService, type WorkflowDefinition, type WorkflowDefinitionStatus } from "~/composables/api/services/workflowService";

const { t, te } = useI18n();
const workflowService = useWorkflowService();

const loading = ref(false);
const creating = ref(false);
const searchQuery = ref("");
const statusFilter = ref("all");
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

const pageSizeOptions = computed(() => [10, 12, 20, 50].map((value) => ({
  label: t("workflow.pagination.pageSize", { value }),
  value,
})));

const publishedCount = computed(() => definitions.value.filter((item) => item.status === "published").length);
const draftCount = computed(() => definitions.value.filter((item) => item.status === "draft").length);

watch([searchQuery, statusFilter, pageSize], () => {
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
    });
    definitions.value = result.items;
    total.value = result.total;
  } finally {
    loading.value = false;
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
      steps: [{ id: "input", type: "system", node_kind: "input.capture", config: {} }],
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

function camelCase(value: string) {
  return value.replace(/_([a-z0-9])/g, (_, char: string) => char.toUpperCase());
}

function formatDate(value?: string | null) {
  if (!value) return t("common.unknown");
  return new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
}

onMounted(loadDefinitions);
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
