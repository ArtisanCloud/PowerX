<script setup lang="ts">
useHead({
  title: "知识空间",
  meta: [{ name: "description", content: "统一管理知识空间的配额、策略与告警入口" }],
});

const quickActions = [
  {
    icon: "i-heroicons-plus-circle",
    title: "创建知识空间",
    description: "通过向导配置租户、策略、配额与 IAM",
    to: "/knowledge-spaces/create",
    primary: true,
  },
  {
    icon: "i-heroicons-book-open",
    title: "操作指南",
    description: "查看配置流程与准入条件",
    to: "/docs/knowledge-spaces",
  },
];

const placeholders = [
  {
    title: "空间列表",
    description: "即将展示租户维度的空间概览、配额与健康状态。",
  },
  {
    title: "入库与融合概览",
    description: "这里会汇总最近的入库任务、错误与融合策略状态。",
  },
  {
    title: "反馈与告警",
    description: "追踪 SLA 超时、告警升级与再加工任务。",
  },
];
</script>

<template>
  <section class="px-6 py-8 space-y-8">
    <header class="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
      <div>
        <p class="text-sm text-gray-500">Knowledge Space</p>
        <h1 class="text-2xl font-semibold text-gray-900">知识空间工作台</h1>
        <p class="text-gray-600 mt-2">
          统一创建与管理多租户知识空间，串联入库、融合与反馈闭环。
        </p>
      </div>
      <div class="flex flex-wrap gap-3">
        <NuxtLink
          v-for="action in quickActions"
          :key="action.title"
          :to="action.to"
          class="inline-flex items-center gap-2 rounded-lg border px-4 py-2 text-sm font-medium transition hover:bg-gray-50"
          :class="action.primary ? 'bg-primary-600 text-white border-primary-600 hover:bg-primary-500' : 'border-gray-200 text-gray-700'"
        >
          <UIcon :name="action.icon" class="w-5 h-5" />
          <span>{{ action.title }}</span>
        </NuxtLink>
      </div>
    </header>

    <UCard>
      <template #header>
        <div class="flex items-center justify-between">
          <div>
            <h2 class="text-lg font-semibold">概览</h2>
            <p class="text-gray-500 text-sm">真实数据将在后续用户故事中接入。</p>
          </div>
          <UBadge color="orange" variant="soft">规划中</UBadge>
        </div>
      </template>

      <div class="grid gap-6 md:grid-cols-3">
        <article
          v-for="item in placeholders"
          :key="item.title"
          class="rounded-xl border border-dashed border-gray-200 p-4 bg-gray-50"
        >
          <h3 class="font-medium text-gray-900">{{ item.title }}</h3>
          <p class="text-sm text-gray-600 mt-2">{{ item.description }}</p>
        </article>
      </div>
    </UCard>
  </section>
</template>
