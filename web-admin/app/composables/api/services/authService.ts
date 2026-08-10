import { useApiClient } from "../index";
import type {
  ApiResponse,
  PaginatedResponse,
  PaginationParams,
} from "../types/types";

/**
 * 认证相关类型定义
 */
export interface LoginParams {
  tenant: string;
  identifier: string;
  password: string;
  remember?: boolean;
}

export interface RegisterParams {
  username: string;
  email: string;
  phone?: string;
  password: string;
  display_name?: string;
  avatar_url?: string;
  invite_token?: string;
  meta?: Record<string, any>;
}

// 前端表单数据接口（包含确认密码）
export interface RegisterFormData {
  tenantName: string;
  contact: string;
  password: string;
  confirmPassword: string;
  verificationCode: string;
  tenantKey?: string;
  inviteCode?: string;
  channel?: string;
  email?: string;
  phone?: string;
  displayName?: string;
  avatarUrl?: string;
  inviteToken?: string;
}

// 注册响应接口
export interface RegisterResponse {
  user: User;
  member: Member;
  tenant?: Tenant;
}

export interface User {
  id: string;
  created_at: string;
  updated_at: string;
  email: string;
  phone?: string;
  display_name?: string;
  avatar_url?: string;
  status: number; // 1=active, 0=disabled
  meta?: Record<string, any>;
  last_login_at?: number;
}

export interface Tenant {
  id: string;
  created_at: string;
  updated_at: string;
  key: string; // 全局唯一，如 system、acme
  name: string;
  status: number; // 1=active, 0=disabled
  plan: string; // 默认 'free'
}

export interface Member {
  id: string;
  created_at: string;
  updated_at: string;
  tenant_uuid: string;
  user_id: number;
  username: string; // 建议统一小写
  display_name?: string;
  avatar_url?: string;
  status: number; // 1=active, 0=disabled
  meta?: Record<string, any>;
}

export interface Role {
  id: string;
  name: string;
  code: string;
  description?: string;
  permissions: Permission[];
  createdAt: string;
  updatedAt: string;
}

export interface Permission {
  id: string;
  name: string;
  code: string;
  resource: string;
  action: string;
  description?: string;
}

export interface Department {
  id: string;
  name: string;
  code: string;
  parentId?: string;
  parent?: Department;
  children?: Department[];
  description?: string;
  createdAt: string;
  updatedAt: string;
}

export interface LoginResponse {
  token_type: string;
  access_token: string;
  expires_in: number;
  refresh_token: string;
  scope: string;
}

export interface SaaSSignupParams {
  tenant_key?: string;
  tenant_name: string;
  plan?: string;
  owner_email?: string;
  owner_phone?: string;
  owner_password: string;
  owner_display_name?: string;
  verification_code?: string;
  invite_code?: string;
  channel?: string;
  campaign?: string;
}

export interface SaaSSignupResponse {
  token_type?: string;
  access_token: string;
  expires_in?: number;
  refresh_token: string;
  scope?: string;
  context: Record<string, any>;
}

// 用户信息响应接口（用于获取用户详情）
export interface UserInfoResponse {
  user: User;
  member?: Member; // 当前租户下的成员信息
  tenant?: Tenant; // 当前租户信息
}

export interface RefreshTokenParams {
  refreshToken: string;
}

export interface ChangePasswordParams {
  oldPassword: string;
  newPassword: string;
  confirmPassword: string;
}

export interface ResetPasswordParams {
  email: string;
}

export interface ResetPasswordConfirmParams {
  token: string;
  newPassword: string;
  confirmPassword: string;
}

/**
 * 认证服务 API
 */
export interface UserFilters extends PaginationParams {
  department?: string;
  role?: string;
  status?: "active" | "inactive" | "suspended";
  search?: string;
}

export interface RoleFilters extends PaginationParams {
  search?: string;
}

export interface DepartmentFilters extends PaginationParams {
  parentId?: string;
  search?: string;
}

export const useAuthService = () => {
  const apiClient = useApiClient();
  const adminBaseUrl = "/admin"; // 添加管理员基础URL
  const baseUrl = adminBaseUrl + "/user/auth";

  return {
    /**
     * 用户登录
     */
    login: (data: LoginParams) => {
      return apiClient.post<ApiResponse<LoginResponse>>(
        `${baseUrl}/login`,
        data,
        { skipAuth: true }
      );
    },

    /**
     * 用户注册
     */
    register: (data: RegisterParams) => {
      return apiClient.post<ApiResponse<RegisterResponse>>(
        `${baseUrl}/register`,
        data,
        { skipAuth: true }
      );
    },

    /**
     * 前端表单注册（转换数据格式）
     */
    registerFromForm: (formData: RegisterFormData) => {
      const contact = formData.contact.trim();
      const isEmail = contact.includes("@");
      const registerData: SaaSSignupParams = {
        tenant_key: formData.tenantKey || undefined,
        tenant_name: formData.tenantName || "",
        owner_email: isEmail ? contact : undefined,
        owner_phone: isEmail ? undefined : contact,
        owner_password: formData.password,
        owner_display_name: formData.displayName || contact,
        verification_code: formData.verificationCode || undefined,
        invite_code: formData.inviteCode || undefined,
        channel: formData.channel || undefined,
        plan: "free",
      };
      return apiClient.post<ApiResponse<SaaSSignupResponse>>(
        "/public/saas/signup",
        registerData,
        { skipAuth: true }
      );
    },

    sendSignupVerificationCode: (contact: string, channel?: string) => {
      return apiClient.post<ApiResponse<{ sent: boolean; contact: string }>>(
        "/public/saas/signup/verification-code",
        { contact, channel },
        { skipAuth: true }
      );
    },

    /**
     * 用户登出
     */
    logout: (data?: { refresh_token: string }) => {
      return apiClient.post<ApiResponse<null>>(`${baseUrl}/logout`, data);
    },

    /**
     * 刷新访问令牌
     */
    refreshToken: (data: RefreshTokenParams) => {
      return apiClient.post<ApiResponse<LoginResponse>>(
        `${baseUrl}/refresh`,
        data
      );
    },

    /**
     * 获取当前用户信息
     */
    getCurrentUser: () => {
      return apiClient.get<ApiResponse<UserInfoResponse>>(`${baseUrl}/me`);
    },

    /**
     * 更新用户资料
     */
    updateProfile: (data: Partial<User>) => {
      return apiClient.put<ApiResponse<User>>(`${baseUrl}/profile`, data);
    },

    /**
     * 修改密码
     */
    changePassword: (data: ChangePasswordParams) => {
      return apiClient.post<ApiResponse<null>>(
        `${baseUrl}/change-password`,
        data
      );
    },

    /**
     * 重置密码请求
     */
    resetPassword: (data: ResetPasswordParams) => {
      return apiClient.post<ApiResponse<null>>(
        `${baseUrl}/reset-password`,
        data
      );
    },

    /**
     * 确认重置密码
     */
    resetPasswordConfirm: (data: ResetPasswordConfirmParams) => {
      return apiClient.post<ApiResponse<null>>(
        `${baseUrl}/reset-password/confirm`,
        data
      );
    },

    /**
     * 验证令牌有效性
     */
    validateToken: () => {
      return apiClient.get<ApiResponse<{ valid: boolean }>>(
        `${baseUrl}/validate`
      );
    },

    /**
     * 获取用户权限
     */
    getUserPermissions: () => {
      return apiClient.get<ApiResponse<Permission[]>>(`${baseUrl}/permissions`);
    },

    // ========== 用户管理 API ==========
    /**
     * 获取用户列表（分页）
     */
    getUsers: (filters?: UserFilters) => {
      return apiClient.get<ApiResponse<PaginatedResponse<User>>>(
        `${adminBaseUrl}/users`,
        { params: filters }
      );
    },

    /**
     * 获取指定用户
     */
    getUser: (id: string) => {
      return apiClient.get<ApiResponse<User>>(`${adminBaseUrl}/users/${id}`);
    },

    /**
     * 创建用户
     */
    createUser: (data: RegisterParams) => {
      return apiClient.post<ApiResponse<User>>(`${adminBaseUrl}/users`, data);
    },

    /**
     * 更新用户
     */
    updateUser: (id: string, data: Partial<User>) => {
      return apiClient.put<ApiResponse<User>>(
        `${adminBaseUrl}/users/${id}`,
        data
      );
    },

    /**
     * 删除用户
     */
    deleteUser: (id: string) => {
      return apiClient.delete<ApiResponse<null>>(`${adminBaseUrl}/users/${id}`);
    },

    /**
     * 批量删除用户
     */
    batchDeleteUsers: (ids: string[]) => {
      return apiClient.post<ApiResponse<null>>(
        `${adminBaseUrl}/users/batch-delete`,
        {
          ids,
        }
      );
    },

    /**
     * 启用/禁用用户
     */
    toggleUserStatus: (id: string, status: number) => {
      return apiClient.patch<ApiResponse<User>>(
        `${adminBaseUrl}/users/${id}/status`,
        {
          status,
        }
      );
    },

    // ========== 角色管理 API ==========
    /**
     * 获取角色列表（分页）
     */
    getRoles: (filters?: RoleFilters) => {
      return apiClient.get<ApiResponse<PaginatedResponse<Role>>>(
        `${adminBaseUrl}/roles`,
        { params: filters }
      );
    },

    /**
     * 获取指定角色
     */
    getRole: (id: string) => {
      return apiClient.get<ApiResponse<Role>>(`${adminBaseUrl}/roles/${id}`);
    },

    /**
     * 创建角色
     */
    createRole: (data: Omit<Role, "id" | "createdAt" | "updatedAt">) => {
      return apiClient.post<ApiResponse<Role>>(`${adminBaseUrl}/roles`, data);
    },

    /**
     * 更新角色
     */
    updateRole: (id: string, data: Partial<Role>) => {
      return apiClient.put<ApiResponse<Role>>(
        `${adminBaseUrl}/roles/${id}`,
        data
      );
    },

    /**
     * 删除角色
     */
    deleteRole: (id: string) => {
      return apiClient.delete<ApiResponse<null>>(`${adminBaseUrl}/roles/${id}`);
    },

    /**
     * 分配角色权限
     */
    assignRolePermissions: (roleId: string, permissionIds: string[]) => {
      return apiClient.post<ApiResponse<Role>>(
        `${adminBaseUrl}/roles/${roleId}/permissions`,
        { permissionIds }
      );
    },

    // ========== 部门管理 API ==========
    /**
     * 获取部门列表（分页）
     */
    getDepartments: (filters?: DepartmentFilters) => {
      return apiClient.get<ApiResponse<PaginatedResponse<Department>>>(
        `${adminBaseUrl}/departments`,
        { params: filters }
      );
    },

    /**
     * 获取部门树形结构
     */
    getDepartmentTree: () => {
      return apiClient.get<ApiResponse<Department[]>>(
        `${adminBaseUrl}/departments/tree`
      );
    },

    /**
     * 获取指定部门
     */
    getDepartment: (id: string) => {
      return apiClient.get<ApiResponse<Department>>(
        `${adminBaseUrl}/departments/${id}`
      );
    },

    /**
     * 创建部门
     */
    createDepartment: (
      data: Omit<Department, "id" | "createdAt" | "updatedAt" | "children">
    ) => {
      return apiClient.post<ApiResponse<Department>>(
        `${adminBaseUrl}/departments`,
        data
      );
    },

    /**
     * 更新部门
     */
    updateDepartment: (id: string, data: Partial<Department>) => {
      return apiClient.put<ApiResponse<Department>>(
        `${adminBaseUrl}/departments/${id}`,
        data
      );
    },

    /**
     * 删除部门
     */
    deleteDepartment: (id: string) => {
      return apiClient.delete<ApiResponse<null>>(
        `${adminBaseUrl}/departments/${id}`
      );
    },

    // ========== 权限管理 API ==========
    /**
     * 获取所有权限
     */
    getAllPermissions: () => {
      return apiClient.get<ApiResponse<Permission[]>>(
        `${adminBaseUrl}/permissions`
      );
    },

    /**
     * 获取权限分组
     */
    getPermissionGroups: () => {
      return apiClient.get<ApiResponse<Record<string, Permission[]>>>(
        `${adminBaseUrl}/permissions/groups`
      );
    },
  };
};
