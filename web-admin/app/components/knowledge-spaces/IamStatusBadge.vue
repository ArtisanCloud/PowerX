<script setup lang="ts">
const props = defineProps<{
  status: "pending_iam" | "ready" | string;
  auditToken?: string;
}>();

const statusCopy: Record<string, { label: string; color: string; hint: string }> = {
  pending_iam: {
    label: "IAM 待确认",
    color: "orange",
    hint: "角色同步完成后向导会自动激活空间。",
  },
  ready: {
    label: "已激活",
    color: "green",
    hint: "可以继续触发入库与融合步骤。",
  },
};
</script>

<template>
  <div class="flex items-center gap-3 rounded-xl border border-gray-200 bg-white p-3">
    <UBadge :color="statusCopy[props.status]?.color || 'gray'" variant="soft">
      {{ statusCopy[props.status]?.label || "状态未知" }}
    </UBadge>
    <div class="text-sm text-gray-600">
      {{ statusCopy[props.status]?.hint || "等待最新状态" }}
      <span v-if="auditToken" class="ml-3 text-xs text-gray-400">
        审计令牌：{{ auditToken }}
      </span>
    </div>
  </div>
</template>
