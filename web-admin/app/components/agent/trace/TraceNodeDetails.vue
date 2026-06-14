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

defineProps<{
  node?: AgentTraceNode | null;
}>();

const format = (value: unknown) => JSON.stringify(value || {}, null, 2);
</script>
