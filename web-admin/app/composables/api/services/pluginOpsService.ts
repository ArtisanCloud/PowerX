import { useApiClient } from "../index";

export interface PluginLifecycleAuditRecord {
  id: number | string;
  plugin_id: string;
  from_version?: string;
  to_version?: string;
  action: string;
  result: string;
  gate_result?: string;
  gate_reason?: string;
  operator: string;
  trace_id?: string;
  detail?: string;
  created_at?: string;
}

export interface PluginLifecycleListResult {
  items: PluginLifecycleAuditRecord[];
  pagination?: {
    total: number;
    page: number;
    page_size: number;
  };
}

export interface TriggerPluginLifecycleActionPayload {
  plugin_id?: string;
  from_version?: string;
  to_version?: string;
  action: "switch" | "rollback" | "install" | "uninstall";
  reason?: string;
}

const unwrap = <T>(payload: unknown): T => {
  if (payload && typeof payload === "object" && "data" in (payload as any)) {
    return (payload as any).data as T;
  }
  return payload as T;
};

export const usePluginOpsService = () => {
  const api = useApiClient();

  return {
    async listAudits(pluginId: string, params?: { page?: number; pageSize?: number }): Promise<PluginLifecycleListResult> {
      const response = await api.get(`/admin/plugins/${encodeURIComponent(pluginId)}/audit`, {
        params: {
          page: params?.page,
          page_size: params?.pageSize,
        },
      });
      const data = unwrap<{ items?: PluginLifecycleAuditRecord[]; pagination?: any }>(response) || {};
      return {
        items: Array.isArray(data.items) ? data.items : [],
        pagination: data.pagination,
      };
    },

    async triggerAction(pluginId: string, payload: TriggerPluginLifecycleActionPayload): Promise<PluginLifecycleAuditRecord> {
      const response = await api.post(`/admin/plugins/${encodeURIComponent(pluginId)}/actions`, payload);
      const data = unwrap<{ audit: PluginLifecycleAuditRecord }>(response);
      return data.audit;
    },
  };
};
