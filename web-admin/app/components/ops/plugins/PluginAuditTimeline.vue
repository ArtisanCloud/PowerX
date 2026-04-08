<template>
  <ol class="space-y-3">
    <li
      v-for="item in items"
      :key="String(item.id)"
      class="rounded border border-gray-200 bg-white p-3"
    >
      <div class="flex items-center justify-between text-sm">
        <span class="font-medium text-gray-900">{{ item.action }}: {{ item.from_version || "-" }} -> {{ item.to_version || "-" }}</span>
        <span class="text-xs text-gray-500">{{ item.result }}</span>
      </div>
      <p class="mt-1 text-xs text-gray-600">{{ item.gate_reason || item.detail || "无备注" }}</p>
      <p class="mt-1 text-[11px] text-gray-400">operator={{ item.operator }} trace={{ item.trace_id || "-" }}</p>
    </li>
    <li v-if="items.length === 0" class="rounded border border-dashed border-gray-200 p-4 text-center text-sm text-gray-400">
      暂无审计记录
    </li>
  </ol>
</template>

<script setup lang="ts">
import type { PluginLifecycleAuditRecord } from "~/composables/api/services/pluginOpsService";

defineProps<{
  items: PluginLifecycleAuditRecord[];
}>();
</script>
