import type { DatabaseMaintenanceRun, MaintenanceSnapshot } from '~/composables/api/services/storageDatabaseMaintenanceService';
export const maintenanceScope = 'a'.repeat(64);
export function maintenanceSnapshot(): MaintenanceSnapshot {
  return { database_oid: 1, database_fingerprint: 'b'.repeat(64), database_bytes: 20 * 2 ** 20, local_disk_verified: true, wal_budget_bytes: 2 ** 30, rewrite_safe: true, tables: [
    { id: 'capability-event-publications', oid: 100, heap_bytes: 4 * 2 ** 20, index_bytes: 2 ** 20, total_bytes: 5 * 2 ** 20, live_rows_estimate: 1000, dead_rows_estimate: 500, can_maintain: true, indexes_valid: true, default_tablespace: true },
    { id: 'capability-invocation-traces', oid: 101, heap_bytes: 8 * 2 ** 20, index_bytes: 2 * 2 ** 20, total_bytes: 10 * 2 ** 20, live_rows_estimate: 1000, dead_rows_estimate: 500, can_maintain: true, indexes_valid: true, default_tablespace: true },
  ] };
}
export function maintenanceRun(state: DatabaseMaintenanceRun['state'] = 'preview', id = crypto.randomUUID()): DatabaseMaintenanceRun {
  const at = new Date(Date.now() - 5000).toISOString(), updated = new Date().toISOString();
  const run: DatabaseMaintenanceRun = { id, version: 1, scope_id: maintenanceScope, state, mode: 'compact', actor: 1, at, updated_at: updated, tables: ['capability-invocation-traces', 'capability-event-publications'], before: maintenanceSnapshot(), peak_bytes: 3 * 2 ** 30, steps: [] };
  if (state === 'preview') run.expires_at = new Date(Date.now() + 300000).toISOString(); else if (state !== 'consumed') run.preview_id = crypto.randomUUID();
  if (state === 'completed') { run.after = { ...maintenanceSnapshot(), database_bytes: 12 * 2 ** 20, tables: maintenanceSnapshot().tables.map(row => ({ ...row, heap_bytes: row.heap_bytes / 2, index_bytes: row.index_bytes / 2, total_bytes: row.total_bytes / 2 })) }; run.steps = run.tables.map(table_id => ({ table_id, state: 'completed', started_at: at, finished_at: updated })); }
  return run;
}
