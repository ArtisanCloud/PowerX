<template>
  <div class="qa-bridge-card">
    <div class="card-header">
      <div>
        <p class="text-sm text-gray-500">QA Bridge</p>
        <h3 class="text-lg font-semibold">检索健康监控</h3>
      </div>
      <button class="refresh-btn" @click="$emit('refresh')">
        刷新
      </button>
    </div>

    <div
      v-if="status.degradeCount > 0"
      class="degrade-badge"
      data-test="degrade-badge"
    >
      {{ status.degradeCount }} 个空间降级
    </div>

    <div class="metrics">
      <div>
        <p class="metric-label">检索 P95</p>
        <p class="metric-value" data-test="latency">
          {{ latencySeconds }}s
        </p>
      </div>
      <div>
        <p class="metric-label">引用覆盖</p>
        <p class="metric-value" data-test="coverage">
          {{ coveragePercent }}%
        </p>
      </div>
      <div>
        <p class="metric-label">工具成功率</p>
        <p class="metric-value" data-test="tool-success">
          {{ toolSuccessPercent }}%
        </p>
      </div>
    </div>

    <p v-if="status.lastAuditId" class="audit" data-test="audit-link">
      最近审计：<span>{{ status.lastAuditId }}</span>
    </p>
    <p v-if="status.lastUpdatedAt" class="timestamp">
      更新时间：{{ new Date(status.lastUpdatedAt).toLocaleString() }}
    </p>
  </div>
</template>

<script setup lang="ts">
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
</script>

<style scoped>
.qa-bridge-card {
  @apply rounded-lg border border-gray-200 bg-white p-4 shadow-sm;
}
.card-header {
  @apply mb-4 flex items-center justify-between;
}
.refresh-btn {
  @apply rounded border border-gray-300 px-3 py-1 text-sm text-gray-700 hover:bg-gray-50;
}
.metrics {
  @apply grid grid-cols-3 gap-4;
}
.metric-label {
  @apply text-xs text-gray-500;
}
.metric-value {
  @apply text-xl font-semibold;
}
.degrade-badge {
  @apply mb-4 rounded bg-orange-100 px-3 py-1 text-sm text-orange-700;
}
.audit {
  @apply mt-4 text-sm text-gray-600;
}
.timestamp {
  @apply text-xs text-gray-400;
}
</style>
