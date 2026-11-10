import { expect, it } from 'vitest';
import { tableGovernance, compareTableValues } from '~/utils/storage-table-list';
import { tableInventory } from './database-tables-fixture';
import type { SystemTableRetentionStatus } from '~/composables/api/services/storageDatabaseTablesService';

it('keeps unknown values last in either direction and sorts signed growth numerically', () => {
  const values = [null, 1024 ** 3, -128, undefined, 0, 2048];
  expect(values.slice().sort((a, b) => compareTableValues(a, b, 'asc')).slice(0, 4)).toEqual([-128, 0, 2048, 1024 ** 3]);
  expect(values.slice().sort((a, b) => compareTableValues(a, b, 'desc')).slice(0, 4)).toEqual([1024 ** 3, 2048, 0, -128]);
  expect(compareTableValues(null, undefined, 'desc')).toBe(0);
});

it('distinguishes missing policy evidence, disabled rules, blocked execution and protected data', () => {
  const inventory = tableInventory(), row = { ...inventory.items[0]!, id: 'capability_registry_health_probe_results', cleanup_mechanism: 'typed_policy' as const };
  const policy: SystemTableRetentionStatus = {configuration:{authority:'database',state:'applied',ready:true,execution_ready:true,desired_revision:1,applied_revision:1,snapshot_revision:1,legacy_digest:'a'.repeat(64)}, id: row.id, revision: 0, policy: { enabled: false, retention_days: 21, batch_limit: 100 }, ready: true, worker_enabled: true, enforcement_allowed: true, state_configured: true, runs: [] };
  expect(tableGovernance(row, inventory, {}).state).toBe('unknown');
  expect(tableGovernance(row, inventory, { [row.id]: policy })).toMatchObject({ state: 'disabled', days: 21, inactive: true });
  policy.policy.enabled = true; policy.ready = false;
  expect(tableGovernance(row, inventory, { [row.id]: policy })).toMatchObject({ state: 'blocked', inactive: true });
  policy.ready = true;
  expect(tableGovernance(row, inventory, { [row.id]: policy })).toMatchObject({ state: 'enabled', inactive: false });
  expect(tableGovernance(inventory.items[2]!, inventory, {})).toMatchObject({ state: 'protected', days: null });
  expect(tableGovernance(inventory.items[1]!, inventory, {}).state).toBe('unsupported');
  expect(tableGovernance(inventory.items[1]!, inventory, {}).configurable).toBeUndefined();
});

it('does not interpret retained revision counts or an absent policy as retention days', () => {
  const inventory = tableInventory(), row = inventory.items[0]!;
  expect(tableGovernance(row, inventory, {})).toMatchObject({ state: 'unknown', days: null });
  expect(tableGovernance({ ...row, id: 'ops_storage_policy_revisions', cleanup_mechanism: 'metadata_window' }, inventory, {})).toMatchObject({ state: 'managed', days: null, values: { count: 100 } });
  expect(tableGovernance({ ...row, id: 'ops_storage_cleanup_runs', cleanup_mechanism: 'metadata_window' }, inventory, {})).toMatchObject({ state: 'managed', days: 30, retention: 'eligibleDays' });
});
