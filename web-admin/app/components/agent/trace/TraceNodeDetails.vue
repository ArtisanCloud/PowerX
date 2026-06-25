<template>
  <div class="rounded-lg border border-gray-200 bg-white p-4">
    <div v-if="node" class="space-y-4">
      <div class="flex items-start justify-between gap-3">
        <div class="min-w-0">
          <div class="truncate text-base font-semibold text-gray-900">{{ node.node_kind }}</div>
          <div class="truncate text-xs text-gray-500">{{ node.node_id }}</div>
        </div>
        <UBadge :color="node.phase_status === 'error' ? 'error' : 'success'" variant="soft">
          {{ node.phase_status }}
        </UBadge>
      </div>
      <dl class="grid grid-cols-2 gap-3 text-sm">
        <div>
          <dt class="text-xs text-gray-500">Node Ref</dt>
          <dd class="truncate text-gray-900">{{ node.node_ref || "-" }}</dd>
        </div>
        <div>
          <dt class="text-xs text-gray-500">Capability</dt>
          <dd class="truncate text-gray-900">{{ node.capability_id || "-" }}</dd>
        </div>
        <div>
          <dt class="text-xs text-gray-500">Skill</dt>
          <dd class="truncate text-gray-900">{{ node.skill_id || "-" }}</dd>
        </div>
        <div>
          <dt class="text-xs text-gray-500">Plugin</dt>
          <dd class="truncate text-gray-900">{{ node.plugin_id || "-" }}</dd>
        </div>
      </dl>
      <div v-if="node.error_summary" class="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-700">
        {{ node.error_summary }}
      </div>
      <div v-if="responsePlanVisible" class="rounded-md border border-sky-200 bg-sky-50 p-3 text-sm text-sky-900">
        <div class="mb-2 font-medium">Response Plan</div>
        <dl class="grid grid-cols-2 gap-2">
          <div v-if="responseMode">
            <dt class="text-xs text-sky-600">Mode</dt>
            <dd class="truncate">{{ responseMode }}</dd>
          </div>
          <div v-if="modelName">
            <dt class="text-xs text-sky-600">Model</dt>
            <dd class="truncate">{{ modelName }}</dd>
          </div>
          <div v-if="targetCapabilities.length" class="col-span-2">
            <dt class="text-xs text-sky-600">Capabilities</dt>
            <dd class="truncate">{{ targetCapabilities.join(", ") }}</dd>
          </div>
          <div v-if="contextLayers.length" class="col-span-2">
            <dt class="text-xs text-sky-600">Context Layers</dt>
            <dd class="truncate">{{ contextLayers.join(", ") }}</dd>
          </div>
        </dl>
      </div>
      <div>
        <div class="mb-2 text-xs font-medium text-gray-500">Input</div>
        <pre class="max-h-56 overflow-auto rounded-md bg-gray-950 p-3 text-xs text-gray-100">{{ format(node.input_summary) }}</pre>
      </div>
      <div>
        <div class="mb-2 text-xs font-medium text-gray-500">Output</div>
        <pre class="max-h-56 overflow-auto rounded-md bg-gray-950 p-3 text-xs text-gray-100">{{ format(node.output_summary) }}</pre>
      </div>
    </div>
    <div v-else class="py-12 text-center text-sm text-gray-500">选择一个节点查看详情</div>
  </div>
</template>

<script setup lang="ts">
import type { AgentTraceNode } from "~/composables/api/types/agentTrace";

const props = defineProps<{
  node?: AgentTraceNode | null;
}>();

const format = (value: unknown) => JSON.stringify(value || {}, null, 2);

const attrs = computed(() => props.node?.attributes || {});
const output = computed(() => props.node?.output_summary || {});

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

function readList(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.map((item) => String(item).trim()).filter(Boolean);
}
</script>
