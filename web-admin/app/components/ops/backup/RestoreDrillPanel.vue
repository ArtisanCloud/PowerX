<template>
  <div class="rounded-lg border border-gray-200 bg-white p-4">
    <div class="mb-2 flex items-center justify-between">
      <h3 class="text-sm font-medium text-gray-700">恢复演练</h3>
      <span class="text-xs" :class="streaming ? 'text-emerald-600' : 'text-gray-400'">{{ streaming ? "实时推送已连接" : "实时推送未连接" }}</span>
    </div>
    <p class="text-sm text-gray-500">最近一次演练状态：{{ drill?.status || "-" }}</p>
    <p class="text-sm text-gray-500">RTO(秒)：{{ drill?.rto_seconds ?? "-" }}</p>
    <p class="text-xs text-gray-400">报告：{{ drill?.report_uri || "无" }}</p>

    <div class="mt-3 border-t border-gray-100 pt-3">
      <p class="mb-2 text-xs font-semibold text-gray-500">演练历史（最近 10 条）</p>
      <div v-for="item in history" :key="String(item.id)" class="mb-2 rounded border border-gray-100 p-2 text-xs">
        <div class="flex items-center justify-between">
          <span class="font-mono">#{{ item.id }}</span>
          <span :class="item.status === 'success' ? 'text-emerald-600' : item.status === 'failed' ? 'text-red-600' : 'text-amber-600'">{{ item.status }}</span>
        </div>
        <div class="text-gray-500">source_job: {{ item.source_job_id }} · rto: {{ item.rto_seconds ?? "-" }}s</div>
        <div class="text-gray-500">trace: {{ item.trace_id || "-" }}</div>
      </div>
      <p v-if="history.length === 0" class="text-xs text-gray-400">暂无演练历史</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { RestoreDrillRecord } from "~/composables/api/services/backupOpsService";

defineProps<{
  drill: RestoreDrillRecord | null;
  history: RestoreDrillRecord[];
  streaming?: boolean;
}>();
</script>
