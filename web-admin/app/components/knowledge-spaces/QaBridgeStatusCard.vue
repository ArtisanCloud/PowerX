<template>
  <div class="rounded-lg border border-gray-200 bg-white p-4 shadow-sm">
    <div class="mb-4 flex items-center justify-between">
      <div>
        <p class="text-sm text-gray-500">{{ t("knowledgeSpaces.qaCard.badge") }}</p>
        <h3 class="text-lg font-semibold">{{ t("knowledgeSpaces.qaCard.title") }}</h3>
      </div>
      <button
        class="rounded border border-gray-300 px-3 py-1 text-sm text-gray-700 hover:bg-gray-50"
        @click="$emit('refresh')"
      >
        {{ t("knowledgeSpaces.qaCard.refresh") }}
      </button>
    </div>

    <div
      v-if="status.degradeCount > 0"
      class="mb-4 rounded bg-orange-100 px-3 py-1 text-sm text-orange-700"
      data-test="degrade-badge"
    >
      {{ degradeLabel }}
    </div>

    <div class="grid grid-cols-3 gap-4">
      <div>
        <p class="text-xs text-gray-500">{{ t("knowledgeSpaces.qaCard.metrics.latency") }}</p>
        <p class="text-xl font-semibold" data-test="latency">
          {{ latencySeconds }}s
        </p>
      </div>
      <div>
        <p class="text-xs text-gray-500">{{ t("knowledgeSpaces.qaCard.metrics.coverage") }}</p>
        <p class="text-xl font-semibold" data-test="coverage">
          {{ coveragePercent }}%
        </p>
      </div>
      <div>
        <p class="text-xs text-gray-500">{{ t("knowledgeSpaces.qaCard.metrics.toolSuccess") }}</p>
        <p class="text-xl font-semibold" data-test="tool-success">
          {{ toolSuccessPercent }}%
        </p>
      </div>
    </div>

    <p v-if="status.lastAuditId" class="mt-4 text-sm text-gray-600" data-test="audit-link">
      {{ t("knowledgeSpaces.qaCard.audit", { id: status.lastAuditId }) }}
    </p>
    <p v-if="status.lastUpdatedAt" class="text-xs text-gray-400">
      {{ t("knowledgeSpaces.qaCard.updatedAt", { time: new Date(status.lastUpdatedAt).toLocaleString() }) }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "#imports";

interface QaBridgeStatus {
  latencyMsP95: number;
  citationCoverage: number;
  toolSuccessRate: number;
  degradeCount: number;
  lastAuditId?: string;
  lastUpdatedAt?: string;
}

const props = defineProps<{
  status: QaBridgeStatus;
}>();

const { t } = useI18n();

defineEmits<{
  (e: "refresh"): void;
}>();

const latencySeconds = computed(
  () => Math.round((props.status.latencyMsP95 ?? 0) / 100) / 10,
);
const coveragePercent = computed(
  () => Math.round((props.status.citationCoverage ?? 0) * 100),
);
const toolSuccessPercent = computed(
  () => Math.round((props.status.toolSuccessRate ?? 0) * 100),
);

const degradeLabel = computed(() =>
  t("knowledgeSpaces.qaCard.degrade", { count: props.status.degradeCount }),
);
</script>

<style scoped></style>
