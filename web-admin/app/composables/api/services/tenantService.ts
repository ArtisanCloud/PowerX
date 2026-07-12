import { useApiClient } from "../index";
import type { ApiResponse } from "../types/types";

// 租户接口定义
export interface Tenant {
  id: number;
  uuid: string;
  name: string;
  domain?: string;
  status: string;
  createdAt: string;
  updatedAt: string;
}

// 租户列表查询参数
export interface TenantListParams {
  q?: string;
  tenant_uuid?: string;
  page?: number;
  page_size?: number;
  status?: string;
}

// 租户列表响应
export interface TenantListResponse {
  items: Tenant[];
  pagination: {
    total: number;
    page: number;
    page_size: number;
    pages: number;
  };
}

/**
 * 租户服务 API
 */
export const useTenantService = () => {
  const apiClient = useApiClient();
  const baseUrl = "/admin/tenants";

  return {
    /**
     * 获取租户列表
     */
    getTenants: async (params?: TenantListParams) => {
      const queryParams = new URLSearchParams();

      if (params?.q) queryParams.append("q", params.q);
      if (params?.tenant_uuid)
        queryParams.append("tenant_uuid", params.tenant_uuid);
      if (params?.page) queryParams.append("page", params.page.toString());
      if (params?.page_size)
        queryParams.append("page_size", params.page_size.toString());
      if (params?.status) queryParams.append("status", params.status);

      const url = queryParams.toString()
        ? `${baseUrl}?${queryParams.toString()}`
        : baseUrl;
      return apiClient.get<ApiResponse<TenantListResponse>>(url);
    },

    /**
     * 获取单个租户信息
     */
    getTenant: (id: number) => {
      return apiClient.get<ApiResponse<Tenant>>(`${baseUrl}/${id}`);
    },
  };
};
