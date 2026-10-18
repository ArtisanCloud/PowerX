<template>
  <div class="grid min-w-0 items-start gap-5 sm:grid-cols-[11rem_minmax(0,1fr)]" data-testid="backup-section-tabs">
    <div class="min-w-0">
      <label class="block space-y-2 text-sm font-medium sm:hidden" :for="`${id}-select`">
        <span>{{ label }}</span>
        <select :id="`${id}-select`" :value="modelValue" data-section-select class="min-h-11 w-full rounded-lg border border-gray-300 bg-white px-3 text-gray-900 dark:border-gray-700 dark:bg-gray-900 dark:text-gray-100" @change="selectSection">
          <option v-for="item in items" :key="item.value" :value="item.value">{{ item.label }}</option>
        </select>
      </label>
    <div role="tablist" aria-orientation="vertical" :aria-label="label" class="hidden flex-col gap-1 rounded-xl border border-gray-200 bg-white/50 p-2 sm:flex dark:border-gray-700 dark:bg-gray-900/40">
      <button
        v-for="(item, index) in items" :id="`${id}-tab-${item.value}`" :key="item.value"
        type="button" role="tab" :aria-label="item.label" :aria-selected="modelValue === item.value" :aria-controls="`${id}-panel-${item.value}`"
        :tabindex="modelValue === item.value ? 0 : -1"
        class="flex min-h-10 items-center gap-2 rounded-lg px-3 py-2 text-left text-sm font-medium transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary-500"
        :class="modelValue === item.value ? 'bg-primary-500/15 text-primary-700 dark:text-primary-400' : 'text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-gray-800'"
        @click="modelValue = item.value" @keydown="navigate($event, index)"
      >
        <UIcon v-if="item.icon" :name="item.icon" class="size-4 shrink-0" />
        <span class="min-w-0 flex-1">{{ item.label }}</span>
        <span v-if="item.count" aria-hidden="true" class="text-xs tabular-nums opacity-75">{{ item.count }}</span>
      </button>
    </div>
    </div>
    <div class="min-w-0">
      <!-- 保留组件实例和任务订阅；只有当前面板参与布局及键盘访问。 -->
      <div v-for="item in items" v-show="modelValue === item.value" :id="`${id}-panel-${item.value}`" :key="item.value" role="tabpanel" :aria-labelledby="`${id}-tab-${item.value}`" :data-section="item.value" class="min-w-0">
        <slot :name="item.value" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useId } from 'vue';
type Section = { value: string; label: string; icon?: string; count?: number };
const props = defineProps<{ items: Section[]; label: string }>();
const modelValue = defineModel<string>({ required: true });
const id = `backup-section-${useId()}`;
function selectSection(event: Event) {
  modelValue.value = (event.target as HTMLSelectElement).value;
}
function navigate(event: KeyboardEvent, index: number) {
  const step = event.key === 'ArrowDown' ? 1 : event.key === 'ArrowUp' ? -1 : 0;
  if (!step && !['Home', 'End'].includes(event.key)) return;
  event.preventDefault();
  const next = event.key === 'Home' ? 0 : event.key === 'End' ? props.items.length - 1 : (index + step + props.items.length) % props.items.length;
  modelValue.value = props.items[next]!.value;
  const tabs = (event.currentTarget as HTMLElement).parentElement?.querySelectorAll<HTMLButtonElement>('[role="tab"]');
  tabs?.[next]?.focus();
}
</script>
