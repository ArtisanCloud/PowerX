<template>
  <div class="flex flex-col gap-4 sm:min-h-[100px] sm:flex-row sm:items-start sm:justify-between">
    <div class="flex min-w-0 items-start gap-3.5">
      <div class="flex size-11 shrink-0 items-center justify-center rounded-xl border border-primary-200/70 bg-primary-50 text-primary-700 dark:border-primary-400/20 dark:bg-primary-400/10 dark:text-primary-300">
        <UIcon :name="icon" class="size-6" />
      </div>
      <div class="min-w-0">
        <h1 class="text-xl font-semibold tracking-tight text-[var(--text-primary)]">{{ title }}</h1>
        <p class="mt-1 text-sm leading-6 text-[var(--text-secondary)]">{{ description }}</p>
        <div v-if="$slots.meta" class="mt-2 flex flex-wrap items-center gap-2 text-xs text-[var(--text-secondary)]"><slot name="meta" /></div>
      </div>
    </div>
    <div class="flex shrink-0 flex-wrap items-center gap-2 sm:pl-4"><slot name="actions" /></div>
  </div>
</template>

<script setup lang="ts">
defineProps<{ title: string; description: string; icon: string }>();
</script>
