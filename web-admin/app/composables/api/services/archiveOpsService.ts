import { useApiClient } from '../index';
export interface ArchiveSource { id: string; name: string; kind: 'media' | 'postgresql_logs' | 'database_files' | 'system_logs' | 'audit_logs' | 'application_logs' | 'journal'; directory: string; ready: boolean; problem?: string }
export interface ArchivePolicy { revision?: number; remote_max_bytes?: number; remote_cleanup_enabled?: boolean; id?: string; name: string; source_id: string; interval_minutes: number; retention_days: number; retention_count: number; remote_retention_days: number; remote_retention_count: number; target_id: string; include_database: boolean; enabled: boolean }
export interface ArchiveTarget { driver?: string; verification_valid?: boolean; revision?: number; endpoint?: string; bucket?: string; prefix?: string; region?: string; force_path_style?: boolean; verified_at?: string; executor?: string; id?: string; name: string; host: string; port: number; user: string; directory: string; credential_ref: string }
export interface ArchiveEntry { path: string; size: number; sha256: string; modified_at: string }
export interface ArchiveJob { recovered?: boolean; migrated_from?: string; files?: ArchiveEntry[]; imported?: boolean; provenance_verified?: boolean; source_file_name?: string; id: string; one_time?: boolean; policy_id: string; source_id: string; kind: string; status: string; phase: string; created_at: string; file_count: number; size_bytes: number; new_bytes: number; database?: string; error?: string; remote_state: string; remote_path?: string; remote_error?: string; attempts: number; retry_at?: string; protected: boolean; warnings: string[]; trace_id?: string }
export interface ArchiveRestore { id: string; job_id: string; status: string; directory: string; database_copy_id?: string; database_copy_name?: string; file_count: number; error?: string }
export interface RemoteGCResult { id: string; policy_id: string; state: string; candidate_versions: number; candidate_bytes: number; deleted_versions: number; confirmed_bytes: number; protected: Record<string, number>; problem?: string; created_at: string }
export interface ArchiveCatalog { items: ArchiveJob[]; pagination: { total: number; page: number; page_size: number; pages: number } }
export interface ArchiveOverview { log_pending_transfers?: number; scheduler_started?: boolean; custody?: { protocol: number; node_id: string; legacy_detected: boolean; managed: boolean; problem?: string }; remote_runs?: RemoteGCResult[]; s3_credentials?: string[]; instance: string; directory: string; sources: ArchiveSource[]; credentials: string[]; policies: ArchivePolicy[]; targets: ArchiveTarget[]; jobs: ArchiveJob[]; restores: ArchiveRestore[]; pending_transfers: number }
export const useArchiveOpsService = () => {
  const api = useApiClient(); const base = '/admin/ops/backup/archives'; const config = { useGlobalLoading: false, useGlobalError: false, retry: 0 };
  return {
    async overview(signal?: AbortSignal): Promise<ArchiveOverview> { return api.unwrap<{ overview: ArchiveOverview }>(await api.get(base, { ...config, signal })).overview; },
    async migrate(id: string, policy_id: string, key: string, signal?: AbortSignal): Promise<ArchiveJob> { return api.unwrap<{ job: ArchiveJob }>(await api.post(`${base}/jobs/${encodeURIComponent(id)}/migrations`, { policy_id, confirm: true }, { ...config, signal, headers: { 'Idempotency-Key': key } })).job; },
    async catalog(input: { q?: string; source_id?: string; from?: string; to?: string; page: number; page_size: number }, signal?: AbortSignal): Promise<ArchiveCatalog> { return api.unwrap<ArchiveCatalog>(await api.get(`${base}/catalog`, { ...config, signal, params: input })); },
    async job(id: string, signal?: AbortSignal): Promise<ArchiveJob> { return api.unwrap<{ job: ArchiveJob }>(await api.get(`${base}/jobs/${encodeURIComponent(id)}`, { ...config, signal })).job; },
    async preview(id: string, entry: string, signal?: AbortSignal): Promise<{ name: string; text: string; truncated: boolean; verified: boolean }> { return api.unwrap(await api.get(`${base}/jobs/${encodeURIComponent(id)}/preview`, { ...config, signal, timeout: 120000, params: { entry } })); },
    async saveCredential(input: { ref: string; expected_revision: number; access_key: string; secret_key: string; session_token?: string }, signal?: AbortSignal) { return api.unwrap<{ ref: string; revision: number }>(await api.post(`${base}/s3-credentials`, input, { ...config, signal })); },
    async importLegacy(id: string, signal?: AbortSignal) { return api.unwrap<{ imported: number; existing: number; failed: number; unconfirmed: number }>(await api.post(`${base}/targets/${encodeURIComponent(id)}/imports`, { confirm: true }, { ...config, signal, timeout: 600000 })); },
    async previewRemoteGC(id: string, signal?: AbortSignal): Promise<RemoteGCResult> { return api.unwrap(await api.post(`${base}/policies/${encodeURIComponent(id)}/remote-cleanup-previews`, {}, { ...config, signal, timeout: 120000 })); },
    async reconcileRemoteGC(id: string, signal?: AbortSignal): Promise<RemoteGCResult> { return api.unwrap(await api.post(`${base}/remote-cleanup-runs/${encodeURIComponent(id)}/reconcile`, {}, { ...config, signal, timeout: 120000 })); },
    async startRemoteGC(id: string, signal?: AbortSignal): Promise<RemoteGCResult> { return api.unwrap(await api.post(`${base}/remote-cleanup-runs/${encodeURIComponent(id)}`, { confirmation: id }, { ...config, signal })); },
    async createOneTime(sourceId: string, key: string): Promise<ArchiveJob> { return api.unwrap<{ job: ArchiveJob }>(await api.post(`${base}/jobs`, { source_id: sourceId }, { ...config, headers: { 'Idempotency-Key': key } })).job; },
    async savePolicy(policy: ArchivePolicy, signal?: AbortSignal): Promise<ArchivePolicy> { return api.unwrap<{ policy: ArchivePolicy }>(await api.post(`${base}/policies`, policy, { ...config, signal })).policy; },
    async saveTarget(target: ArchiveTarget, signal?: AbortSignal): Promise<ArchiveTarget> { return api.unwrap<{ target: ArchiveTarget }>(await api.post(`${base}/targets`, target, { ...config, signal })).target; },
    async testTarget(id: string, signal?: AbortSignal) { await api.post(`${base}/targets/${encodeURIComponent(id)}/test`, {}, { ...config, signal }); },
    async run(id: string, key: string, signal?: AbortSignal): Promise<ArchiveJob> { return api.unwrap<{ job: ArchiveJob }>(await api.post(`${base}/policies/${encodeURIComponent(id)}/run`, {}, { ...config, signal, headers: { 'Idempotency-Key': key } })).job; },
    async retry(id: string) { await api.post(`${base}/jobs/${encodeURIComponent(id)}/retry`, {}, config); },
    async protect(id: string, value: boolean, signal?: AbortSignal) { await api.patch(`${base}/jobs/${encodeURIComponent(id)}/protection`, { protected: value }, { ...config, signal }); },
    async cleanup() { return api.unwrap<{ pruned: number }>(await api.post(`${base}/cleanup`, {}, config)).pruned; },
    async restore(id: string, older: boolean): Promise<ArchiveRestore> { return api.unwrap<{ restore: ArchiveRestore }>(await api.post(`${base}/jobs/${encodeURIComponent(id)}/restore`, { confirmation: id, allow_older_server: older }, config)).restore; },
    async removeRestore(id: string, confirmation: string) { await api.delete(`${base}/restores/${encodeURIComponent(id)}`, { ...config, headers: { 'X-Restore-Confirmation': confirmation } }); },
    async download(id: string, signal?: AbortSignal) {
      const blob = await api.get<Blob>(`${base}/jobs/${encodeURIComponent(id)}/download`, { ...config, signal, timeout: 600000, responseType: 'blob' } as any);
      if (signal?.aborted) return;
      const url = URL.createObjectURL(blob); const a = document.createElement('a'); a.href = url; a.download = `powerx-archive-${id}.tar.gz`; a.click(); setTimeout(() => URL.revokeObjectURL(url), 1000);
    },
  };
};
