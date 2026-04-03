<template>
  <div
    class="rounded-lg border border-[var(--border-color)] bg-[var(--card-bg)]"
  >
    <div
      class="px-4 py-3 border-b border-[var(--border-color)] font-medium text-[var(--text-primary)]"
    >
      连接测试
    </div>
    <div class="p-4 space-y-3">
      <div class="text-sm text-[var(--text-secondary)]">
        当前测试对象：{{ currentTitle }}（{{
          currentState.provider || "未选择 Provider"
        }}
        / {{ currentState.model || "未选择 Model" }}）
      </div>
      <div class="flex items-center gap-2">
        <UButton
          icon="i-heroicons-wifi"
          color="primary"
          size="sm"
          class="whitespace-nowrap"
          :disabled="disabled"
          @click="onTestConnection?.()"
        >
          测试连接
        </UButton>
        <UButton
          icon="i-heroicons-command-line"
          variant="ghost"
          size="sm"
          class="whitespace-nowrap"
          :disabled="disabled"
          @click="onTestQuickCall?.()"
        >
          试跑一次
        </UButton>
      </div>
      <div v-if="disabled && disabledReason" class="text-xs text-[var(--text-secondary)]">
        {{ disabledReason }}
      </div>
      <div
        class="rounded-md border border-[var(--border-color)] bg-[var(--bg-secondary)] p-3 min-h-[80px] text-xs text-[var(--text-secondary)]"
      >
        <template v-if="lastTestMessage">
          <div class="font-medium text-[var(--text-primary)] mb-1">
            测试结果
          </div>
          <pre class="whitespace-pre-wrap break-all">{{ lastTestMessage }}</pre>
          <details
            v-if="lastTestDetail"
            class="mt-2 rounded border border-[var(--border-color)] bg-[var(--card-bg)] p-2"
          >
            <summary class="cursor-pointer select-none text-[var(--text-primary)]">
              查看技术详情
            </summary>
            <pre class="mt-2 whitespace-pre-wrap break-all">{{ lastTestDetail }}</pre>
          </details>
        </template>
        <template v-else>
          <div class="text-[var(--text-secondary)]">暂无测试结果</div>
        </template>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  currentTitle: string;
  currentState: {
    provider: string;
    model: string;
    apiKey?: string;
    baseURL?: string;
    region?: string;
    organization?: string;
  };
  lastTestMessage?: string;
  lastTestDetail?: string;
  disabled?: boolean;
  disabledReason?: string;
  onTestConnection?: () => void;
  onTestQuickCall?: () => void;
}>();
</script>
