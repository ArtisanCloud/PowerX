import { useApiClient } from "../index";
import type {
  ApiResponse,
  PaginatedResponse,
  PaginationParams,
} from "../types/types";

/**
 * 插件相关类型定义
 */
export interface Plugin {
  id: string;
  name: string;
  slug: string;
  version: string;
  description: string;
  author: string;
  authorUrl?: string;
  homepage?: string;
  repository?: string;
  license?: string;
  icon?: string;
  screenshots?: string[];
  category: string;
  tags: string[];
  status: "active" | "inactive" | "installed" | "available";
  isInstalled: boolean;
  isActive: boolean;
  installPath?: string;
  configSchema?: Record<string, any>;
  config?: Record<string, any>;
  dependencies?: string[];
  requirements?: {
    minVersion?: string;
    maxVersion?: string;
    php?: string;
    extensions?: string[];
  };
  downloadUrl?: string;
  downloadCount: number;
  rating: number;
  reviewCount: number;
  lastUpdated: string;
  createdAt: string;
  updatedAt: string;
}

export interface PluginCategory {
  id: string;
  name: string;
  slug: string;
  description?: string;
  icon?: string;
  pluginCount: number;
}

export interface PluginInstallParams {
  source: "marketplace" | "upload" | "url";
  pluginId?: string;
  file?: File;
  url?: string;
  version?: string;
}

export interface PluginUpdateParams {
  version?: string;
  autoUpdate?: boolean;
}

export interface PluginConfigParams {
  config: Record<string, any>;
}

export interface PluginFilters extends PaginationParams {
  category?: string;
  status?: Plugin["status"];
  tags?: string[];
  search?: string;
  author?: string;
}

export interface PluginMarketplaceItem extends Plugin {
  isPremium: boolean;
  price?: number;
  currency?: string;
  trialDays?: number;
  supportUrl?: string;
  documentationUrl?: string;
}

/**
 * 插件服务 API
 */
export const usePluginService = () => {
  const apiClient = useApiClient();
  const baseUrl = "/plugins";

  return {
    /**
     * 获取插件列表
     */
    getPlugins: (filters?: PluginFilters) => {
      return apiClient.get<ApiResponse<PaginatedResponse<Plugin>>>(baseUrl, {
        params: filters,
      });
    },

    /**
     * 获取指定插件详情
     */
    getPlugin: (id: string) => {
      return apiClient.get<ApiResponse<Plugin>>(`${baseUrl}/${id}`);
    },

    /**
     * 安装插件
     */
    installPlugin: (data: PluginInstallParams) => {
      const formData = new FormData();
      Object.entries(data).forEach(([key, value]) => {
        if (value !== undefined) {
          if (value instanceof File) {
            formData.append(key, value);
          } else {
            formData.append(key, String(value));
          }
        }
      });

      return apiClient.upload<ApiResponse<Plugin>>(
        `${baseUrl}/install`,
        formData
      );
    },

    /**
     * 卸载插件
     */
    uninstallPlugin: (id: string) => {
      return apiClient.delete<ApiResponse<null>>(`${baseUrl}/${id}/uninstall`);
    },

    /**
     * 激活插件
     */
    activatePlugin: (id: string) => {
      return apiClient.post<ApiResponse<Plugin>>(`${baseUrl}/${id}/activate`);
    },

    /**
     * 停用插件
     */
    deactivatePlugin: (id: string) => {
      return apiClient.post<ApiResponse<Plugin>>(`${baseUrl}/${id}/deactivate`);
    },

    /**
     * 更新插件
     */
    updatePlugin: (id: string, data?: PluginUpdateParams) => {
      return apiClient.put<ApiResponse<Plugin>>(
        `${baseUrl}/${id}/update`,
        data
      );
    },

    /**
     * 获取插件配置
     */
    getPluginConfig: (id: string) => {
      return apiClient.get<ApiResponse<Record<string, any>>>(
        `${baseUrl}/${id}/config`
      );
    },

    /**
     * 更新插件配置
     */
    updatePluginConfig: (id: string, data: PluginConfigParams) => {
      return apiClient.put<ApiResponse<Record<string, any>>>(
        `${baseUrl}/${id}/config`,
        data
      );
    },

    /**
     * 获取插件分类
     */
    getPluginCategories: () => {
      return apiClient.get<ApiResponse<PluginCategory[]>>(
        `${baseUrl}/categories`
      );
    },

    /**
     * 获取市场插件列表
     */
    getMarketplacePlugins: (filters?: PluginFilters) => {
      return apiClient.get<
        ApiResponse<PaginatedResponse<PluginMarketplaceItem>>
      >(`${baseUrl}/marketplace`, { params: filters });
    },

    /**
     * 搜索插件
     */
    searchPlugins: (query: string, filters?: Omit<PluginFilters, "search">) => {
      return apiClient.get<ApiResponse<PaginatedResponse<Plugin>>>(
        `${baseUrl}/search`,
        { params: { search: query, ...filters } }
      );
    },

    /**
     * 检查插件更新
     */
    checkUpdates: () => {
      return apiClient.get<ApiResponse<Plugin[]>>(`${baseUrl}/check-updates`);
    },

    /**
     * 批量更新插件
     */
    batchUpdatePlugins: (pluginIds: string[]) => {
      return apiClient.post<ApiResponse<Plugin[]>>(`${baseUrl}/batch-update`, {
        pluginIds,
      });
    },

    /**
     * 获取插件日志
     */
    getPluginLogs: (id: string, filters?: PaginationParams) => {
      return apiClient.get<ApiResponse<PaginatedResponse<any>>>(
        `${baseUrl}/${id}/logs`,
        { params: filters }
      );
    },

    /**
     * 验证插件
     */
    validatePlugin: (file: File) => {
      const formData = new FormData();
      formData.append("file", file);

      return apiClient.upload<
        ApiResponse<{ valid: boolean; errors?: string[] }>
      >(`${baseUrl}/validate`, formData);
    },
  };
};
