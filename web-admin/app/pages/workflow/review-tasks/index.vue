<template>
  <div class="workflow-review-page">
    <div class="review-header">
      <div>
        <UButton icon="i-heroicons-arrow-left" color="neutral" variant="ghost" to="/workflow">
          {{ t("common.backToList") }}
        </UButton>
        <h1>{{ t("workflow.review.title") }}</h1>
      </div>
      <div class="review-actions">
        <USelectMenu v-model="status" :items="statusOptions" value-key="value" label-key="label" class="w-44" />
        <UButton icon="i-heroicons-arrow-path" color="neutral" variant="ghost" :loading="loading" @click="loadTasks">
          {{ t("common.refresh") }}
        </UButton>
      </div>
    </div>

    <UCard>
      <div v-if="!loading && tasks.length === 0" class="empty">{{ t("workflow.review.empty") }}</div>
      <div v-else class="space-y-2">
        <div v-for="task in tasks" :key="task.review_task_uuid" class="task-row">
          <div>
            <div class="task-title">{{ reviewTypeLabel(task.review_type) }}</div>
            <div class="task-meta">
              {{ t("workflow.review.instance", { uuid: shortUUID(task.workflow_instance_uuid) }) }}
              <span>{{ t("workflow.review.step", { step: task.step_id }) }}</span>
            </div>
          </div>
          <div class="task-actions">
            <UBadge :color="statusColor(task.status)" variant="soft">{{ t(`workflow.review.status.${task.status}`) }}</UBadge>
            <UButton
              v-if="task.status === 'pending'"
              icon="i-heroicons-check"
              color="success"
              variant="soft"
              size="sm"
              :loading="actingTaskUUID === task.review_task_uuid && actingAction === 'approve'"
              @click="openActionDialog(task, 'approve')"
            >
              {{ t("workflow.review.approve") }}
            </UButton>
            <UButton
              v-if="task.status === 'pending'"
              icon="i-heroicons-x-mark"
              color="error"
              variant="soft"
              size="sm"
              :loading="actingTaskUUID === task.review_task_uuid && actingAction === 'reject'"
              @click="openActionDialog(task, 'reject')"
            >
              {{ t("workflow.review.reject") }}
            </UButton>
            <UButton
              icon="i-heroicons-eye"
              color="neutral"
              variant="ghost"
              :to="`/workflow/instances/${task.workflow_instance_uuid}`"
              :aria-label="t('workflow.review.openInstance')"
            />
          </div>
        </div>
      </div>
      <div class="pagination-row">
        <span>{{ t("workflow.pagination.total", { total }) }}</span>
        <UPagination v-model:page="page" :total="total" :page-count="pageSize" :max="5" />
      </div>
    </UCard>

    <UModal
      v-model:open="actionDialogOpen"
      :title="actionDialogTitle"
      :description="actionDialogDescription"
      :close="{ disabled: Boolean(actingTaskUUID) }"
      :ui="{ content: 'max-w-xl w-full' }"
    >
      <template #body>
        <div class="space-y-4">
          <div v-if="selectedTask" class="review-summary">
            <div>
              <span>{{ t("workflow.review.reviewType") }}</span>
              <strong>{{ reviewTypeLabel(selectedTask.review_type) }}</strong>
            </div>
            <div>
              <span>{{ t("workflow.review.instanceLabel") }}</span>
              <strong>{{ shortUUID(selectedTask.workflow_instance_uuid) }}</strong>
            </div>
            <div>
              <span>{{ t("workflow.review.stepLabel") }}</span>
              <strong>{{ selectedTask.step_id }}</strong>
            </div>
          </div>
          <UFormField :label="t('workflow.review.commentLabel')">
            <UTextarea
              v-model="actionComment"
              class="w-full"
              :rows="4"
              :placeholder="t('workflow.review.commentPlaceholder')"
            />
          </UFormField>
        </div>
      </template>
      <template #footer>
        <div class="modal-footer">
          <UButton
            type="button"
            color="neutral"
            variant="subtle"
            :disabled="Boolean(actingTaskUUID)"
            @click="closeActionDialog"
          >
            {{ t("common.cancel") }}
          </UButton>
          <UButton
            type="button"
            :color="selectedAction === 'approve' ? 'success' : 'error'"
            :icon="selectedAction === 'approve' ? 'i-heroicons-check' : 'i-heroicons-x-mark'"
            :loading="Boolean(actingTaskUUID)"
            :disabled="!selectedTask || !selectedAction"
            @click="confirmAction"
          >
            {{ selectedAction === "approve" ? t("workflow.review.confirmApprove") : t("workflow.review.confirmReject") }}
          </UButton>
        </div>
      </template>
    </UModal>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useI18n, useToast } from "#imports";
import { useWorkflowService, type HumanReviewStatus, type HumanReviewTask } from "~/composables/api/services/workflowService";

const { t, te } = useI18n();
const toast = useToast();
const workflowService = useWorkflowService();
const loading = ref(false);
const tasks = ref<HumanReviewTask[]>([]);
const total = ref(0);
const page = ref(1);
const pageSize = ref(20);
const status = ref("pending");
const actingTaskUUID = ref("");
const actingAction = ref("");
const actionDialogOpen = ref(false);
const selectedTask = ref<HumanReviewTask | null>(null);
const selectedAction = ref<"approve" | "reject" | "">("");
const actionComment = ref("");

const statusOptions = computed(() => [
  { label: t("workflow.review.status.pending"), value: "pending" },
  { label: t("workflow.review.status.approved"), value: "approved" },
  { label: t("workflow.review.status.rejected"), value: "rejected" },
  { label: t("workflow.review.status.changes_requested"), value: "changes_requested" },
  { label: t("workflow.review.status.canceled"), value: "canceled" },
]);

watch([page, status], loadTasks);

const actionDialogTitle = computed(() => {
  if (selectedAction.value === "approve") return t("workflow.review.approveDialogTitle");
  if (selectedAction.value === "reject") return t("workflow.review.rejectDialogTitle");
  return t("workflow.review.actionDialogTitle");
});

const actionDialogDescription = computed(() => {
  if (selectedAction.value === "approve") return t("workflow.review.approveDialogDescription");
  if (selectedAction.value === "reject") return t("workflow.review.rejectDialogDescription");
  return "";
});

async function loadTasks() {
  loading.value = true;
  try {
    const result = await workflowService.listReviewTasks({
      page: page.value,
      pageSize: pageSize.value,
      status: status.value as HumanReviewStatus,
    });
    tasks.value = result.items;
    total.value = result.total;
  } finally {
    loading.value = false;
  }
}

function openActionDialog(task: HumanReviewTask, action: "approve" | "reject") {
  selectedTask.value = task;
  selectedAction.value = action;
  actionComment.value = "";
  actionDialogOpen.value = true;
}

function closeActionDialog() {
  if (actingTaskUUID.value) return;
  actionDialogOpen.value = false;
  selectedTask.value = null;
  selectedAction.value = "";
  actionComment.value = "";
}

async function confirmAction() {
  if (!selectedTask.value || !selectedAction.value) return;
  await actTask(selectedTask.value, selectedAction.value);
}

async function actTask(task: HumanReviewTask, action: "approve" | "reject") {
  actingTaskUUID.value = task.review_task_uuid;
  actingAction.value = action;
  try {
    await workflowService.actReviewTask(task.review_task_uuid, {
      action,
      comment: actionComment.value.trim(),
      payload: {
        workflow_instance_uuid: task.workflow_instance_uuid,
        step_id: task.step_id,
      },
    });
    toast.add({
      title: action === "approve" ? t("workflow.review.approveSuccess") : t("workflow.review.rejectSuccess"),
      color: action === "approve" ? "success" : "warning",
    });
    closeActionDialog();
    await loadTasks();
  } catch (err: any) {
    toast.add({
      title: t("workflow.review.actionFailed"),
      description: err?.message || t("workflow.review.actionFailed"),
      color: "error",
    });
  } finally {
    actingTaskUUID.value = "";
    actingAction.value = "";
  }
}

function reviewTypeLabel(value: string) {
  const key = `workflow.review.type.${camelCase(value)}`;
  return te(key) ? t(key) : value.replace(/_/g, " ");
}

function camelCase(value: string) {
  return value
    .trim()
    .replace(/[^a-zA-Z0-9]+(.)/g, (_, char: string) => char.toUpperCase())
    .replace(/^[A-Z]/, (char) => char.toLowerCase());
}

function statusColor(value: string) {
  if (value === "approved") return "success";
  if (value === "rejected" || value === "canceled") return "error";
  if (value === "changes_requested") return "warning";
  return "info";
}

function shortUUID(value: string) {
  return value ? `${value.slice(0, 8)}...${value.slice(-6)}` : t("common.unknown");
}

onMounted(loadTasks);
</script>

<style scoped>
.workflow-review-page {
  padding: 24px;
  max-width: 1180px;
  margin: 0 auto;
}

.review-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
  margin-bottom: 16px;
}

.review-header h1 {
  margin-top: 10px;
  font-size: 24px;
  font-weight: 700;
  color: var(--ui-text-highlighted);
}

.review-actions,
.task-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.task-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  border: 1px solid var(--ui-border);
  border-radius: 8px;
  padding: 12px;
}

.task-title {
  font-weight: 700;
  color: var(--ui-text-highlighted);
}

.task-meta,
.empty,
.pagination-row {
  color: var(--ui-text-muted);
}

.task-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.empty {
  padding: 32px;
  text-align: center;
}

.pagination-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding-top: 14px;
}

.review-summary {
  display: grid;
  gap: 10px;
  border: 1px solid var(--ui-border);
  border-radius: 8px;
  padding: 12px;
}

.review-summary div {
  display: grid;
  gap: 4px;
}

.review-summary span {
  color: var(--ui-text-muted);
  font-size: 12px;
}

.review-summary strong {
  overflow-wrap: anywhere;
  color: var(--ui-text-highlighted);
}

.modal-footer {
  display: flex;
  width: 100%;
  justify-content: flex-end;
  gap: 8px;
}
</style>
