import { useApiClient } from "../index";
import type {
  ApiResponse,
  PaginatedResponse,
  PaginationParams,
} from "../types/types";

// 用户相关接口类型定义
export interface User {
  uuid: string;
  createdAt: string;
  updatedAt: string;
  DeletedAt?: string | null;
  email?: string;
  phone?: string;
  display_name: string;
  avatar_url?: string;
  status: number;
  is_root?: boolean;
  meta?: any;
}

export interface Member {
  uuid: string;
  createdAt: string;
  updatedAt: string;
  DeletedAt?: string | null;
  tenant_uuid: string;
  user_uuid: string;
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
  tenant_uuid: string; // 租户筛选
  status?: number; // 状态筛选
  sort_by?: string;
  sort_order?: string;
}

export interface CreateSystemUserParams {
  tenant_uuid: string;

  // User 基础字段
  email?: string;
  phone?: string;
  display_name: string;
  avatar_url?: string;
  status?: number;
  meta?: any;

  // 扩展字段
  username: string;
  initial_password?: string;
  dept_ids?: number[];
  role_uuids?: string[];
}

export interface UpdateUserParams {
  tenant_uuid: string;
  username: string;
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

export interface ResetPasswordParams {
  new_password: string;
}

export interface AddUserToTenantParams {
}

export interface SetUserRolesParams {
  tenant_uuid: string;
  role_uuids: string[];
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
    getUsers: (params: UserListParams) => {
      return apiClient.get<ApiResponse<PaginatedResponse<MemberWithProfile>>>(
        baseUrl,
        {
          params,
        }
      );
    },

    /**
     * 获取指定用户信息
     * GET /api/admin/system/users/:user_uuid
     */
    getUser: (userUuid: string) => {
      return apiClient.get<ApiResponse<MemberWithProfile>>(
        `${baseUrl}/${userUuid}`
      );
    },

    /**
     * 创建系统用户（Root视角：创建全局User并在指定租户创建Member）
     * POST /api/admin/system/users
     */
    createSystemUser: (data: CreateSystemUserParams) => {
      return apiClient.post<ApiResponse<{ user_uuid: string }>>(baseUrl, data);
    },

    /**
     * 更新用户信息
     * PATCH /api/admin/system/users/:user_uuid
     */
    updateUser: (userUuid: string, data: UpdateUserParams) => {
      return apiClient.patch<ApiResponse<{ ok: boolean }>>(
        `${baseUrl}/${userUuid}`,
        data
      );
    },

    /**
     * 设置用户状态
     * PUT /api/admin/system/users/:user_uuid/status
     */
    setUserStatus: (userUuid: string, data: SetStatusParams) => {
      return apiClient.put<ApiResponse<{ ok: boolean }>>(
        `${baseUrl}/${userUuid}/status`,
        data
      );
    },

    /**
     * 删除用户
     * DELETE /api/admin/system/users/:user_uuid
     */
    deleteUser: (userUuid: string) => {
      return apiClient.delete<ApiResponse<{ ok: boolean }>>(
        `${baseUrl}/${userUuid}`
      );
    },

    /**
     * 恢复用户
     * PUT /api/admin/system/users/:user_uuid/restore
     */
    restoreUser: (userUuid: string) => {
      return apiClient.put<ApiResponse<{ ok: boolean }>>(
        `${baseUrl}/${userUuid}/restore`
      );
    },

    /**
     * 强制用户下线
     * POST /api/admin/system/users/:user_uuid/force-logout
     */
    forceLogout: (userUuid: string, data: ForceLogoutParams) => {
      return apiClient.post<ApiResponse<{ ok: boolean }>>(
        `${baseUrl}/${userUuid}/force-logout`,
        data
      );
    },

    /**
     * 管理员重置用户密码
     * PUT /api/admin/system/users/:user_uuid/password
     */
    resetUserPassword: (userUuid: string, data: ResetPasswordParams) => {
      return apiClient.put<ApiResponse<{ ok: boolean }>>(
        `${baseUrl}/${userUuid}/password`,
        data
      );
    },

    /**
     * 获取用户在当前租户下的角色 ID 列表
     * GET /api/admin/system/users/:user_uuid/roles
     */
    getUserRoles: (userUuid: string, params: { tenant_uuid: string }) => {
      return apiClient.get<ApiResponse<{ role_uuids: string[] }>>(
        `${baseUrl}/${userUuid}/roles`,
        { params }
      );
    },

    /**
     * 设置用户在当前租户下的角色
     * PUT /api/admin/system/users/:user_uuid/roles
     */
    setUserRoles: (userUuid: string, data: SetUserRolesParams) => {
      return apiClient.put<ApiResponse<{ ok: boolean }>>(
        `${baseUrl}/${userUuid}/roles`,
        data
      );
    },

    /**
     * 将已存在的用户加入某个租户
     * PATCH /api/admin/system/users/:user_uuid/add-to-tenant
     */
    addUserToTenant: (userUuid: string, data: AddUserToTenantParams) => {
      return apiClient.patch<ApiResponse<{ member_uuid: string }>>(
        `${baseUrl}/${userUuid}/add-to-tenant`,
        data
      );
    },
  };
};
