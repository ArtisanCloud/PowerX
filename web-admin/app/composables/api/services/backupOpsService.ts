import { useApiClient } from "../index";

export interface BackupPolicy {
  id: number | string;
  name: string;
  interval_hours: number;
  retention_count: number;
  timezone: string;
  drill_enabled: boolean;
  drill_interval_days: number;
  target_ref: string;
  enabled: boolean;
}

export interface BackupJob {
  id: number | string;
  policy_id: number | string;
  status: string;
  trigger_type: string;
  started_at?: string;
  ended_at?: string;
  error_message?: string;
  operator: string;
  trace_id?: string;
}

export interface RestoreDrillRecord {
  id: number | string;
  source_job_id: number | string;
  status: string;
  rto_seconds: number;
  report_uri?: string;
}

export interface BackupPolicyFilters {
  status?: "enabled" | "disabled";
  keyword?: string;
  timezone?: string;
  page?: number;
  pageSize?: number;
}

const unwrap = <T>(payload: unknown): T => {
  if (payload && typeof payload === "object" && "data" in (payload as any)) {
    return (payload as any).data as T;
  }
  return payload as T;
};

const adminBase = "/admin/ops/backup";

export const useBackupOpsService = () => {
  const api = useApiClient();

  return {
    async listPolicies(filters?: BackupPolicyFilters): Promise<{ items: BackupPolicy[]; total: number; page: number; pageSize: number }> {
      const resp = await api.get(`${adminBase}/policies`, {
        params: {
          status: filters?.status,
          keyword: filters?.keyword,
          timezone: filters?.timezone,
          page: filters?.page,
          page_size: filters?.pageSize,
        },
      });
      const data = unwrap<{ items?: BackupPolicy[]; pagination?: { total?: number; page?: number; page_size?: number } }>(resp) || {};
      const pagination = data.pagination || {};
      return {
        items: Array.isArray(data.items) ? data.items : [],
        total: Number(pagination.total || 0),
        page: Number(pagination.page || 1),
        pageSize: Number(pagination.page_size || 20),
      };
    },

    async createPolicy(payload: {
      name: string;
      interval_hours?: number;
      retention_count?: number;
      timezone?: string;
      drill_enabled?: boolean;
      drill_interval_days?: number;
      target_ref?: string;
    }): Promise<BackupPolicy> {
      const resp = await api.post(`${adminBase}/policies`, payload);
      const data = unwrap<{ policy: BackupPolicy }>(resp);
      return data.policy;
    },

    async updatePolicy(policyId: string | number, payload: {
      name?: string;
      interval_hours?: number;
      retention_count?: number;
      timezone?: string;
      drill_enabled?: boolean;
      drill_interval_days?: number;
      target_ref?: string;
    }): Promise<BackupPolicy> {
      const resp = await api.patch(`${adminBase}/policies/${policyId}`, payload);
      const data = unwrap<{ policy: BackupPolicy }>(resp);
      return data.policy;
    },

    async enablePolicy(policyId: string | number): Promise<void> {
      await api.post(`${adminBase}/policies/${policyId}/enable`, {});
    },

    async disablePolicy(policyId: string | number): Promise<void> {
      await api.post(`${adminBase}/policies/${policyId}/disable`, {});
    },

    async triggerJob(policyId: string | number): Promise<BackupJob> {
      const resp = await api.post(`${adminBase}/jobs/run`, { policy_id: String(policyId) });
      const data = unwrap<{ job: BackupJob }>(resp);
      return data.job;
    },

    async listJobs(policyId?: string | number): Promise<BackupJob[]> {
      const resp = await api.get(`${adminBase}/jobs`, {
        params: { policy_id: policyId ? String(policyId) : undefined, page: 1, page_size: 50 },
      });
      const data = unwrap<{ items?: BackupJob[] }>(resp) || {};
      return Array.isArray(data.items) ? data.items : [];
    },

    async triggerCleanup(): Promise<void> {
      await api.post(`${adminBase}/cleanup`, {});
    },

    async triggerRestoreDrill(sourceJobId: string | number): Promise<RestoreDrillRecord> {
      const resp = await api.post(`${adminBase}/restore-drills/run`, { source_job_id: String(sourceJobId) });
      const data = unwrap<{ drill: RestoreDrillRecord }>(resp);
      return data.drill;
    },
  };
};
