<template>
  <div class="min-w-0">
    <label class="block space-y-1.5 text-sm lg:hidden" :for="`${idBase}-select`">
      <span class="font-medium">{{ label }}</span>
      <select :id="`${idBase}-select`" :value="modelValue" :disabled="disabled" class="min-h-11 w-full rounded-lg border border-default bg-default px-3" data-testid="storage-policy-source" @change="emit('update:modelValue', ($event.target as HTMLSelectElement).value)">
        <option v-for="item in items" :key="item.id" :value="item.id">{{ item.name }} · {{ item.state }}</option>
      </select>
    </label>
    <div role="tablist" aria-orientation="vertical" :aria-label="label" class="hidden space-y-1 rounded-xl border border-default bg-elevated/30 p-1.5 lg:block" data-testid="storage-resource-tabs">
      <button v-for="(item, index) in items" :id="`${idBase}-tab-${item.id}`" :key="item.id" type="button" role="tab" :aria-selected="modelValue === item.id" :aria-controls="`${idBase}-panel`" :tabindex="modelValue === item.id ? 0 : -1" :disabled="disabled" :data-resource-tab="item.id" class="w-full rounded-lg px-3 py-3 text-left transition-colors focus-visible:outline-2 focus-visible:outline-primary-500 disabled:opacity-60" :class="modelValue === item.id ? 'bg-primary-500/12 text-primary-700 dark:text-primary-300' : 'text-muted hover:bg-elevated'" @click="emit('update:modelValue', item.id)" @keydown="navigate($event, index)">
        <span class="flex items-start justify-between gap-2"><span class="text-sm font-medium leading-5">{{ item.name }}</span><span v-if="item.dirty" class="mt-1.5 size-1.5 shrink-0 rounded-full bg-warning" :title="dirtyLabel"><span class="sr-only">{{ dirtyLabel }}</span></span></span>
        <span class="mt-1.5 flex flex-wrap items-center justify-between gap-x-2 gap-y-1 text-[11px]"><span>{{ item.state }}</span><span class="tabular-nums">{{ item.usage }}</span></span>
      </button>
    </div>
  </div>
</template>
<script setup lang="ts">
const props = defineProps<{ modelValue: string; items: { id: string; name: string; state: string; usage: string; dirty: boolean }[]; label: string; dirtyLabel: string; idBase: string; disabled: boolean }>();
const emit = defineEmits<{ 'update:modelValue': [value: string] }>();
function navigate(event: KeyboardEvent, index: number) {
  if (props.disabled || !props.items.length) return;
  const step = event.key === 'ArrowDown' ? 1 : event.key === 'ArrowUp' ? -1 : 0;
  if (!step && !['Home', 'End'].includes(event.key)) return;
  event.preventDefault();
  const next = event.key === 'Home' ? 0 : event.key === 'End' ? props.items.length - 1 : (index + step + props.items.length) % props.items.length;
  emit('update:modelValue', props.items[next]!.id);
  (event.currentTarget as HTMLElement).parentElement?.querySelectorAll<HTMLButtonElement>('[role="tab"]')[next]?.focus();
}
</script>
