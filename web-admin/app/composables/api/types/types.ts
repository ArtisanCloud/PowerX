/**
 * API 请求参数类型定义
 */

// 基础分页请求参数
export interface PaginationParams {
  page?: number;
  pageSize?: number;
  sortBy?: string;
  sortOrder?: "asc" | "desc";
}

// 基础响应结构
export interface ApiResponse<T> {
  code: number;
  message: string;
  data: T;
  timestamp: number;
}

// 分页响应结构
export interface PaginatedResponse<T> {
  items: T[];
  total: number;
  page: number;
  pageSize: number;
  totalPages: number;
}

// 请求配置扩展
export interface ApiRequestConfig extends RequestInit {
  params?: Record<string, any>;
  useGlobalError?: boolean; // 是否使用全局错误处理
  useGlobalLoading?: boolean; // 是否使用全局加载状态
  skipAuth?: boolean; // 是否跳过认证
}

// 请求方法类型
export type HttpMethod = "GET" | "POST" | "PUT" | "DELETE" | "PATCH";

// 请求拦截器
export interface RequestInterceptor {
  onRequest?: (
    config: ApiRequestConfig
  ) => ApiRequestConfig | Promise<ApiRequestConfig>;
  onRequestError?: (error: any) => any | Promise<any>;
}

// 响应拦截器
export interface ResponseInterceptor {
  onResponse?: (response: any) => any | Promise<any>;
  onResponseError?: (error: any) => any | Promise<any>;
}
