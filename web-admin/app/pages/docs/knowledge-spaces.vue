<script setup lang="ts">
const { t } = useI18n()

const links = [
  {
    label: "Runbook",
    href: "https://github.com/ArtisanCloud/PowerX/tree/main/docs/guides/knowledge_space/runbook.md",
  },
  {
    label: "Smoke Checklist",
    href: "https://github.com/ArtisanCloud/PowerX/tree/main/docs/guides/knowledge_space/smoke_checklist.md",
  },
  {
    label: "Performance Validation",
    href: "https://github.com/ArtisanCloud/PowerX/tree/main/docs/guides/knowledge_space/perf_validation.md",
  },
]
</script>

<template>
  <div class="p-6 space-y-4">
    <h1 class="text-2xl font-semibold">
      {{ t("knowledgeSpaces.docs.title", "Knowledge Spaces 文档") }}
    </h1>
    <p class="text-sm text-gray-500">
      {{ t("knowledgeSpaces.docs.desc", "这里汇总了知识空间的运行手册与校验清单。") }}
    </p>
    <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
      <UCard v-for="item in links" :key="item.href" class="h-full">
        <template #header>{{ item.label }}</template>
        <p class="text-sm text-gray-500">
          {{ t("knowledgeSpaces.docs.openOnGitHub", "将在 GitHub 中打开") }}
        </p>
        <div class="mt-3">
          <UButton as="a" :href="item.href" target="_blank" icon="i-heroicons-arrow-top-right-on-square">
            {{ t("knowledgeSpaces.docs.view", "查看") }}
          </UButton>
        </div>
      </UCard>
    </div>
    <p class="text-xs text-gray-400">
      {{ t("knowledgeSpaces.docs.note", "如果需要离线版本，可在仓库 docs/guides/knowledge_space/ 下查阅。") }}
    </p>
  </div>
</template>
