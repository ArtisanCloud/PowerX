<template>
  <div class="rounded-xl border border-[var(--border-color)] bg-[var(--card-bg)] p-4 sm:p-5 shadow-sm">
    <div class="flex items-start justify-between gap-3">
      <div class="min-w-0">
        <div class="flex items-center gap-2">
          <span class="inline-flex items-center rounded-full bg-primary-500/15 px-2 py-0.5 text-xs font-medium text-primary-500">
            {{ t("knowledgeSpaces.qaCard.badge") }}
          </span>
          <h3 class="truncate text-base font-semibold text-[var(--text-primary)]">
            {{ t("knowledgeSpaces.qaCard.title") }}
          </h3>
        </div>
        <p v-if="status.lastUpdatedAt" class="mt-1 text-xs text-[var(--text-secondary)]">
          {{ t("knowledgeSpaces.qaCard.updatedAt", { time: new Date(status.lastUpdatedAt).toLocaleString() }) }}
        </p>
      </div>

      <button
        type="button"
        class="inline-flex items-center gap-2 rounded-md border border-[var(--border-color)] bg-transparent px-3 py-1.5 text-sm text-[var(--text-primary)] transition hover:bg-[var(--hover-bg)]"
        @click="$emit('refresh')"
      >
        {{ t("knowledgeSpaces.qaCard.refresh") }}
      </button>
    </div>

    <div
      v-if="status.degradeCount > 0"
      class="mt-3 inline-flex rounded-md border border-orange-300/50 bg-orange-100/40 px-3 py-1 text-sm text-orange-700 dark:border-orange-500/30 dark:bg-orange-500/10 dark:text-orange-300"
      data-test="degrade-badge"
    >
      {{ degradeLabel }}
    </div>

    <div class="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-3">
      <div class="rounded-lg border border-[var(--border-color)] bg-[var(--bg-secondary)] p-3">
        <p class="text-xs text-[var(--text-secondary)]">{{ t("knowledgeSpaces.qaCard.metrics.latency") }}</p>
        <p class="mt-1 text-2xl font-semibold text-[var(--text-primary)]" data-test="latency">
          {{ latencySeconds }}s
        </p>
      </div>
      <div class="rounded-lg border border-[var(--border-color)] bg-[var(--bg-secondary)] p-3">
        <p class="text-xs text-[var(--text-secondary)]">{{ t("knowledgeSpaces.qaCard.metrics.coverage") }}</p>
        <p class="mt-1 text-2xl font-semibold text-[var(--text-primary)]" data-test="coverage">
          {{ coveragePercent }}%
        </p>
      </div>
      <div class="rounded-lg border border-[var(--border-color)] bg-[var(--bg-secondary)] p-3">
        <p class="text-xs text-[var(--text-secondary)]">{{ t("knowledgeSpaces.qaCard.metrics.toolSuccess") }}</p>
        <p class="mt-1 text-2xl font-semibold text-[var(--text-primary)]" data-test="tool-success">
          {{ toolSuccessPercent }}%
        </p>
      </div>
    </div>

    <p v-if="status.lastAuditId" class="mt-4 text-sm text-[var(--text-secondary)]" data-test="audit-link">
      {{ t("knowledgeSpaces.qaCard.audit", { id: status.lastAuditId }) }}
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
