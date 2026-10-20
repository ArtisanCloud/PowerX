import { afterEach, describe, expect, it, vi } from 'vitest';
import { clearIndependentRecoverySession } from '~/utils/recovery-session';

afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks(); });
describe('independent recovery session logout', () => {
  it('uses the transient CSRF value to revoke the HttpOnly session', async () => {
    const fetch = vi.fn().mockResolvedValueOnce({ ok: true, json: async () => ({ csrf: 'transient-csrf' }) }).mockResolvedValueOnce({ ok: true });
    vi.stubGlobal('fetch', fetch);
    const storage = vi.spyOn(Storage.prototype, 'setItem');
    await clearIndependentRecoverySession();
    expect(fetch).toHaveBeenNthCalledWith(2, '/__powerx_recovery/session', expect.objectContaining({ method: 'DELETE', credentials: 'same-origin', headers: { 'X-PowerX-Recovery-CSRF': 'transient-csrf' } }));
    expect(storage).not.toHaveBeenCalled();
  });
  it('does not issue a mutation when the normal site has no recovery service', async () => {
    const fetch = vi.fn().mockResolvedValue({ ok: false });vi.stubGlobal('fetch', fetch);
    await clearIndependentRecoverySession();expect(fetch).toHaveBeenCalledTimes(1);
  });
});
