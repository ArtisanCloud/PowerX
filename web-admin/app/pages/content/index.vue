<template>
  <div class="p-6">
    <div class="mb-6">
      <h1 class="text-2xl font-bold text-gray-900 dark:text-white">内容管理</h1>
      <p class="text-gray-600 dark:text-gray-400">管理您的内容和文章</p>
    </div>

    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
      <!-- 文章管理卡片 -->
      <UCard>
        <template #header>
          <div class="flex items-center gap-3">
            <UIcon
              class="w-6 h-6 text-blue-500 inline-block"
              name="i-lucide-document-text"
            />
            <h3 class="text-lg font-semibold">文章管理</h3>
          </div>
        </template>

        <div class="space-y-4">
          <p class="text-gray-600 dark:text-gray-400">创建和管理您的文章内容</p>

          <div class="flex gap-2">
            <UButton
              :to="localePath('/content/articles')"
              color="primary"
              size="sm"
            >
              查看文章
            </UButton>
            <UButton
              :to="localePath('/content/articles/create')"
              color="success"
              variant="outline"
              size="sm"
            >
              新建文章
            </UButton>
          </div>
        </div>
      </UCard>

      <!-- 媒体库卡片 -->
      <UCard>
        <template #header>
          <div class="flex items-center gap-3">
            <span class="w-6 h-6 text-purple-500 inline-block">
              <UIcon
                class="w-6 h-6 text-purple-500 inline-block"
                name="i-lucide-photo"
              />
            </span>
            <h3 class="text-lg font-semibold">媒体库</h3>
          </div>
        </template>

        <div class="space-y-4">
          <p class="text-gray-600 dark:text-gray-400">
            管理图片、视频等媒体文件
          </p>

          <UButton
            :to="localePath('/content/media')"
            color="purple"
            variant="outline"
            size="sm"
          >
            打开媒体库
          </UButton>
        </div>
      </UCard>

      <!-- 分类管理卡片 -->
      <UCard>
        <template #header>
          <div class="flex items-center gap-3">
            <span class="w-6 h-6 text-orange-500 inline-block">
              <UIcon
                class="w-6 h-6 text-orange-500 inline-block"
                name="i-lucide-tag"
              />
            </span>
            <h3 class="text-lg font-semibold">分类管理</h3>
          </div>
        </template>

        <div class="space-y-4">
          <p class="text-gray-600 dark:text-gray-400">管理内容分类和标签</p>

          <UButton
            :to="localePath('/content/categories')"
            color="orange"
            variant="outline"
            size="sm"
          >
            管理分类
          </UButton>
        </div>
      </UCard>
    </div>

    <!-- 最近文章 -->
    <div class="mt-8">
      <h2 class="text-xl font-semibold mb-4">最近文章</h2>
      <UCard>
        <div class="space-y-4">
          <div
            v-for="i in 3"
            :key="i"
            class="flex items-center justify-between p-4 border border-gray-200 dark:border-gray-700 rounded-lg"
          >
            <div>
              <h3 class="font-medium">示例文章标题 {{ i }}</h3>
              <p class="text-sm text-gray-600 dark:text-gray-400">
                发布于 2024-01-{{ 10 + i }}
              </p>
            </div>
            <div class="flex gap-2">
              <UButton size="xs" variant="outline">编辑</UButton>
              <UButton size="xs" color="error" variant="outline">删除</UButton>
            </div>
          </div>
        </div>
      </UCard>
    </div>
  </div>
</template>

<script setup lang="ts">
// 页面元数据
definePageMeta({
  title: "内容管理",
  layout: "default",
});

// 国际化
const localePath = useLocalePath();
</script>
