import { useApiClient } from "../index";
import type { ApiResponse } from "../types/types";
import type { CreateSystemUserParams, ResetPasswordParams, SetStatusParams, SetUserRolesParams, UpdateUserParams, UserListParams, UserListResponse } from "./userService";

// 租户管理只操作登录上下文中的成员；不向服务端传入目标租户或系统用户标识。
export const useTenantUserService = () => {
  const client = useApiClient();
  const base = "/admin/iam/members";
  const status = (value?: number) => value === 0 ? 2 : value;
  return {
    getUsers: ({ tenant_uuid: _tenant, ...params }: UserListParams) =>
      client.get<ApiResponse<UserListResponse>>(base, { params: { ...params, status: status(params.status) } }),
    createUser: (data: CreateSystemUserParams) => client.post(base, {
      member: { username: data.username, display_name: data.display_name, avatar_url: data.avatar_url, status: status(data.status), meta: data.meta },
      user: { email: data.email, phone: data.phone },
      initial_password: data.initial_password,
      dept_ids: data.dept_ids,
      role_uuids: data.role_uuids,
    }),
    updateUser: (memberUuid: string, data: UpdateUserParams & { role_uuids?: string[] }) => client.patch(`${base}/${memberUuid}`, {
      member: { username: data.username, display_name: data.display_name, avatar_url: data.avatar_url, status: status(data.status) },
      contact: { email: data.email, phone: data.phone },
      dept_ids: data.dept_ids,
      role_uuids: data.role_uuids,
    }),
    getUserRoles: (memberUuid: string, _params?: { tenant_uuid: string }) =>
      client.get<ApiResponse<{ role_uuids: string[] }>>(`${base}/${memberUuid}/roles`),
    setUserRoles: (memberUuid: string, data: SetUserRolesParams) =>
      client.put(`${base}/${memberUuid}/roles`, { role_uuids: data.role_uuids }),
    setUserStatus: (memberUuid: string, data: SetStatusParams) =>
      client.put(`${base}/${memberUuid}/status`, { status: status(data.status) }),
    deleteUser: (memberUuid: string) => client.delete(`${base}/${memberUuid}`),
    resetUserPassword: (memberUuid: string, data: ResetPasswordParams) =>
      client.put(`${base}/${memberUuid}/password`, data),
  };
};
