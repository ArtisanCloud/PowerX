<template>
  <div class="overflow-hidden rounded-lg border border-gray-200 bg-white">
    <div class="grid grid-cols-[120px_1fr_96px_96px] border-b border-gray-100 bg-gray-50 px-3 py-2 text-xs font-medium text-gray-500">
      <div>节点</div>
      <div>引用</div>
      <div>阶段</div>
      <div>耗时</div>
    </div>
    <button
      v-for="event in events"
      :key="`${event.node_id}-${event.phase}-${event.created_at}`"
      type="button"
      class="grid w-full grid-cols-[120px_1fr_96px_96px] items-center border-b border-gray-100 px-3 py-2 text-left text-sm last:border-b-0 hover:bg-gray-50"
      @click="$emit('select', event.node_id)"
    >
      <div class="truncate font-medium text-gray-900">{{ event.node_kind }}</div>
      <div class="truncate text-gray-600">{{ event.node_ref || event.node_id }}</div>
      <div>
        <UBadge :color="event.status === 'error' ? 'error' : event.phase === 'start' ? 'neutral' : 'success'" variant="soft">
          {{ event.phase }}
        </UBadge>
      </div>
      <div class="text-xs text-gray-500">{{ event.duration_ms || 0 }}ms</div>
    </button>
    <div v-if="events.length === 0" class="px-3 py-8 text-center text-sm text-gray-500">暂无 Timeline</div>
  </div>
</template>

<script setup lang="ts">
import type { AgentTraceEvent } from "~/composables/api/types/agentTrace";

defineProps<{
  events: AgentTraceEvent[];
}>();

defineEmits<{
  select: [nodeId: string];
}>();
</script>
