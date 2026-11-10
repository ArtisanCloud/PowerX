import { useApiClient } from '../index';
import type { ApiRequestConfig } from '../types/types';

export type ExecutionAction = 'pause' | 'resume' | 'begin_handoff' | 'complete_handoff' | 'acknowledge_restore' | 'prepare_restore';
export interface ExecutionChange {
  request_id: string;
  expected_revision: number;
  action: ExecutionAction;
  reason: string;
  confirm: boolean;
  external_stopped: boolean;
  admitted_tasks_checked: boolean;
  expected_recovery_epoch?: string;
  environment_checked?: boolean;
}
export interface ExecutionConfiguration {
  legacy_handoff_available?: boolean;
  mode: 'unmanaged' | 'open' | 'paused' | 'handoff' | 'recovery' | 'unavailable';
  desired_revision: number;
  applied_revision: number;
  confirmed: boolean;
  can_pause: boolean;
  can_resume: boolean;
  worker_ready: boolean;
  policies: { domain: string; kind: 'archives' | 'backups' | 'snapshots' | 'invocations' | 'probes' | 'discovery' | 'file_logs' | 'unknown'; resource_name?: string; paused: boolean; confirmed: boolean; problem?: string }[];
  problem?: string;
  recovery?: { state: 'not_connected' | 'none' | 'pending' | 'acknowledged' | 'unavailable'; epoch?: string; job_id?: string; snapshot_id?: string; created_at?: string; prepared?: boolean; resources?: { domain: string; kind: string; revision: number; state: 'pending' | 'ready'; problem?: string }[] };
}
export interface ExecutionMutation {
  request_id: string;
  operation_revision: number;
  operation: ExecutionChange;
  configuration: ExecutionConfiguration;
}
const unwrap = (r: any) => r && typeof r === 'object' && 'data' in r ? r.data : r;
const count = (n: unknown) => Number.isSafeInteger(n) && Number(n) >= 0;
const uuid = (id: unknown) => typeof id === 'string' && /^[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}$/i.test(id);
export function decodeExecutionConfiguration(raw: unknown): ExecutionConfiguration {
  const c = unwrap(raw);
  if (!c || !['unmanaged', 'open', 'paused', 'handoff', 'recovery', 'unavailable'].includes(c.mode) || !count(c.desired_revision) || !count(c.applied_revision) || c.applied_revision > c.desired_revision ||
    (c.legacy_handoff_available !== undefined && typeof c.legacy_handoff_available !== 'boolean') ||
    !['confirmed', 'can_pause', 'can_resume', 'worker_ready'].every(k => typeof c[k] === 'boolean') ||
    (c.confirmed && (!c.desired_revision || c.desired_revision !== c.applied_revision || ['unmanaged', 'recovery', 'unavailable'].includes(c.mode))) ||
    (c.can_resume && (c.mode !== 'paused' || !c.confirmed || !c.worker_ready)) ||
    (c.mode === 'unmanaged' && (c.desired_revision !== 0 || c.applied_revision !== 0)) ||
    !Array.isArray(c.policies) || c.policies.length > 128 || !c.policies.every((p: any) => p && typeof p.domain === 'string' && p.domain.length <= 96 && ['archives', 'backups', 'snapshots', 'invocations', 'probes', 'discovery', 'file_logs', 'unknown'].includes(p.kind) && (p.resource_name === undefined || typeof p.resource_name === 'string') && typeof p.paused === 'boolean' && typeof p.confirmed === 'boolean')) {
    throw new Error('STORAGE_EXECUTION_RESPONSE_INVALID');
  }
  const r = c.recovery;
  if (r !== undefined && (!r || !['not_connected', 'none', 'pending', 'acknowledged', 'unavailable'].includes(r.state) ||
    (['pending', 'acknowledged'].includes(r.state) && (!uuid(r.epoch) || !uuid(r.job_id) || !uuid(r.snapshot_id) || !Number.isFinite(Date.parse(r.created_at)))) ||
    (['none', 'not_connected'].includes(r.state) && r.epoch))) throw new Error('STORAGE_EXECUTION_RESPONSE_INVALID');
  if (r && ((r.prepared !== undefined && typeof r.prepared !== 'boolean') || (r.resources !== undefined && (!Array.isArray(r.resources) || r.resources.length > 128 || !r.resources.every((x: any) => x && typeof x.domain === 'string' && x.domain.length <= 96 && typeof x.kind === 'string' && count(x.revision) && ['pending', 'ready'].includes(x.state) && (x.problem === undefined || typeof x.problem === 'string')))))) throw new Error('STORAGE_EXECUTION_RESPONSE_INVALID');
  if (c.mode === 'recovery' && (r?.state !== 'pending' || c.can_pause || c.can_resume || c.confirmed)) throw new Error('STORAGE_EXECUTION_RESPONSE_INVALID');
  if (r?.state === 'pending' && c.mode !== 'recovery') throw new Error('STORAGE_EXECUTION_RESPONSE_INVALID');
  return c;
}
export function decodeExecutionMutation(raw: unknown): ExecutionMutation {
  const m = unwrap(raw), o = m?.operation;
  if (!m || !uuid(m.request_id) || !count(m.operation_revision) || !m.operation_revision || !o || o.request_id !== m.request_id || !count(o.expected_revision) || (o.action === 'prepare_restore' ? m.operation_revision <= o.expected_revision : o.expected_revision + 1 !== m.operation_revision) ||
    !['pause', 'resume', 'begin_handoff', 'complete_handoff', 'acknowledge_restore', 'prepare_restore'].includes(o.action) || typeof o.reason !== 'string' || Array.from(o.reason.trim()).length < 3 || Array.from(o.reason).length > 500 || o.confirm !== true ||
    !['external_stopped', 'admitted_tasks_checked'].every(k => typeof o[k] === 'boolean') || (o.expected_recovery_epoch !== undefined && typeof o.expected_recovery_epoch !== 'string') || (o.environment_checked !== undefined && typeof o.environment_checked !== 'boolean') || (['complete_handoff', 'acknowledge_restore', 'prepare_restore'].includes(o.action) && (!o.external_stopped || !o.admitted_tasks_checked)) || (['acknowledge_restore', 'prepare_restore'].includes(o.action) && (!uuid(o.expected_recovery_epoch) || o.environment_checked !== true)) || (!['acknowledge_restore', 'prepare_restore'].includes(o.action) && (o.expected_recovery_epoch || o.environment_checked))) {
    throw new Error('STORAGE_EXECUTION_RESPONSE_INVALID');
  }
  decodeExecutionConfiguration(m.configuration);
  return m;
}
export const sameExecutionOperation = (a: ExecutionChange, b: ExecutionChange) =>
  a.request_id === b.request_id && a.expected_revision === b.expected_revision && a.action === b.action && a.reason === b.reason && a.confirm === b.confirm && a.external_stopped === b.external_stopped && a.admitted_tasks_checked === b.admitted_tasks_checked && (a.expected_recovery_epoch || '') === (b.expected_recovery_epoch || '') && !!a.environment_checked === !!b.environment_checked;

export function useStorageExecutionService() {
  const api = useApiClient(), path = '/admin/ops/storage/execution-control';
  const config = (signal?: AbortSignal): ApiRequestConfig => ({ signal, retry: 0, useGlobalLoading: false, timeout: 15000 });
  return {
    async configuration(signal?: AbortSignal) { return decodeExecutionConfiguration(await api.get(path, config(signal))); },
    async change(input: ExecutionChange, signal?: AbortSignal) { return decodeExecutionMutation(await api.post(path + '/changes', input, config(signal))); },
    async request(id: string, signal?: AbortSignal) { return decodeExecutionMutation(await api.get(path + '/requests/' + encodeURIComponent(id), config(signal))); },
  };
}
