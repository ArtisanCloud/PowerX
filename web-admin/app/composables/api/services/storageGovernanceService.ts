import { useApiClient } from '../index';
import type { ApiRequestConfig } from '../types/types';
import type { StorageRuntime } from './storageRuntimeService';

export interface StorageUsage {
  id: string;
  name: string;
  kind: string;
  scope: string;
  instance_id?: string;
  filesystem_id?: string;
  state: 'observed' | 'partial' | 'unavailable';
  physical_bytes: number | null;
  logical_bytes: number | null;
  attributed_bytes: number | null;
  partial_bytes?: number;
  files: number;
  skipped: number;
  problem?: string;
}

export interface StorageOverview {
  mode: 'observe' | 'enforce';
  cleanup_enabled: boolean;
  identity: { host_id?: string; filesystem_id?: string; instance_id?: string; environment?: string };
  observed_at: string;
  expires_at: string;
  filesystem: {
    id?: string;
    view: 'process_filesystem';
    state: 'observed' | 'unavailable';
    total_bytes: number | null;
    used_bytes: number | null;
    available_bytes: number | null;
    reserved_bytes: number | null;
    problem?: string;
  };
  resources: StorageUsage[];
  coverage: 'partial';
  unattributed_bytes: number | null;
  limitations: string[];
  database?: StorageDatabaseUsage;
  alerts?: { code: string; level: 'unknown' | 'warning' | 'critical'; resource_id?: string }[];
  sampling?: StorageSamplingStatus;
  inventory?: { kind: string; state: 'connected' | 'not_connected' | 'not_applicable' | 'disabled' }[];
  runtime?: StorageRuntime;
}

export interface StorageSamplingStatus {
  configured: boolean;
  worker_enabled: boolean;
  interval_seconds: 300;
  metadata_retention_days: 15;
}

export interface StorageDatabaseUsage {
  state: 'observed' | 'partial' | 'unavailable';
  instance_id: string; environment: string; cluster_id?: string;
  database_bytes: number | null; wal_bytes: number | null; reusable_bytes: number | null; wal_state: string;
  relations: { id: string; state: string; heap_bytes: number | null; indexes_bytes: number | null; toast_bytes: number | null; total_bytes: number | null; problem?: string }[];
}
export interface StorageResourcePolicy {
  retention_mode?: 'days' | 'count' | 'protected';
  resource_id: string; target_days: number; max_bytes: number; retention_count: number; min_verified_points: number;
  protect_latest: boolean; require_verified_archive: boolean;
}
export interface StoragePolicy {
  schema_version: 1; revision: number; state: 'draft'; identity: { host_id?: string; filesystem_id?: string; instance_id?: string; environment?: string };
  budget: { max_managed_bytes: number; min_free_bytes: number; critical_free_bytes: number };
  resources: StorageResourcePolicy[]; updated_at: string | null; updated_by: number; cleanup_enabled: false; legacy_policies_changed: false;
  catalog_version: 1;
  catalog: StoragePolicyOption[];
}
export interface StorageResourceRef { id: string; name: string; kind: string; scope: string; instance_id?: string }
export interface StoragePolicyOption {
  resource: StorageResourceRef;
  allowed_modes: ('days' | 'count' | 'protected')[];
  preset: { version: 1; intent: string; retention_mode: 'days' | 'count' | 'protected'; target_days: number; max_bytes: number | null; retention_count: number; min_verified_points: number; protect_latest: boolean; require_verified_archive: boolean; suggested_cadence: string };
  protection_rules: string[];
  cleanup?: { native_managed?: boolean; supported: boolean; retention_days: number; retention_count: number; blocked_reason?: string };
}
export interface StorageResourceSample { id: string; state: string; bytes: number | null; files: number; problem?: string; net_change_bytes: number | null }
export interface StorageCleanupRecord {
  id: string; resource: StorageResourceRef; state: 'preview' | 'completed' | 'partial' | 'unconfirmed';
  created_at: string; expires_at: string; target_days: number; actor_id: number; can_execute: boolean; blocked_reason?: string;
  items: { name: string; bytes: number; outcome: string }[]; skipped: Record<string, number>;
  deleted_files: number; removed_file_bytes: number; reclaimed_bytes: number | null;
}
export interface StoragePolicyInput {
  schema_version: 1; expected_revision: number; state: 'draft'; budget: StoragePolicy['budget']; resources: StorageResourcePolicy[];
}
export interface StorageHistory {
  metric_version: string; window_days: number; sample_count: number; expected_samples: number; missing_samples: number; metric_changes: number; unavailable_samples: number;
  state: 'sampling_required' | 'occupancy_only'; rate_basis: 'ingress_not_instrumented'; daily_increment_bytes: null;
  points: { observed_at: string; filesystem_id?: string; used_bytes: number | null; available_bytes: number | null; database_bytes: number | null; database_problem?: string; coverage: string; resources?: StorageResourceSample[] }[];
  sources?: StorageResourceRef[];
  sampling?: StorageSamplingStatus;
}

export interface StorageEstimateInput {
  resource_id?: string;
  retention_mode?: 'days' | 'count' | 'protected';
  retention_count?: number;
  target_days: number;
  max_bytes: number;
  daily_increment_bytes?: number;
  protected_bytes: number;
}
export interface StorageEstimate {
  state: 'sampling_required' | 'budget_feasible' | 'budget_insufficient' | 'artifact_inventory_required' | 'retention_not_applicable';
  basis: 'unavailable' | 'manual_assumption';
  required_bytes: number | null;
  deficit_bytes: number | null;
  supported_days: number | null;
  safety_factor: number;
  can_enable: false;
  resource?: StorageResourceRef;
  retention_mode?: string;
  retention_count?: number;
}

const unwrap = (raw: unknown): any => raw && typeof raw === 'object' && 'data' in raw ? raw.data : raw;
const bytes = (value: unknown) => value === null || (Number.isSafeInteger(value) && Number(value) >= 0);
const samplingValid = (value: any) => value === undefined || (value && typeof value.configured === 'boolean' && typeof value.worker_enabled === 'boolean' &&
  (!value.worker_enabled || value.configured) && value.interval_seconds === 300 && value.metadata_retention_days === 15);

// 拒绝缺字段的旧实例响应；不把 malformed data 默认为零或成功。
export function decodeStorageOverview(raw: unknown): StorageOverview {
  const data = unwrap(raw);
  const fs = data?.filesystem;
  if (!data || !['observe', 'enforce'].includes(data.mode) || typeof data.cleanup_enabled !== 'boolean' || data.coverage !== 'partial' ||
      !data.identity || !Number.isFinite(Date.parse(data.observed_at)) || !Number.isFinite(Date.parse(data.expires_at)) ||
      !fs || fs.view !== 'process_filesystem' || !['observed', 'unavailable'].includes(fs.state) ||
      !['total_bytes', 'used_bytes', 'available_bytes', 'reserved_bytes'].every(key => bytes(fs[key])) ||
      (fs.state === 'observed' && ['total_bytes', 'used_bytes', 'available_bytes', 'reserved_bytes'].some(key => fs[key] === null)) ||
      !bytes(data.unattributed_bytes) || !Array.isArray(data.resources) || !Array.isArray(data.limitations) || !samplingValid(data.sampling) ||
      !data.resources.every((r: any) => r && typeof r.id === 'string' && typeof r.name === 'string' &&
        typeof r.scope === 'string' && ['observed', 'partial', 'unavailable'].includes(r.state) &&
        bytes(r.physical_bytes) && bytes(r.logical_bytes) && bytes(r.attributed_bytes) &&
        (r.partial_bytes === undefined || (r.partial_bytes !== null && bytes(r.partial_bytes))) &&
        (r.state !== 'observed' || (r.physical_bytes !== null && r.logical_bytes !== null && r.attributed_bytes !== null)))) {
    throw new Error('STORAGE_OBSERVATION_RESPONSE_INVALID');
  }
  if (data.database !== undefined) {
    const db = data.database;
    if (!db || !['observed', 'partial', 'unavailable'].includes(db.state) || !['database_bytes', 'wal_bytes', 'reusable_bytes'].every(k => bytes(db[k])) ||
        !Array.isArray(db.relations) || !db.relations.every((r: any) => r && typeof r.id === 'string' && ['observed', 'partial', 'unavailable'].includes(r.state) &&
          ['heap_bytes', 'indexes_bytes', 'toast_bytes', 'total_bytes'].every(k => bytes(r[k])) &&
          (r.state !== 'observed' || ['heap_bytes', 'indexes_bytes', 'toast_bytes', 'total_bytes'].every(k => r[k] !== null)))) throw new Error('STORAGE_DATABASE_RESPONSE_INVALID');
  }
  if (data.alerts !== undefined && (!Array.isArray(data.alerts) || !data.alerts.every((a: any) => a && typeof a.code === 'string' && ['unknown', 'warning', 'critical'].includes(a.level)))) throw new Error('STORAGE_ALERT_RESPONSE_INVALID');
  return data;
}

export function decodeStoragePolicy(raw: unknown): StoragePolicy {
  const p = unwrap(raw);
  if (!p || p.schema_version !== 1 || p.state !== 'draft' || !Number.isSafeInteger(p.revision) || p.revision < 0 || !p.identity ||
      !p.budget || !['max_managed_bytes', 'min_free_bytes', 'critical_free_bytes'].every(k => bytes(p.budget[k]) && p.budget[k] !== null) ||
      p.cleanup_enabled !== false || p.legacy_policies_changed !== false || !Array.isArray(p.resources) || !p.resources.every((r: any) => r && typeof r.resource_id === 'string' &&
        ['target_days', 'max_bytes', 'retention_count', 'min_verified_points'].every(k => Number.isSafeInteger(r[k]) && r[k] >= 0) &&
        (r.retention_mode === undefined || ['days', 'count', 'protected'].includes(r.retention_mode)) && typeof r.protect_latest === 'boolean' && typeof r.require_verified_archive === 'boolean')) throw new Error('STORAGE_POLICY_RESPONSE_INVALID');
  if (p.catalog_version !== 1) throw new Error('STORAGE_POLICY_CATALOG_UNAVAILABLE');
  if (p.catalog === undefined) p.catalog = [];
  if (!Array.isArray(p.catalog) || !p.catalog.every((o: any) => o && validResourceRef(o.resource) && Array.isArray(o.allowed_modes) && o.allowed_modes.length > 0 && o.allowed_modes.every((m: string) => ['days', 'count', 'protected'].includes(m)) &&
      Array.isArray(o.protection_rules) && o.protection_rules.every((s: any) => typeof s === 'string') && o.preset?.version === 1 && o.allowed_modes.includes(o.preset.retention_mode) && bytes(o.preset.max_bytes) &&
      ['target_days', 'retention_count', 'min_verified_points'].every(k => Number.isSafeInteger(o.preset[k]) && o.preset[k] >= 0) && typeof o.preset.protect_latest === 'boolean' && typeof o.preset.require_verified_archive === 'boolean')) throw new Error('STORAGE_POLICY_CATALOG_INVALID');
  return p;
}
function validResourceRef(r: any) { return r && typeof r.id === 'string' && typeof r.name === 'string' && typeof r.kind === 'string' && ['prod', 'dev', 'test', 'staging', 'shared'].includes(r.scope) && (r.scope === 'shared' || typeof r.instance_id === 'string'); }
function validResourceSample(r: any) { return r && typeof r.id === 'string' && ['observed', 'partial', 'unavailable'].includes(r.state) && bytes(r.bytes) && (r.net_change_bytes === null || Number.isSafeInteger(r.net_change_bytes)); }
function validHistoryPoint(p: any) { return p && Number.isFinite(Date.parse(p.observed_at)) && ['used_bytes', 'available_bytes', 'database_bytes'].every(k => bytes(p[k])) && (p.resources === undefined || (Array.isArray(p.resources) && p.resources.every(validResourceSample))); }

function decodeCleanup(raw: unknown): StorageCleanupRecord {
  const d = unwrap(raw);
  if (!d || typeof d.id !== 'string' || !validResourceRef(d.resource) || !['preview', 'completed', 'partial', 'unconfirmed'].includes(d.state) ||
      !Number.isFinite(Date.parse(d.created_at)) || !Number.isFinite(Date.parse(d.expires_at)) || !Number.isSafeInteger(d.target_days) || d.target_days < 1 || typeof d.can_execute !== 'boolean' ||
      !Number.isSafeInteger(d.deleted_files) || d.deleted_files < 0 || !bytes(d.removed_file_bytes) || d.removed_file_bytes === null || !bytes(d.reclaimed_bytes) || !d.skipped ||
      !Object.values(d.skipped).every(n => Number.isSafeInteger(n) && Number(n) >= 0) || !Array.isArray(d.items) || d.items.length > 32 || !d.items.every((i: any) => i && typeof i.name === 'string' && !i.name.includes('/') && bytes(i.bytes) && i.bytes !== null && typeof i.outcome === 'string')) throw new Error('STORAGE_CLEANUP_RESPONSE_INVALID');
  return d;
}

export const useStorageGovernanceService = () => {
  const api = useApiClient();
  return {
    runtimeEvents(signal: AbortSignal) { return api.stream('/admin/ops/storage/runtime/events', signal); },
    async previewCleanup(resource_id: string, target_days: number, signal?: AbortSignal): Promise<StorageCleanupRecord> {
      return decodeCleanup(await api.post('/admin/ops/storage/cleanup-previews', { resource_id, target_days }, { signal, retry: 0, timeout: 15000, useGlobalLoading: false } as ApiRequestConfig));
    },
    async executeCleanup(preview_id: string, signal?: AbortSignal): Promise<StorageCleanupRecord> {
      return decodeCleanup(await api.post('/admin/ops/storage/cleanup-runs', { preview_id, confirm: true }, { signal, retry: 0, timeout: 20000, useGlobalLoading: false } as ApiRequestConfig));
    },
    async cleanupRun(id: string, signal?: AbortSignal): Promise<StorageCleanupRecord> {
      return decodeCleanup(await api.get(`/admin/ops/storage/cleanup-runs/${encodeURIComponent(id)}`, { signal, retry: 0, useGlobalLoading: false } as ApiRequestConfig));
    },
    async cleanupRuns(signal?: AbortSignal): Promise<StorageCleanupRecord[]> {
      const d = unwrap(await api.get('/admin/ops/storage/cleanup-runs', { signal, useGlobalLoading: false } as ApiRequestConfig));
      if (!d || !Array.isArray(d.items)) throw new Error('STORAGE_CLEANUP_RESPONSE_INVALID');
      return d.items.map(decodeCleanup);
    },
    async policy(signal?: AbortSignal): Promise<StoragePolicy> {
      return decodeStoragePolicy(await api.get('/admin/ops/storage/policy', { signal, useGlobalLoading: false } as ApiRequestConfig));
    },
    async savePolicy(input: StoragePolicyInput, signal?: AbortSignal): Promise<StoragePolicy> {
      return decodeStoragePolicy(await api.put('/admin/ops/storage/policy', input, { signal, useGlobalLoading: false } as ApiRequestConfig));
    },
    async history(signal?: AbortSignal): Promise<StorageHistory> {
      const data = unwrap(await api.get('/admin/ops/storage/history', { signal, useGlobalLoading: false } as ApiRequestConfig));
      if (!data || !['sampling_required', 'occupancy_only'].includes(data.state) || data.rate_basis !== 'ingress_not_instrumented' || data.daily_increment_bytes !== null || !samplingValid(data.sampling) ||
          !['sample_count', 'expected_samples', 'missing_samples', 'metric_changes', 'unavailable_samples', 'window_days'].every(k => Number.isSafeInteger(data[k]) && data[k] >= 0) ||
          !Array.isArray(data.points) || data.sample_count !== data.points.length || (data.sources !== undefined && (!Array.isArray(data.sources) || !data.sources.every(validResourceRef))) ||
          !data.points.every(validHistoryPoint)) throw new Error('STORAGE_HISTORY_RESPONSE_INVALID');
      return data;
    },
    async overview(signal?: AbortSignal): Promise<StorageOverview> {
      return decodeStorageOverview(await api.get('/admin/ops/storage/overview', { signal, useGlobalLoading: false } as ApiRequestConfig));
    },
    async estimate(input: StorageEstimateInput, signal?: AbortSignal): Promise<StorageEstimate> {
      const data = unwrap(await api.post('/admin/ops/storage/estimates', input, { signal, useGlobalLoading: false } as ApiRequestConfig));
      if (!data || !['sampling_required', 'budget_feasible', 'budget_insufficient', 'artifact_inventory_required', 'retention_not_applicable'].includes(data.state) ||
          !['unavailable', 'manual_assumption'].includes(data.basis) || data.safety_factor !== 1.3 ||
          data.can_enable !== false || !bytes(data.required_bytes) || !bytes(data.deficit_bytes) ||
          (data.supported_days !== null && (!Number.isFinite(data.supported_days) || data.supported_days < 0)) ||
          (['budget_feasible', 'budget_insufficient'].includes(data.state) && (data.basis !== 'manual_assumption' || data.required_bytes === null || data.deficit_bytes === null || data.supported_days === null)) ||
          (input.resource_id !== undefined && (!validResourceRef(data.resource) || data.resource.id !== input.resource_id || data.retention_mode !== input.retention_mode))) {
        throw new Error('STORAGE_ESTIMATE_RESPONSE_INVALID');
      }
      return data;
    },
  };
};
