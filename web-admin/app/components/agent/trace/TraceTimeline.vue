<template>
  <section class="overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-white/10 dark:bg-[#111a2b]">
    <header class="flex flex-wrap items-start justify-between gap-3 border-b border-gray-100 px-4 py-3 dark:border-white/10">
      <div>
        <h2 class="text-sm font-semibold text-gray-900 dark:text-[#fdfcff]">
          {{ t("agent.chat.traceTimeline.title") }}
        </h2>
        <p class="mt-1 text-xs text-gray-500 dark:text-[#d6e2ff]/70">
          {{ t("agent.chat.traceTimeline.description") }}
        </p>
      </div>
      <UBadge color="neutral" variant="soft">
        {{ t("agent.chat.traceTimeline.nodeCount", { count: items.length }) }}
      </UBadge>
    </header>

    <ol v-if="items.length" class="px-4 py-2">
      <li
        v-for="(item, index) in items"
        :key="item.key"
        class="relative pl-10"
      >
        <div
          v-if="index < items.length - 1"
          class="absolute bottom-[-0.5rem] left-[15px] top-9 w-px bg-gray-200 dark:bg-white/15"
        />
        <div
          class="absolute left-1 top-5 flex h-6 w-6 items-center justify-center rounded-full border-2 bg-white dark:bg-[#111a2b]"
          :class="statusDotClass(item.status)"
        >
          <UIcon :name="statusIcon(item.status)" class="h-3.5 w-3.5" />
        </div>

        <button
          type="button"
          class="my-2 w-full rounded-lg border p-3 text-left transition-colors"
          :class="item.nodeId === selectedNodeId
            ? 'border-primary-400 bg-primary-50/70 dark:border-primary-400/70 dark:bg-primary-400/10'
            : 'border-gray-200 bg-white hover:border-primary-300 hover:bg-gray-50 dark:border-white/10 dark:bg-white/[0.03] dark:hover:border-primary-400/50 dark:hover:bg-white/[0.06]'"
          :aria-current="item.nodeId === selectedNodeId ? 'step' : undefined"
          @click="$emit('select', item.nodeId)"
        >
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <span class="text-xs font-medium text-gray-400 dark:text-[#d6e2ff]/60">
                  {{ t("agent.chat.traceTimeline.step", { index: index + 1 }) }}
                </span>
                <span class="text-sm font-semibold text-gray-900 dark:text-[#fdfcff]">
                  {{ formatNodeKind(item.nodeKind) }}
                </span>
                <span class="truncate font-mono text-xs text-gray-500 dark:text-[#d6e2ff]/70">
                  {{ item.nodeRef }}
                </span>
              </div>
              <div class="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-gray-500 dark:text-[#d6e2ff]/70">
                <span>{{ formatTime(item.startedAt) }}</span>
                <span>{{ formatOffset(item.startedAt) }}</span>
                <span>{{ t("agent.chat.traceTimeline.sequence", { sequence: displaySequence(item.nodeSeq) }) }}</span>
              </div>
            </div>
            <div class="flex items-center gap-2">
              <span class="text-xs tabular-nums text-gray-500 dark:text-[#d6e2ff]/70">
                {{ formatDuration(item.durationMs) }}
              </span>
              <UBadge :color="statusColor(item.status)" variant="soft">
                {{ t(`agent.chat.traceTimeline.status.${item.status}`) }}
              </UBadge>
            </div>
          </div>

          <div
            v-if="item.status === 'error'"
            class="mt-3 rounded-md border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700 dark:border-red-400/30 dark:bg-red-400/10 dark:text-red-200"
          >
            <div v-if="item.errorCode" class="font-mono font-medium">{{ item.errorCode }}</div>
            <div v-if="item.errorSummary" class="mt-1 whitespace-pre-wrap">{{ item.errorSummary }}</div>
            <div v-if="!item.errorCode && !item.errorSummary">
              {{ t("agent.chat.traceTimeline.errorWithoutDetails") }}
            </div>
          </div>
        </button>
      </li>
    </ol>

    <div v-else class="px-4 py-10 text-center text-sm text-gray-500 dark:text-[#d6e2ff]/70">
      {{ t("agent.chat.traceTimeline.empty") }}
    </div>
  </section>
</template>

<script setup lang="ts">
import type { AgentTraceEvent } from "~/composables/api/types/agentTrace";
import {
  buildAgentTraceTimeline,
  type AgentTraceTimelineStatus,
} from "~/utils/agent/traceTimeline";

const props = defineProps<{
  events: AgentTraceEvent[];
  selectedNodeId?: string;
}>();

defineEmits<{
  select: [nodeId: string];
}>();

const { locale, t } = useI18n();
const items = computed(() => buildAgentTraceTimeline(props.events));
const startTime = computed(() =>
  items.value.length ? new Date(items.value[0].startedAt).getTime() : 0
);

const nodeKindKeys: Record<string, string> = {
  receive_message: "receiveMessage",
  response_planner: "responsePlanner",
  intent_recognition: "intentRecognition",
  planner: "planner",
  context_builder: "contextBuilder",
  agent_handoff: "agentHandoff",
  skill: "skill",
  tooling: "tooling",
  workflow: "workflow",
  llm_call: "llmCall",
  final_response: "finalResponse",
};

const formatNodeKind = (kind: string) => {
  const key = nodeKindKeys[String(kind || "").trim()];
  return key ? t(`agent.chat.traceTimeline.nodeKinds.${key}`) : kind;
};

const formatTime = (value: string) => {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat(locale.value, {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    fractionalSecondDigits: 3,
    hour12: false,
  }).format(date);
};

const formatOffset = (value: string) => {
  const offset = Math.max(0, new Date(value).getTime() - startTime.value);
  return `+${formatDuration(offset)}`;
};

const formatDuration = (milliseconds: number) => {
  if (milliseconds < 1000) return `${milliseconds}ms`;
  return `${(milliseconds / 1000).toFixed(milliseconds < 10000 ? 2 : 1)}s`;
};

const displaySequence = (sequence: number) =>
  Number.isSafeInteger(sequence) && sequence > 0 ? sequence : "-";

const statusColor = (status: AgentTraceTimelineStatus) => {
  if (status === "error") return "error" as const;
  if (status === "success") return "success" as const;
  return "info" as const;
};

const statusIcon = (status: AgentTraceTimelineStatus) => {
  if (status === "error") return "i-heroicons-x-mark";
  if (status === "success") return "i-heroicons-check";
  return "i-heroicons-ellipsis-horizontal";
};

const statusDotClass = (status: AgentTraceTimelineStatus) => {
  if (status === "error") return "border-red-500 text-red-600 dark:border-red-400 dark:text-red-300";
  if (status === "success") return "border-emerald-500 text-emerald-600 dark:border-emerald-400 dark:text-emerald-300";
  return "border-sky-500 text-sky-600 dark:border-sky-400 dark:text-sky-300";
};
</script>
