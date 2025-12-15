import { defineStore } from "pinia";
import { ref, computed, readonly } from "vue"; // ✅ 补充 readonly
import { useApiClient } from "~/composables/api";
import { getStoredTenantUUID } from "~/utils/tenant-context";

// 权限类型定义
export interface Permission {
  id: number;
  plugin: string;
  resource: string;
  action: string;
  effect: string;
  description?: string;
  status: "active" | "deprecated";
  source?: string;
  introduced?: string;
  deprecated_at?: number | null;
  meta?: {
    label?: string;
    module?: string;
    type?: "menu" | "action" | "api" | "data" | string;
    api_endpoint?: string;
    http_method?: string;
  };
}

// ✅ 追加到顶部类型区
export type PermissionMeta = {
  label?: string;
  module?: string;
  type?: "menu" | "action" | "data" | "api";
  api_endpoint?: string;
  http_method?: "GET" | "POST" | "PUT" | "DELETE" | "PATCH";
};

export type PermissionDTO = {
  id: number;
  plugin: string;
  resource: string;
  action: string;
  description?: string;
  status: "active" | "deprecated";
  meta?: PermissionMeta;
};

// Catalog 类型：module -> type -> Permission[]
export type PermissionCatalog = Record<string, Record<string, Permission[]>>;

// List 查询参数
export interface PermissionListQuery {
  plugin?: string;
  resource?: string;
  action?: string;
  module?: string;
  type?: string;
  status?: "active" | "deprecated";
  keyword?: string;
  page?: number;
  size?: number;
  sort?: string;
}

// List 响应
export interface PermissionListResponse {
  items: Permission[];
  pagination: {
    total: number;
    page: number;
    page_size: number;
    pages: number;
  };
}

// 租户权限关联类型
export interface TenantPermission {
  id: number;
  tenant_uuid: string;
  permission_id: number;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export const usePermissionStore = defineStore("permission", () => {
  // 使用 API 客户端
  const { get, post, put, delete: del } = useApiClient();

  // API 基础路径
  const baseUrl = "/admin/iam";

  // 状态
  const catalog = ref<PermissionCatalog>({});
  const listData = ref<PermissionListResponse>({
    items: [],
    pagination: { total: 0, page: 1, page_size: 20, pages: 0 },
  });
  const tenantPermissions = ref<TenantPermission[]>([]);
  const isLoading = ref(false);
  const error = ref<string | null>(null);
  const lastSyncTime = ref<number | null>(null);

  // ✅ 在 defineStore 内部追加 state
  // ✅ 在 defineStore 内部追加 state
  const roleSelection = ref<Record<number, number[]>>({}); // roleId -> 已选权限ID列表（本地缓存）
  const roleInitialSelection = ref<Record<number, number[]>>({}); // roleId -> 初始权限ID列表（用于判断dirty）

  // 计算属性
  const catalogTree = computed(() => {
    return Object.entries(catalog.value).map(([module, groups]) => ({
      id: module,
      label: module,
      children: Object.entries(groups).map(([type, items]) => ({
        id: `${module}:${type}`,
        label: type,
        children: items.map((p) => ({
          id: p.id,
          label: p.meta?.label || `${p.resource}:${p.action}`,
          raw: p,
        })),
      })),
    }));
  });

  // ✅ 映射把后端权限记录转成前端展示结构
  const normalizedList = computed(() => {
    const items: any[] = (listData.value?.items ?? []) as any[];
    return items.map((p: PermissionDTO) => {
      const m = p.meta || {};
      const code = `${p.resource || ""}.${p.action || ""}`.replace(/^\./, "");
      return {
        id: p.id,
        // 展示名称优先 label
        name: m.label || code,
        code,
        module: m.module || p.plugin || "",
        description: p.description || "",
        type: (m.type as any) || "action",
        apiEndpoint: m.api_endpoint,
        httpMethod: m.http_method,
        __raw: p,
      };
    });
  });

  const enabledPermissionsCount = computed(() => {
    return tenantPermissions.value.filter((tp) => tp.enabled).length;
  });

  const totalPermissionsCount = computed(() => {
    return Object.values(catalog.value)
      .flatMap((groups) => Object.values(groups))
      .flatMap((items) => items).length;
  });

  // 获取权限目录（用于角色授权树形结构）
  const fetchCatalog = async (forceRefresh = false) => {
    if (!forceRefresh && Object.keys(catalog.value).length > 0) {
      return catalog.value;
    }

    isLoading.value = true;
    error.value = null;

    try {
      const response = await get<any>(`${baseUrl}/permissions/catalog`);
      // 处理后端返回的包装结构 { code, message, data, timestamp }
      catalog.value = response.data || response;
      lastSyncTime.value = Date.now();
      return catalog.value;
    } catch (err) {
      error.value = err instanceof Error ? err.message : "获取权限目录失败";
      throw err;
    } finally {
      isLoading.value = false;
    }
  };

  // 获取权限列表（用于管理表格）
  const fetchList = async (query: PermissionListQuery = {}) => {
    isLoading.value = true;
    error.value = null;

    try {
      const response = await get<any>(`${baseUrl}/permissions`, {
        params: query,
      });
      // 处理后端返回的包装结构 { code, message, data, timestamp }
      listData.value = response.data || response;
      return listData.value;
    } catch (err) {
      error.value = err instanceof Error ? err.message : "获取权限列表失败";
      throw err;
    } finally {
      isLoading.value = false;
    }
  };

  // === 新增：全量拉取 ===
  const fetchAllActive = async () => {
    isLoading.value = true;
    error.value = null;
    try {
      const pageSize = 200; // 分批取，避免一次超大
      let page = 1;
      let pages = 1;
      const all: Permission[] = [];

      while (page <= pages) {
        const res = await get<any>(`${baseUrl}/permissions`, {
          params: {
            page,
            page_size: pageSize,
            status: "active",
            sort: "plugin asc, resource asc, action asc",
          },
        });
        const payload = res?.data?.data || res?.data || res;
        const items: Permission[] = payload?.items ?? [];
        const pgn = payload?.pagination ?? {};
        pages = Number(pgn?.pages || 1);
        all.push(...items);
        // console.log(
        //   `fetchAllActive: page=${page}, pages=${pages}, total=${pgn.total}, items=${items.length}`
        // );
        page++;
      }

      // 全量塞进 listData，页面直接用 normalizedList 渲染
      listData.value = {
        items: all,
        pagination: {
          total: all.length,
          page: 1,
          page_size: all.length,
          pages: 1,
        },
      };
      return all;
    } catch (err: any) {
      error.value = err?.message || "获取权限失败";
      throw err;
    } finally {
      isLoading.value = false;
    }
  };

  // 创建权限（仅 Root 用户）
  const createPermission = async (data: Partial<Permission>) => {
    isLoading.value = true;
    error.value = null;

    try {
      const response = await post<any>(`${baseUrl}/permissions`, data);
      const newPermission = response.data || response;
      await fetchCatalog(true);
      return newPermission;
    } catch (err) {
      error.value = err instanceof Error ? err.message : "创建权限失败";
      throw err;
    } finally {
      isLoading.value = false;
    }
  };

  // 更新权限（仅 Root 用户）
  const updatePermission = async (id: number, data: Partial<Permission>) => {
    isLoading.value = true;
    error.value = null;

    try {
      const response = await put<any>(`${baseUrl}/permissions/${id}`, data);
      const updatedPermission = response.data || response;
      await fetchCatalog(true);
      return updatedPermission;
    } catch (err) {
      error.value = err instanceof Error ? err.message : "更新权限失败";
      throw err;
    } finally {
      isLoading.value = false;
    }
  };

  // 删除权限（仅 Root 用户）
  const deletePermission = async (id: number) => {
    isLoading.value = true;
    error.value = null;

    try {
      await del(`${baseUrl}/permissions/${id}`);
      await fetchCatalog(true);
    } catch (err) {
      error.value = err instanceof Error ? err.message : "删除权限失败";
      throw err;
    } finally {
      isLoading.value = false;
    }
  };

  // 获取租户权限配置
  const fetchTenantPermissions = async (tenantUuid?: string) => {
    isLoading.value = true;
    error.value = null;

    try {
      const resolvedTenant =
        tenantUuid?.trim() || getStoredTenantUUID() || undefined;
      const url = resolvedTenant
        ? `${baseUrl}/tenant-permissions?tenant_uuid=${encodeURIComponent(
            resolvedTenant
          )}`
        : `${baseUrl}/tenant-permissions`;

      const response = await get<any>(url);
      tenantPermissions.value = response.data || response;
      return tenantPermissions.value;
    } catch (err) {
      error.value = err instanceof Error ? err.message : "获取租户权限失败";
      throw err;
    } finally {
      isLoading.value = false;
    }
  };

  // 更新租户权限配置
  const updateTenantPermission = async (
    tenantUuid: string,
    permissionId: number,
    enabled: boolean
  ) => {
    isLoading.value = true;
    error.value = null;

    try {
      const resolvedTenant =
        tenantUuid?.trim() || getStoredTenantUUID() || "";
      if (!resolvedTenant) {
        throw new Error("请先选择租户上下文");
      }

      const response = await put<any>(`${baseUrl}/tenant-permissions`, {
        tenant_uuid: resolvedTenant,
        permission_id: permissionId,
        enabled,
      });

      const tenantPermission = response.data || response;

      // 更新本地状态
      const index = tenantPermissions.value.findIndex(
        (tp) =>
          tp.tenant_uuid === resolvedTenant && tp.permission_id === permissionId
      );

      if (index >= 0) {
        tenantPermissions.value[index] = tenantPermission;
      } else {
        tenantPermissions.value.push(tenantPermission);
      }

      return tenantPermission;
    } catch (err) {
      error.value = err instanceof Error ? err.message : "更新租户权限失败";
      throw err;
    } finally {
      isLoading.value = false;
    }
  };

  // 同步权限（刷新目录）
  const syncPermissions = async () => {
    isLoading.value = true;
    error.value = null;

    try {
      await post(`${baseUrl}/permissions/sync`);
      await fetchCatalog(true);
      lastSyncTime.value = Date.now();
    } catch (err) {
      error.value = err instanceof Error ? err.message : "同步权限失败";
      throw err;
    } finally {
      isLoading.value = false;
    }
  };

  // ✅ 读取角色权限ID（用于勾选）
  // ✅ 读取角色权限ID（用于勾选）
  const fetchRolePermissionIDs = async (roleId: number) => {
    const res = await get(`${baseUrl}/roles/${roleId}/permissions`);
    const items = Array.isArray(res?.data?.items)
      ? res.data.items
      : (res?.data ?? []);
    const ids = items.map((p: any) => p.id);
    roleSelection.value[roleId] = ids;
    roleInitialSelection.value[roleId] = [...ids]; // 记录初始态
    return ids;
  };

  // ✅ 一次性设置角色的整套权限（页面"保存"调用）
  // ✅ 一次性设置角色的整套权限（页面"保存"调用）
  const setRolePermissionIDs = async (roleId: number, ids: number[]) => {
    const payload = { ids };
    const res = await put(
      `${baseUrl}/roles/${roleId}/permissions/set-ids`,
      payload
    );
    // 后端返回 { added, removed, now, skipped_deprecated }
    const now: number[] = res?.data?.now ?? ids;
    roleSelection.value[roleId] = now;
    roleInitialSelection.value[roleId] = [...now]; // 同步初始态
    return res?.data;
  };

  // ✅ 这里紧接着返回暴露的状态与方法
  return {
    // 状态
    catalog: readonly(catalog),
    listData: readonly(listData),
    tenantPermissions: readonly(tenantPermissions),
    isLoading: readonly(isLoading),
    error: readonly(error),
    lastSyncTime: readonly(lastSyncTime),

    // 计算属性
    // 计算属性
    catalogTree,
    enabledPermissionsCount,
    totalPermissionsCount,
    normalizedList,
    roleSelection,
    roleInitialSelection,

    // 方法
    fetchCatalog,
    fetchList,
    fetchAllActive,
    createPermission,
    updatePermission,
    deletePermission,
    fetchTenantPermissions,
    updateTenantPermission,
    syncPermissions,
    fetchRolePermissionIDs,
    setRolePermissionIDs,
  };
}); // ✅ 正常闭合 defineStore
