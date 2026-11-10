<template>
  <nav :aria-label="t('plugins.sources.navigation')" class="flex min-w-0 gap-1 overflow-x-auto border-b border-[var(--border-color)]">
    <NuxtLink
      v-for="item in items" :key="item.to" :to="item.to" :aria-current="route.path === item.to ? 'page' : undefined" :class="[
      'relative flex min-h-12 shrink-0 items-center gap-2 px-3 pb-3 pt-2 sm:px-4 text-sm font-medium transition-colors focus-visible:outline-2 focus-visible:outline-primary-500',
      route.path === item.to ? 'text-primary-700 after:absolute after:inset-x-3 sm:after:inset-x-4 after:bottom-0 after:h-0.5 after:rounded-full after:bg-primary-500 dark:text-primary-300' : 'text-[var(--text-secondary)] hover:text-[var(--text-primary)]',
    ]">
      <UIcon :name="item.icon" class="size-4" />{{ item.label }}
    </NuxtLink>
  </nav>
</template>

<script setup lang="ts">
const { t } = useI18n();
const route = useRoute();
const items = computed(() => [
  { to: '/plugins/market', label: t('plugins.presentation.market'), icon: 'i-heroicons-building-storefront' },
  { to: '/plugins/local', label: t('plugins.sources.local'), icon: 'i-heroicons-computer-desktop' },
  { to: '/plugins/installed', label: t('plugins.sources.installed'), icon: 'i-heroicons-squares-2x2' },
]);
</script>
