<template>
  <section class="min-w-0 overflow-hidden rounded-xl border border-[var(--border-color)] bg-[#ffffff] dark:bg-[#111b2c]">
    <div class="grid min-w-0 md:grid-cols-[208px_minmax(0,1fr)]">
      <div data-testid="plugin-section-rail" class="min-w-0 border-b border-[var(--border-color)] bg-[#f6f8fa] p-3 dark:bg-[#0e1727] md:border-r md:border-b-0 md:p-4">
        <p class="mb-3 hidden px-3 text-xs font-medium tracking-wide text-[var(--text-secondary)] md:block">{{ t('plugins.presentation.browse') }}</p>
        <nav :aria-label="ariaLabel" data-testid="plugin-section-nav" class="flex gap-1 overflow-x-auto md:flex-col md:overflow-visible">
          <button
      v-for="item in items" :key="item.value" type="button" :aria-label="item.count === undefined ? item.label : `${item.label} (${item.count})`" :aria-pressed="modelValue === item.value" :disabled="disabled" :class="[
            'flex min-h-11 shrink-0 items-center gap-2.5 rounded-lg px-3 py-2.5 text-left text-sm font-medium transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary-500 disabled:cursor-wait md:w-full',
            modelValue === item.value ? 'bg-[#ffffff] text-primary-700 shadow-sm ring-1 ring-gray-200 dark:bg-[#1b2b40] dark:text-primary-300 dark:ring-primary-400/20' : 'text-[var(--text-secondary)] hover:bg-[#eaf0f5] hover:text-[var(--text-primary)] dark:hover:bg-[#17243a]',
          ]" @click="emit('update:modelValue', item.value)">
            <UIcon :name="item.icon" class="size-4 shrink-0" />
            <span class="whitespace-nowrap">{{ item.label }}</span>
            <span v-if="item.count !== undefined" class="ml-auto rounded-md bg-gray-100 px-1.5 py-0.5 text-xs tabular-nums text-[var(--text-secondary)] dark:bg-[#223047]">{{ item.count }}</span>
          </button>
        </nav>
      </div>
      <div data-testid="plugin-section-content" class="min-w-0 p-4 sm:p-5 lg:p-6">
        <div class="mb-5 flex min-h-10 flex-wrap items-center justify-between gap-3">
          <div class="min-w-0">
            <h2 class="text-sm font-semibold text-[var(--text-primary)]">{{ title }}</h2>
            <p v-if="description" class="mt-1 text-xs text-[var(--text-secondary)]">{{ description }}</p>
          </div>
          <slot name="tools" />
        </div>
        <slot />
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
export interface PluginViewItem { value: string; label: string; icon: string; count?: number }
defineProps<{ modelValue: string; items: PluginViewItem[]; ariaLabel: string; title: string; description?: string; disabled?: boolean }>();
const emit = defineEmits<{ 'update:modelValue': [value: string] }>();
const { t } = useI18n();
</script>
