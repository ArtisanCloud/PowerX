import { useApiClient } from "../index";

export interface MonitorLogCapabilities {
  supports_label_query: boolean;
  supports_trace_query: boolean;
  supports_job_query: boolean;
  supports_policy_query: boolean;
  supports_grafana_link: boolean;
  history_limited: boolean;
  limitation_note?: string;
}

export interface MonitorLogConfig {
  driver: "loki" | "file" | "stdio";
  capabilities: MonitorLogCapabilities;
  grafana_base_url?: string;
}

export interface MonitorLogQueryMeta {
  driver: "loki" | "file" | "stdio";
  degraded: boolean;
  hint?: string;
  grafana_url?: string;
}

export interface MonitorLogEntry {
  ts: string;
  level: "debug" | "info" | "warn" | "error" | string;
  module?: string;
  trace_id?: string;
  job_id?: number;
  policy_id?: number;
  message?: string;
  raw?: string;
}

export interface MonitorLogQueryFilters {
  trace_id?: string;
  job_id?: string | number;
  policy_id?: string | number;
  keyword?: string;
  from?: string;
  to?: string;
  page?: number;
  page_size?: number;
}

const unwrap = <T>(payload: unknown): T => {
  if (payload && typeof payload === "object" && "data" in (payload as any)) {
    return (payload as any).data as T;
  }
  return payload as T;
};

const adminBase = "/admin/monitor/logs";

export const useMonitorService = () => {
  const api = useApiClient();

  return {
    async getLogConfig(): Promise<MonitorLogConfig> {
      const resp = await api.get(`${adminBase}/config`);
      return unwrap<MonitorLogConfig>(resp);
    },

    async queryLogs(filters?: MonitorLogQueryFilters): Promise<{
      items: MonitorLogEntry[];
      pagination: { total: number; page: number; page_size: number };
      query_meta: MonitorLogQueryMeta;
    }> {
      const resp = await api.get(`${adminBase}/query`, {
        params: {
          trace_id: filters?.trace_id || undefined,
          job_id: filters?.job_id ? String(filters.job_id) : undefined,
          policy_id: filters?.policy_id ? String(filters.policy_id) : undefined,
          keyword: filters?.keyword || undefined,
          from: filters?.from || undefined,
          to: filters?.to || undefined,
          page: filters?.page ?? 1,
          page_size: filters?.page_size ?? 50,
        },
      });

      const data = unwrap<{
        items?: MonitorLogEntry[];
        pagination?: { total?: number; page?: number; page_size?: number };
        query_meta?: MonitorLogQueryMeta;
      }>(resp) || {};

      return {
        items: Array.isArray(data.items) ? data.items : [],
        pagination: {
          total: Number(data.pagination?.total || 0),
          page: Number(data.pagination?.page || 1),
          page_size: Number(data.pagination?.page_size || 50),
        },
        query_meta: data.query_meta || { driver: "stdio", degraded: true, hint: "query_meta unavailable" },
      };
    },
  };
};
