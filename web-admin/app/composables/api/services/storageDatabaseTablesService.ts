import { useApiClient } from '../index';
import type { ApiRequestConfig } from '../types/types';
import type { DatabaseRetentionPolicy } from './storageRuntimeService';

export interface SystemTableObservation {
  id: string; schema: string; category: 'observability' | 'runtime' | 'security' | 'usage' | 'recovery' | 'protected';
  time_column: string; cleanup_mechanism: 'invocation_policy' | 'typed_policy' | 'metadata_window' | 'review_required' | 'protected'; protection: string;
  state: 'observed' | 'not_present' | 'unavailable';
  heap_bytes: number | null; indexes_bytes: number | null; toast_bytes: number | null; total_bytes: number | null; estimated_rows: number | null;
  time_state: 'observed' | 'empty' | 'no_timestamps' | 'unavailable'; oldest_at: string | null; newest_at: string | null;
  problem?: string; time_problem?: string;
}
export interface DatabaseTableInventory {
  observed_at: string; expires_at: string; identity: { instance_id?: string; environment?: string };
  scope: 'current_database_registered_tables_including_soft_deleted'; items: SystemTableObservation[];
  invocation_policy: DatabaseRetentionPolicy | null; policy_problem?: string;
  monitoring: TableMonitoringStatus;
}

export interface TableMonitoringPolicy { revision: number; owner_instance: string; enabled: boolean; warning_bytes: number; growth_bytes: number; growth_window_minutes: number; stale_after_minutes: number; failure_limit: number; updated_at: string; actor: number }
export type TableMonitoringSettings = Pick<TableMonitoringPolicy, 'enabled' | 'warning_bytes' | 'growth_bytes' | 'growth_window_minutes' | 'stale_after_minutes' | 'failure_limit'>;
export interface TableMonitoringConfiguration {
  authority: 'legacy' | 'database' | 'unavailable'; ready: boolean;
  state: 'legacy' | 'pending' | 'applied' | 'failed' | 'unconfirmed' | 'unavailable';
  desired_revision: number; applied_revision: number; snapshot_revision: number | null;
  desired_policy: TableMonitoringSettings | null; legacy_revision: number; legacy_digest: string; legacy_enabled: boolean; problem?: string;
}
export interface TableMonitoringMutation { request_id: string; operation_revision: number; operation_policy: TableMonitoringSettings; configuration: TableMonitoringConfiguration }
export interface TableHealth { id: string; state: string; bytes: number | null; observed_at: string; baseline_at: string | null; net_change_bytes: number | null; level: string; problem?: string }
export interface TableMonitoringStatus {
  state: string; state_configured: boolean; sampling_configured: boolean; worker_enabled: boolean; policy: TableMonitoringPolicy; sampled_at: string | null;
  tables: TableHealth[]; alerts: { code: string; level: string; resource_id?: string }[];
  cleanup: null | { state: string; started_at: string | null; finished_at: string | null; last_success_at: string | null; consecutive_failures: number };
  backlog: null | { state: string; observed_at: string | null; since: string | null; eligible_present: boolean; quarantine_present: boolean };
  configuration?: TableMonitoringConfiguration;
  problem?: string;
}
export interface TableHistory { id: string; window_hours: number; metric: 'allocated_bytes_net_change_not_ingress'; points: { observed_at: string; state: string; bytes: number | null }[] }
export interface SystemTableRetentionPolicy { enabled: boolean; retention_days: number; batch_limit: number; actor?: number; updated_at?: string }
export interface SystemTableRetentionConfiguration {authority:'database'|'legacy'|'unavailable';state:string;ready:boolean;execution_ready:boolean;desired_revision:number;applied_revision:number;snapshot_revision:number|null;legacy_digest:string;problem?:string}
export interface SystemTableRetentionMutation {table_id:string;request_id:string;operation_revision:number;configuration:SystemTableRetentionConfiguration}
export interface SystemTableRetentionChange {request_id:string;expected_revision:number;action:'save'|'status';settings?:{retention_days:number;batch_limit:number};enabled?:boolean;confirm:true}
export interface SystemTableRetentionRun {preview_id?:string;expires_at?:string;trigger?:string; id: string; table_id: string; revision: number; state: string; at: string; cutoff: string; rows: number; actor: number; problem?: string; policy_snapshot: SystemTableRetentionPolicy }
export interface SystemTableRetentionStatus {configuration?:SystemTableRetentionConfiguration;unconfirmed_runs?:number;adapter_problem?:string; id: string; revision: number; policy: SystemTableRetentionPolicy; state_configured: boolean; enforcement_allowed: boolean; worker_enabled: boolean; ready: boolean; problem?: string; runs: SystemTableRetentionRun[] }

const amount = (n: unknown) => n === null || (Number.isSafeInteger(n) && Number(n) >= 0);
const timestamp = (s: unknown) => typeof s === 'string' && Number.isFinite(Date.parse(s));
const identifier = (s: unknown) => typeof s === 'string' && /^[a-z][a-z0-9_]{0,100}$/.test(s);
const unwrap = (raw: any) => raw && typeof raw === 'object' && 'data' in raw ? raw.data : raw;
const nullableTimestamp = (v: unknown) => v === null || timestamp(v);
export function decodeTableMonitoring(raw: unknown): TableMonitoringStatus {
  const d = unwrap(raw), p = d?.policy;
  if (!d || !['disabled', 'observed', 'unavailable', 'sampling_required', 'stale'].includes(d.state) ||
    !['state_configured', 'sampling_configured', 'worker_enabled'].every(k => typeof d[k] === 'boolean') ||
    !p || typeof p.enabled !== 'boolean' || !Number.isSafeInteger(p.revision) || p.revision < 0 || !Number.isSafeInteger(p.warning_bytes) || p.warning_bytes < 64 * 2 ** 20 || !Number.isSafeInteger(p.growth_bytes) || p.growth_bytes < 2 ** 20 ||
    !['growth_window_minutes', 'stale_after_minutes', 'failure_limit'].every(k => Number.isSafeInteger(p[k]) && p[k] > 0) ||
    !(d.sampled_at === null || timestamp(d.sampled_at)) || !Array.isArray(d.tables) || d.tables.length > 96 ||
    !d.tables.every((r: any) => identifier(r.id) && ['observed', 'not_present', 'unavailable'].includes(r.state) && amount(r.bytes) &&
      (r.state === 'observed' ? r.bytes !== null : r.bytes === null) && timestamp(r.observed_at) && (r.net_change_bytes === null ? r.baseline_at === null : timestamp(r.baseline_at) && Number.isSafeInteger(r.net_change_bytes)) && ['normal', 'warning', 'unknown'].includes(r.level)) ||
    new Set(d.tables.map((r: any) => r.id)).size !== d.tables.length ||
    !Array.isArray(d.alerts) || d.alerts.length > 200 || !d.alerts.every((a: any) => typeof a.code === 'string' && ['warning', 'critical', 'unknown'].includes(a.level)) ||
    !(d.cleanup === null || (d.cleanup && ['not_started', 'running', 'completed', 'failed'].includes(d.cleanup.state) && ['started_at', 'finished_at', 'last_success_at'].every(k => nullableTimestamp(d.cleanup[k])) && Number.isSafeInteger(d.cleanup.consecutive_failures) && d.cleanup.consecutive_failures >= 0)) ||
    !(d.backlog === null || (d.backlog && ['not_checked', 'disabled', 'observed', 'unavailable'].includes(d.backlog.state) && nullableTimestamp(d.backlog.observed_at) && nullableTimestamp(d.backlog.since) && typeof d.backlog.eligible_present === 'boolean' && typeof d.backlog.quarantine_present === 'boolean'))) throw new Error('STORAGE_TABLE_MONITORING_RESPONSE_INVALID');
  if (d.configuration !== undefined) decodeMonitoringConfiguration(d.configuration);
  return d;
}

// 未知量保留 null；缺失字段、错误分项或非法时间范围不能伪装成有效观测。
export function decodeDatabaseTableInventory(raw: unknown): DatabaseTableInventory {
  const data: any = raw && typeof raw === 'object' && 'data' in raw ? raw.data : raw;
  if (!data || !timestamp(data.observed_at) || !timestamp(data.expires_at) || !data.identity ||
    data.scope !== 'current_database_registered_tables_including_soft_deleted' || !Array.isArray(data.items) || data.items.length > 96 ||
    !data.items.every((r: any) => r && identifier(r.id) && typeof r.schema === 'string' && identifier(r.time_column) &&
      ['observability', 'runtime', 'security', 'usage', 'recovery', 'protected'].includes(r.category) &&
      ['invocation_policy', 'typed_policy', 'metadata_window', 'review_required', 'protected'].includes(r.cleanup_mechanism) && typeof r.protection === 'string' &&
      ['observed', 'not_present', 'unavailable'].includes(r.state) &&
      ['heap_bytes', 'indexes_bytes', 'toast_bytes', 'total_bytes', 'estimated_rows'].every(k => amount(r[k])) &&
      (r.state !== 'observed' || (['heap_bytes', 'indexes_bytes', 'toast_bytes', 'total_bytes'].every(k => r[k] !== null) && r.total_bytes === r.heap_bytes + r.indexes_bytes + r.toast_bytes)) &&
      (r.state === 'observed' || ['heap_bytes', 'indexes_bytes', 'toast_bytes', 'total_bytes'].every(k => r[k] === null)) &&
      ['observed', 'empty', 'no_timestamps', 'unavailable'].includes(r.time_state) &&
      (r.time_state === 'observed' ? timestamp(r.oldest_at) && timestamp(r.newest_at) && Date.parse(r.oldest_at) <= Date.parse(r.newest_at) : r.oldest_at === null && r.newest_at === null)) ||
    new Set(data.items.map((r: any) => r.id)).size !== data.items.length ||
    !(data.invocation_policy === null || (data.invocation_policy && typeof data.invocation_policy.enabled === 'boolean' && Number.isInteger(data.invocation_policy.success_days) && Number.isInteger(data.invocation_policy.failure_days)))) {
    throw new Error('STORAGE_TABLE_INVENTORY_RESPONSE_INVALID');
  }
  return { ...data, monitoring: decodeTableMonitoring(data.monitoring) };
}

export function useStorageDatabaseTablesService() {
  const api = useApiClient();
  const config = (signal: AbortSignal) => ({ signal, retry: 0, timeout: 15000, useGlobalLoading: false } as ApiRequestConfig);
  const retentionPath = (id: string) => `/admin/ops/storage/database-tables/${encodeURIComponent(id)}/retention`;
  return {
    async inventory(signal: AbortSignal, id?: string): Promise<DatabaseTableInventory> {
      return decodeDatabaseTableInventory(await api.get(`/admin/ops/storage/database-tables${id ? '/' + encodeURIComponent(id) : ''}`, { signal, retry: 0, timeout: 15000, useGlobalLoading: false } as ApiRequestConfig));
    },
    async monitoring(signal: AbortSignal): Promise<TableMonitoringStatus> { return decodeTableMonitoring(await api.get('/admin/ops/storage/database-tables/monitoring', config(signal))); },
    async saveMonitoring(signal: AbortSignal, policy: TableMonitoringSettings, revision: number, requestID: string): Promise<TableMonitoringMutation> {
      return decodeMonitoringMutation(await api.put('/admin/ops/storage/database-tables/monitoring', { expected_revision: revision, request_id: requestID, policy: monitoringSettings(policy), confirm: true }, config(signal)), requestID);
    },
    async adoptMonitoring(signal: AbortSignal, configuration: TableMonitoringConfiguration, requestID: string): Promise<TableMonitoringMutation> {
      return decodeMonitoringMutation(await api.post('/admin/ops/storage/database-tables/monitoring/adoptions', { legacy_revision: configuration.legacy_revision, legacy_digest: configuration.legacy_digest, request_id: requestID, confirm: true }, config(signal)), requestID);
    },
    async monitoringRequest(signal: AbortSignal, requestID: string): Promise<TableMonitoringMutation> {
      return decodeMonitoringMutation(await api.get('/admin/ops/storage/database-tables/monitoring/requests/' + encodeURIComponent(requestID), config(signal)), requestID);
    },
    async history(signal: AbortSignal, id: string): Promise<TableHistory> {
      const d = unwrap(await api.get(`/admin/ops/storage/database-tables/${encodeURIComponent(id)}/history`, { signal, retry: 0, useGlobalLoading: false } as ApiRequestConfig));
      if (!d || d.id !== id || d.metric !== 'allocated_bytes_net_change_not_ingress' || d.window_hours !== 24 || !Array.isArray(d.points) || d.points.length > 289 || !d.points.every((p: any) => timestamp(p.observed_at) && amount(p.bytes) && ['observed', 'not_present', 'unavailable'].includes(p.state))) throw new Error('STORAGE_TABLE_HISTORY_RESPONSE_INVALID');
      return d;
    },
    async retention(signal: AbortSignal, id: string): Promise<SystemTableRetentionStatus> { return decodeTableRetention(await api.get(retentionPath(id), config(signal)), id); },
    async adoptRetention(signal:AbortSignal,id:string,input:{request_id:string;legacy_digest:string;confirm:true}):Promise<SystemTableRetentionMutation>{return decodeSystemRetentionMutation(await api.post(retentionPath(id)+'/adoptions',input,config(signal)),id,input.request_id);},
    async changeRetention(signal:AbortSignal,id:string,input:SystemTableRetentionChange):Promise<SystemTableRetentionMutation>{return decodeSystemRetentionMutation(await api.post(retentionPath(id)+'/changes',input,config(signal)),id,input.request_id);},
    async retentionRequest(signal:AbortSignal,id:string,key:string):Promise<SystemTableRetentionMutation>{return decodeSystemRetentionMutation(await api.get(retentionPath(id)+'/requests/'+encodeURIComponent(key),config(signal)),id,key);},
    async retentionRunRequest(signal:AbortSignal,id:string,key:string):Promise<SystemTableRetentionRun>{const r=decodeTableRetentionRun(unwrap(await api.get(retentionPath(id)+'/runs/'+encodeURIComponent(key),config(signal))),id);if(r.id!==key||r.state!=='completed')throw new Error('SYSTEM_RETENTION_RECEIPT_INVALID');return r;},
    async previewRetention(signal: AbortSignal, id: string, revision: number): Promise<SystemTableRetentionRun> { return decodeTableRetentionRun(unwrap(await api.post(retentionPath(id) + '/previews', { expected_revision: revision }, config(signal))), id); },
    async runRetention(signal: AbortSignal, id: string, preview: SystemTableRetentionRun, requestID: string): Promise<SystemTableRetentionRun> { return decodeTableRetentionRun(unwrap(await api.post(retentionPath(id) + '/runs', { expected_revision: preview.revision, preview_id: preview.id, request_id: requestID, confirm: true }, config(signal))), id); },
  };
}

function retentionPolicy(p: any) { return p && typeof p.enabled === 'boolean' && Number.isInteger(p.retention_days) && p.retention_days >= 7 && p.retention_days <= 3650 && Number.isInteger(p.batch_limit) && p.batch_limit >= 1 && p.batch_limit <= 500; }
export function decodeTableRetentionRun(r: any, id: string): SystemTableRetentionRun {
  if (!r || r.table_id !== id || (typeof r.id !== 'string' || !/^[0-9a-f-]{36}$/i.test(r.id)) || !['preview', 'completed', 'failed', 'unconfirmed'].includes(r.state) || (r.rows===null || !amount(r.rows)) || !timestamp(r.at) || !timestamp(r.cutoff) || !Number.isSafeInteger(r.revision) || !retentionPolicy(r.policy_snapshot)) throw new Error('STORAGE_TABLE_RETENTION_RUN_INVALID');
  return r;
}
export function decodeTableRetention(raw: unknown, id: string): SystemTableRetentionStatus {
  const d = unwrap(raw);
  if (!d || d.id !== id || !Number.isSafeInteger(d.revision) || d.revision < 0 || !retentionPolicy(d.policy) || !['state_configured', 'enforcement_allowed', 'worker_enabled', 'ready'].every(k => typeof d[k] === 'boolean') || !Array.isArray(d.runs) || d.runs.length > 300) throw new Error('STORAGE_TABLE_RETENTION_RESPONSE_INVALID');
  if(d.configuration)decodeSystemRetentionConfiguration(d.configuration);d.runs.forEach((r: any) => decodeTableRetentionRun(r, id)); return d;
}

export function monitoringSettings(p: TableMonitoringSettings): TableMonitoringSettings {
  return { enabled: p.enabled, warning_bytes: p.warning_bytes, growth_bytes: p.growth_bytes, growth_window_minutes: p.growth_window_minutes, stale_after_minutes: p.stale_after_minutes, failure_limit: p.failure_limit };
}
function validMonitoringSettings(p: any): boolean {
  return p && typeof p.enabled === 'boolean' && Number.isSafeInteger(p.warning_bytes) && p.warning_bytes >= 64 * 2 ** 20 && p.warning_bytes <= 2 ** 50 && Number.isSafeInteger(p.growth_bytes) && p.growth_bytes >= 2 ** 20 && p.growth_bytes <= 2 ** 50 && Number.isInteger(p.growth_window_minutes) && p.growth_window_minutes >= 5 && p.growth_window_minutes <= 1440 && Number.isInteger(p.stale_after_minutes) && p.stale_after_minutes >= 15 && p.stale_after_minutes <= 1440 && Number.isInteger(p.failure_limit) && p.failure_limit >= 1 && p.failure_limit <= 20;
}
export function decodeMonitoringConfiguration(c: any): TableMonitoringConfiguration {
  const revision = (n: unknown) => Number.isSafeInteger(n) && Number(n) >= 0;
  if (!c || !['legacy','database','unavailable'].includes(c.authority) || !['legacy','pending','applied','failed','unconfirmed','unavailable'].includes(c.state) || typeof c.ready !== 'boolean' || !revision(c.desired_revision) || !revision(c.applied_revision) || c.applied_revision > c.desired_revision || !(c.snapshot_revision === null || revision(c.snapshot_revision)) || !(c.desired_policy === null || validMonitoringSettings(c.desired_policy)) || !revision(c.legacy_revision) || typeof c.legacy_digest !== 'string' || typeof c.legacy_enabled !== 'boolean' || c.authority === 'database' && (c.desired_revision < 1 || !validMonitoringSettings(c.desired_policy)) || c.state === 'applied' && (c.authority !== 'database' || c.applied_revision !== c.desired_revision || c.snapshot_revision !== c.applied_revision)) throw new Error('STORAGE_TABLE_CONFIGURATION_RESPONSE_INVALID');
  return c;
}
export function decodeMonitoringMutation(raw: unknown, key: string): TableMonitoringMutation {
  const d=unwrap(raw);
  if (!d || d.request_id !== key || !Number.isSafeInteger(d.operation_revision) || d.operation_revision < 1 || !validMonitoringSettings(d.operation_policy)) throw new Error('STORAGE_TABLE_MUTATION_RESPONSE_INVALID');
  return {...d, configuration:decodeMonitoringConfiguration(d.configuration)};
}

export function decodeSystemRetentionConfiguration(c:any):SystemTableRetentionConfiguration{const n=(v:unknown)=>Number.isSafeInteger(v)&&Number(v)>=0;if(!c||!['database','legacy','unavailable'].includes(c.authority)||!['legacy','unconfigured','pending','applied','failed','unconfirmed','unavailable'].includes(c.state)||typeof c.ready!=='boolean'||typeof c.execution_ready!=='boolean'||![c.desired_revision,c.applied_revision].every(n)||c.applied_revision>c.desired_revision||!(c.snapshot_revision===null||n(c.snapshot_revision))||typeof c.legacy_digest!=='string'||c.authority!=='unavailable'&&!/^[a-f0-9]{64}$/.test(c.legacy_digest))throw new Error('SYSTEM_RETENTION_CONFIGURATION_INVALID');return c;}
export function decodeSystemRetentionMutation(raw:unknown,id:string,key:string):SystemTableRetentionMutation{const m=unwrap(raw);if(!m||m.table_id!==id||m.request_id!==key||!/^[0-9a-f-]{36}$/i.test(key)||!Number.isSafeInteger(m.operation_revision)||m.operation_revision<1)throw new Error('SYSTEM_RETENTION_RECEIPT_INVALID');decodeSystemRetentionConfiguration(m.configuration);return m;}
