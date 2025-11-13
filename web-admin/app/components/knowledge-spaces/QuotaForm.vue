<script setup lang="ts">
const props = defineProps<{
  quotas: {
    cpuCores: number;
    storageGb: number;
    ingestionConcurrency: number;
  };
  iamEmail: string;
}>();

const emit = defineEmits<{
  (e: "update:quotas", value: typeof props.quotas): void;
  (e: "update:iamEmail", value: string): void;
}>();

const updateQuota = (key: keyof typeof props.quotas, value: number) => {
  const next = { ...props.quotas, [key]: value };
  emit("update:quotas", next);
};
</script>

<template>
  <div class="space-y-6">
    <div class="grid gap-6 md:grid-cols-3">
      <label class="flex flex-col gap-2">
        <span class="text-sm font-medium text-gray-700">CPU 核心</span>
        <input
          type="number"
          min="1"
          class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
          :value="quotas.cpuCores"
          @input="updateQuota('cpuCores', Number($event.target?.value || 0))"
        />
        <span class="text-xs text-gray-500">用于嵌入/预处理的计算配额</span>
      </label>

      <label class="flex flex-col gap-2">
        <span class="text-sm font-medium text-gray-700">存储容量 (GB)</span>
        <input
          type="number"
          min="50"
          step="10"
          class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
          :value="quotas.storageGb"
          @input="updateQuota('storageGb', Number($event.target?.value || 0))"
        />
        <span class="text-xs text-gray-500">包含 chunk/向量/图谱的可用空间</span>
      </label>

      <label class="flex flex-col gap-2">
        <span class="text-sm font-medium text-gray-700">入库并发</span>
        <input
          type="number"
          min="1"
          max="10"
          class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
          :value="quotas.ingestionConcurrency"
          @input="
            updateQuota('ingestionConcurrency', Number($event.target?.value || 0))
          "
        />
        <span class="text-xs text-gray-500">控制 OCR/解析任务同时运行的数量</span>
      </label>
    </div>

    <label class="flex flex-col gap-2">
      <span class="text-sm font-medium text-gray-700">IAM 通知邮箱</span>
      <input
        type="email"
        placeholder="iam-alerts@company.com"
        class="rounded-lg border border-gray-200 px-3 py-2 text-sm shadow-sm focus:border-primary-500 focus:outline-none"
        :value="iamEmail"
        @input="emit('update:iamEmail', String($event.target?.value || ''))"
      />
      <span class="text-xs text-gray-500"
        >用于接收角色同步结果与 SLA 越界通知</span
      >
    </label>
  </div>
</template>
