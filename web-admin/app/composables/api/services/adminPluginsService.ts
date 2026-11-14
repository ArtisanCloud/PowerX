import type { PluginInstallTask } from "~/utils/plugin-install-task";
import { useApiClient } from "../index";
import type { InstallInspection } from '~/utils/plugin-install-recovery';

export interface PluginProviderIdentity { id: string; authority: string }

export interface PluginInstallOrigin {
  type: 'local' | 'marketplace' | 'unknown';
  source_id?: string;
  channel?: string;
  sha256?: string;
}

export type AdminPluginItem = {
  install_origin?: PluginInstallOrigin;
  provider?: PluginProviderIdentity;
  revision?: string;
  state?: string;
  id: string;
  name?: string;
  menus?: { title: string }[];
  version?: string;
  description?: string;
  author?: string;
  icon?: string;
  tags?: string[];
  isSystemInstalled?: boolean;
  isSystemEnabled?: boolean;
  systemStatus?: string;
  tenantEnabled?: boolean;
  tenantStatus?: string;
  tenantInstance?: TenantPluginInstance | null;
};

export type TenantPluginInstance = {
  tenant_uuid: string;
  plugin_id: string;
  version: string;
  enabled: boolean;
  status?: string;
  drain_job_id?: string;
  config?: Record<string, any>;
};

export const useAdminPluginsService = () => {
  const api = useApiClient();
  const base = "/admin/plugins";
  const unwrap = (r: any) => (r && typeof r === 'object' && 'data' in r ? (r as any).data : r);
  const listTenantInstances = async (): Promise<TenantPluginInstance[]> => {
    const d = unwrap(await api.get(`${base}/tenant-instances`));
    if (Array.isArray(d)) return d as TenantPluginInstance[];
    if (d && Array.isArray((d as any).items)) return (d as any).items as TenantPluginInstance[];
    return [];
  };

  return {
    createInstallation: async (input: Record<string, unknown>, signal?: AbortSignal): Promise<PluginInstallTask> =>
      unwrap(await api.post(`${base}/installations`, input, { signal, useGlobalLoading: false, useGlobalError: false })),
    uploadInstallation: async (id: string, body: FormData, signal: AbortSignal): Promise<PluginInstallTask> =>
      unwrap(await api.put(`${base}/installations/${encodeURIComponent(id)}/package`, body, { signal, timeout: 0, useGlobalLoading: false, useGlobalError: false })),
    getInstallationTask: async (id: string, signal?: AbortSignal): Promise<PluginInstallTask> =>
      unwrap(await api.get(`${base}/installations/${encodeURIComponent(id)}`, { signal, useGlobalLoading: false, useGlobalError: false })),
    listInstallationTasks: async (signal?: AbortSignal): Promise<PluginInstallTask[]> =>
      unwrap(await api.get(`${base}/installations`, { signal, useGlobalLoading: false, useGlobalError: false })).items || [],
    discardInstallationSource: async (id: string, signal?: AbortSignal): Promise<PluginInstallTask> =>
      unwrap(await api.delete(`${base}/installations/${encodeURIComponent(id)}/source`, { signal, useGlobalLoading: false, useGlobalError: false })),
    installationEvents: (id: string, signal: AbortSignal) => api.stream(`${base}/installations/${encodeURIComponent(id)}/events`, signal),
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
    enable: (id: string, expected?: { expected_version: string; expected_revision: string }) =>
      api.post(`${base}/${encodeURIComponent(id)}/enable`, expected, {
        timeout: 120000, // 120s 防止启动耗时导致超时
        retry: 0,
      }),
    inspectInstallation: async (id: string, signal?: AbortSignal): Promise<InstallInspection> => {
      const data = unwrap(await api.get(`${base}/${encodeURIComponent(id)}/tenant_config`, { signal, retry: 0, useGlobalLoading: false }));
      if (!data || data.plugin_id !== id || !data.installation || typeof data.installation.ready !== 'boolean') throw new Error('PLUGIN_INSTALL_STATE_INVALID');
      return data as InstallInspection;
    },
    disable: (id: string) =>
      api.post(`${base}/${encodeURIComponent(id)}/disable`, undefined, {
        timeout: 120000,
      }),

    // 安装（从 URL）
    installFromUrl: (payload: { url: string; sha256?: string; enable?: boolean; force?: boolean; expected_revision?: string; confirm_legacy_provider?: boolean; metadata?: Record<string, any> }) =>
      api.post(`${base}/install/url`, payload, {
        timeout: 120000, // 构建/解压/启用可能超过默认 30s
        retry: 0,
      }),

    // 本地安装（文件上传）
    installFromLocal: (formData: FormData) =>
      api.upload(`${base}/install/local`, formData, {
        timeout: 120000, // 大包上传 + 安装流程可能超过默认 30s
        retry: 0,
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

    createDrainJob: async (id: string, payload?: { version?: string; reason?: string; mode?: string }) =>
      unwrap(
        await api.post(
          `${base}/${encodeURIComponent(id)}/drain`,
          payload || {},
          { timeout: 120000 }
        )
      ),
    listDrainJobs: async (id: string, params?: Record<string, any>) =>
      unwrap(await api.get(`${base}/${encodeURIComponent(id)}/drain`, { params } as any)),
    refreshDrainJob: async (jobId: string) =>
      unwrap(await api.post(`${base}/drain/${encodeURIComponent(jobId)}/refresh`, {}, { timeout: 120000 })),
    listDrainBlockers: async (
      id: string,
      params?: { kind?: "event_task" | "scheduler_job"; page?: number; page_size?: number }
    ) =>
      unwrap(await api.get(`${base}/${encodeURIComponent(id)}/drain/blockers`, { params } as any)),
    cancelDrainBlockers: async (id: string, payload?: { reason?: string; event_task_ids?: number[]; scheduler_job_uuids?: string[] }) =>
      unwrap(
        await api.post(
          `${base}/${encodeURIComponent(id)}/drain/cancel_blockers`,
          payload || {},
          { timeout: 120000 }
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
    listTenantInstances,
    getTenantConfig: async (id: string) => {
      const rows = await listTenantInstances();
      return rows.find((row: TenantPluginInstance) => row.plugin_id === id) || null;
    },
    setTenantEnabled: async (id: string, enabled: boolean) => {
      const endpoint = enabled ? "enable" : "disable";
      return unwrap(await api.post(`${base}/tenant-instances/${encodeURIComponent(id)}/${endpoint}`, {}));
    },
    getCredentials: async (id: string) => unwrap(await api.get(`${base}/${encodeURIComponent(id)}/credentials`)),
    rotateCredentials: async (id: string) => unwrap(await api.post(`${base}/${encodeURIComponent(id)}/credentials/rotate`)),
    deleteTenantConfig: (id: string) => api.delete(`${base}/${encodeURIComponent(id)}/tenant_config`),
  };
};
