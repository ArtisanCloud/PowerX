// 搜索结果类型
export interface SearchResult {
  id: string;
  title: string;
  content: string;
  type: SearchResultType;
  category: string;
  url: string;
  highlight?: string;
  score: number;
  createdAt: Date;
  updatedAt: Date;
  metadata?: Record<string, any>;
}

// 搜索结果类型常量（既是值也是类型）
export const SearchResultType = {
  USER: "user",
  CONTENT: "content",
  PRODUCT: "product",
  ORDER: "order",
  PLUGIN: "plugin",
  AGENT: "agent",
  WORKFLOW: "workflow",
  SETTING: "setting",
  NOTIFICATION: "notification",
} as const;

export type SearchResultType =
  (typeof SearchResultType)[keyof typeof SearchResultType];

// 搜索过滤器
export interface SearchFilter {
  type?: SearchResultType[];
  category?: string[];
  dateRange?: {
    start: Date;
    end: Date;
  };
  sortBy?: "relevance" | "date" | "title";
  sortOrder?: "asc" | "desc";
}

// 搜索请求参数
export interface SearchParams {
  query: string;
  filter?: SearchFilter;
  page?: number;
  limit?: number;
}

// 搜索响应
export interface SearchResponse {
  results: SearchResult[];
  total: number;
  page: number;
  limit: number;
  hasMore: boolean;
  suggestions?: string[];
  facets?: SearchFacets;
}

// 搜索分面统计
export interface SearchFacets {
  types: Record<SearchResultType, number>;
  categories: Record<string, number>;
}

// 搜索建议
export interface SearchSuggestion {
  text: string;
  type: "query" | "filter";
  count?: number;
}

// 搜索历史
export interface SearchHistory {
  id: string;
  query: string;
  timestamp: Date;
  resultCount: number;
}
