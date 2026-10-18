import { useApiClient } from "../index";
 import type { RecoveryJob } from "./systemRecoveryService";

export interface SnapshotComponent {
  kind: string;
  status: string;
  file_count: number;
  size_bytes: number;
}
export interface SystemSnapshot {
  attempt?: number;
  retry_at?: string;
  policy_id?: string;
  policy_name?: string;
 recovery_supported?:boolean;
 coordinator?:{recovery:RecoveryJob;progress_token:string};
  id: string;
  status: string;
  phase: string;
  created_at: string;
  captured_at?: string;
  updated_at: string;
  trigger: string;
  instance: string;
  version: string;
  components: SnapshotComponent[];
  protected: boolean;
  imported: boolean;
  business_verified: boolean;
  error?: string;
  error_code?: string;
  trace_id?: string;
}
export interface SystemSnapshotPolicy {
  enabled: boolean;
  interval_minutes: number;
  retention_days: number;
  retention_count: number;
}
export interface SystemSnapshotPolicyInput extends SystemSnapshotPolicy { name: string }
export interface NamedSnapshotPolicy extends SystemSnapshotPolicyInput {
  id: string;
  is_default: boolean;
  next_run_at?: string;
}
export interface SystemSnapshotRestore {
  id: string;
  snapshot_id: string;
  status: string;
  phase: string;
  created_at: string;
  directory: string;
  database_copy_id?: string;
  database?: string;
  components: SnapshotComponent[];
  business_verified: boolean;
  cutover_available: boolean;
  warnings: string[];
  error?: string;
}
export interface SystemSnapshotOverview {
 directory: string;
 directory_source: "recovery_profile" | "environment" | "configuration" | "install_root" | "default" | "explicit";
  ready: boolean;
  problems: string[];
  instance: string;
  policy: SystemSnapshotPolicy;
  policies: NamedSnapshotPolicy[];
  default_policy_id: string;
  next_run_at?: string;
  scheduler_error?: string;
  snapshots: SystemSnapshot[];
  restores: SystemSnapshotRestore[];
  cutover_available: boolean;
}

export const useSystemSnapshotService = () => {
  const api = useApiClient();
  const base = "/admin/ops/backup/system-snapshots";
  const config = { useGlobalLoading: false, useGlobalError: false };
  return {
    async overview(): Promise<SystemSnapshotOverview> {
      return api.unwrap<{ overview: SystemSnapshotOverview }>(
        await api.get(base, config),
      ).overview;
    },
    async savePolicy(
      policy: SystemSnapshotPolicyInput,
      id?: string,
    ): Promise<NamedSnapshotPolicy> {
      return api.unwrap<{ policy: NamedSnapshotPolicy }>(
        id ? await api.put(`${base}/policies/${encodeURIComponent(id)}`, policy, config) : await api.post(`${base}/policies`, policy, config),
      ).policy;
    },
    async setPolicyEnabled(id: string, enabled: boolean): Promise<NamedSnapshotPolicy> {
      return api.unwrap<{ policy: NamedSnapshotPolicy }>(await api.patch(`${base}/policies/${encodeURIComponent(id)}/status`, {enabled}, config)).policy;
    },
    async setDefaultPolicy(id: string): Promise<NamedSnapshotPolicy> {
      return api.unwrap<{ policy: NamedSnapshotPolicy }>(await api.patch(`${base}/policies/${encodeURIComponent(id)}/status`, {enabled:false,make_default:true}, config)).policy;
    },
    async create(key: string, policyID: string): Promise<SystemSnapshot> {
      const result=api.unwrap<{snapshot:SystemSnapshot;coordinator?:{recovery:RecoveryJob;progress_token:string}}>(await api.post(base,{policy_id:policyID}, {...config,headers:{"Idempotency-Key":key}}));
      return {...result.snapshot,coordinator:result.coordinator};
    },
    async exportFile(id: string, password: string): Promise<Blob> {
      return api.post<Blob>(
        `${base}/${encodeURIComponent(id)}/export`,
        { password },
        { ...config, timeout: 600000, responseType: "blob" } as any,
      );
    },
    async importFile(file: File, password: string): Promise<SystemSnapshot> {
      const form = new FormData();
      form.append("file", file);
      form.append("password", password);
      return api.unwrap<{ snapshot: SystemSnapshot }>(
        await api.upload(`${base}/imports`, form, {
          ...config,
          timeout: 600000,
        }),
      ).snapshot;
    },
    async restore(id: string, key: string): Promise<SystemSnapshotRestore> {
      return api.unwrap<{ restore: SystemSnapshotRestore }>(
        await api.post(
          `${base}/${encodeURIComponent(id)}/restores`,
          { confirmation: id },
          { ...config, headers: { "Idempotency-Key": key } },
        ),
      ).restore;
    },
    async protect(id: string, value: boolean) {
      return api.unwrap<{ snapshot: SystemSnapshot }>(
        await api.patch(
          `${base}/${encodeURIComponent(id)}/protection`,
          { protected: value },
          config,
        ),
      ).snapshot;
    },
  };
};
