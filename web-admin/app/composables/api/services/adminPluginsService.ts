import { useApiClient } from "../index";

export type AdminPluginItem = {
  id: string;
  name?: string;
  version?: string;
  description?: string;
  author?: string;
  icon?: string;
  tags?: string[];
  isSystemInstalled?: boolean;
  isSystemEnabled?: boolean;
  systemStatus?: string;
};

export const useAdminPluginsService = () => {
  const api = useApiClient();
  const base = "/admin/plugins";
  const unwrap = (r: any) => (r && typeof r === 'object' && 'data' in r ? (r as any).data : r);

  return {
    // 市场列表
    getMarketplace: async (): Promise<AdminPluginItem[]> => {
      const r = await api.get<any>(`${base}/marketplace/plugins`);
      const d = unwrap(r);
      if (d && typeof d === 'object') {
        if (Array.isArray(d.items)) return d.items as AdminPluginItem[];
        if (Array.isArray((d as any).list)) return (d as any).list as AdminPluginItem[];
      }
      return Array.isArray(r) ? (r as AdminPluginItem[]) : [];
    },

    // 系统级列表
    list: async (): Promise<AdminPluginItem[]> => {
      const r = await api.get<any>(`${base}/`)
      const d = unwrap(r)
      if (Array.isArray(d)) return d as AdminPluginItem[]
      if (d && Array.isArray((d as any).items)) return (d as any).items as AdminPluginItem[]
      if (d && Array.isArray((d as any).plugins)) return (d as any).plugins as AdminPluginItem[]
      return []
    },

    // 系统启用/停用（启用可能耗时，单独放宽超时时间）
    enable: (id: string) =>
      api.post(`${base}/${encodeURIComponent(id)}/enable`, undefined, {
        timeout: 120000, // 120s 防止启动耗时导致超时
      }),
    disable: (id: string) =>
      api.post(`${base}/${encodeURIComponent(id)}/disable`, undefined, {
        timeout: 120000,
      }),

    // 安装（从 URL）
    installFromUrl: (payload: { url: string; sha256?: string; enable?: boolean; force?: boolean; metadata?: Record<string, any> }) =>
      api.post(`${base}/install/url`, payload, {
        timeout: 120000, // 构建/解压/启用可能超过默认 30s
      }),

    // 本地安装（文件上传）
    installFromLocal: (formData: FormData) =>
      api.upload(`${base}/install/local`, formData, {
        timeout: 120000, // 大包上传 + 安装流程可能超过默认 30s
      }),

    // 卸载（可扩展 purge 等参数）
    uninstall: async (id: string, payload?: Record<string, any>) =>
      unwrap(
        await api.post(
          `${base}/${encodeURIComponent(id)}/uninstall`,
          payload || {},
          { timeout: 120000 } // 卸载可能较慢，放宽超时
        )
      ),

    // 运行状态/日志
    status: async (id: string) => unwrap(await api.get(`${base}/${encodeURIComponent(id)}/status`)),
    logs: async (id: string, params?: Record<string, any>) => unwrap(await api.get(`${base}/${encodeURIComponent(id)}/logs`, { params } as any)),

    // 重启与切换版本
    restart: async (id: string) => unwrap(await api.post(`${base}/${encodeURIComponent(id)}/restart`)),
    switchVersion: async (id: string, version: string, payload?: Record<string, any>) =>
      unwrap(await api.post(`${base}/${encodeURIComponent(id)}/switch_version`, { version, ...(payload || {}) })),

    // 租户级
    getTenantConfig: async (id: string) => unwrap(await api.get(`${base}/${encodeURIComponent(id)}/tenant_config`)),
    setTenantEnabled: async (id: string, enabled: boolean) => unwrap(await api.post(`${base}/${encodeURIComponent(id)}/tenant_enable`, { enabled })),
    getCredentials: async (id: string) => unwrap(await api.get(`${base}/${encodeURIComponent(id)}/credentials`)),
    rotateCredentials: async (id: string) => unwrap(await api.post(`${base}/${encodeURIComponent(id)}/credentials/rotate`)),
    deleteTenantConfig: (id: string) => api.delete(`${base}/${encodeURIComponent(id)}/tenant_config`),
  };
};
