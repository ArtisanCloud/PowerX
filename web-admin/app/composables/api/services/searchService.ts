import type {
  SearchParams,
  SearchResponse,
  SearchResult,
  SearchSuggestion,
  SearchHistory,
} from "~/types/search";

// 模拟搜索数据
const mockSearchData: SearchResult[] = [
  {
    id: "1",
    title: "用户管理系统",
    content: "完整的用户管理功能，包括用户创建、编辑、删除和权限管理",
    type: "user",
    category: "系统管理",
    url: "/settings/users",
    score: 0.95,
    createdAt: new Date("2024-01-15"),
    updatedAt: new Date("2024-01-20"),
    metadata: { module: "user-management" },
  },
  {
    id: "2",
    title: "PowerX AI 助手",
    content: "智能对话助手，支持多种AI模型，可以处理各种业务场景",
    type: "agent",
    category: "AI助手",
    url: "/agent",
    score: 0.92,
    createdAt: new Date("2024-01-10"),
    updatedAt: new Date("2024-01-18"),
    metadata: { model: "gpt-4", status: "active" },
  },
  {
    id: "3",
    title: "工作流自动化",
    content: "可视化工作流编辑器，支持拖拽式流程设计和自动化执行",
    type: "workflow",
    category: "自动化",
    url: "/workflow",
    score: 0.88,
    createdAt: new Date("2024-01-12"),
    updatedAt: new Date("2024-01-19"),
    metadata: { nodes: 15, status: "published" },
  },
  {
    id: "4",
    title: "插件应用广场",
    content: "丰富的插件生态，包括各种功能扩展和第三方集成",
    type: "plugin",
    category: "扩展功能",
    url: "/plugins",
    score: 0.85,
    createdAt: new Date("2024-01-08"),
    updatedAt: new Date("2024-01-16"),
    metadata: { totalPlugins: 24, installed: 8 },
  },
  {
    id: "5",
    title: "产品目录管理",
    content: "产品信息管理，包括分类、库存、价格等完整的电商功能",
    type: "product",
    category: "电商管理",
    url: "/products",
    score: 0.82,
    createdAt: new Date("2024-01-14"),
    updatedAt: new Date("2024-01-21"),
    metadata: { totalProducts: 156, categories: 12 },
  },
  {
    id: "6",
    title: "订单处理系统",
    content: "完整的订单生命周期管理，从下单到发货的全流程跟踪",
    type: "order",
    category: "电商管理",
    url: "/orders",
    score: 0.79,
    createdAt: new Date("2024-01-11"),
    updatedAt: new Date("2024-01-17"),
    metadata: { totalOrders: 89, pending: 12 },
  },
  {
    id: "7",
    title: "系统配置中心",
    content: "系统参数配置，包括基础设置、安全配置、集成配置等",
    type: "setting",
    category: "系统管理",
    url: "/settings/config",
    score: 0.76,
    createdAt: new Date("2024-01-09"),
    updatedAt: new Date("2024-01-15"),
    metadata: { configItems: 45, modified: 8 },
  },
  {
    id: "8",
    title: "消息通知中心",
    content: "统一的消息通知管理，支持多种通知类型和推送方式",
    type: "notification",
    category: "系统功能",
    url: "/notifications",
    score: 0.73,
    createdAt: new Date("2024-01-13"),
    updatedAt: new Date("2024-01-20"),
    metadata: { unread: 5, total: 23 },
  },
];

// 搜索建议数据
const mockSuggestions: SearchSuggestion[] = [
  { text: "用户管理", type: "query", count: 156 },
  { text: "AI助手", type: "query", count: 89 },
  { text: "工作流", type: "query", count: 67 },
  { text: "插件", type: "query", count: 45 },
  { text: "产品管理", type: "query", count: 34 },
  { text: "订单处理", type: "query", count: 28 },
  { text: "系统配置", type: "query", count: 23 },
  { text: "消息通知", type: "query", count: 19 },
];

export class SearchService {
  // 执行搜索
  static async search(params: SearchParams): Promise<SearchResponse> {
    await new Promise((resolve) => setTimeout(resolve, 300)); // 模拟网络延迟

    const { query, filter, page = 1, limit = 10 } = params;

    // 过滤和搜索逻辑
    let results = mockSearchData.filter((item) => {
      // 文本匹配
      const textMatch =
        query === "" ||
        item.title.toLowerCase().includes(query.toLowerCase()) ||
        item.content.toLowerCase().includes(query.toLowerCase()) ||
        item.category.toLowerCase().includes(query.toLowerCase());

      if (!textMatch) return false;

      // 类型过滤
      if (filter?.type && filter.type.length > 0) {
        if (!filter.type.includes(item.type)) return false;
      }

      // 分类过滤
      if (filter?.category && filter.category.length > 0) {
        if (!filter.category.includes(item.category)) return false;
      }

      // 日期范围过滤
      if (filter?.dateRange) {
        const itemDate = item.updatedAt;
        if (
          itemDate < filter.dateRange.start ||
          itemDate > filter.dateRange.end
        ) {
          return false;
        }
      }

      return true;
    });

    // 排序
    if (filter?.sortBy) {
      results.sort((a, b) => {
        let comparison = 0;
        switch (filter.sortBy) {
          case "relevance":
            comparison = b.score - a.score;
            break;
          case "date":
            comparison = b.updatedAt.getTime() - a.updatedAt.getTime();
            break;
          case "title":
            comparison = a.title.localeCompare(b.title);
            break;
        }
        return filter.sortOrder === "desc" ? comparison : -comparison;
      });
    }

    // 添加高亮
    if (query) {
      results = results.map((item) => ({
        ...item,
        highlight: this.generateHighlight(item, query),
      }));
    }

    // 分页
    const startIndex = (page - 1) * limit;
    const endIndex = startIndex + limit;
    const paginatedResults = results.slice(startIndex, endIndex);

    // 生成分面统计
    const facets = this.generateFacets(results);

    return {
      results: paginatedResults,
      total: results.length,
      page,
      limit,
      hasMore: endIndex < results.length,
      suggestions: query ? await this.getSuggestions(query) : [],
      facets,
    };
  }

  // 获取搜索建议
  static async getSuggestions(query: string): Promise<SearchSuggestion[]> {
    await new Promise((resolve) => setTimeout(resolve, 100));

    if (!query) return mockSuggestions.slice(0, 5);

    return mockSuggestions
      .filter((suggestion) =>
        suggestion.text.toLowerCase().includes(query.toLowerCase())
      )
      .slice(0, 5);
  }

  // 获取搜索历史
  static async getSearchHistory(): Promise<SearchHistory[]> {
    const history = localStorage.getItem("search-history");
    if (!history) return [];

    try {
      return JSON.parse(history).map((item: any) => ({
        ...item,
        timestamp: new Date(item.timestamp),
      }));
    } catch {
      return [];
    }
  }

  // 保存搜索历史
  static async saveSearchHistory(
    query: string,
    resultCount: number
  ): Promise<void> {
    if (!query.trim()) return;

    const history = await this.getSearchHistory();
    const newItem: SearchHistory = {
      id: Date.now().toString(),
      query: query.trim(),
      timestamp: new Date(),
      resultCount,
    };

    // 去重并限制数量
    const filteredHistory = history.filter(
      (item) => item.query !== newItem.query
    );
    const updatedHistory = [newItem, ...filteredHistory].slice(0, 20);

    localStorage.setItem("search-history", JSON.stringify(updatedHistory));
  }

  // 清除搜索历史
  static async clearSearchHistory(): Promise<void> {
    localStorage.removeItem("search-history");
  }

  // 生成高亮文本
  private static generateHighlight(item: SearchResult, query: string): string {
    const text = `${item.title} ${item.content}`;
    const index = text.toLowerCase().indexOf(query.toLowerCase());

    if (index === -1) return item.content.substring(0, 100) + "...";

    const start = Math.max(0, index - 50);
    const end = Math.min(text.length, index + query.length + 50);
    let highlight = text.substring(start, end);

    // 添加高亮标记
    const regex = new RegExp(`(${query})`, "gi");
    highlight = highlight.replace(regex, "<mark>$1</mark>");

    return (
      (start > 0 ? "..." : "") + highlight + (end < text.length ? "..." : "")
    );
  }

  // 生成分面统计
  private static generateFacets(results: SearchResult[]) {
    const types: Record<string, number> = {};
    const categories: Record<string, number> = {};

    results.forEach((item) => {
      types[item.type] = (types[item.type] || 0) + 1;
      categories[item.category] = (categories[item.category] || 0) + 1;
    });

    return { types, categories };
  }
}
