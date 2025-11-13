<script setup lang="ts">
const props = defineProps<{
  modelValue: string;
  featureFlags: string[];
  options: { label: string; value: string; description: string }[];
}>();

const emit = defineEmits<{
  (e: "update:modelValue", value: string): void;
  (e: "update:featureFlags", value: string[]): void;
}>();

const featureChoices = [
  {
    key: "masking.audit",
    label: "启用掩码审计",
    description: "生成审计摘要并在 Review 步提示审核者。",
  },
  {
    key: "fusion.guardrails",
    label: "启用融合冲突告警",
    description: "当融合策略降级或冲突时推送通知。",
  },
  {
    key: "ingestion.dual-chunk",
    label: "双粒度切片",
    description: "同时生成 800/300 token 两种粒度的 chunk。",
  },
];

const toggleFlag = (flag: string, enabled: boolean) => {
  const next = new Set(props.featureFlags);
  if (enabled) {
    next.add(flag);
  } else {
    next.delete(flag);
  }
  emit("update:featureFlags", Array.from(next));
};
</script>

<template>
  <div class="space-y-6">
    <label class="flex flex-col gap-2">
      <span class="text-sm font-medium text-gray-700">策略模版</span>
      <select
        class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
        :value="modelValue"
        @change="emit('update:modelValue', ($event.target as HTMLSelectElement).value)"
      >
        <option
          v-for="option in options"
          :key="option.value"
          :value="option.value"
        >
          {{ option.label }}
        </option>
      </select>
      <p class="text-xs text-gray-500">
        {{ options.find((o) => o.value === modelValue)?.description }}
      </p>
    </label>

    <div class="rounded-xl border border-gray-200 p-4">
      <p class="mb-3 text-sm font-medium text-gray-700">可选能力</p>
      <div class="space-y-3">
        <label
          v-for="choice in featureChoices"
          :key="choice.key"
          class="flex items-start gap-3"
        >
          <input
            type="checkbox"
            class="mt-1 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
            :checked="featureFlags.includes(choice.key)"
            @change="toggleFlag(choice.key, ($event.target as HTMLInputElement).checked)"
          />
          <span>
            <span class="block text-sm font-medium text-gray-800">{{
              choice.label
            }}</span>
            <span class="text-xs text-gray-500">{{ choice.description }}</span>
          </span>
        </label>
      </div>
    </div>
  </div>
</template>
