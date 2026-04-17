<template>
  <div class="rounded-lg border border-gray-200 bg-white p-4">
    <h3 class="mb-3 text-sm font-medium text-gray-700">备份任务</h3>
    <div class="overflow-x-auto">
      <table class="min-w-full table-fixed text-sm">
        <colgroup>
          <col class="w-14" />
          <col class="w-14" />
          <col class="w-24" />
          <col class="w-24" />
          <col class="w-72" />
          <col class="w-[36rem]" />
          <col class="w-24" />
          <col class="w-56" />
          <col class="w-32" />
        </colgroup>
        <thead>
          <tr class="border-b border-gray-200 text-left text-gray-500">
            <th class="px-2 py-2">任务ID</th>
            <th class="px-2 py-2">策略ID</th>
            <th class="px-2 py-2">状态</th>
            <th class="px-2 py-2">触发类型</th>
            <th class="px-2 py-2">Trace</th>
            <th class="px-2 py-2">产物路径</th>
            <th class="px-2 py-2">大小</th>
            <th class="px-2 py-2">错误</th>
            <th class="px-2 py-2">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="job in items" :key="String(job.id)" class="border-b border-gray-100">
            <td class="px-2 py-2 align-top">{{ job.id }}</td>
            <td class="px-2 py-2 align-top">{{ job.policy_id }}</td>
            <td class="px-2 py-2 align-top">
              <span
                class="inline-flex rounded-full px-2 py-0.5 text-xs font-medium"
                :class="job.status === 'success' ? 'bg-emerald-500/15 text-emerald-600' : job.status === 'failed' ? 'bg-red-500/15 text-red-600' : 'bg-gray-500/15 text-gray-300'"
              >
                {{ job.status }}
              </span>
            </td>
            <td class="px-2 py-2 align-top">
              <span class="inline-flex rounded-full bg-blue-500/15 px-2 py-0.5 text-xs font-medium text-blue-300">
                {{ job.trigger_type }}
              </span>
            </td>
            <td class="px-2 py-2 align-top break-all whitespace-normal text-xs leading-5 text-gray-500">{{ job.trace_id || "-" }}</td>
            <td class="px-2 py-2 align-top break-all whitespace-normal text-xs leading-5 text-gray-400">{{ job.storage_uri || "-" }}</td>
            <td class="px-2 py-2 align-top text-xs text-gray-300">{{ formatSize(job.size_bytes) }}</td>
            <td class="px-2 py-2 align-top break-all whitespace-normal text-xs leading-5 text-red-500">{{ job.error_message || "-" }}</td>
            <td class="px-2 py-2 align-middle">
              <div class="flex w-full items-center justify-center">
                <UButton
                  v-if="canRestore(job)"
                  size="xs"
                  color="secondary"
                  variant="outline"
                  class="w-full max-w-[112px] justify-center whitespace-nowrap"
                  @click="emit('restore-verify', job)"
                >
                  恢复验证
                </UButton>
                <span v-else class="text-xs text-gray-500">-</span>
              </div>
            </td>
          </tr>
          <tr v-if="items.length === 0">
            <td colspan="9" class="py-4 text-center text-gray-400">暂无任务</td>
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

const emit = defineEmits<{
  "restore-verify": [job: BackupJob];
}>();

const formatSize = (bytes?: number) => {
  if (!bytes || bytes <= 0) return "-";
  if (bytes < 1024) return `${bytes} B`;
  const kb = bytes / 1024;
  if (kb < 1024) return `${kb.toFixed(1)} KB`;
  const mb = kb / 1024;
  if (mb < 1024) return `${mb.toFixed(1)} MB`;
  const gb = mb / 1024;
  return `${gb.toFixed(2)} GB`;
};

const canRestore = (job: BackupJob) => {
  return job.status === "success" && Boolean(job.storage_uri);
};
</script>
