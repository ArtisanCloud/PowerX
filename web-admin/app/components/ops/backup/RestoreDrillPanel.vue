<template>
  <div class="rounded-lg border border-gray-200 bg-white p-4">
    <h3 class="mb-2 text-sm font-medium text-gray-700">恢复演练</h3>
    <p class="text-sm text-gray-500">最近一次演练状态：{{ drill?.status || "-" }}</p>
    <p class="text-sm text-gray-500">RTO(秒)：{{ drill?.rto_seconds ?? "-" }}</p>
    <p class="text-xs text-gray-400">报告：{{ drill?.report_uri || "无" }}</p>
  </div>
</template>

<script setup lang="ts">
import type { RestoreDrillRecord } from "~/composables/api/services/backupOpsService";

defineProps<{
  drill: RestoreDrillRecord | null;
}>();
</script>
