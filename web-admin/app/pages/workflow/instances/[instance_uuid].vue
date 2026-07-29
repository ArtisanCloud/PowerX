<template>
  <div class="workflow-detail-page">
    <div class="detail-header">
      <div>
        <UButton icon="i-heroicons-arrow-left" color="neutral" variant="ghost" to="/workflow">
          {{ t("common.backToList") }}
        </UButton>
        <h1>{{ t("workflow.instance.title") }}</h1>
      </div>
      <UButton icon="i-heroicons-arrow-path" color="neutral" variant="ghost" :loading="loading" @click="loadInstance">
        {{ t("common.refresh") }}
      </UButton>
    </div>

    <UCard v-if="instance">
      <div class="instance-summary">
        <div>
          <p class="label">{{ t("workflow.instance.state") }}</p>
          <UBadge :color="stateColor(instance.state)" variant="soft">{{ stateLabel(instance.state) }}</UBadge>
        </div>
        <div>
          <p class="label">{{ t("workflow.instance.definition") }}</p>
          <p class="value">{{ shortUUID(instance.definition_uuid) }}</p>
        </div>
        <div>
          <p class="label">{{ t("workflow.instance.trace") }}</p>
          <p class="value">{{ instance.trace_id || t("common.unknown") }}</p>
        </div>
      </div>
    </UCard>

    <UCard class="mt-4">
      <template #header>
        <h2>{{ t("workflow.instance.steps") }}</h2>
      </template>
      <div v-if="!instance?.steps?.length" class="empty">{{ t("workflow.instance.noSteps") }}</div>
      <div v-else class="space-y-2">
        <div v-for="step in instance.steps" :key="step.step_id" class="step-row">
          <div>
            <div class="step-title">{{ step.step_id }}</div>
            <div class="step-meta">{{ step.node_kind }}</div>
          </div>
          <UBadge :color="stateColor(step.state)" variant="soft">{{ step.state }}</UBadge>
        </div>
      </div>
    </UCard>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useI18n, useRoute } from "#imports";
import { useWorkflowService, type WorkflowInstance } from "~/composables/api/services/workflowService";

const { t } = useI18n();
const route = useRoute();
const workflowService = useWorkflowService();
const loading = ref(false);
const instance = ref<WorkflowInstance | null>(null);
const instanceUUID = computed(() => String(route.params.instance_uuid || ""));

async function loadInstance() {
  loading.value = true;
  try {
    instance.value = await workflowService.getInstance(instanceUUID.value, true);
  } finally {
    loading.value = false;
  }
}

function stateColor(state: string) {
  if (state === "succeeded" || state === "approved") return "success";
  if (state === "failed" || state === "rejected" || state === "canceled") return "error";
  if (state === "waiting" || state === "pending") return "warning";
  return "info";
}

function stateLabel(state: string) {
  const key = `workflow.state.${state}`;
  return t(key);
}

function shortUUID(value: string) {
  return value ? `${value.slice(0, 8)}...${value.slice(-6)}` : t("common.unknown");
}

onMounted(loadInstance);
</script>

<style scoped>
.workflow-detail-page {
  padding: 24px;
  max-width: 1180px;
  margin: 0 auto;
}

.detail-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
  margin-bottom: 16px;
}

.detail-header h1 {
  margin-top: 10px;
  font-size: 24px;
  font-weight: 700;
  color: var(--ui-text-highlighted);
}

.instance-summary {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
}

.label {
  font-size: 12px;
  color: var(--ui-text-muted);
  margin-bottom: 6px;
}

.value {
  color: var(--ui-text-highlighted);
  font-weight: 600;
}

.empty {
  color: var(--ui-text-muted);
  padding: 32px;
  text-align: center;
}

.step-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  border: 1px solid var(--ui-border);
  border-radius: 8px;
  padding: 12px;
}

.step-title {
  font-weight: 700;
  color: var(--ui-text-highlighted);
}

.step-meta {
  font-size: 12px;
  color: var(--ui-text-muted);
}
</style>
