import type { DatabaseTableInventory, SystemTableObservation, SystemTableRetentionStatus } from '~/composables/api/services/storageDatabaseTablesService';

export type CleanupState = 'enabled' | 'disabled' | 'blocked' | 'managed' | 'unsupported' | 'protected' | 'unknown';
export type TableSort = 'capacity' | 'growth' | 'oldest' | 'newest' | 'retention' | 'name';
export type SortDirection = 'asc' | 'desc';
export interface TableGovernance {
  state: CleanupState;
  days: number | null;
  retention: string;
  values?: Record<string, number>;
  inactive?: boolean;
  configurable?: boolean;
}
export const typedRetentionTables = ['capability_registry_health_probe_results', 'capability_registry_discovery_cache'] as const;
export const supportsTableRetention = (id: string) => (typedRetentionTables as readonly string[]).includes(id);

export function tableGovernance(row: SystemTableObservation, inventory: DatabaseTableInventory, typed: Partial<Record<string, SystemTableRetentionStatus>>): TableGovernance {
  if (row.cleanup_mechanism === 'protected') return { state: 'protected', days: null, retention: 'protectedRetention' };
  if (row.cleanup_mechanism === 'review_required') return { state: 'unsupported', days: null, retention: 'notConfigured' };
  if (row.cleanup_mechanism === 'metadata_window') {
    if (row.id === 'ops_storage_samples') return { state: 'managed', days: 15, retention: 'days', values: { days: 15 } };
    if (row.id === 'ops_storage_cleanup_runs') return { state: 'managed', days: 30, retention: 'eligibleDays', values: { days: 30 } };
    if (row.id === 'ops_storage_policy_revisions') return { state: 'managed', days: null, retention: 'versions', values: { count: 100 } };
  }
  if (row.cleanup_mechanism === 'invocation_policy') {
    const p = inventory.invocation_policy;
    if (!p) return { state: 'unknown', days: null, retention: 'unknownRetention', configurable: true };
    const state = !p.enabled ? 'disabled' : inventory.monitoring.worker_enabled && inventory.monitoring.state_configured ? 'enabled' : 'blocked';
    return { state, days: Math.max(p.success_days, p.failure_days), retention: 'invocationDays', values: { success: p.success_days, failure: p.failure_days }, inactive: state !== 'enabled', configurable: true };
  }
  if (row.cleanup_mechanism === 'typed_policy' && supportsTableRetention(row.id)) {
    const status = typed[row.id];
    if (!status) return { state: 'unknown', days: null, retention: 'unknownRetention', configurable: true };
    const state = !status.policy.enabled ? 'disabled' : status.configuration?.authority === 'database' && status.configuration.execution_ready && status.ready && status.state_configured && status.enforcement_allowed && status.worker_enabled ? 'enabled' : 'blocked';
    return { state, days: status.policy.retention_days, retention: 'days', values: { days: status.policy.retention_days }, inactive: state !== 'enabled', configurable: true };
  }
  return { state: 'unknown', days: null, retention: 'unknownRetention' };
}

// Unknown values are always last, including ascending sorts. Ties remain deterministic.
export function compareTableValues(a: number | null | undefined, b: number | null | undefined, direction: SortDirection): number {
  const knownA = typeof a === 'number' && Number.isFinite(a), knownB = typeof b === 'number' && Number.isFinite(b);
  if (!knownA || !knownB) return knownA ? -1 : knownB ? 1 : 0;
  return direction === 'asc' ? a - b : b - a;
}
