import { useApiClient } from '../index';
import type { ApiRequestConfig } from '../types/types';

export const databaseMaintenanceTables = ['capability-event-publications', 'capability-invocation-traces'] as const;
export type MaintenanceTableID = typeof databaseMaintenanceTables[number];
export type MaintenanceMode = 'reuse' | 'compact' | 'incremental';
export type MaintenanceState = 'preview' | 'consumed' | 'queued' | 'running' | 'cancel_requested' | 'cancelled' | 'completed' | 'failed' | 'unconfirmed' | 'reviewed';
export interface MaintenanceTable {
  id: MaintenanceTableID; oid: number; heap_bytes: number; index_bytes: number; total_bytes: number;
  live_rows_estimate: number; dead_rows_estimate: number; can_maintain: boolean; indexes_valid: boolean; default_tablespace: boolean; incremental_problem?: string;
}
export interface MaintenanceSnapshot {
  database_oid: number; database_fingerprint: string; database_bytes: number; local_disk_verified: boolean;
  wal_budget_bytes: number; rewrite_safe: boolean; incremental_problem?: string; tables: MaintenanceTable[];
}
const incrementalStopPhases = ['checks', 'measurement', 'eligibility', 'relocation', 'vacuum', 'tail_refilled', 'tail_not_truncated', 'table_not_smaller'] as const;
const incrementalStopReasons = ['time_budget', 'batch_limit'] as const;
export interface MaintenanceIncrementalBatch {
  elapsed_ms: number;
  vacuum_attempts: number;
  truncate_lock_waits?: number;
  heap_reclaimed_bytes: number;
  table_change_bytes?: number;
  stop_phase?: typeof incrementalStopPhases[number];
}
export interface DatabaseMaintenanceRun {
  id: string; version: number; scope_id: string; state: MaintenanceState; mode: MaintenanceMode; actor: number;
  at: string; updated_at: string; expires_at?: string; preview_id?: string; tables: MaintenanceTableID[];
  before: MaintenanceSnapshot; after?: MaintenanceSnapshot; peak_bytes: number;
  steps: {
    incremental?: { slices_completed: number; heap_reclaimed_bytes: number };
    incremental_batches?: MaintenanceIncrementalBatch[];
    incremental_stop_reason?: typeof incrementalStopReasons[number];
    table_id: MaintenanceTableID; state: 'running' | 'completed'; started_at: string; finished_at?: string;
  }[];
  problem?: string; cancelled_by?: number; reviewed_by?: number;
  wal_stop?: {
    phase: 'relocation' | 'vacuum';
    metric: 'wal_file_bytes' | 'wal_quiet_interval_bytes';
    observed_bytes: number;
    limit_bytes: number;
  };
  filesystem_available_before?: number; filesystem_available_after?: number;
}
export interface DatabaseMaintenanceStatus {
  scope_id: string; ready: boolean; worker_enabled: boolean; worker_running: boolean; problem?: string;
  snapshot?: MaintenanceSnapshot; active?: DatabaseMaintenanceRun; runs: DatabaseMaintenanceRun[];
}
const uuid = (v: unknown): v is string => typeof v === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(v);
const digest = (v: unknown) => typeof v === 'string' && /^[0-9a-f]{64}$/.test(v);
const size = (v: unknown): v is number => Number.isSafeInteger(v) && (v as number) >= 0;
const time = (v: unknown): v is string => typeof v === 'string' && Number.isFinite(Date.parse(v));
const table = (v: unknown): v is MaintenanceTableID => databaseMaintenanceTables.includes(v as MaintenanceTableID);
const unwrap = (raw: any) => raw && typeof raw === 'object' && 'data' in raw ? raw.data : raw;
function invalid(): never { throw new Error('DATABASE_MAINTENANCE_RESPONSE_INVALID'); }
function validIncrementalBatches(step: DatabaseMaintenanceRun['steps'][number], mode: MaintenanceMode): boolean {
  const batches = step.incremental_batches;
  if (step.incremental_stop_reason !== undefined && (mode !== 'incremental' || !incrementalStopReasons.includes(step.incremental_stop_reason))) return false;
  if (batches === undefined) return step.incremental_stop_reason === undefined;
  if (mode !== 'incremental' || !Array.isArray(batches) || !batches.length || batches.length > 64) return false;
  let slices = 0, reclaimed = 0;
  for (const [index, batch] of batches.entries()) {
    if (!batch || !size(batch.elapsed_ms) || !size(batch.vacuum_attempts) || batch.vacuum_attempts > 3 || !size(batch.heap_reclaimed_bytes) ||
        batch.truncate_lock_waits !== undefined && (!size(batch.truncate_lock_waits) || batch.truncate_lock_waits > batch.vacuum_attempts) ||
        batch.table_change_bytes !== undefined && !Number.isSafeInteger(batch.table_change_bytes)) return false;
    if (batch.stop_phase !== undefined) {
      if (!incrementalStopPhases.includes(batch.stop_phase) || index !== batches.length - 1 || step.incremental_stop_reason !== undefined) return false;
    } else if (!batch.vacuum_attempts || !batch.heap_reclaimed_bytes || batch.table_change_bytes === undefined || batch.table_change_bytes >= 0) return false;
    if (batch.heap_reclaimed_bytes > 0) {
      slices++;
      reclaimed += batch.heap_reclaimed_bytes;
    }
  }
  return slices === (step.incremental?.slices_completed ?? 0) && reclaimed === (step.incremental?.heap_reclaimed_bytes ?? 0);
}
export function decodeMaintenanceSnapshot(raw: unknown): MaintenanceSnapshot {
  const d: any = raw;
  if (!d || !digest(d.database_fingerprint) || !size(d.database_oid) || !d.database_oid || !size(d.database_bytes) || !size(d.wal_budget_bytes) ||
      typeof d.local_disk_verified !== 'boolean' || typeof d.rewrite_safe !== 'boolean' || !Array.isArray(d.tables) || d.tables.length !== 2 ||
      d.incremental_problem !== undefined && typeof d.incremental_problem !== 'string' ||
      new Set(d.tables.map((x: any) => x.id)).size !== 2 || !d.tables.every((x: any) => x && table(x.id) && size(x.oid) && x.oid > 0 &&
        ['heap_bytes', 'index_bytes', 'total_bytes', 'live_rows_estimate', 'dead_rows_estimate'].every(k => size(x[k])) && x.total_bytes >= x.heap_bytes + x.index_bytes &&
        (x.incremental_problem === undefined || typeof x.incremental_problem === 'string') &&
        ['can_maintain', 'indexes_valid', 'default_tablespace'].every(k => typeof x[k] === 'boolean'))) invalid();
  return d;
}
export function decodeDatabaseMaintenanceRun(raw: unknown, expectedID?: string): DatabaseMaintenanceRun {
  const d: any = unwrap(raw);
  if (!d || !uuid(d.id) || expectedID && d.id !== expectedID || !size(d.version) || !d.version || !digest(d.scope_id) ||
      !['preview', 'consumed', 'queued', 'running', 'cancel_requested', 'cancelled', 'completed', 'failed', 'unconfirmed', 'reviewed'].includes(d.state) ||
      !['reuse', 'compact', 'incremental'].includes(d.mode) || !size(d.actor) || !d.actor || !time(d.at) || !time(d.updated_at) || Date.parse(d.updated_at) < Date.parse(d.at) ||
      !Array.isArray(d.tables) || !d.tables.length || d.tables.length > 2 || !d.tables.every(table) || new Set(d.tables).size !== d.tables.length || !size(d.peak_bytes) ||
      d.expires_at !== undefined && (!time(d.expires_at) || Date.parse(d.expires_at) <= Date.parse(d.at)) || d.state === 'preview' && !d.expires_at ||
      d.preview_id !== undefined && !uuid(d.preview_id) || d.state !== 'preview' && d.state !== 'consumed' && !uuid(d.preview_id) ||
      !Array.isArray(d.steps) || d.steps.length > d.tables.length || new Set(d.steps.map((x: any) => x.table_id)).size !== d.steps.length ||
      !d.steps.every((x: any) => x && d.tables.includes(x.table_id) && ['running', 'completed'].includes(x.state) && time(x.started_at) &&
        (x.state === 'completed' ? time(x.finished_at) && Date.parse(x.finished_at) >= Date.parse(x.started_at) : x.finished_at === undefined) &&
        (x.incremental === undefined || d.mode === 'incremental' && x.incremental &&
          size(x.incremental.slices_completed) && x.incremental.slices_completed > 0 &&
          size(x.incremental.heap_reclaimed_bytes) && x.incremental.heap_reclaimed_bytes > 0) &&
        validIncrementalBatches(x, d.mode)) ||
      ['filesystem_available_before', 'filesystem_available_after'].some(k => d[k] !== undefined && !size(d[k]))) invalid();
  if (d.mode === 'incremental' && d.tables.length !== 1) invalid();
  if (d.wal_stop !== undefined) {
    const stop = d.wal_stop;
    if (d.mode !== 'incremental' || d.problem !== 'DATABASE_MAINTENANCE_INCREMENTAL_WAL_BUDGET' ||
        !stop || !['relocation', 'vacuum'].includes(stop.phase) ||
        !(stop.metric === 'wal_file_bytes' || stop.metric === 'wal_quiet_interval_bytes' && stop.phase === 'relocation') ||
        !size(stop.observed_bytes) || !size(stop.limit_bytes) || !stop.limit_bytes || stop.observed_bytes < stop.limit_bytes) invalid();
  }
  decodeMaintenanceSnapshot(d.before); if (d.after !== undefined) decodeMaintenanceSnapshot(d.after);
  if (d.state === 'completed' && (d.steps.length !== d.tables.length || d.steps.some((x: any) => x.state !== 'completed'))) invalid();
  return d;
}
export function decodeDatabaseMaintenanceStatus(raw: unknown): DatabaseMaintenanceStatus {
  const d: any = unwrap(raw);
  if (!d || typeof d.ready !== 'boolean' || typeof d.worker_enabled !== 'boolean' || typeof d.worker_running !== 'boolean' ||
      typeof d.scope_id !== 'string' || d.scope_id !== '' && !digest(d.scope_id) || !Array.isArray(d.runs) || d.runs.length > 100 ||
      d.ready && (!d.worker_enabled || !d.worker_running || !digest(d.scope_id) || !d.snapshot)) invalid();
  if (d.snapshot) decodeMaintenanceSnapshot(d.snapshot);
  const runs = d.runs.map((x: unknown) => decodeDatabaseMaintenanceRun(x));
  if (new Set(runs.map((x: DatabaseMaintenanceRun) => x.id)).size !== runs.length || runs.some((x: DatabaseMaintenanceRun) => x.scope_id !== d.scope_id)) invalid();
  if (d.active) { decodeDatabaseMaintenanceRun(d.active); if (d.active.scope_id !== d.scope_id || !['queued', 'running', 'cancel_requested', 'unconfirmed'].includes(d.active.state)) invalid(); }
  return { ...d, runs };
}
export function useStorageDatabaseMaintenanceService() {
  const api = useApiClient(), path = '/admin/ops/storage/database-maintenance';
  const config = (signal?: AbortSignal) => ({ signal, retry: 0, timeout: 15000, useGlobalLoading: false } as ApiRequestConfig);
  return {
    async status(signal?: AbortSignal) { return decodeDatabaseMaintenanceStatus(await api.get(path, config(signal))); },
    async preview(mode: MaintenanceMode, tables: MaintenanceTableID[], signal?: AbortSignal) {
      return decodeDatabaseMaintenanceRun(await api.post(path + '/previews', { mode, tables }, config(signal)));
    },
    async start(request_id: string, preview_id: string, exclusive_lock_confirmed: boolean, signal?: AbortSignal) {
      return decodeDatabaseMaintenanceRun(await api.post(path + '/runs', { request_id, preview_id, exclusive_lock_confirmed, confirm: true }, config(signal)), request_id);
    },
    async run(id: string, signal?: AbortSignal) { return decodeDatabaseMaintenanceRun(await api.get(path + '/runs/' + encodeURIComponent(id), config(signal)), id); },
    async cancel(id: string, previewID?: string, signal?: AbortSignal) {
      return decodeDatabaseMaintenanceRun(await api.post(path + '/runs/' + encodeURIComponent(id) + '/cancellations', { confirm: true, ...(previewID ? { preview_id: previewID } : {}) }, config(signal)), id);
    },
    async review(id: string, signal?: AbortSignal) { return decodeDatabaseMaintenanceRun(await api.post(path + '/runs/' + encodeURIComponent(id) + '/reviews', { confirm: true }, config(signal)), id); },
  };
}
