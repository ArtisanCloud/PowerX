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
            <div class="task-title">{{ task.review_type }}</div>
            <div class="task-meta">
              {{ t("workflow.review.instance", { uuid: shortUUID(task.workflow_instance_uuid) }) }}
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
              @click="actTask(task, 'approve')"
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
              @click="actTask(task, 'reject')"
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
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useI18n, useToast } from "#imports";
import { useWorkflowService, type HumanReviewStatus, type HumanReviewTask } from "~/composables/api/services/workflowService";

const { t } = useI18n();
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

const statusOptions = computed(() => [
  { label: t("workflow.review.status.pending"), value: "pending" },
  { label: t("workflow.review.status.approved"), value: "approved" },
  { label: t("workflow.review.status.rejected"), value: "rejected" },
  { label: t("workflow.review.status.changes_requested"), value: "changes_requested" },
  { label: t("workflow.review.status.canceled"), value: "canceled" },
]);

watch([page, status], loadTasks);

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

async function actTask(task: HumanReviewTask, action: "approve" | "reject") {
  actingTaskUUID.value = task.review_task_uuid;
  actingAction.value = action;
  try {
    await workflowService.actReviewTask(task.review_task_uuid, {
      action,
      payload: {
        workflow_instance_uuid: task.workflow_instance_uuid,
        step_id: task.step_id,
      },
    });
    toast.add({
      title: action === "approve" ? t("workflow.review.approveSuccess") : t("workflow.review.rejectSuccess"),
      color: action === "approve" ? "success" : "warning",
    });
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
</style>
