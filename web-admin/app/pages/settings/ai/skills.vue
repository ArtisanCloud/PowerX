<template>
  <div class="space-y-6 p-4">
    <div class="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
      <div>
        <h1 class="text-lg font-semibold text-[var(--text-primary)]">Skills</h1>
        <p class="text-sm text-[var(--text-secondary)]">
          管理 PowerX Skills 的官方目录、导入、发布与回滚。
        </p>
      </div>
      <div class="flex flex-wrap gap-2">
        <UButton variant="ghost" icon="i-heroicons-arrow-left" :to="localePath('/settings/ai')">
          返回 AI 设置
        </UButton>
      </div>
    </div>

    <UAlert
      color="primary"
      variant="soft"
      icon="i-heroicons-squares-plus"
      title="Skills 管理页（Phase 1 骨架）"
      description="后续阶段将补充目录列表、导入表单、发布回滚和审计抽屉。"
    />
  </div>
</template>

<script setup lang="ts">
const localePath = useLocalePath();

definePageMeta({
  title: "Skills",
  layout: "default",
});
</script>
