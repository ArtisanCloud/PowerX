<template>
  <UModal v-model:open="open" :title="`审计记录 · ${skillId || '-'}`">
    <template #body>
      <div class="space-y-3">
        <div class="flex items-center justify-between">
          <span class="text-sm text-[var(--text-secondary)]">最近 {{ limit }} 条</span>
          <UButton size="xs" variant="ghost" :loading="loading" @click="fetchAudits">刷新</UButton>
        </div>
        <div v-if="loading" class="text-sm text-[var(--text-secondary)]">加载中...</div>
        <div v-else-if="items.length === 0" class="text-sm text-[var(--text-secondary)]">暂无审计记录</div>
        <ul v-else class="space-y-2">
          <li v-for="item in items" :key="item.audit_id" class="rounded border border-[var(--border-color)] p-3">
            <div class="flex flex-wrap items-center gap-2">
              <UBadge size="xs" variant="soft">{{ item.action }}</UBadge>
              <span class="text-xs text-[var(--text-secondary)]">{{ item.version }}</span>
              <span class="text-xs text-[var(--text-secondary)]">{{ item.operator }}</span>
              <span class="text-xs text-[var(--text-secondary)]">{{ item.created_at }}</span>
            </div>
            <div class="mt-1 text-xs text-[var(--text-secondary)]">
              <span>result: {{ item.result }}</span>
              <span v-if="item.trace_id" class="ml-2">trace: {{ item.trace_id }}</span>
            </div>
            <p v-if="item.reason" class="mt-1 text-xs">{{ item.reason }}</p>
          </li>
        </ul>
      </div>
    </template>
  </UModal>
</template>

<script setup lang="ts">
import { useSkillsService, type SkillAuditRecord } from "~/composables/api/services";

const props = defineProps<{
  modelValue: boolean;
  skillId: string;
  limit?: number;
}>();

const emit = defineEmits<{
  "update:modelValue": [value: boolean];
}>();

const skillsService = useSkillsService();
const toast = useToast();
const loading = ref(false);
const items = ref<SkillAuditRecord[]>([]);
const limit = computed(() => props.limit || 20);
const open = computed({
  get: () => props.modelValue,
  set: (value: boolean) => emit("update:modelValue", value),
});

async function fetchAudits() {
  if (!props.skillId) return;
  loading.value = true;
  try {
    const res = await skillsService.listAudits(props.skillId, limit.value);
    items.value = res.items || [];
  } catch (error: any) {
    toast.add({ title: "加载审计失败", description: error?.message || "请求失败", color: "error" });
  } finally {
    loading.value = false;
  }
}

watch(
  () => [open.value, props.skillId],
  async ([opened, skillId]) => {
    if (opened && skillId) {
      await fetchAudits();
    }
  },
  { immediate: true },
);
</script>
