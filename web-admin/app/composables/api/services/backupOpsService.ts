import { useApiClient } from "../index";

export interface BackupPolicy {
  id: number | string;
  name: string;
  backup_type: string;
  schedule: string;
  retention_days: number;
  enabled: boolean;
  storage_target: string;
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

const unwrap = <T>(payload: unknown): T => {
  if (payload && typeof payload === "object" && "data" in (payload as any)) {
    return (payload as any).data as T;
  }
  return payload as T;
};

export const useBackupOpsService = () => {
  const api = useApiClient();

  return {
    async listPolicies(enabledOnly = false): Promise<BackupPolicy[]> {
      const resp = await api.get("/admin/backup/policies", {
        params: { enabled_only: enabledOnly ? "true" : undefined },
      });
      const data = unwrap<{ items?: BackupPolicy[] }>(resp) || {};
      return Array.isArray(data.items) ? data.items : [];
    },

    async upsertPolicy(payload: {
      name: string;
      backup_type: string;
      schedule: string;
      retention_days: number;
      enabled: boolean;
      storage_target: string;
    }): Promise<BackupPolicy> {
      const resp = await api.post("/admin/backup/policies", payload);
      const data = unwrap<{ policy: BackupPolicy }>(resp);
      return data.policy;
    },

    async triggerJob(policyId: string | number): Promise<BackupJob> {
      const resp = await api.post("/admin/backup/jobs/run", { policy_id: String(policyId) });
      const data = unwrap<{ job: BackupJob }>(resp);
      return data.job;
    },

    async listJobs(policyId?: string | number): Promise<BackupJob[]> {
      const resp = await api.get("/admin/backup/jobs", {
        params: { policy_id: policyId ? String(policyId) : undefined, page: 1, page_size: 50 },
      });
      const data = unwrap<{ items?: BackupJob[] }>(resp) || {};
      return Array.isArray(data.items) ? data.items : [];
    },

    async triggerCleanup(): Promise<void> {
      await api.post("/admin/backup/cleanup", {});
    },

    async triggerRestoreDrill(sourceJobId: string | number): Promise<RestoreDrillRecord> {
      const resp = await api.post("/admin/backup/restore-drills/run", { source_job_id: String(sourceJobId) });
      const data = unwrap<{ drill: RestoreDrillRecord }>(resp);
      return data.drill;
    },
  };
};
