import { useApiClient } from '../index';

export interface RestoreCopySource { source_job_id?: string; import_id?: string }
export interface RestoreCopyPlan {
  host: string; port: number; current_database: string; target_pattern: string;
  archive_name: string; archive_sha256: string; archive_size: number;
  archive_version: string; server_version: string; extensions: string[];
  requires_compatibility_consent: boolean; ready: boolean; problems: string[];
}
export interface RestoreCopy {
  id: string; database: string; current_database: string; host: string; port: number;
  source_name: string; source_sha256: string; status: 'queued' | 'running' | 'success' | 'failed' | 'deleted';
  phase: string; created_at: string; updated_at: string; table_count: number; size_bytes: number;
  schema_counts?: Record<string, number>; business_counts?: Record<string, number>;
  warnings: string[]; error_code?: string; error_message?: string; trace_id?: string;
}
export interface RestoreCopyConnection {
  access_mode: 'direct' | 'ssh_tunnel';
  tunnel?: { host: string; port: number };
  host: string; port: number; database: string; username: string; password: string;
  ssl_mode: string; read_only: boolean; expires_at: string;
}
export const useRestoreCopyService = () => {
  const api = useApiClient();
  const base = '/admin/ops/backup/restore-copies';
  const config = { useGlobalLoading: false, useGlobalError: false };
  return {
    async list(): Promise<RestoreCopy[]> {
      return api.unwrap<{ items: RestoreCopy[] }>(await api.get(base, config)).items;
    },
    async importFile(file: File): Promise<{ id: string; name: string; size_bytes: number }> {
      const form = new FormData(); form.append('file', file);
      return api.unwrap<{ import: { id: string; name: string; size_bytes: number } }>(await api.upload(`${base}/imports`, form, { ...config, timeout: 120000 })).import;
    },
    async plan(source: RestoreCopySource): Promise<RestoreCopyPlan> {
      return api.unwrap<{ plan: RestoreCopyPlan }>(await api.post(`${base}/plan`, source, config)).plan;
    },
    async create(source: RestoreCopySource, allowOlder: boolean, key: string): Promise<RestoreCopy> {
      return api.unwrap<{ copy: RestoreCopy }>(await api.post(base, { ...source, allow_older_server: allowOlder }, { ...config, headers: { 'Idempotency-Key': key } })).copy;
    },
    async connection(id: string): Promise<RestoreCopyConnection> {
      return api.unwrap<{ connection: RestoreCopyConnection }>(await api.post(`${base}/${encodeURIComponent(id)}/connection`, {}, config)).connection;
    },
    async remove(id: string, database: string) {
      await api.delete(`${base}/${encodeURIComponent(id)}`, { ...config, headers: { 'X-Restore-Confirmation': database } });
    },
  };
};
