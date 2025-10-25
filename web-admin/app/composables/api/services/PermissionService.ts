// app/api/services/PermissionService.ts

export const statusText = (s?: string) => {
  switch ((s || "").toLowerCase()) {
    case "active":
      return "生效";
    case "beta":
      return "内测";
    case "preview":
      return "预览";
    case "deprecated":
      return "已废弃";
    case "disabled":
      return "已禁用";
    default:
      return s || "-";
  }
};

export const statusColor = (s?: string) => {
  switch ((s || "").toLowerCase()) {
    case "active":
      return "green";
    case "beta":
    case "preview":
      return "blue";
    case "deprecated":
      return "gray";
    case "disabled":
      return "red";
    default:
      return "gray";
  }
};

// 权限类型定义（按照新的架构）
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

export class PermissionService {
  private baseUrl = "/api/admin/iam/permissions";

  /**
   * 获取权限目录（树形结构，用于角色授权）
   * 返回：module -> type -> Permission[]
   */
  async getCatalog(): Promise<PermissionCatalog> {
    try {
      const response = await $fetch<PermissionCatalog>(
        `${this.baseUrl}/catalog`
      );
      return response;
    } catch (error) {
      console.error("获取权限目录失败:", error);
      throw error;
    }
  }

  /**
   * 获取权限列表（平铺结构，用于管理表格）
   */
  async getList(
    query: PermissionListQuery = {}
  ): Promise<PermissionListResponse> {
    try {
      const response = await $fetch<PermissionListResponse>(this.baseUrl, {
        params: query,
      });
      return response;
    } catch (error) {
      console.error("获取权限列表失败:", error);
      // 返回模拟数据
      throw error;
    }
  }

  /**
   * 创建权限
   */
  async create(data: Partial<Permission>): Promise<Permission> {
    try {
      const response = await $fetch<Permission>(this.baseUrl, {
        method: "POST",
        body: data,
      });
      return response;
    } catch (error) {
      console.error("创建权限失败:", error);
      throw error;
    }
  }

  /**
   * 更新权限
   */
  async update(id: number, data: Partial<Permission>): Promise<Permission> {
    try {
      const response = await $fetch<Permission>(`${this.baseUrl}/${id}`, {
        method: "PUT",
        body: data,
      });
      return response;
    } catch (error) {
      console.error("更新权限失败:", error);
      throw error;
    }
  }

  /**
   * 删除权限
   */
  async delete(id: number): Promise<void> {
    try {
      await $fetch(`${this.baseUrl}/${id}`, {
        method: "DELETE",
      });
    } catch (error) {
      console.error("删除权限失败:", error);
      throw error;
    }
  }

  /**
   * 同步权限（刷新目录）
   */
  async sync(): Promise<void> {
    try {
      await $fetch(`${this.baseUrl}/sync`, {
        method: "POST",
      });
    } catch (error) {
      console.error("同步权限失败:", error);
      throw error;
    }
  }
}

export const permissionService = new PermissionService();
