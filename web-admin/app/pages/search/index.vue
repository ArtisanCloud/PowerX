<script setup lang="ts">
import { SearchResultType } from "~/types/search";
import {
  useSearch,
  getSearchResultTypeLabel,
  getSearchResultTypeIcon,
} from "~/composables/useSearch";

const { t } = useI18n();
const route = useRoute();
const router = useRouter();

// 使用搜索功能
const {
  isLoading,
  searchResults,
  searchQuery,
  searchFilter,
  suggestions,
  searchHistory,
  hasResults,
  totalPages,
  currentPage,
  resultTypeStats,
  resultCategoryStats,
  performSearch,
  loadSuggestions,
  clearHistory,
  resetSearch,
  updateFilter,
  changePage,
} = useSearch();

// 页面元数据
definePageMeta({
  layout: "default",
});

useHead({
  title: t("search.title"),
  meta: [{ name: "description", content: t("search.description") }],
});

// 搜索输入状态
const searchInput = ref("");
const showSuggestions = ref(false);
const showHistory = ref(false);

// 过滤器状态
const showFilters = ref(false);
const selectedTypes = ref<SearchResultType[]>([]);
const selectedCategories = ref<string[]>([]);
const sortBy = ref<"relevance" | "date" | "title">("relevance");
const sortOrder = ref<"asc" | "desc">("desc");

// 初始化搜索
onMounted(() => {
  // 从URL参数获取搜索查询
  const query = route.query.q as string;
  if (query) {
    searchInput.value = query;
    performSearch(query);
  }
});

// 监听搜索输入变化
watch(searchInput, async (newValue) => {
  if (newValue.trim()) {
    await loadSuggestions(newValue);
    showSuggestions.value = true;
    showHistory.value = false;
  } else {
    showSuggestions.value = false;
    showHistory.value = true;
  }
});

// 执行搜索
const handleSearch = () => {
  if (!searchInput.value.trim()) return;

  // 更新URL
  router.push({
    path: "/search",
    query: { q: searchInput.value },
  });

  // 应用过滤器
  const filter = {
    type: selectedTypes.value.length > 0 ? selectedTypes.value : undefined,
    category:
      selectedCategories.value.length > 0
        ? selectedCategories.value
        : undefined,
    sortBy: sortBy.value,
    sortOrder: sortOrder.value,
  };

  performSearch(searchInput.value, filter);
  showSuggestions.value = false;
  showHistory.value = false;
};

// 选择建议
const selectSuggestion = (suggestion: string) => {
  searchInput.value = suggestion;
  handleSearch();
};

// 选择历史记录
const selectHistory = (query: string) => {
  searchInput.value = query;
  handleSearch();
};

// 清除搜索
const handleClear = () => {
  searchInput.value = "";
  resetSearch();
  router.push("/search");
};

// 应用过滤器
const applyFilters = () => {
  const filter = {
    type: selectedTypes.value.length > 0 ? selectedTypes.value : undefined,
    category:
      selectedCategories.value.length > 0
        ? selectedCategories.value
        : undefined,
    sortBy: sortBy.value,
    sortOrder: sortOrder.value,
  };

  updateFilter(filter);
  showFilters.value = false;
};

// 重置过滤器
const resetFilters = () => {
  selectedTypes.value = [];
  selectedCategories.value = [];
  sortBy.value = "relevance";
  sortOrder.value = "desc";
  updateFilter({});
};

// 获取所有可用的类型选项
const typeOptions = computed(() => {
  return Object.values(SearchResultType).map((type) => ({
    value: type,
    label: getSearchResultTypeLabel(type),
    icon: getSearchResultTypeIcon(type),
  }));
});

// 获取所有可用的分类选项
const categoryOptions = computed(() => {
  const categories = new Set<string>();
  if (searchResults.value) {
    searchResults.value.results.forEach((result) => {
      categories.add(result.category);
    });
  }
  return Array.from(categories).map((category) => ({
    value: category,
    label: category,
  }));
});

// 格式化时间
const formatTime = (date: Date) => {
  return date.toLocaleString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
};

// 处理搜索结果点击
const handleResultClick = (url: string) => {
  router.push(url);
};
</script>

<template>
  <div class="min-h-screen bg-gray-50 dark:bg-gray-900">
    <div class="max-w-6xl mx-auto px-4 py-8">
      <!-- 搜索头部 -->
      <div class="mb-8">
        <div class="text-center mb-6">
          <h1 class="text-3xl font-bold text-gray-900 dark:text-white mb-2">
            {{ t("search.title") }}
          </h1>
          <p class="text-gray-600 dark:text-gray-400">
            {{ t("search.subtitle") }}
          </p>
        </div>

        <!-- 搜索框 -->
        <div class="relative max-w-2xl mx-auto">
          <div class="relative">
            <UInput
              v-model="searchInput"
              :placeholder="t('search.placeholder')"
              icon="i-heroicons-magnifying-glass"
              size="lg"
              class="w-full"
              @keyup.enter="handleSearch"
              @focus="showHistory = !searchInput.trim()"
              @blur="
                setTimeout(() => {
                  showSuggestions = false;
                  showHistory = false;
                }, 200)
              "
            />

            <div
              class="absolute right-2 top-1/2 transform -translate-y-1/2 flex items-center space-x-2"
            >
              <UButton
                v-if="searchInput"
                variant="ghost"
                size="sm"
                icon="i-heroicons-x-mark"
                @click="handleClear"
              />
              <UButton
                variant="solid"
                size="sm"
                icon="i-heroicons-magnifying-glass"
                @click="handleSearch"
              >
                {{ t("search.searchButton") }}
              </UButton>
            </div>
          </div>

          <!-- 搜索建议 -->
          <div
            v-if="showSuggestions && suggestions.length > 0"
            class="absolute top-full left-0 right-0 mt-1 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg shadow-lg z-50"
          >
            <div class="p-2">
              <div class="text-xs text-gray-500 mb-2">
                {{ t("search.suggestions") }}
              </div>
              <div
                v-for="suggestion in suggestions"
                :key="suggestion.text"
                class="flex items-center justify-between px-3 py-2 hover:bg-gray-50 dark:hover:bg-gray-700 rounded cursor-pointer"
                @click="selectSuggestion(suggestion.text)"
              >
                <div class="flex items-center space-x-2">
                  <UIcon
                    name="i-heroicons-magnifying-glass"
                    class="w-4 h-4 text-gray-400"
                  />
                  <span class="text-sm">{{ suggestion.text }}</span>
                </div>
                <span v-if="suggestion.count" class="text-xs text-gray-400">
                  {{ suggestion.count }}
                </span>
              </div>
            </div>
          </div>

          <!-- 搜索历史 -->
          <div
            v-if="showHistory && searchHistory.length > 0"
            class="absolute top-full left-0 right-0 mt-1 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg shadow-lg z-50"
          >
            <div class="p-2">
              <div class="flex items-center justify-between mb-2">
                <div class="text-xs text-gray-500">
                  {{ t("search.history") }}
                </div>
                <UButton variant="ghost" size="xs" @click="clearHistory">
                  {{ t("search.clearHistory") }}
                </UButton>
              </div>
              <div
                v-for="history in searchHistory.slice(0, 5)"
                :key="history.id"
                class="flex items-center justify-between px-3 py-2 hover:bg-gray-50 dark:hover:bg-gray-700 rounded cursor-pointer"
                @click="selectHistory(history.query)"
              >
                <div class="flex items-center space-x-2">
                  <UIcon
                    name="i-heroicons-clock"
                    class="w-4 h-4 text-gray-400"
                  />
                  <span class="text-sm">{{ history.query }}</span>
                </div>
                <div class="text-xs text-gray-400">
                  {{ history.resultCount }} 结果
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- 搜索结果区域 -->
      <div v-if="searchQuery" class="grid grid-cols-1 lg:grid-cols-4 gap-6">
        <!-- 过滤器侧边栏 -->
        <div class="lg:col-span-1">
          <UCard>
            <template #header>
              <div class="flex items-center justify-between">
                <h3 class="text-lg font-semibold">{{ t("search.filters") }}</h3>
                <UButton variant="ghost" size="sm" @click="resetFilters">
                  {{ t("search.resetFilters") }}
                </UButton>
              </div>
            </template>

            <div class="space-y-6">
              <!-- 结果类型过滤 -->
              <div>
                <h4
                  class="text-sm font-medium text-gray-900 dark:text-white mb-3"
                >
                  {{ t("search.resultType") }}
                </h4>
                <div class="space-y-2">
                  <div
                    v-for="option in typeOptions"
                    :key="option.value"
                    class="flex items-center space-x-2"
                  >
                    <UCheckbox
                      :id="option.value"
                      :model-value="selectedTypes.includes(option.value)"
                      @update:model-value="
                        (checked) => {
                          if (checked) {
                            selectedTypes.push(option.value);
                          } else {
                            const index = selectedTypes.indexOf(option.value);
                            if (index > -1) selectedTypes.splice(index, 1);
                          }
                        }
                      "
                    />
                    <label
                      :for="option.value"
                      class="flex items-center space-x-2 text-sm cursor-pointer"
                    >
                      <UIcon :name="option.icon" class="w-4 h-4" />
                      <span>{{ option.label }}</span>
                      <span
                        v-if="resultTypeStats[option.value]"
                        class="text-xs text-gray-500"
                      >
                        ({{ resultTypeStats[option.value] }})
                      </span>
                    </label>
                  </div>
                </div>
              </div>

              <!-- 分类过滤 -->
              <div v-if="categoryOptions.length > 0">
                <h4
                  class="text-sm font-medium text-gray-900 dark:text-white mb-3"
                >
                  {{ t("search.category") }}
                </h4>
                <div class="space-y-2">
                  <div
                    v-for="option in categoryOptions"
                    :key="option.value"
                    class="flex items-center space-x-2"
                  >
                    <UCheckbox
                      :id="option.value"
                      :model-value="selectedCategories.includes(option.value)"
                      @update:model-value="
                        (checked) => {
                          if (checked) {
                            selectedCategories.push(option.value);
                          } else {
                            const index = selectedCategories.indexOf(
                              option.value
                            );
                            if (index > -1) selectedCategories.splice(index, 1);
                          }
                        }
                      "
                    />
                    <label :for="option.value" class="text-sm cursor-pointer">
                      {{ option.label }}
                      <span
                        v-if="resultCategoryStats[option.value]"
                        class="text-xs text-gray-500"
                      >
                        ({{ resultCategoryStats[option.value] }})
                      </span>
                    </label>
                  </div>
                </div>
              </div>

              <!-- 排序选项 -->
              <div>
                <h4
                  class="text-sm font-medium text-gray-900 dark:text-white mb-3"
                >
                  {{ t("search.sortBy") }}
                </h4>
                <div class="space-y-2">
                  <USelect
                    v-model="sortBy"
                    :options="[
                      { value: 'relevance', label: t('search.sortRelevance') },
                      { value: 'date', label: t('search.sortDate') },
                      { value: 'title', label: t('search.sortTitle') },
                    ]"
                  />
                  <USelect
                    v-model="sortOrder"
                    :options="[
                      { value: 'desc', label: t('search.sortDesc') },
                      { value: 'asc', label: t('search.sortAsc') },
                    ]"
                  />
                </div>
              </div>

              <!-- 应用过滤器按钮 -->
              <UButton block @click="applyFilters">
                {{ t("search.applyFilters") }}
              </UButton>
            </div>
          </UCard>
        </div>

        <!-- 搜索结果列表 -->
        <div class="lg:col-span-3">
          <!-- 加载状态 -->
          <div v-if="isLoading" class="text-center py-12">
            <UIcon
              name="i-heroicons-arrow-path"
              class="w-8 h-8 animate-spin mx-auto mb-4"
            />
            <p class="text-gray-600 dark:text-gray-400">
              {{ t("search.searching") }}
            </p>
          </div>

          <!-- 搜索结果 -->
          <div v-else-if="hasResults">
            <!-- 结果统计 -->
            <div class="mb-6">
              <p class="text-sm text-gray-600 dark:text-gray-400">
                {{
                  t("search.resultsCount", {
                    total: searchResults?.total || 0,
                    query: searchQuery,
                  })
                }}
              </p>
            </div>

            <!-- 结果列表 -->
            <div class="space-y-4">
              <UCard
                v-for="result in searchResults?.results"
                :key="result.id"
                class="hover:shadow-md transition-shadow cursor-pointer"
                @click="handleResultClick(result.url)"
              >
                <div class="flex items-start space-x-4">
                  <div class="flex-shrink-0">
                    <div
                      class="w-10 h-10 bg-blue-100 dark:bg-blue-900 rounded-lg flex items-center justify-center"
                    >
                      <UIcon
                        :name="getSearchResultTypeIcon(result.type)"
                        class="w-5 h-5 text-blue-600 dark:text-blue-400"
                      />
                    </div>
                  </div>

                  <div class="flex-1 min-w-0">
                    <div class="flex items-center space-x-2 mb-1">
                      <h3
                        class="text-lg font-semibold text-blue-600 dark:text-blue-400 hover:underline"
                      >
                        {{ result.title }}
                      </h3>
                      <UBadge
                        :label="getSearchResultTypeLabel(result.type)"
                        size="xs"
                        variant="soft"
                      />
                    </div>

                    <p class="text-sm text-gray-600 dark:text-gray-400 mb-2">
                      {{ result.category }}
                    </p>

                    <div
                      v-if="result.highlight"
                      class="text-sm text-gray-700 dark:text-gray-300 mb-2"
                      v-html="result.highlight"
                    />
                    <p
                      v-else
                      class="text-sm text-gray-700 dark:text-gray-300 mb-2"
                    >
                      {{ result.content.substring(0, 200) }}...
                    </p>

                    <div
                      class="flex items-center justify-between text-xs text-gray-500"
                    >
                      <span>{{ result.url }}</span>
                      <span>{{ formatTime(result.updatedAt) }}</span>
                    </div>
                  </div>
                </div>
              </UCard>
            </div>

            <!-- 分页 -->
            <div v-if="totalPages > 1" class="mt-8 flex justify-center">
              <UPagination
                v-model:page="currentPage"
                :total="searchResults?.total || 0"
                :items-per-page="10"
                :sibling-count="1"
                show-edges
                @update:page="changePage"
              />
            </div>
          </div>

          <!-- 无结果 -->
          <div v-else-if="searchQuery && !isLoading" class="text-center py-12">
            <UIcon
              name="i-heroicons-magnifying-glass"
              class="w-16 h-16 text-gray-400 mx-auto mb-4"
            />
            <h3
              class="text-lg font-semibold text-gray-900 dark:text-white mb-2"
            >
              {{ t("search.noResults") }}
            </h3>
            <p class="text-gray-600 dark:text-gray-400 mb-4">
              {{ t("search.noResultsDesc", { query: searchQuery }) }}
            </p>
            <UButton variant="outline" @click="resetFilters">
              {{ t("search.tryDifferentFilters") }}
            </UButton>
          </div>

          <!-- 初始状态 -->
          <div v-else class="text-center py-12">
            <UIcon
              name="i-heroicons-magnifying-glass"
              class="w-16 h-16 text-gray-400 mx-auto mb-4"
            />
            <h3
              class="text-lg font-semibold text-gray-900 dark:text-white mb-2"
            >
              {{ t("search.startSearching") }}
            </h3>
            <p class="text-gray-600 dark:text-gray-400">
              {{ t("search.startSearchingDesc") }}
            </p>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
:deep(mark) {
  background-color: #fef08a;
  color: #92400e;
  padding: 0 2px;
  border-radius: 2px;
}

:deep(.dark mark) {
  background-color: #451a03;
  color: #fbbf24;
}
</style>
