<template>
  <UModal v-model:open="open" title="Planner 执行图（按 Plan ID）" :ui="{ content: 'max-w-4xl w-full' }">
    <template #body>
      <div class="space-y-3">
        <div class="grid grid-cols-1 gap-3 md:grid-cols-[1fr_auto]">
          <UInput v-model="planIdInput" placeholder="输入 plan_id，例如 plan_1742360000000" />
          <UButton :loading="loading" @click="fetchTraces">查询</UButton>
        </div>

        <div v-if="loading" class="text-sm text-[var(--text-secondary)]">加载中...</div>
        <div v-else-if="items.length === 0" class="text-sm text-[var(--text-secondary)]">暂无节点轨迹</div>

        <div v-else class="space-y-2">
          <div class="text-xs text-[var(--text-secondary)]">共 {{ items.length }} 条节点事件</div>
          <ul class="space-y-2">
            <li v-for="item in items" :key="`${item.trace_id}-${item.node_id}-${item.created_at}`" class="rounded border border-[var(--border-color)] p-3">
              <div class="flex flex-wrap items-center gap-2">
                <UBadge size="xs" variant="soft">{{ item.node_status || '-' }}</UBadge>
                <span class="text-xs text-[var(--text-secondary)]">node: {{ item.node_id || '-' }}</span>
                <span class="text-xs text-[var(--text-secondary)]">flow: {{ item.skill_id || '-' }}</span>
                <span class="text-xs text-[var(--text-secondary)]">trace: {{ item.trace_id || '-' }}</span>
                <span class="text-xs text-[var(--text-secondary)]">{{ item.created_at || '-' }}</span>
              </div>
              <p v-if="item.retry_trace" class="mt-1 text-xs text-red-500">error: {{ item.retry_trace }}</p>
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
  initialPlanId?: string;
}>();

const emit = defineEmits<{
  "update:modelValue": [value: boolean];
}>();

const skillsService = useSkillsService();
const toast = useToast();
const loading = ref(false);
const items = ref<SkillTraceRecord[]>([]);
const planIdInput = ref("");

const open = computed({
  get: () => props.modelValue,
  set: (value: boolean) => emit("update:modelValue", value),
});

async function fetchTraces() {
  const planId = planIdInput.value.trim();
  if (!planId) {
    toast.add({ title: "请输入 plan_id", color: "warning" });
    return;
  }
  loading.value = true;
  try {
    const res = await skillsService.listTraces({ plan_id: planId, limit: 200, offset: 0 });
    items.value = (res.items || []).slice().sort((a, b) => String(a.created_at || "").localeCompare(String(b.created_at || "")));
  } catch (error: any) {
    toast.add({ title: "查询失败", description: error?.message || "请求失败", color: "error" });
  } finally {
    loading.value = false;
  }
}

watch(
  () => [open.value, props.initialPlanId],
  async ([opened, initialPlanId]) => {
    if (!opened) return;
    const v = String(initialPlanId || "").trim();
    if (v) {
      planIdInput.value = v;
      await fetchTraces();
    }
  },
  { immediate: true },
);
</script>
