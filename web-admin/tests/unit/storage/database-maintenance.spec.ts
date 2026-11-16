import { beforeEach, expect, it, vi } from 'vitest';
import { decodeDatabaseMaintenanceRun, decodeDatabaseMaintenanceStatus, decodeMaintenanceSnapshot, useStorageDatabaseMaintenanceService } from '~/composables/api/services/storageDatabaseMaintenanceService';
import { clearMaintenanceIntent, readMaintenanceIntent, saveMaintenanceIntent, type MaintenanceIntent } from '~/utils/database-maintenance-intent';
import { maintenanceRun, maintenanceScope, maintenanceSnapshot } from './database-maintenance-fixture';
const api = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn() }));
vi.mock('~/composables/api/index', () => ({ useApiClient: () => api }));
beforeEach(() => vi.clearAllMocks());
it('keeps missing space observations unknown and rejects invalid table scope', () => {
  expect(decodeMaintenanceSnapshot(maintenanceSnapshot()).tables).toHaveLength(2);
  for (const bad of [{}, { ...maintenanceSnapshot(), database_bytes: undefined }, { ...maintenanceSnapshot(), tables: [maintenanceSnapshot().tables[0], maintenanceSnapshot().tables[0]] }]) expect(() => decodeMaintenanceSnapshot(bad)).toThrow('DATABASE_MAINTENANCE_RESPONSE_INVALID');
  expect(decodeDatabaseMaintenanceStatus({ scope_id: '', ready: false, worker_enabled: false, worker_running: false, runs: [] }).snapshot).toBeUndefined();
  expect(() => decodeDatabaseMaintenanceStatus({ scope_id: maintenanceScope, ready: true, worker_enabled: false, worker_running: false, runs: [] })).toThrow();
});
it('validates original identity, monotonic steps and measured results', () => {
  const run = maintenanceRun('completed'); expect(decodeDatabaseMaintenanceRun(run, run.id).after?.database_bytes).toBeLessThan(run.before.database_bytes);
  expect(() => decodeDatabaseMaintenanceRun(run, crypto.randomUUID())).toThrow();
  expect(() => decodeDatabaseMaintenanceRun({ ...run, steps: [] })).toThrow();
  expect(() => decodeDatabaseMaintenanceRun({ ...run, steps: [run.steps[0], run.steps[0]] })).toThrow();
  expect(() => decodeDatabaseMaintenanceRun({ ...run, filesystem_available_after: -1 })).toThrow();
  expect(decodeDatabaseMaintenanceRun(maintenanceRun('unconfirmed')).after).toBeUndefined();
});
it('uses fixed maintenance contracts, no write retry, and the same request identity', async () => {
  const service = useStorageDatabaseMaintenanceService(), run = maintenanceRun('queued'), signal = new AbortController().signal;
  api.post.mockResolvedValue({ data: run });
  await service.start(run.id, run.preview_id!, true, signal);
  expect(api.post).toHaveBeenLastCalledWith('/admin/ops/storage/database-maintenance/runs', { request_id: run.id, preview_id: run.preview_id, exclusive_lock_confirmed: true, confirm: true }, expect.objectContaining({ signal, retry: 0, useGlobalLoading: false }));
  api.get.mockResolvedValue({ data: run }); await service.run(run.id, signal);
  expect(api.get).toHaveBeenLastCalledWith('/admin/ops/storage/database-maintenance/runs/' + run.id, expect.objectContaining({ retry: 0, signal }));
  api.post.mockResolvedValue({ data: { ...run, state: 'cancelled' } }); await service.cancel(run.id, run.preview_id, signal);
  expect(api.post).toHaveBeenLastCalledWith('/admin/ops/storage/database-maintenance/runs/' + run.id + '/cancellations', { confirm: true, preview_id: run.preview_id }, expect.objectContaining({ retry: 0, signal }));
});
it('keeps unresolved intent across reloads and isolates tenant and login identities', () => {
  const values = new Map<string, string>(), store = { getItem: (key: string) => values.get(key) ?? null, setItem: (key: string, value: string) => { values.set(key, value); }, removeItem: (key: string) => { values.delete(key); } };
  const identity = { sessionId: 'owned-login', tenantUuid: 'tenant-a' };
  const intent: MaintenanceIntent = { ...identity, version: 1, scopeId: maintenanceScope, requestId: crypto.randomUUID(), previewId: crypto.randomUUID(), exclusiveLockConfirmed: true, at: Date.now() };
  saveMaintenanceIntent(store, intent); expect(readMaintenanceIntent(store, identity)).toEqual(intent);
  expect(readMaintenanceIntent(store, { ...identity, tenantUuid: 'tenant-b' })).toBeUndefined(); expect(readMaintenanceIntent(store, { ...identity, sessionId: 'other-login' })).toBeUndefined();
  expect(() => saveMaintenanceIntent(store, { ...intent, requestId: crypto.randomUUID() })).toThrow('DATABASE_MAINTENANCE_RESULT_UNCONFIRMED');
  clearMaintenanceIntent(store, identity, crypto.randomUUID()); expect(readMaintenanceIntent(store, identity)).toEqual(intent);
  clearMaintenanceIntent(store, identity, intent.requestId); expect(readMaintenanceIntent(store, identity)).toBeUndefined();
  saveMaintenanceIntent(store, intent); values.set([...values.keys()][0]!, '{broken'); expect(() => readMaintenanceIntent(store, identity)).toThrow('DATABASE_MAINTENANCE_INTENT_INVALID'); expect(values.size).toBe(1);
});
