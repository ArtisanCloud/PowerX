import { useApiClient } from "../index";
import type {
  ApiResponse,
  PaginatedResponse,
  PaginationParams,
} from "../types/types";

// 产品相关接口类型定义
export interface Product {
  id: string;
  name: string;
  description: string;
  price: number;
  image?: string;
  category: string;
  stock: number;
  createdAt: string;
  updatedAt: string;
}

export interface ProductCreateParams {
  name: string;
  description: string;
  price: number;
  image?: string;
  category: string;
  stock: number;
}

export interface ProductUpdateParams {
  name?: string;
  description?: string;
  price?: number;
  image?: string;
  category?: string;
  stock?: number;
}

export interface ProductSearchParams extends PaginationParams {
  keyword?: string;
  category?: string;
  minPrice?: number;
  maxPrice?: number;
}

/**
 * 产品服务 API
 */
export const useProductService = () => {
  const apiClient = useApiClient();
  const baseUrl = "/products";

  return {
    /**
     * 获取产品列表
     */
    getProducts: (params?: ProductSearchParams) => {
      return apiClient.get<ApiResponse<PaginatedResponse<Product>>>(baseUrl, {
        params,
      });
    },

    /**
     * 获取指定产品信息
     */
    getProduct: (id: string) => {
      return apiClient.get<ApiResponse<Product>>(`${baseUrl}/${id}`);
    },

    /**
     * 创建产品
     */
    createProduct: (data: ProductCreateParams) => {
      return apiClient.post<ApiResponse<Product>>(baseUrl, data);
    },

    /**
     * 更新产品信息
     */
    updateProduct: (id: string, data: ProductUpdateParams) => {
      return apiClient.put<ApiResponse<Product>>(`${baseUrl}/${id}`, data);
    },

    /**
     * 删除产品
     */
    deleteProduct: (id: string) => {
      return apiClient.delete<ApiResponse<null>>(`${baseUrl}/${id}`);
    },

    /**
     * 获取产品分类列表
     */
    getCategories: () => {
      return apiClient.get<ApiResponse<string[]>>(`${baseUrl}/categories`);
    },
  };
};
