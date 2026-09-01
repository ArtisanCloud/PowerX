<template>
  <UModal
    v-model:open="open"
    :title="t('agent.chat.traceReport.title')"
    :ui="{ content: 'max-w-7xl w-[96vw]' }"
  >
    <template #body>
      <div v-if="loading" class="py-12 text-center text-sm text-gray-500">
        {{ t('agent.chat.traceReport.loading') }}
      </div>
      <UAlert
        v-else-if="loadError"
        color="error"
        variant="soft"
        :title="t('agent.chat.traceReport.loadFailed')"
        :description="loadError"
      />
      <div v-else-if="report" class="space-y-4">
        <div class="flex flex-wrap items-center justify-between gap-3 rounded-md border border-gray-200 bg-gray-50 p-3 text-xs text-gray-600">
          <div class="flex flex-wrap gap-x-4 gap-y-1">
            <span>{{ t('agent.chat.traceReport.runId') }}: <code>{{ report.run_id }}</code></span>
            <span>{{ t('agent.chat.traceReport.traceId') }}: <code>{{ report.trace_id }}</code></span>
          </div>
          <UBadge :color="isFailed ? 'error' : 'success'" variant="soft">
            {{ String(report.summary?.status || '-') }}
          </UBadge>
        </div>
        <div class="grid max-h-[72dvh] gap-5 overflow-y-auto pr-1 xl:grid-cols-[minmax(0,1fr)_460px]">
          <TraceTimeline
            :events="timeline"
            :selected-node-id="selectedNode?.node_id"
            @select="selectedNodeID = $event"
          />
          <TraceNodeDetails :node="selectedNode" />
        </div>
      </div>
      <div v-else class="py-12 text-center text-sm text-gray-500">
        {{ t('agent.chat.traceReport.empty') }}
      </div>
    </template>
  </UModal>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useAgentTraceService } from "~/composables/api/services/agentTraceService";
import type { AgentTraceNode, AgentTraceQuery, AgentTraceReport } from "~/composables/api/types/agentTrace";
import { filterTraceEventsForRun } from "~/utils/agent/traceTimeline";
import TraceNodeDetails from "./TraceNodeDetails.vue";
import TraceTimeline from "./TraceTimeline.vue";

const props = defineProps<{
  modelValue: boolean;
  query?: AgentTraceQuery | null;
}>();

const emit = defineEmits<{
  "update:modelValue": [value: boolean];
}>();

const { t } = useI18n();
const service = useAgentTraceService();
const loading = ref(false);
const loadError = ref("");
const report = ref<AgentTraceReport | null>(null);
const selectedNodeID = ref("");

const open = computed({
  get: () => props.modelValue,
  set: (value: boolean) => emit("update:modelValue", value),
});
const timeline = computed(() => filterTraceEventsForRun(report.value?.timeline || [], String(report.value?.run_id || "")));
const selectedNode = computed<AgentTraceNode | null>(() =>
  report.value?.nodes?.find((node) => node.node_id === selectedNodeID.value) || report.value?.nodes?.[0] || null,
);
const isFailed = computed(() => String(report.value?.summary?.status || "").toLowerCase() === "failed");

const load = async () => {
  if (!props.query?.tenant_uuid || !props.query.session_id || !props.query.message_id || !props.query.run_id) {
    loadError.value = t("agent.chat.traceReport.missingIdentity");
    report.value = null;
    return;
  }
  loading.value = true;
  loadError.value = "";
  try {
    report.value = await service.getReport(props.query);
    selectedNodeID.value = report.value.nodes?.[0]?.node_id || "";
  } catch (error) {
    report.value = null;
    loadError.value = error instanceof Error ? error.message : t("agent.chat.traceReport.loadFailed");
  } finally {
    loading.value = false;
  }
};

watch(
  () => [open.value, props.query?.tenant_uuid, props.query?.session_id, props.query?.message_id, props.query?.run_id],
  ([isOpen]) => {
    if (isOpen) void load();
  },
  { immediate: true },
);
</script>
