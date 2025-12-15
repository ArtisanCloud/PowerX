import { useApiClient } from "../index";
import type { ApiResponse } from "../types/types";

// 角色接口定义
export interface Role {
  id: number;
  createdAt: string;
  updatedAt: string;
  DeletedAt?: string | null;
  scope: "system" | "tenant";
  tenant_uuid?: string;
  code: string;
  name: string;
  description?: string;
  builtin: boolean;
}

// 角色列表查询参数
export interface RoleListParams {
  scope?: string;
  tenant_uuid?: string;
  keyword?: string;
  builtin?: boolean;
  page?: number;
  page_size?: number;
  sort?: string;
}

// 角色创建参数
// 角色创建参数
export interface RoleCreateParams {
  scope: "system" | "tenant";
  tenant_uuid?: string; // 可选，系统角色不需要
  code: string;
  name: string;
  description?: string;
  perm_ids?: number[]; // 新增：创建时直接分配权限
}

// 角色更新参数
// 角色更新参数
export interface RoleUpdateParams {
  name: string;
  code?: string; // 添加 code 字段
  description?: string;
}

// 权限设置结果
export interface SetIDsResult {
  added: number[] | null;
  removed: number[] | null;
  now: number[];
  skipped_deprecated: number[] | null;
}

// 创建角色响应（带权限）
export interface CreateRoleWithPermsResponse {
  role: Role;
  perm?: SetIDsResult;
}

// 分页响应
export interface RoleListResponse {
  items: Role[];
  pagination: {
    total: number;
    page: number;
    page_size: number;
    pages: number;
  };
}

/**
 * 角色服务 API
 */
export const useRoleService = () => {
  const apiClient = useApiClient();
  const baseUrl = "/admin/iam/roles";

  return {
    /**
     * 获取角色列表
     */
    getRoles: async (params?: RoleListParams) => {
      const queryParams = new URLSearchParams();

      if (params?.scope) queryParams.append("scope", params.scope);
      if (params?.tenant_uuid)
        queryParams.append("tenant_uuid", params.tenant_uuid);
      if (params?.keyword) queryParams.append("keyword", params.keyword);
      if (params?.builtin !== undefined)
        queryParams.append("builtin", params.builtin.toString());
      if (params?.page) queryParams.append("page", params.page.toString());
      if (params?.page_size)
        queryParams.append("page_size", params.page_size.toString());
      if (params?.sort) queryParams.append("sort", params.sort);

      const url = queryParams.toString()
        ? `${baseUrl}?${queryParams.toString()}`
        : baseUrl;
      return apiClient.get<ApiResponse<RoleListResponse>>(url);
    },

    /**
     * 获取单个角色信息
     */
    getRole: (id: number) => {
      return apiClient.get<ApiResponse<Role>>(`${baseUrl}/${id}`);
    },

    /**
     * 创建角色
     */
    createRole: (data: RoleCreateParams) => {
      return apiClient.post<ApiResponse<Role>>(baseUrl, data);
    },

    /**
     * 更新角色
     */
    updateRole: (id: number, data: RoleUpdateParams) => {
      return apiClient.patch<ApiResponse<{ updated: boolean }>>(
        `${baseUrl}/${id}`,
        data
      );
    },

    /**
     * 删除角色
     */
    deleteRole: (id: number) => {
      return apiClient.delete<ApiResponse<{ deleted: boolean }>>(
        `${baseUrl}/${id}`
      );
    },
  };
};
