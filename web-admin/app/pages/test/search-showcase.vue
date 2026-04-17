<template>
  <div class="min-h-screen bg-gray-50 dark:bg-gray-900">
    <!-- 页面标题 -->
    <div class="bg-white dark:bg-gray-800 shadow-sm border-b">
      <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-6">
        <h1 class="text-3xl font-bold text-gray-900 dark:text-white">
          搜索系统展示页面
        </h1>
        <p class="mt-2 text-gray-600 dark:text-gray-400">
          展示各种搜索界面形态和交互效果
        </p>
      </div>
    </div>

    <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8 space-y-12">
      <!-- 1. Header 搜索框演示 -->
      <section class="bg-white dark:bg-gray-800 rounded-lg shadow-sm p-6">
        <h2 class="text-xl font-semibold mb-4 text-gray-900 dark:text-white">
          1. Header 全局搜索框
        </h2>
        <p class="text-gray-600 dark:text-gray-400 mb-6">
          位于页面顶部的全局搜索入口，支持实时搜索建议
        </p>

        <!-- 模拟 Header 搜索框 -->
        <div class="relative max-w-md">
          <UInput
            v-model="headerSearchQuery"
            :placeholder="$t('header.searchPlaceholder')"
            icon="i-heroicons-magnifying-glass"
            size="lg"
            @input="handleHeaderSearch"
            @keyup.enter="goToSearchPage"
          />

          <!-- 搜索建议下拉框 -->
          <div
            v-if="showSuggestions && headerSuggestions.length > 0"
            class="absolute top-full left-0 right-0 mt-1 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg shadow-lg z-50"
          >
            <div class="p-2">
              <div class="text-xs text-gray-500 dark:text-gray-400 px-3 py-2">
                {{ $t("search.suggestions") }}
              </div>
              <div
                v-for="suggestion in headerSuggestions"
                :key="suggestion.id"
                class="flex items-center px-3 py-2 hover:bg-gray-100 dark:hover:bg-gray-700 rounded cursor-pointer"
                @click="selectSuggestion(suggestion)"
              >
                <UIcon
                  :name="getTypeIcon(suggestion.type)"
                  class="w-4 h-4 mr-3 text-gray-400"
                />
                <div class="flex-1">
                  <div
                    class="text-sm font-medium text-gray-900 dark:text-white"
                  >
                    {{ suggestion.title }}
                  </div>
                  <div class="text-xs text-gray-500 dark:text-gray-400">
                    {{ getTypeLabel(suggestion.type) }}
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      <!-- 2. 搜索结果页面演示 -->
      <section class="bg-white dark:bg-gray-800 rounded-lg shadow-sm p-6">
        <h2 class="text-xl font-semibold mb-4 text-gray-900 dark:text-white">
          2. 搜索结果页面
        </h2>
        <p class="text-gray-600 dark:text-gray-400 mb-6">
          专门的搜索结果展示页面，支持过滤和排序
        </p>

        <!-- 搜索框 -->
        <div class="mb-6">
          <UInput
            v-model="searchQuery"
            :placeholder="$t('search.placeholder')"
            icon="i-heroicons-magnifying-glass"
            size="lg"
            @keyup.enter="performSearch"
          >
            <template #trailing>
              <UButton
                color="primary"
                variant="solid"
                size="sm"
                @click="performSearch"
              >
                {{ $t("search.searchButton") }}
              </UButton>
            </template>
          </UInput>
        </div>

        <!-- 过滤器区域 -->
        <div class="mb-6 p-4 bg-gray-50 dark:bg-gray-700 rounded-lg">
          <div class="flex flex-wrap gap-4 items-center">
            <div>
              <label
                class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1"
              >
                {{ $t("search.resultType") }}
              </label>
              <USelectMenu
                v-model="selectedType"
                :options="typeOptions"
                placeholder="全部类型"
                size="sm"
              />
            </div>

            <div>
              <label
                class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1"
              >
                {{ $t("search.sortBy") }}
              </label>
              <USelectMenu
                v-model="selectedSort"
                :options="sortOptions"
                placeholder="相关性"
                size="sm"
              />
            </div>

            <div class="flex items-end">
              <UButton
                color="gray"
                variant="outline"
                size="sm"
                @click="resetFilters"
              >
                {{ $t("search.resetFilters") }}
              </UButton>
            </div>
          </div>
        </div>

        <!-- 搜索结果统计 -->
        <div class="mb-4">
          <p class="text-sm text-gray-600 dark:text-gray-400">
            找到
            <span class="font-semibold">{{ mockResults.length }}</span> 个关于
            "<span class="font-semibold">{{ searchQuery || "PowerX" }}</span
            >" 的结果
          </p>
        </div>

        <!-- 搜索结果列表 -->
        <div class="space-y-4">
          <div
            v-for="result in mockResults"
            :key="result.id"
            class="border border-gray-200 dark:border-gray-700 rounded-lg p-4 hover:shadow-md transition-shadow"
          >
            <div class="flex items-start justify-between">
              <div class="flex-1">
                <div class="flex items-center mb-2">
                  <UIcon
                    :name="getTypeIcon(result.type)"
                    class="w-5 h-5 mr-2 text-blue-500"
                  />
                  <span
                    class="text-xs px-2 py-1 bg-blue-100 dark:bg-blue-900 text-blue-800 dark:text-blue-200 rounded-full"
                  >
                    {{ getTypeLabel(result.type) }}
                  </span>
                </div>
                <h3
                  class="text-lg font-semibold text-gray-900 dark:text-white mb-2"
                >
                  {{ result.title }}
                </h3>
                <p class="text-gray-600 dark:text-gray-400 mb-3">
                  {{ result.description }}
                </p>
                <div
                  class="flex items-center text-sm text-gray-500 dark:text-gray-400"
                >
                  <span>{{ formatDate(result.updatedAt) }}</span>
                  <span class="mx-2">•</span>
                  <span>{{ result.category }}</span>
                </div>
              </div>
              <UButton color="primary" variant="outline" size="sm">
                查看详情
              </UButton>
            </div>
          </div>
        </div>

        <!-- 分页 -->
        <div class="mt-8 flex justify-center">
          <UPagination
            v-model="currentPage"
            :page-count="10"
            :total="mockResults.length"
          />
        </div>
      </section>

      <!-- 3. 搜索历史演示 -->
      <section class="bg-white dark:bg-gray-800 rounded-lg shadow-sm p-6">
        <h2 class="text-xl font-semibold mb-4 text-gray-900 dark:text-white">
          3. 搜索历史记录
        </h2>
        <p class="text-gray-600 dark:text-gray-400 mb-6">
          用户的搜索历史记录，支持快速重新搜索
        </p>

        <div class="space-y-2">
          <div
            v-for="history in searchHistory"
            :key="history.id"
            class="flex items-center justify-between p-3 hover:bg-gray-50 dark:hover:bg-gray-700 rounded-lg cursor-pointer"
          >
            <div class="flex items-center">
              <UIcon
                name="i-heroicons-clock"
                class="w-4 h-4 mr-3 text-gray-400"
              />
              <span class="text-gray-900 dark:text-white">{{
                history.query
              }}</span>
              <span class="ml-2 text-xs text-gray-500 dark:text-gray-400">
                {{ formatDate(history.timestamp) }}
              </span>
            </div>
            <UButton
              color="gray"
              variant="ghost"
              size="xs"
              icon="i-heroicons-x-mark"
            />
          </div>
        </div>

        <div class="mt-4 pt-4 border-t border-gray-200 dark:border-gray-700">
          <UButton color="gray" variant="outline" size="sm">
            {{ $t("search.clearHistory") }}
          </UButton>
        </div>
      </section>

      <!-- 4. 空状态演示 -->
      <section class="bg-white dark:bg-gray-800 rounded-lg shadow-sm p-6">
        <h2 class="text-xl font-semibold mb-4 text-gray-900 dark:text-white">
          4. 空状态展示
        </h2>
        <p class="text-gray-600 dark:text-gray-400 mb-6">
          无搜索结果时的空状态界面
        </p>

        <div class="text-center py-12">
          <UIcon
            name="i-heroicons-magnifying-glass"
            class="w-16 h-16 mx-auto text-gray-400 mb-4"
          />
          <h3 class="text-lg font-semibold text-gray-900 dark:text-white mb-2">
            {{ $t("search.noResults") }}
          </h3>
          <p class="text-gray-600 dark:text-gray-400 mb-6">
            没有找到与 "不存在的关键词" 相关的结果
          </p>
          <UButton color="primary" variant="outline">
            {{ $t("search.tryDifferentFilters") }}
          </UButton>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { SearchResult, SearchSuggestion } from "~/types/search";

// 页面元数据
definePageMeta({
  title: "搜索系统展示",
  description: "展示搜索系统的各种界面形态",
});

// 响应式数据
const headerSearchQuery = ref("");
const searchQuery = ref("PowerX");
const showSuggestions = ref(false);
const selectedType = ref("");
const selectedSort = ref("");
const currentPage = ref(1);

// 模拟搜索建议数据
const headerSuggestions = ref<SearchSuggestion[]>([
  {
    id: "1",
    title: "PowerX 管理系统",
    type: "content",
    category: "系统",
  },
  {
    id: "2",
    title: "用户管理",
    type: "user",
    category: "功能",
  },
  {
    id: "3",
    title: "产品目录",
    type: "product",
    category: "商品",
  },
]);

// 模拟搜索结果数据
const mockResults = ref<SearchResult[]>([
  {
    id: "1",
    title: "PowerX 核心管理系统",
    description:
      "基于 Vue 3 和 Nuxt 3 构建的现代化管理系统，提供完整的用户管理、权限控制、数据分析等功能。",
    type: "content",
    category: "系统文档",
    url: "/dashboard",
    score: 0.95,
    createdAt: new Date("2024-01-15"),
    updatedAt: new Date("2024-03-10"),
  },
  {
    id: "2",
    title: "智能 Agent 对话系统",
    description:
      "集成多种 AI 模型的智能对话系统，支持自定义 Agent 配置、多轮对话、消息类型扩展等高级功能。",
    type: "product",
    category: "AI 功能",
    url: "/agent",
    score: 0.88,
    createdAt: new Date("2024-02-01"),
    updatedAt: new Date("2024-03-08"),
  },
  {
    id: "3",
    title: "用户权限管理",
    description:
      "完整的 RBAC 权限管理系统，支持角色分配、权限控制、部门管理等企业级功能。",
    type: "user",
    category: "用户管理",
    url: "/settings/users",
    score: 0.82,
    createdAt: new Date("2024-01-20"),
    updatedAt: new Date("2024-03-05"),
  },
  {
    id: "4",
    title: "数据分析仪表板",
    description:
      "基于 ECharts 的数据可视化仪表板，提供实时数据监控、趋势分析、报表生成等功能。",
    type: "content",
    category: "数据分析",
    url: "/dashboard",
    score: 0.75,
    createdAt: new Date("2024-02-10"),
    updatedAt: new Date("2024-03-01"),
  },
]);

// 搜索历史数据
const searchHistory = ref([
  { id: "1", query: "PowerX 系统", timestamp: new Date("2024-03-10T10:30:00") },
  { id: "2", query: "用户管理", timestamp: new Date("2024-03-09T15:20:00") },
  { id: "3", query: "Agent 配置", timestamp: new Date("2024-03-08T09:15:00") },
  { id: "4", query: "权限设置", timestamp: new Date("2024-03-07T14:45:00") },
]);

// 选项数据
const typeOptions = [
  { label: "全部类型", value: "" },
  { label: "用户", value: "user" },
  { label: "内容", value: "content" },
  { label: "产品", value: "product" },
  { label: "订单", value: "order" },
];

const sortOptions = [
  { label: "相关性", value: "relevance" },
  { label: "日期（新到旧）", value: "date_desc" },
  { label: "日期（旧到新）", value: "date_asc" },
  { label: "标题（A-Z）", value: "title_asc" },
];

// 方法
const handleHeaderSearch = () => {
  showSuggestions.value = headerSearchQuery.value.length > 0;
};

const selectSuggestion = (suggestion: SearchSuggestion) => {
  headerSearchQuery.value = suggestion.title;
  showSuggestions.value = false;
  // 跳转到搜索结果页面
  navigateTo(`/search?q=${encodeURIComponent(suggestion.title)}`);
};

const goToSearchPage = () => {
  if (headerSearchQuery.value.trim()) {
    navigateTo(`/search?q=${encodeURIComponent(headerSearchQuery.value)}`);
  }
};

const performSearch = () => {
  console.info("执行搜索:", searchQuery.value);
  // 这里会调用实际的搜索 API
};

const resetFilters = () => {
  selectedType.value = "";
  selectedSort.value = "";
};

const getTypeIcon = (type: string) => {
  const icons = {
    user: "i-heroicons-user",
    content: "i-heroicons-document-text",
    product: "i-heroicons-cube",
    order: "i-heroicons-shopping-cart",
  };
  return icons[type as keyof typeof icons] || "i-heroicons-document";
};

const getTypeLabel = (type: string) => {
  const labels = {
    user: "用户",
    content: "内容",
    product: "产品",
    order: "订单",
  };
  return labels[type as keyof typeof labels] || "其他";
};

const formatDate = (date: Date) => {
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "short",
    day: "numeric",
  }).format(date);
};

// 点击外部关闭建议框
onMounted(() => {
  document.addEventListener("click", (e) => {
    const target = e.target as HTMLElement;
    if (!target.closest(".relative")) {
      showSuggestions.value = false;
    }
  });
});
</script>
