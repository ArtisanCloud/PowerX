<template>
  <aside class="rounded-lg border border-gray-200 bg-white p-4 xl:sticky xl:top-5 xl:self-start">
    <div v-if="node" class="space-y-4">
      <div class="flex items-start justify-between gap-3">
        <div class="min-w-0">
          <div class="truncate text-base font-semibold text-gray-900">{{ formatNodeKind(node.node_kind) }}</div>
          <div class="truncate text-xs text-gray-500">{{ node.node_id }}</div>
        </div>
        <UBadge :color="node.phase_status === 'error' ? 'error' : 'success'" variant="soft">
          {{ formatStatus(node.phase_status) }}
        </UBadge>
      </div>
      <dl v-if="metadata.length" class="grid grid-cols-2 gap-3 text-sm">
        <div v-for="item in metadata" :key="item.label">
          <dt class="text-xs text-gray-500">{{ item.label }}</dt>
          <dd class="truncate text-gray-900">{{ item.value }}</dd>
        </div>
      </dl>
      <div v-if="node.error_summary" class="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-700">
        {{ node.error_summary }}
      </div>
      <div v-if="responsePlanVisible" class="rounded-md border border-sky-200 bg-sky-50 p-3 text-sm text-sky-900">
        <div class="mb-2 font-medium">{{ t("agent.chat.traceNodeDetails.responsePlan") }}</div>
        <dl class="grid grid-cols-2 gap-2">
          <div v-if="responseMode">
            <dt class="text-xs text-sky-600">{{ t("agent.chat.traceNodeDetails.mode") }}</dt>
            <dd class="truncate">{{ responseMode }}</dd>
          </div>
          <div v-if="modelName">
            <dt class="text-xs text-sky-600">{{ t("agent.chat.traceNodeDetails.model") }}</dt>
            <dd class="truncate">{{ modelName }}</dd>
          </div>
          <div v-if="targetCapabilities.length" class="col-span-2">
            <dt class="text-xs text-sky-600">{{ t("agent.chat.traceNodeDetails.capabilities") }}</dt>
            <dd class="truncate">{{ targetCapabilities.join(", ") }}</dd>
          </div>
          <div v-if="contextLayers.length" class="col-span-2">
            <dt class="text-xs text-sky-600">{{ t("agent.chat.traceNodeDetails.contextLayers") }}</dt>
            <dd class="truncate">{{ contextLayers.join(", ") }}</dd>
          </div>
        </dl>
      </div>
      <div>
        <div class="mb-2 text-xs font-medium text-gray-500">{{ t("agent.chat.traceNodeDetails.input") }}</div>
        <pre class="max-h-56 overflow-auto rounded-md bg-gray-950 p-3 text-xs text-gray-100">{{ format(node.input_summary) }}</pre>
      </div>
      <div>
        <div class="mb-2 text-xs font-medium text-gray-500">{{ t("agent.chat.traceNodeDetails.output") }}</div>
        <pre class="max-h-56 overflow-auto rounded-md bg-gray-950 p-3 text-xs text-gray-100">{{ format(node.output_summary) }}</pre>
      </div>
    </div>
    <div v-else class="py-12 text-center text-sm text-gray-500">{{ t("agent.chat.traceNodeDetails.empty") }}</div>
  </aside>
</template>

<script setup lang="ts">
import type { AgentTraceNode } from "~/composables/api/types/agentTrace";

const props = defineProps<{
  node?: AgentTraceNode | null;
}>();

const { t } = useI18n();

const format = (value: unknown) => JSON.stringify(value ?? {}, null, 2);

const attrs = computed(() => props.node?.attributes || {});
const output = computed(() => props.node?.output_summary || {});
const metadata = computed(() => [
  { label: t("agent.chat.traceNodeDetails.nodeRef"), value: normalizeOptional(props.node?.node_ref) },
  { label: t("agent.chat.traceNodeDetails.capability"), value: normalizeOptional(props.node?.capability_id) },
  { label: t("agent.chat.traceNodeDetails.skill"), value: normalizeOptional(props.node?.skill_id) },
  { label: t("agent.chat.traceNodeDetails.plugin"), value: normalizeOptional(props.node?.plugin_id) },
].filter((item) => item.value));

const responseMode = computed(() => readString(attrs.value.response_mode) || readString(output.value.response_mode));
const targetCapabilities = computed(() => readList(attrs.value.target_capability_ids || output.value.target_capability_ids));
const contextLayers = computed(() => readList(attrs.value.used_context_layers || output.value.used_context_layers));
const modelName = computed(() => {
  const selection = (attrs.value.model_selection || output.value.model_selection) as Record<string, unknown> | undefined;
  return readString(selection?.model) || readString(selection?.provider);
});
const responsePlanVisible = computed(() => Boolean(responseMode.value || targetCapabilities.value.length || contextLayers.value.length || modelName.value));

function readString(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function normalizeOptional(value: unknown): string {
  const normalized = readString(value);
  return ["<nil>", "null", "undefined"].includes(normalized.toLowerCase()) ? "" : normalized;
}

function readList(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.map((item) => String(item).trim()).filter(Boolean);
}

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

function formatNodeKind(kind: string): string {
  const key = nodeKindKeys[String(kind || "").trim()];
  return key ? t(`agent.chat.traceTimeline.nodeKinds.${key}`) : kind;
}

function formatStatus(status: string): string {
  const normalized = String(status || "running").trim().toLowerCase();
  return t(`agent.chat.traceTimeline.status.${normalized}`);
}
</script>
