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
    title_i18n?: Record<string, string>;
    description_i18n?: Record<string, string>;
    module?: string;
    type?: "menu" | "action" | "api" | "data" | string;
    api_endpoint?: string;
    http_method?: string;
    plugin_id?: string;
    plugin_name?: string;
    menu_id?: string;
    origin?: string;
  };
}

// ✅ 追加到顶部类型区
export type PermissionMeta = {
  label?: string;
  title_i18n?: Record<string, string>;
  description_i18n?: Record<string, string>;
  module?: string;
  type?: "menu" | "action" | "data" | "api";
  api_endpoint?: string;
  http_method?: "GET" | "POST" | "PUT" | "DELETE" | "PATCH";
  plugin_id?: string;
  plugin_name?: string;
  menu_id?: string;
  origin?: string;
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

export interface PluginPermissionCatalogItem {
  id: number;
  type: "menu" | "page" | "action" | "api" | string;
  permission_code: string;
  effective_permission_code: string;
  module: string;
  resource: string;
  action: string;
  menu_path?: string[];
  page_permission_codes?: string[];
  title_i18n?: Record<string, string>;
  description_i18n?: Record<string, string>;
  risk_level?: string;
  data_scope?: string;
  business_permission_code?: string;
  protocol_bindings?: unknown;
  default_role_grants?: string[];
  status: "active" | "deprecated";
  registration_status: "registered" | "invalid" | string;
  registration_errors?: string[];
}

export interface PluginPermissionCatalogMenuNode {
  key: string;
  label_i18n?: Record<string, string>;
  permission?: PluginPermissionCatalogItem;
  page_permission_codes?: string[];
  children?: PluginPermissionCatalogMenuNode[];
}

export interface PluginPermissionCatalogBusinessResource {
  resource: string;
  pages?: PluginPermissionCatalogItem[];
  actions?: PluginPermissionCatalogItem[];
}

export interface PluginPermissionCatalogBusinessModule {
  module: string;
  resources: PluginPermissionCatalogBusinessResource[];
}

export interface PluginPermissionCatalogAPIBinding {
  business_permission_code: string;
  independent: boolean;
  permission: PluginPermissionCatalogItem;
  protocol_bindings?: unknown;
}

export interface PluginPermissionCatalogPlugin {
  plugin_id: string;
  menu_tree?: PluginPermissionCatalogMenuNode[];
  business_modules?: PluginPermissionCatalogBusinessModule[];
  api_bindings?: PluginPermissionCatalogAPIBinding[];
}

export interface PluginPermissionCatalogResponse {
  plugins: PluginPermissionCatalogPlugin[];
}

const countMenuCatalogPermissions = (
  nodes: PluginPermissionCatalogMenuNode[] = [],
): number =>
  nodes.reduce(
    (total, node) =>
      total +
      (node.permission ? 1 : 0) +
      countMenuCatalogPermissions(node.children || []),
    0,
  );

const countPluginCatalogPermissions = (
  plugin: PluginPermissionCatalogPlugin,
): number =>
  countMenuCatalogPermissions(plugin.menu_tree || []) +
  (plugin.business_modules || []).reduce(
    (total, module) =>
      total +
      module.resources.reduce(
        (resourceTotal, resource) =>
          resourceTotal +
          (resource.pages || []).length +
          (resource.actions || []).length,
        0,
      ),
    0,
  ) +
  (plugin.api_bindings || []).length;

export interface PluginInvalidPermissionCleanupResult {
  plugin_id: string;
  deleted_permission_ids: number[];
  deleted_bindings: number;
  deleted_permissions: number;
}

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
  const pluginCatalog = ref<PluginPermissionCatalogResponse>({ plugins: [] });
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
      const moduleName = m.module || p.plugin || (p as any).module || "";
      const resource = p.resource || "";
      const action = p.action || "";
      const code =
        moduleName && resource && action
          ? `${moduleName}:${resource}:${action}`
          : `${resource}.${action}`.replace(/^\./, "");
      return {
        id: p.id,
        // 展示名称由消费端按当前 locale 解析；这里保留稳定兜底。
        name: m.label || code,
        code,
        module: moduleName,
        plugin: p.plugin || (p as any).module || moduleName,
        resource,
        action,
        description: p.description || "",
        type: (m.type as any) || (moduleName === "menu" ? "menu" : "action"),
        apiEndpoint: m.api_endpoint,
        httpMethod: m.http_method,
        meta: m,
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

  const pluginPermissionCount = computed(() =>
    pluginCatalog.value.plugins
      .reduce((total, plugin) => total + countPluginCatalogPermissions(plugin), 0),
  );

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

  const fetchPluginCatalog = async (query: {
    plugin_id?: string;
    module?: string;
    type?: string;
    status?: "active" | "deprecated";
  } = {}) => {
    isLoading.value = true;
    error.value = null;

    try {
      const response = await get<any>(`${baseUrl}/permissions/plugin-catalog`, {
        params: query,
      });
      pluginCatalog.value = response.data || response || { plugins: [] };
      return pluginCatalog.value;
    } catch (err) {
      error.value =
        err instanceof Error ? err.message : "plugin_permission_catalog_load_failed";
      throw err;
    } finally {
      isLoading.value = false;
    }
  };

  const cleanupInvalidPluginPermissions = async (pluginId: string) => {
    const normalizedPluginId = pluginId.trim();
    if (!normalizedPluginId) {
      throw new Error("plugin_id_required");
    }
    isLoading.value = true;
    error.value = null;
    try {
      const response = await del<any>(`${baseUrl}/permissions/plugin-invalid`, {
        params: { plugin_id: normalizedPluginId },
      });
      const result = (response.data || response) as PluginInvalidPermissionCleanupResult;
      const deletedIDs = new Set(result.deleted_permission_ids || []);
      if (deletedIDs.size > 0) {
        for (const roleId of Object.keys(roleSelection.value)) {
          roleSelection.value[Number(roleId)] = (
            roleSelection.value[Number(roleId)] || []
          ).filter((id) => !deletedIDs.has(id));
        }
        for (const roleId of Object.keys(roleInitialSelection.value)) {
          roleInitialSelection.value[Number(roleId)] = (
            roleInitialSelection.value[Number(roleId)] || []
          ).filter((id) => !deletedIDs.has(id));
        }
      }
      await fetchAllActive();
      await fetchPluginCatalog();
      return result;
    } catch (err) {
      error.value =
        err instanceof Error ? err.message : "plugin_invalid_permission_cleanup_failed";
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
      const all: Permission[] = [];

      const menuRes = await get<any>(`${baseUrl}/permissions`, {
        params: {
          page: 1,
          size: pageSize,
          module: "menu",
          status: "active",
          sort: "resource asc, action asc",
        },
      });
      const menuPayload = menuRes?.data || menuRes;
      all.push(...(menuPayload?.items ?? []));

      let page = 1;
      let pages = 1;

      while (page <= pages) {
        const res = await get<any>(`${baseUrl}/permissions`, {
          params: {
            page,
            size: pageSize,
            status: "active",
            sort: "module asc, resource asc, action asc",
          },
        });
        const payload = res?.data || res;
        const items: Permission[] = payload?.items ?? [];
        const pgn = payload?.pagination ?? {};
        pages = Number(pgn?.pages || 1);
        all.push(...items.filter((p: any) => p.module !== "menu"));
        // console.info(
        //   `fetchAllActive: page=${page}, pages=${pages}, total=${pgn.total}, items=${items.length}`
        // );
        page++;
      }

      const seen = new Set<number>();
      const deduped = all.filter((item) => {
        if (seen.has(item.id)) return false;
        seen.add(item.id);
        return true;
      });

      // 全量塞进 listData，页面直接用 normalizedList 渲染
      listData.value = {
        items: deduped,
        pagination: {
          total: deduped.length,
          page: 1,
          page_size: deduped.length,
          pages: 1,
        },
      };
      return deduped;
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
            resolvedTenant,
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
    enabled: boolean,
  ) => {
    isLoading.value = true;
    error.value = null;

    try {
      const resolvedTenant = tenantUuid?.trim() || getStoredTenantUUID() || "";
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
          tp.tenant_uuid === resolvedTenant &&
          tp.permission_id === permissionId,
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
      payload,
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
    pluginCatalog: readonly(pluginCatalog),
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
    pluginPermissionCount,
    normalizedList,
    roleSelection,
    roleInitialSelection,

    // 方法
    fetchCatalog,
    fetchPluginCatalog,
    fetchList,
    fetchAllActive,
    createPermission,
    updatePermission,
    deletePermission,
    cleanupInvalidPluginPermissions,
    fetchTenantPermissions,
    updateTenantPermission,
    syncPermissions,
    fetchRolePermissionIDs,
    setRolePermissionIDs,
  };
}); // ✅ 正常闭合 defineStore
