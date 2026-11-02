import { useApiClient } from '../index';
import type { ApiRequestConfig } from '../types/types';

export interface RuntimeSource { id: string; name: string; kind: string; scope: string; instance_id: string; format: string; supported: boolean; require_archive: boolean; problem?: string }
export interface RuntimePolicy { cleanup_mode?: 'disabled' | 'expire_delete' | 'archive_then_delete'; source_id: string; revision: number; enabled: boolean; retention_days: number; max_bytes: number; require_archive: boolean; owner_instance: string; operator_id: number; updated_at: string }
export interface RuntimeRun { id: string; request_key: string; source_id: string; policy_revision: number; instance_id: string; state: 'executing' | 'completed' | 'partial' | 'blocked' | 'unconfirmed'; started_at: string; finished_at: string | null; deleted_files: number; removed_bytes: number; removed_allocated_bytes: number; available_before: number; available_after: number | null; acknowledged_at?: string; acknowledged_by?: number; pending_quarantine?: string; pending_name?: string; problem?: string }
export interface RuntimeBudgetPolicy { revision: number; min_free_bytes: number; critical_free_bytes: number; max_used_bytes: number; max_concurrent: number }
export interface RuntimePreview { source_id: string; policy_revision: number; candidate_names: string[]; candidate_bytes: number; protected: Record<string, number>; observed_bytes: number; over_budget: boolean; complete: boolean; problem?: string }
export interface StorageRuntime {
  configured: boolean; worker_enabled: boolean; admission_enabled: boolean; notifications_configured: boolean; database_cleanup_enabled: boolean; problem?: string;
  last_observed_at?: string; last_notification_at?: string; notification_problem?: string;
  budget_policy?: RuntimeBudgetPolicy;
  budget?: { available_bytes: number; used_bytes: number; reserved_bytes: number; min_free_bytes: number; critical_free_bytes: number; active: { id: string; operation: string; instance_id: string; peak_bytes: number; written_bytes: number; expires_at: string }[] };
  lifecycle?: { configured: boolean; allow_enforce: boolean; revision: number; sources: RuntimeSource[]; policies: RuntimePolicy[]; runs: RuntimeRun[]; database_cleanup_enabled: false; problem?: string };
  log_delivery?: { accepted: number; delivered: number; dropped: number; failed: number; retries: number; queued_entries: number; queued_bytes: number; queue_capacity: number; max_queue_bytes: number; last_error?: string; closed: boolean };
}
export interface DatabaseRetentionPolicy { revision: number; owner_instance: string; enabled: boolean; success_days: number; failure_days: number; pending_hours: number; quarantine_days: number; batch_limit: number; max_bytes: number; adopt_legacy: boolean; reject_new_when_full: boolean; updated_at: string; actor: number }
export interface DatabaseRetentionRun { policy_snapshot?: DatabaseRetentionPolicy; id: string; revision: number; state: string; at: string; actor: number; counts: { traces: number; events: number; quarantined: number; classified: number }; problem?: string }
export interface DatabaseRetentionStatus { state_configured?: boolean; enforcement_allowed?: boolean; migration_ready?: boolean; migration_problem?: string; unconfirmed_runs?: number; worker_enabled?: boolean; policy: DatabaseRetentionPolicy; runs: DatabaseRetentionRun[]; ready: boolean; problem?: string; relation_bytes: number | null; scope: string; physical_reclaim: string }
const unwrap = (raw: any): any => raw && typeof raw === 'object' && 'data' in raw ? raw.data : raw;
const count = (v: unknown) => Number.isSafeInteger(v) && Number(v) >= 0;
export function decodeStorageRuntime(raw: unknown): StorageRuntime {
  const d = unwrap(raw);
  if (!d || !['configured', 'worker_enabled', 'admission_enabled', 'notifications_configured'].every(k => typeof d[k] === 'boolean') || typeof d.database_cleanup_enabled !== 'boolean') throw new Error('STORAGE_RUNTIME_RESPONSE_INVALID');
  if (d.budget && (!['available_bytes', 'used_bytes', 'reserved_bytes', 'min_free_bytes', 'critical_free_bytes'].every(k => count(d.budget[k])) || !Array.isArray(d.budget.active))) throw new Error('STORAGE_RUNTIME_RESPONSE_INVALID');
  if (d.lifecycle && (!Array.isArray(d.lifecycle.sources) || !Array.isArray(d.lifecycle.policies) || !Array.isArray(d.lifecycle.runs) || d.lifecycle.sources.length > 64 || d.lifecycle.runs.length > 100 || d.lifecycle.database_cleanup_enabled !== false || typeof d.lifecycle.allow_enforce !== 'boolean' ||
      !d.lifecycle.sources.every((s: any) => s && typeof s.id === 'string' && typeof s.name === 'string' && typeof s.supported === 'boolean') ||
      !d.lifecycle.policies.every((p: any) => p && typeof p.source_id === 'string' && typeof p.enabled === 'boolean' && count(p.revision) && count(p.retention_days) && count(p.max_bytes)))) throw new Error('STORAGE_RUNTIME_RESPONSE_INVALID');
  if (d.budget_policy && (!['revision', 'min_free_bytes', 'critical_free_bytes', 'max_used_bytes', 'max_concurrent'].every(k => count(d.budget_policy[k])) || d.budget_policy.revision < 1 || d.budget_policy.min_free_bytes <= d.budget_policy.critical_free_bytes || d.budget_policy.max_concurrent < 1 || d.budget_policy.max_concurrent > 16)) throw new Error('STORAGE_RUNTIME_RESPONSE_INVALID');
  if (d.lifecycle && !d.lifecycle.runs.every((r: any) => validRun(r))) throw new Error('STORAGE_RUNTIME_RESPONSE_INVALID');
  return d;
}
function validRun(r: any): boolean { return r && typeof r.id === 'string' && typeof r.source_id === 'string' && typeof r.request_key === 'string' && ['executing', 'completed', 'partial', 'blocked', 'unconfirmed'].includes(r.state) && count(r.policy_revision) && count(r.deleted_files) && count(r.removed_bytes) && count(r.removed_allocated_bytes); }
export function decodeRuntimeRun(raw: unknown): RuntimeRun { const r = unwrap(raw); if (!validRun(r)) throw new Error('STORAGE_RUNTIME_RESPONSE_INVALID'); return r; }
export function decodeRuntimePreview(raw: unknown): RuntimePreview { const p = unwrap(raw); if (!p || typeof p.source_id !== 'string' || !count(p.policy_revision) || typeof p.complete !== 'boolean' || typeof p.over_budget !== 'boolean' || !count(p.candidate_bytes) || !count(p.observed_bytes) || !Array.isArray(p.candidate_names) || p.candidate_names.length > 32 || !p.candidate_names.every((s: any) => typeof s === 'string' && s.length > 0 && !/[\\/\u0000]/.test(s)) || !p.protected || !Object.values(p.protected).every(count)) throw new Error('STORAGE_RUNTIME_RESPONSE_INVALID'); return p; }

export function useStorageRuntimeService() {
  const api = useApiClient();
  const config = (signal?: AbortSignal): ApiRequestConfig => ({ signal, retry: 0, useGlobalLoading: false, timeout: 15000 });
  return {
    async databaseStatus(signal?: AbortSignal): Promise<DatabaseRetentionStatus> { return unwrap(await api.get('/admin/ops/storage/database-retention', config(signal))); },
    async saveDatabasePolicy(expected_revision: number, policy: DatabaseRetentionPolicy, signal?: AbortSignal): Promise<DatabaseRetentionPolicy> { return unwrap(await api.put('/admin/ops/storage/database-retention', { expected_revision, policy, confirm: true }, config(signal))); },
    async previewDatabase(expected_revision: number, signal?: AbortSignal): Promise<DatabaseRetentionRun> { return unwrap(await api.post('/admin/ops/storage/database-retention/previews', { expected_revision }, config(signal))); },
    async runDatabase(expected_revision: number, request_id: string, signal?: AbortSignal): Promise<DatabaseRetentionRun> { return unwrap(await api.post('/admin/ops/storage/database-retention/runs', { expected_revision, request_id, confirm: true }, config(signal))); },
    async status(signal?: AbortSignal): Promise<StorageRuntime> { return decodeStorageRuntime(await api.get('/admin/ops/storage/runtime', config(signal))); },
    async savePolicy(source: string, input: { cleanup_mode?: string; expected_revision: number; enabled: boolean; retention_days: number; max_bytes: number; require_archive: boolean; confirm: true }, signal?: AbortSignal): Promise<RuntimePolicy> { return unwrap(await api.put(`/admin/ops/storage/runtime/policies/${encodeURIComponent(source)}`, input, config(signal))); },
    async saveBudget(input: Omit<RuntimeBudgetPolicy, 'revision'> & { expected_revision: number; confirm: true }, signal?: AbortSignal): Promise<RuntimeBudgetPolicy> { return unwrap(await api.put('/admin/ops/storage/runtime/budget', input, config(signal))); },
    async preview(source_id: string, retention_days: number, max_bytes: number, signal?: AbortSignal): Promise<RuntimePreview> { return decodeRuntimePreview(await api.post('/admin/ops/storage/runtime/previews', { source_id, retention_days, max_bytes }, config(signal))); },
    async run(source_id: string, expected_revision: number, request_id: string, signal?: AbortSignal): Promise<RuntimeRun> { return decodeRuntimeRun(await api.post('/admin/ops/storage/runtime/runs', { source_id, expected_revision, request_id, confirm: true }, config(signal))); },
    async acknowledge(id: string, signal?: AbortSignal): Promise<RuntimeRun> { return decodeRuntimeRun(await api.post(`/admin/ops/storage/runtime/runs/${encodeURIComponent(id)}/acknowledge`, { confirm: true }, config(signal))); },
    events(signal: AbortSignal) { return api.stream('/admin/ops/storage/runtime/events', signal); },
  };
}
