import { useApiClient } from '../index';
export interface ArchiveSource { id: string; name: string; kind: 'media' | 'postgresql_logs' | 'database_files'; directory: string; ready: boolean; problem?: string }
export interface ArchivePolicy { id?: string; name: string; source_id: string; interval_minutes: number; retention_days: number; retention_count: number; remote_retention_days: number; remote_retention_count: number; target_id: string; include_database: boolean; enabled: boolean }
export interface ArchiveTarget { id?: string; name: string; host: string; port: number; user: string; directory: string; credential_ref: string }
export interface ArchiveJob { id: string; one_time?: boolean; policy_id: string; source_id: string; kind: string; status: string; phase: string; created_at: string; file_count: number; size_bytes: number; new_bytes: number; database?: string; error?: string; remote_state: string; remote_path?: string; remote_error?: string; attempts: number; retry_at?: string; protected: boolean; warnings: string[]; trace_id?: string }
export interface ArchiveRestore { id: string; job_id: string; status: string; directory: string; database_copy_id?: string; database_copy_name?: string; file_count: number; error?: string }
export interface ArchiveOverview { instance: string; directory: string; sources: ArchiveSource[]; credentials: string[]; policies: ArchivePolicy[]; targets: ArchiveTarget[]; jobs: ArchiveJob[]; restores: ArchiveRestore[]; pending_transfers: number }
export const useArchiveOpsService = () => {
  const api = useApiClient(); const base = '/admin/ops/backup/archives'; const config = { useGlobalLoading: false, useGlobalError: false };
  return {
    async overview(): Promise<ArchiveOverview> { return api.unwrap<{ overview: ArchiveOverview }>(await api.get(base, config)).overview; },
    async createOneTime(sourceId: string, key: string): Promise<ArchiveJob> { return api.unwrap<{ job: ArchiveJob }>(await api.post(`${base}/jobs`, { source_id: sourceId }, { ...config, headers: { 'Idempotency-Key': key } })).job; },
    async savePolicy(policy: ArchivePolicy): Promise<ArchivePolicy> { return api.unwrap<{ policy: ArchivePolicy }>(await api.post(`${base}/policies`, policy, config)).policy; },
    async saveTarget(target: ArchiveTarget): Promise<ArchiveTarget> { return api.unwrap<{ target: ArchiveTarget }>(await api.post(`${base}/targets`, target, config)).target; },
    async testTarget(id: string) { await api.post(`${base}/targets/${encodeURIComponent(id)}/test`, {}, config); },
    async run(id: string, key: string): Promise<ArchiveJob> { return api.unwrap<{ job: ArchiveJob }>(await api.post(`${base}/policies/${encodeURIComponent(id)}/run`, {}, { ...config, headers: { 'Idempotency-Key': key } })).job; },
    async retry(id: string) { await api.post(`${base}/jobs/${encodeURIComponent(id)}/retry`, {}, config); },
    async protect(id: string, value: boolean) { await api.patch(`${base}/jobs/${encodeURIComponent(id)}/protection`, { protected: value }, config); },
    async cleanup() { return api.unwrap<{ pruned: number }>(await api.post(`${base}/cleanup`, {}, config)).pruned; },
    async restore(id: string, older: boolean): Promise<ArchiveRestore> { return api.unwrap<{ restore: ArchiveRestore }>(await api.post(`${base}/jobs/${encodeURIComponent(id)}/restore`, { confirmation: id, allow_older_server: older }, config)).restore; },
    async removeRestore(id: string, confirmation: string) { await api.delete(`${base}/restores/${encodeURIComponent(id)}`, { ...config, headers: { 'X-Restore-Confirmation': confirmation } }); },
    async download(id: string) {
      const blob = await api.get<Blob>(`${base}/jobs/${encodeURIComponent(id)}/download`, { ...config, timeout: 600000, responseType: 'blob' } as any);
      const url = URL.createObjectURL(blob); const a = document.createElement('a'); a.href = url; a.download = `powerx-archive-${id}.tar.gz`; a.click(); setTimeout(() => URL.revokeObjectURL(url), 1000);
    },
  };
};
