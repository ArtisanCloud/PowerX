import { useApiClient } from "../index";
import type {
  ApiResponse,
  PaginatedResponse,
  PaginationParams,
} from "../types/types";

// 用户相关接口类型定义
export interface User {
  id: number;
  uuid: string;
  createdAt: string;
  updatedAt: string;
  DeletedAt?: string | null;
  email?: string;
  phone?: string;
  display_name: string;
  avatar_url?: string;
  status: number;
  meta?: any;
}

export interface Member {
  id: number;
  uuid: string;
  createdAt: string;
  updatedAt: string;
  DeletedAt?: string | null;
  tenant_id: number;
  user_id: number;
  username: string;
  display_name: string;
  avatar_url?: string;
  status: number;
  meta?: any;
}

export interface MemberWithProfile {
  Member: Member;
  User: User;
  DeptIDs: number[] | null;
}

export interface UserListParams extends PaginationParams {
  q?: string; // 关键词搜索
  tenant_id?: number; // 租户筛选
  status?: number; // 状态筛选
  sort_by?: string;
  sort_order?: string;
}

export interface CreateSystemUserParams {
  // User 基础字段
  email?: string;
  phone?: string;
  display_name: string;
  avatar_url?: string;
  status?: number;
  meta?: any;

  // 扩展字段
  username: string;
  tenant_id: number;
  initial_password?: string;
  dept_ids?: number[];
}

export interface UpdateUserParams {
  email?: string;
  phone?: string;
  display_name?: string;
  avatar_url?: string;
  status?: number;
}

export interface SetStatusParams {
  status: number;
}

export interface ForceLogoutParams {
  jti?: string;
}

export interface AddUserToTenantParams {
  tenant_id: number;
}

export interface UserLoginParams {
  email: string;
  password: string;
  remember?: boolean;
}

export interface UserRegisterParams {
  username: string;
  email: string;
  password: string;
}

/**
 * 用户服务 API
 */
export const useUserService = () => {
  const apiClient = useApiClient();
  const baseUrl = "/admin/system/users";

  return {
    /**
     * 获取用户列表
     * GET /api/admin/system/users
     */
    getUsers: (params?: UserListParams) => {
      return apiClient.get<ApiResponse<PaginatedResponse<MemberWithProfile>>>(
        baseUrl,
        {
          params,
        }
      );
    },

    /**
     * 获取指定用户信息
     * GET /api/admin/system/users/:id
     */
    getUser: (id: number) => {
      return apiClient.get<ApiResponse<MemberWithProfile>>(`${baseUrl}/${id}`);
    },

    /**
     * 创建系统用户（Root视角：创建全局User并在指定租户创建Member）
     * POST /api/admin/system/users
     */
    createSystemUser: (data: CreateSystemUserParams) => {
      return apiClient.post<ApiResponse<{ id: number }>>(baseUrl, data);
    },

    /**
     * 更新用户信息
     * PATCH /api/admin/system/users/:id
     */
    updateUser: (id: number, data: UpdateUserParams) => {
      return apiClient.patch<ApiResponse<{ ok: boolean }>>(
        `${baseUrl}/${id}`,
        data
      );
    },

    /**
     * 设置用户状态
     * PUT /api/admin/system/users/:id/status
     */
    setUserStatus: (id: number, data: SetStatusParams) => {
      return apiClient.put<ApiResponse<{ ok: boolean }>>(
        `${baseUrl}/${id}/status`,
        data
      );
    },

    /**
     * 删除用户
     * DELETE /api/admin/system/users/:id
     */
    deleteUser: (id: number) => {
      return apiClient.delete<ApiResponse<{ ok: boolean }>>(`${baseUrl}/${id}`);
    },

    /**
     * 恢复用户
     * PUT /api/admin/system/users/:id/restore
     */
    restoreUser: (id: number) => {
      return apiClient.put<ApiResponse<{ ok: boolean }>>(
        `${baseUrl}/${id}/restore`
      );
    },

    /**
     * 强制用户下线
     * POST /api/admin/system/users/:id/force-logout
     */
    forceLogout: (id: number, data: ForceLogoutParams) => {
      return apiClient.post<ApiResponse<{ ok: boolean }>>(
        `${baseUrl}/${id}/force-logout`,
        data
      );
    },

    /**
     * 将已存在的用户加入某个租户
     * PATCH /api/admin/system/users/:id/add-to-tenant
     */
    addUserToTenant: (id: number, data: AddUserToTenantParams) => {
      return apiClient.patch<ApiResponse<{ member_id: number }>>(
        `${baseUrl}/${id}/add-to-tenant`,
        data
      );
    },

    // 兼容性方法（保持向后兼容）
    /**
     * @deprecated 使用 createSystemUser 替代
     */
    createUser: (data: CreateSystemUserParams) => {
      return apiClient.post<ApiResponse<{ id: number }>>(baseUrl, data);
    },

    /**
     * @deprecated 使用 setUserStatus 替代
     */
    updateUserStatus: (id: number, status: number) => {
      return apiClient.put<ApiResponse<{ ok: boolean }>>(
        `${baseUrl}/${id}/status`,
        { status }
      );
    },
  };
};
