<template>
  <div class="rounded-lg border border-gray-200 bg-white p-4">
    <h3 class="mb-3 text-sm font-medium text-gray-700">备份任务</h3>
    <div class="overflow-x-auto">
      <table class="min-w-full text-sm">
        <thead>
          <tr class="border-b border-gray-200 text-left text-gray-500">
            <th class="py-2">任务ID</th>
            <th class="py-2">策略ID</th>
            <th class="py-2">状态</th>
            <th class="py-2">触发类型</th>
            <th class="py-2">错误</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="job in items" :key="String(job.id)" class="border-b border-gray-100">
            <td class="py-2">{{ job.id }}</td>
            <td class="py-2">{{ job.policy_id }}</td>
            <td class="py-2">{{ job.status }}</td>
            <td class="py-2">{{ job.trigger_type }}</td>
            <td class="py-2 text-xs text-red-600">{{ job.error_message || "-" }}</td>
          </tr>
          <tr v-if="items.length === 0">
            <td colspan="5" class="py-4 text-center text-gray-400">暂无任务</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { BackupJob } from "~/composables/api/services/backupOpsService";

defineProps<{
  items: BackupJob[];
}>();
</script>
