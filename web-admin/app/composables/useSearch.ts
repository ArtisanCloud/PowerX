import { SearchService } from "~/composables/api/services/searchService";
import type {
  SearchParams,
  SearchResponse,
  SearchFilter,
  SearchSuggestion,
  SearchHistory,
  SearchResultType,
} from "~/types/search";

export const useSearch = () => {
  // 搜索状态
  const isLoading = ref(false);
  const searchResults = ref<SearchResponse | null>(null);
  const searchQuery = ref("");
  const searchFilter = ref<SearchFilter>({});
  const suggestions = ref<SearchSuggestion[]>([]);
  const searchHistory = ref<SearchHistory[]>([]);

  // 搜索参数
  const currentPage = ref(1);
  const pageSize = ref(10);

  // 执行搜索
  const performSearch = async (
    query?: string,
    filter?: SearchFilter,
    page?: number
  ) => {
    if (query !== undefined) searchQuery.value = query;
    if (filter !== undefined) searchFilter.value = filter;
    if (page !== undefined) currentPage.value = page;

    if (!searchQuery.value.trim() && !Object.keys(searchFilter.value).length) {
      searchResults.value = null;
      return;
    }

    isLoading.value = true;

    try {
      const params: SearchParams = {
        query: searchQuery.value,
        filter: searchFilter.value,
        page: currentPage.value,
        limit: pageSize.value,
      };

      const response = await SearchService.search(params);
      searchResults.value = response;

      // 保存搜索历史
      if (searchQuery.value.trim()) {
        await SearchService.saveSearchHistory(
          searchQuery.value,
          response.total
        );
        await loadSearchHistory();
      }
    } catch (error) {
      console.error("搜索失败:", error);
      searchResults.value = null;
    } finally {
      isLoading.value = false;
    }
  };

  // 获取搜索建议
  const loadSuggestions = async (query: string) => {
    try {
      suggestions.value = await SearchService.getSuggestions(query);
    } catch (error) {
      console.error("获取搜索建议失败:", error);
      suggestions.value = [];
    }
  };

  // 加载搜索历史
  const loadSearchHistory = async () => {
    try {
      searchHistory.value = await SearchService.getSearchHistory();
    } catch (error) {
      console.error("加载搜索历史失败:", error);
      searchHistory.value = [];
    }
  };

  // 清除搜索历史
  const clearHistory = async () => {
    try {
      await SearchService.clearSearchHistory();
      searchHistory.value = [];
    } catch (error) {
      console.error("清除搜索历史失败:", error);
    }
  };

  // 重置搜索
  const resetSearch = () => {
    searchQuery.value = "";
    searchFilter.value = {};
    searchResults.value = null;
    currentPage.value = 1;
    suggestions.value = [];
  };

  // 更新过滤器
  const updateFilter = (newFilter: Partial<SearchFilter>) => {
    searchFilter.value = { ...searchFilter.value, ...newFilter };
    currentPage.value = 1; // 重置到第一页
    performSearch();
  };

  // 切换页面
  const changePage = (page: number) => {
    currentPage.value = page;
    performSearch();
  };

  // 快速搜索（用于Header搜索框）
  const quickSearch = async (query: string) => {
    if (!query.trim()) return [];

    const params: SearchParams = {
      query,
      limit: 5, // 只返回前5个结果
    };

    try {
      const response = await SearchService.search(params);
      return response.results;
    } catch (error) {
      console.error("快速搜索失败:", error);
      return [];
    }
  };

  // 计算属性
  const hasResults = computed(
    () => searchResults.value && searchResults.value.results.length > 0
  );

  const totalPages = computed(() =>
    searchResults.value
      ? Math.ceil(searchResults.value.total / pageSize.value)
      : 0
  );

  const isFirstPage = computed(() => currentPage.value === 1);
  const isLastPage = computed(() => currentPage.value === totalPages.value);

  // 搜索结果类型统计
  const resultTypeStats = computed(() => {
    if (!searchResults.value?.facets) return {};
    return searchResults.value.facets.types;
  });

  // 搜索结果分类统计
  const resultCategoryStats = computed(() => {
    if (!searchResults.value?.facets) return {};
    return searchResults.value.facets.categories;
  });

  // 初始化
  onMounted(() => {
    loadSearchHistory();
  });

  return {
    // 状态
    isLoading: readonly(isLoading),
    searchResults: readonly(searchResults),
    searchQuery,
    searchFilter,
    suggestions: readonly(suggestions),
    searchHistory: readonly(searchHistory),
    currentPage: readonly(currentPage),
    pageSize: readonly(pageSize),

    // 计算属性
    hasResults,
    totalPages,
    isFirstPage,
    isLastPage,
    resultTypeStats,
    resultCategoryStats,

    // 方法
    performSearch,
    loadSuggestions,
    loadSearchHistory,
    clearHistory,
    resetSearch,
    updateFilter,
    changePage,
    quickSearch,
  };
};

// 搜索结果类型的显示名称
export const getSearchResultTypeLabel = (type: SearchResultType): string => {
  const labels = {
    user: "用户",
    content: "内容",
    product: "产品",
    order: "订单",
    plugin: "插件",
    agent: "Agent",
    workflow: "工作流",
    setting: "设置",
    notification: "通知",
  } satisfies Record<SearchResultType, string>;

  return labels[type] ?? type;
};

// 搜索结果类型的图标
export const getSearchResultTypeIcon = (type: SearchResultType): string => {
  const icons = {
    user: "i-heroicons-users",
    content: "i-heroicons-document-text",
    product: "i-heroicons-cube",
    order: "i-heroicons-shopping-cart",
    plugin: "i-heroicons-puzzle-piece",
    agent: "i-heroicons-cpu-chip",
    workflow: "i-heroicons-arrow-path",
    setting: "i-heroicons-cog-6-tooth",
    notification: "i-heroicons-bell",
  } satisfies Record<SearchResultType, string>;

  return icons[type] ?? "i-heroicons-document";
};
