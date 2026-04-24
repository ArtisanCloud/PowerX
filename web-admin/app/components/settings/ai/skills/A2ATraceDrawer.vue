<template>
  <UModal v-model:open="open" title="A2A 协作审计（按 Team / Handoff）" :ui="{ content: 'max-w-5xl w-full' }">
    <template #body>
      <div class="space-y-3">
        <div class="grid grid-cols-1 gap-3 md:grid-cols-4">
          <UInput v-model="teamIdInput" placeholder="team_id（必填）" />
          <UInput v-model="handoffTaskIdInput" placeholder="handoff_task_id（可选）" />
          <UInput v-model="handoffTraceIdInput" placeholder="handoff_trace_id（可选）" />
          <UButton :loading="loading" @click="fetchTraces">查询</UButton>
        </div>

        <div v-if="loading" class="text-sm text-[var(--text-secondary)]">加载中...</div>
        <div v-else-if="items.length === 0" class="text-sm text-[var(--text-secondary)]">暂无协作审计轨迹</div>

        <div v-else class="space-y-2">
          <div class="text-xs text-[var(--text-secondary)]">共 {{ items.length }} 条记录</div>
          <ul class="space-y-2">
            <li
              v-for="item in items"
              :key="`${item.trace_id}-${item.node_id}-${item.created_at}`"
              class="rounded border border-[var(--border-color)] p-3"
            >
              <div class="flex flex-wrap items-center gap-2">
                <UBadge size="xs" variant="soft">{{ item.node_status || item.status || "-" }}</UBadge>
                <span class="text-xs text-[var(--text-secondary)]">team: {{ item.team_id || "-" }}</span>
                <span class="text-xs text-[var(--text-secondary)]">task: {{ item.handoff_task_id || "-" }}</span>
                <span class="text-xs text-[var(--text-secondary)]">trace: {{ item.trace_id || "-" }}</span>
                <span class="text-xs text-[var(--text-secondary)]">node: {{ item.node_id || "-" }}</span>
                <span class="text-xs text-[var(--text-secondary)]">{{ item.created_at || "-" }}</span>
              </div>
              <div class="mt-1 text-xs text-[var(--text-secondary)]">
                <span>protocol: {{ item.protocol_used || "-" }}</span>
                <span class="ml-2">skill: {{ item.skill_id || "-" }}</span>
              </div>
              <p v-if="item.error_summary || item.retry_trace" class="mt-1 text-xs text-red-500">
                error: {{ item.error_summary || item.retry_trace }}
              </p>
            </li>
          </ul>
        </div>
      </div>
    </template>
  </UModal>
</template>

<script setup lang="ts">
import { useSkillsService, type SkillTraceRecord } from "~/composables/api/services";

const props = defineProps<{
  modelValue: boolean;
  initialTeamId?: string;
  initialHandoffTaskId?: string;
  initialHandoffTraceId?: string;
}>();

const emit = defineEmits<{
  "update:modelValue": [value: boolean];
}>();

const skillsService = useSkillsService();
const toast = useToast();
const loading = ref(false);
const items = ref<SkillTraceRecord[]>([]);
const teamIdInput = ref("");
const handoffTaskIdInput = ref("");
const handoffTraceIdInput = ref("");

const open = computed({
  get: () => props.modelValue,
  set: (value: boolean) => emit("update:modelValue", value),
});

async function fetchTraces() {
  const teamId = teamIdInput.value.trim();
  if (!teamId) {
    toast.add({ title: "请输入 team_id", color: "warning" });
    return;
  }
  loading.value = true;
  try {
    const res = await skillsService.listTraces({
      team_id: teamId,
      handoff_task_id: handoffTaskIdInput.value.trim() || undefined,
      handoff_trace_id: handoffTraceIdInput.value.trim() || undefined,
      limit: 200,
      offset: 0,
    });
    items.value = (res.items || [])
      .slice()
      .sort((a, b) =>
        String(a.created_at || "").localeCompare(String(b.created_at || ""))
      );
  } catch (error: any) {
    toast.add({
      title: "查询失败",
      description: error?.message || "请求失败",
      color: "error",
    });
  } finally {
    loading.value = false;
  }
}

watch(
  () => [open.value, props.initialTeamId, props.initialHandoffTaskId, props.initialHandoffTraceId],
  async ([opened, initialTeamId, initialTaskId, initialTraceId]) => {
    if (!opened) return;
    teamIdInput.value = String(initialTeamId || "").trim();
    handoffTaskIdInput.value = String(initialTaskId || "").trim();
    handoffTraceIdInput.value = String(initialTraceId || "").trim();
    if (teamIdInput.value) {
      await fetchTraces();
    }
  },
  { immediate: true }
);
</script>
