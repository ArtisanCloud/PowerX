import { beforeEach, describe, expect, it, vi } from 'vitest';
import { decodeStorageOverview, decodeStoragePolicy, useStorageGovernanceService } from '~/composables/api/services/storageGovernanceService';
import { formatStorageBytes } from '~/utils/storage-capacity';
import { policy } from './fixtures';
const client = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), put: vi.fn() }));
vi.mock('~/composables/api/index', () => ({ useApiClient: () => client }));
beforeEach(() => vi.clearAllMocks());
describe('storage API contract', () => {
  it('distinguishes unknown, true zero and small allocations', () => {
    expect(formatStorageBytes(null)).toBe('—');
    expect(formatStorageBytes(0)).toBe('0 B');
    expect(formatStorageBytes(4096)).toBe('4 KiB');
    expect(formatStorageBytes(1024 ** 2)).toBe('1 MiB');
    expect(formatStorageBytes(3 * 1024 ** 3)).toBe('3 GiB');
  });
  it('does not convert missing, old, or malformed observations to zero usage', () => {
    for (const raw of [null, {}, { data: {} }, { data: { mode: 'enforce', cleanup_enabled: true } }]) expect(() => decodeStorageOverview(raw)).toThrow('STORAGE_OBSERVATION_RESPONSE_INVALID');
  });
  it('uses only fixed read and pure calculation endpoints', async () => {
    client.get.mockResolvedValue({ data: {} });
    const api = useStorageGovernanceService();
    await expect(api.overview()).rejects.toThrow('STORAGE_OBSERVATION_RESPONSE_INVALID');
    expect(client.get).toHaveBeenCalledWith('/admin/ops/storage/overview', expect.objectContaining({ useGlobalLoading: false }));
    client.post.mockResolvedValue({ data: { state: 'sampling_required', basis: 'unavailable', safety_factor: 1.3, can_enable: false, required_bytes: null, deficit_bytes: null, supported_days: null } });
    await api.estimate({ target_days: 7, max_bytes: 3000, protected_bytes: 0 });
    expect(client.post.mock.calls[0]![0]).toBe('/admin/ops/storage/estimates');
    expect(client.post.mock.calls[0]![1]).not.toHaveProperty('path');
    client.post.mockResolvedValue({ data: { state: 'budget_feasible', can_enable: true } });
    await expect(api.estimate({ target_days: 7, max_bytes: 3000, protected_bytes: 0 })).rejects.toThrow('STORAGE_ESTIMATE_RESPONSE_INVALID');
  });
  it('keeps persisted drafts on a fixed endpoint and rejects an enabled response', async () => {
    const draft = policy();
    client.get.mockResolvedValue({ data: draft }); client.put.mockResolvedValue({ data: draft });
    const api = useStorageGovernanceService(); const abort = new AbortController();
    await api.policy(abort.signal);
    await api.savePolicy({ schema_version: 1, expected_revision: 0, state: 'draft', budget: draft.budget, resources: [] }, abort.signal);
    expect(client.put.mock.calls[0]![0]).toBe('/admin/ops/storage/policy');
    expect(client.put.mock.calls[0]![2]).toEqual(expect.objectContaining({ signal: abort.signal }));
    expect(() => decodeStoragePolicy({ ...draft, cleanup_enabled: true })).toThrow('STORAGE_POLICY_RESPONSE_INVALID');
    expect(() => decodeStoragePolicy({ ...draft, revision: -1 })).toThrow('STORAGE_POLICY_RESPONSE_INVALID');
  });
  it('rejects occupancy data that claims to be a real ingestion rate', async () => {
    const history = { metric_version: 'allocated-occupancy/v1', window_days: 14, sample_count: 0, expected_samples: 4032, missing_samples: 4032, metric_changes: 0, unavailable_samples: 0, state: 'sampling_required', rate_basis: 'ingress_not_instrumented', daily_increment_bytes: null, points: [] };
    const api = useStorageGovernanceService(); client.get.mockResolvedValue({ data: history });
    await api.history(); expect(client.get.mock.calls[0]![0]).toBe('/admin/ops/storage/history');
    client.get.mockResolvedValue({ data: { ...history, daily_increment_bytes: 1000 } });
    await expect(api.history()).rejects.toThrow('STORAGE_HISTORY_RESPONSE_INVALID');
  });
});
