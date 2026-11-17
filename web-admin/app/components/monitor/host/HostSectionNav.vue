<script setup lang="ts">
import { useId } from 'vue';
export type HostSection = 'overview' | 'compute' | 'disks' | 'network' | 'gpu';
const props = defineProps<{ items: { value: HostSection; label: string; icon: string }[]; label: string; modelValue: HostSection }>();
const emit = defineEmits<{ 'update:modelValue': [value: HostSection] }>();
const id = `host-nav-${useId()}`;
function navigate(event: KeyboardEvent, index: number) {
  const step = ['ArrowDown', 'ArrowRight'].includes(event.key) ? 1 : ['ArrowUp', 'ArrowLeft'].includes(event.key) ? -1 : 0;
  if (!step && !['Home', 'End'].includes(event.key)) return;
  event.preventDefault();
  const next = event.key === 'Home' ? 0 : event.key === 'End' ? props.items.length - 1 : (index + step + props.items.length) % props.items.length;
  emit('update:modelValue', props.items[next]!.value);
  (event.currentTarget as HTMLElement).parentElement?.querySelectorAll<HTMLButtonElement>('[role=tab]')[next]?.focus();
}
</script>

<template>
  <div class="grid min-w-0 items-start gap-4 md:grid-cols-[11rem_minmax(0,1fr)]">
    <div class="host-tablist flex min-w-0 gap-1 overflow-x-auto rounded-lg border border-default p-1 md:flex-col" role="tablist" :aria-label="label">
      <button type="button" v-for="(item, index) in items" :id="`${id}-${item.value}`" :key="item.value"
        role="tab" :aria-selected="props.modelValue === item.value" :aria-controls="`${id}-panel`" :tabindex="props.modelValue === item.value ? 0 : -1"
        :class="props.modelValue === item.value ? 'bg-primary/10 text-primary font-semibold' : 'text-slate-700 dark:text-slate-200 hover:bg-elevated hover:text-highlighted'"
        class="flex shrink-0 items-center gap-2 rounded-md px-3 py-2 text-left text-sm whitespace-nowrap focus-visible:outline-2 focus-visible:outline-primary" :data-testid="`host-tab-${item.value}`"
        @click="emit('update:modelValue', item.value)" @keydown="navigate($event, index)"><UIcon :name="item.icon" class="size-4 shrink-0" :class="props.modelValue === item.value ? 'text-primary' : 'text-slate-600 dark:text-slate-200'" />{{ item.label }}</button>
    </div>
    <div :id="`${id}-panel`" role="tabpanel" :aria-labelledby="`${id}-${props.modelValue}`" tabindex="0" class="min-w-0" data-testid="host-panel" :data-section="props.modelValue">
      <slot />
    </div>
  </div>
</template>
