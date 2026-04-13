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

export interface MonitorRetentionRun {
  run_id: string;
  triggered_by: string;
  started_at: string;
  ended_at: string;
  status: "success" | "failed" | string;
  deleted_files: number;
  deleted_rows: number;
  sources: string[];
  error_summary?: string;
  duration_ms: number;
}

export interface MonitorRetentionRuns {
  items: MonitorRetentionRun[];
  next_run?: string;
  enabled: boolean;
  cron: string;
  timezone: string;
}

export interface MonitorRetentionDBTable {
  name: string;
  time_column: string;
  retention_days: number;
}

export interface MonitorRetentionPolicy {
  enabled: boolean;
  cron: string;
  timezone: string;
  default_retention_days: number;
  file_paths: string[];
  batch_size: number;
  max_delete_rows_per_run: number;
  db_tables: MonitorRetentionDBTable[];
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

    async getRetentionRuns(limit = 20): Promise<MonitorRetentionRuns> {
      const resp = await api.get(`${adminBase}/retention/runs`, {
        params: { limit },
      });
      const data = unwrap<MonitorRetentionRuns>(resp) || ({} as MonitorRetentionRuns);
      return {
        items: Array.isArray(data.items) ? data.items : [],
        next_run: data.next_run || undefined,
        enabled: Boolean(data.enabled),
        cron: String(data.cron || ""),
        timezone: String(data.timezone || ""),
      };
    },

    async triggerRetentionRun(): Promise<MonitorRetentionRun> {
      const resp = await api.post(`${adminBase}/retention/run`, {});
      const data = unwrap<{ run?: MonitorRetentionRun }>(resp) || {};
      if (!data.run) {
        throw new Error("retention run response missing run");
      }
      return data.run;
    },

    async getRetentionPolicy(): Promise<MonitorRetentionPolicy> {
      const resp = await api.get(`${adminBase}/retention/policy`);
      const data = unwrap<{ policy?: MonitorRetentionPolicy }>(resp) || {};
      const policy = data.policy || ({} as MonitorRetentionPolicy);
      return {
        enabled: Boolean(policy.enabled),
        cron: String(policy.cron || ""),
        timezone: String(policy.timezone || ""),
        default_retention_days: Number(policy.default_retention_days || 30),
        file_paths: Array.isArray(policy.file_paths) ? policy.file_paths : [],
        batch_size: Number(policy.batch_size || 5000),
        max_delete_rows_per_run: Number(policy.max_delete_rows_per_run || 200000),
        db_tables: Array.isArray(policy.db_tables) ? policy.db_tables : [],
      };
    },

    async updateRetentionPolicy(policy: MonitorRetentionPolicy): Promise<MonitorRetentionPolicy> {
      const resp = await api.put(`${adminBase}/retention/policy`, {
        policy,
      });
      const data = unwrap<{ policy?: MonitorRetentionPolicy }>(resp) || {};
      if (!data.policy) {
        throw new Error("retention policy response missing policy");
      }
      return data.policy;
    },
  };
};
