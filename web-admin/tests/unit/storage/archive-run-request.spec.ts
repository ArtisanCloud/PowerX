import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { archiveRunOwnerKey, beginArchiveRunIntent, decodeArchiveRunReceipt, readArchiveRunIntent, settleArchiveRunIntent, type ArchiveRunIntent } from '~/utils/archive-run-request';
const api = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), unwrap: (d: any) => d.data }));
vi.mock('~/composables/api/index', () => ({ useApiClient: () => api }));
import { useArchiveOpsService } from '~/composables/api/services/archiveOpsService';
const owner = { sessionId: 'owned-login', tenantUuid: crypto.randomUUID() };
let intent: ArchiveRunIntent;
const receipt = () => ({ request_id: intent.request_id, policy_id: intent.policy_id, job_id: crypto.randomUUID(), policy_revision: intent.policy_revision, source_id: intent.source_id, status: 'running', phase: 'capturing', remote_state: 'pending', created_at: new Date().toISOString(), updated_at: new Date().toISOString() });
beforeEach(() => { sessionStorage.clear(); vi.clearAllMocks(); intent = { version: 1, owner, request_id: crypto.randomUUID(), policy_id: crypto.randomUUID(), policy_revision: 7, source_id: 'logs', created_at: new Date().toISOString() }; });
afterEach(() => vi.restoreAllMocks());
it('persists the fixed non-secret intent before POST, restores it, and prevents a second intent', () => {
  beginArchiveRunIntent(sessionStorage, intent);
  expect(readArchiveRunIntent(sessionStorage, owner)).toEqual(intent);
  expect(() => beginArchiveRunIntent(sessionStorage, { ...intent, request_id: crypto.randomUUID() })).toThrow('PENDING');
  expect(sessionStorage.getItem(sessionStorage.key(0)!)).not.toMatch(/accessToken|refreshToken|secret|credential/);
  expect(readArchiveRunIntent(sessionStorage, { ...owner, sessionId: 'another-login' })).toBeUndefined();
  expect(archiveRunOwnerKey(owner)).not.toEqual(archiveRunOwnerKey({ ...owner, tenantUuid: crypto.randomUUID() }));
});
it('only a positive receipt matching all frozen identity fields clears the intent', () => {
  beginArchiveRunIntent(sessionStorage, intent);
  for (const bad of [{ ...receipt(), request_id: crypto.randomUUID() }, { ...receipt(), policy_id: crypto.randomUUID() }, { ...receipt(), policy_revision: 8 }, { ...receipt(), source_id: 'other' }, { ...receipt(), job_id: 'invalid' }]) {
    expect(() => settleArchiveRunIntent(sessionStorage, intent, bad)).toThrow();
    expect(readArchiveRunIntent(sessionStorage, owner)?.request_id).toBe(intent.request_id);
  }
  const out = receipt(); expect(settleArchiveRunIntent(sessionStorage, intent, out)).toEqual(out);
  expect(readArchiveRunIntent(sessionStorage, owner)).toBeUndefined();
});
it('invalid or unavailable session storage never becomes absence or authorizes a POST', () => {
  const fail = { getItem() { throw new Error('blocked'); } } as unknown as Storage;
  expect(() => readArchiveRunIntent(fail, owner)).toThrow();
  const writer = { getItem: () => null, setItem() { throw new Error('quota'); } } as unknown as Storage;
  expect(() => beginArchiveRunIntent(writer, intent)).toThrow('quota');
  const key = 'powerx.archive-run-intent.v1:' + archiveRunOwnerKey(owner);
  for (const raw of ['broken', JSON.stringify({ ...intent, owner: { ...owner, sessionId: 'wrong' } }), JSON.stringify({ ...intent, policy_revision: 0 }), 'x'.repeat(3000)]) {
    sessionStorage.setItem(key, raw); expect(() => readArchiveRunIntent(sessionStorage, owner)).toThrow(); expect(sessionStorage.getItem(key)).toBe(raw);
  }
});
it('uses fixed readonly request lookup and explicit policy revision with write retry disabled', async () => {
  const svc = useArchiveOpsService(), signal = new AbortController().signal, out = receipt();
  api.post.mockResolvedValue({ data: { job: { id: out.job_id } } });
  await svc.run(intent.policy_id, intent.request_id, signal, intent.policy_revision);
  expect(api.post).toHaveBeenCalledWith(`/admin/ops/backup/archives/policies/${intent.policy_id}/run`, { expected_policy_revision: 7 }, expect.objectContaining({ signal, retry: 0, headers: { 'Idempotency-Key': intent.request_id } }));
  api.get.mockResolvedValue({ data: out });
  expect(await svc.runRequest(intent.policy_id, intent.request_id, signal)).toEqual(out);
  expect(api.get).toHaveBeenCalledWith(`/admin/ops/backup/archives/policies/${intent.policy_id}/requests/${intent.request_id}`, expect.objectContaining({ signal, retry: 0, useGlobalLoading: false }));
  api.get.mockRejectedValueOnce(Object.assign(new Error('not found'), { status: 404 }));
  await expect(svc.runRequest(intent.policy_id, intent.request_id, signal)).rejects.toThrow('not found');
  expect(api.post).toHaveBeenCalledTimes(1);
});
it('requires valid bounded receipt identity and dates; drops unknown response fields', () => {
  expect(decodeArchiveRunReceipt({ ...receipt(), secret: 'must not enter UI state' })).not.toHaveProperty('secret');
  for (const bad of [{ ...receipt(), created_at: 'bad' }, { ...receipt(), policy_revision: -1 }, { ...receipt(), request_id: '00000000-0000-0000-0000-000000000000' }, { ...receipt(), remote_state: '' }]) expect(() => decodeArchiveRunReceipt(bad)).toThrow();
});
