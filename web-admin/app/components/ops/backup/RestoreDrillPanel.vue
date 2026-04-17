<template>
  <div class="rounded-lg border border-gray-200 bg-white p-4">
    <div class="mb-2 flex items-center justify-between">
      <h3 class="text-sm font-medium text-gray-700">恢复数据任务</h3>
      <span class="text-xs" :class="streaming ? 'text-emerald-600' : 'text-gray-400'">{{ streaming ? "实时推送已连接" : "实时推送未连接" }}</span>
    </div>
    <div class="rounded border border-gray-100">
      <div class="grid grid-cols-8 gap-2 border-b border-gray-100 bg-gray-50 px-3 py-2 text-xs font-semibold text-gray-500">
        <span>任务</span>
        <span>状态</span>
        <span>目标库</span>
        <span>表数</span>
        <span>耗时</span>
        <span>告警</span>
        <span>Trace</span>
        <span>详情</span>
      </div>
      <template v-for="item in history" :key="String(item.id)">
        <div class="grid grid-cols-8 gap-2 border-b border-gray-100 px-3 py-2 text-xs text-gray-600">
          <span class="font-mono">#{{ item.id }} (源作业 {{ item.source_job_id }})</span>
          <span>
            <span :class="statusClass(item.status)">{{ item.status }}</span>
          </span>
          <span class="truncate" :title="item.restore_target_db || '-'">{{ item.restore_target_db || "-" }}</span>
          <span>{{ item.restored_table_count ?? "-" }}</span>
          <span>{{ item.rto_seconds ?? "-" }}s</span>
          <span>
            <UButton
              v-if="alertByTrace(item.trace_id)"
              size="xs"
              color="warning"
              variant="outline"
              @click="toggleAlert(item.trace_id)"
            >
              {{ selectedAlertTrace === (item.trace_id || "") ? "收起告警" : "查看告警" }}
            </UButton>
            <span v-else>-</span>
          </span>
          <span class="truncate" :title="item.trace_id || '-'">{{ item.trace_id || "-" }}</span>
          <span>
            <UButton size="xs" color="neutral" variant="outline" @click="toggleExpand(item.id)">
              {{ isExpanded(item.id) ? "收起" : "展开" }}
            </UButton>
          </span>
        </div>
        <div v-if="isExpanded(item.id)" class="border-b border-gray-100 bg-gray-50/50 px-3 py-3 text-sm text-gray-700">
          <p class="mb-2 text-xs font-semibold text-gray-500">任务详情：#{{ item.id }}（源作业 {{ item.source_job_id }}）</p>
          <p>状态：<span :class="statusClass(item.status)">{{ item.status || "-" }}</span></p>
          <p>目标库：{{ item.restore_target_db || "-" }}</p>
          <p>导入表数：{{ item.restored_table_count ?? "-" }}</p>
          <p>RTO：{{ item.rto_seconds ?? "-" }} 秒</p>
          <p>告警状态：{{ alertStatusLabel(item.trace_id) }}</p>
          <p class="mt-1 break-all text-xs text-gray-500">产物路径：{{ item.artifact_path || "-" }}</p>
          <p class="break-all text-xs text-gray-500">Trace：{{ item.trace_id || "-" }}</p>
          <details class="mt-1 text-xs text-gray-500">
            <summary class="cursor-pointer select-none">调试详情</summary>
            <p class="mt-1 break-all">{{ item.report_uri || "无" }}</p>
          </details>
        </div>
      </template>
      <p v-if="history.length === 0" class="px-3 py-3 text-xs text-gray-400">暂无恢复任务</p>
    </div>

    <div v-if="selectedAlert" class="mt-3 rounded border border-amber-300/40 bg-amber-500/10 p-3 text-xs text-amber-100">
      <div class="mb-1 flex items-center justify-between">
        <span class="font-semibold">关联告警详情</span>
        <span>{{ selectedAlert.alert_type }} / {{ selectedAlert.level }}</span>
      </div>
      <p class="break-all">message: {{ selectedAlert.message }}</p>
      <p class="mt-1">状态：{{ selectedAlert.acknowledged ? "已确认" : "未确认" }}</p>
      <p class="mt-1 break-all">trace: {{ selectedAlert.trace_id || "-" }}</p>
    </div>

  </div>
</template>

<script setup lang="ts">
import type { BackupAlert, RestoreDrillRecord } from "~/composables/api/services/backupOpsService";
import { computed, ref, watch } from "vue";

const props = defineProps<{
  drill: RestoreDrillRecord | null;
  history: RestoreDrillRecord[];
  alerts?: BackupAlert[];
  streaming?: boolean;
}>();
const selectedAlertTrace = ref("");
const expandedTaskId = ref("");

const statusClass = (status?: string) => {
  if (status === "success") return "text-emerald-600";
  if (status === "failed") return "text-red-600";
  return "text-gray-500";
};

const alertStatusLabel = (traceId?: string) => {
  if (!traceId || !props.alerts || props.alerts.length === 0) return "未告警";
  const matched = props.alerts.find((a) => (a.trace_id || "") === traceId);
  if (!matched) return "未告警";
  return matched.acknowledged ? `已确认（${matched.level}）` : `未确认（${matched.level}）`;
};

const alertByTrace = (traceId?: string) => {
  if (!traceId || !props.alerts || props.alerts.length === 0) return null;
  return props.alerts.find((a) => (a.trace_id || "") === traceId) || null;
};

const toggleAlert = (traceId?: string) => {
  const key = traceId || "";
  if (!key) return;
  selectedAlertTrace.value = selectedAlertTrace.value === key ? "" : key;
};

const selectedAlert = computed(() => alertByTrace(selectedAlertTrace.value));
const isExpanded = (id: string | number) => expandedTaskId.value === String(id);

const toggleExpand = (id: string | number) => {
  const key = String(id);
  expandedTaskId.value = expandedTaskId.value === key ? "" : key;
};

watch(
  () => [props.history, props.drill] as const,
  () => {
    if (expandedTaskId.value) return;
    if (props.history.length > 0) {
      expandedTaskId.value = String(props.history[0].id);
      return;
    }
    if (props.drill?.id) expandedTaskId.value = String(props.drill.id);
  },
  { immediate: true, deep: true },
);
</script>
