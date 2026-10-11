import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AUTH_SESSION_KEY, AuthSessionChangedError, captureAuthSession, clearAuthSession, commitAuthSession, readAuthSession, refreshAuthSession } from '~/utils/auth-session';
const token = (tid: string, exp = Math.floor(Date.now()/1000) + 3600) => `header.${btoa(JSON.stringify({tid, exp}))}.signature`;
const payload = (tid: string) => ({access_token: token(tid), refresh_token: `refresh-${tid}`, expires_in: 3600});
const deferred = <T,>() => { let resolve!: (value: T) => void, reject!: (reason: unknown) => void; const promise = new Promise<T>((yes,no) => {resolve=yes;reject=no}); return {promise,resolve,reject}; };
beforeEach(() => {
  (process as any).client = true; localStorage.clear();
  let tail: Promise<unknown> = Promise.resolve();
  Object.defineProperty(navigator, 'locks', { configurable: true, value: {request: vi.fn((_key: string, callback: () => unknown) => {
    const next = tail.then(callback); tail = next.catch(() => undefined); return next;
  })}});
});
describe('authoritative host session', () => {
  it('requires login for unversioned scalar storage', () => {
    localStorage.setItem('access_token', token('old')); expect(readAuthSession()).toBeNull();
  });
  it('is unaffected by old plugin mirror writes and broad auth-key cleanup', () => {
    const current = commitAuthSession(payload('current'));
    localStorage.setItem('access_token', token('old')); localStorage.setItem('refresh_token', 'old-refresh');
    Object.keys(localStorage).filter(key => key.includes('auth') || key.includes('token')).forEach(key => localStorage.removeItem(key));
    expect(readAuthSession()).toEqual(current); expect(localStorage.getItem(AUTH_SESSION_KEY)).not.toBeNull();
  });
  it('serializes concurrent API and WS refreshes into one request', async () => {
    commitAuthSession(payload('current')); const reply = deferred<ReturnType<typeof payload>>();
    const request = vi.fn(() => reply.promise); const one = refreshAuthSession(request), two = refreshAuthSession(request);
    await vi.waitFor(() => expect(request).toHaveBeenCalledTimes(1));
    reply.resolve({...payload('current'), refresh_token: 'rotated'});
    expect(await one).toBe(token('current')); expect(await two).toBe(token('current'));
    expect(request).toHaveBeenCalledTimes(1); expect(readAuthSession()?.revision).toBe(2);
    expect(readAuthSession()?.refreshToken).toBe('rotated');
  });
  it('rejects delayed refresh success after another login', async () => {
    commitAuthSession(payload('old')); const reply = deferred<ReturnType<typeof payload>>();
    const result = refreshAuthSession(() => reply.promise); const assertion = expect(result).rejects.toBeInstanceOf(AuthSessionChangedError);
    await Promise.resolve(); await Promise.resolve(); const current = commitAuthSession(payload('current'));
    reply.resolve(payload('old')); await assertion;
    expect(readAuthSession()).toEqual(current); expect(document.cookie).toContain(encodeURIComponent(current.accessToken));
  });
  it('rejects delayed refresh failure without clearing another login', async () => {
    commitAuthSession(payload('old')); const reply = deferred<ReturnType<typeof payload>>();
    const result = refreshAuthSession(() => reply.promise); const assertion = expect(result).rejects.toBeInstanceOf(AuthSessionChangedError);
    await Promise.resolve(); await Promise.resolve(); const current = commitAuthSession(payload('current'));
    reply.reject({status:401}); await assertion; expect(readAuthSession()).toEqual(current);
  });
  it('cannot revive a signed-out generation', async () => {
    commitAuthSession(payload('old')); const reply = deferred<ReturnType<typeof payload>>();
    const result = refreshAuthSession(() => reply.promise); const assertion = expect(result).rejects.toBeInstanceOf(AuthSessionChangedError);
    await Promise.resolve(); await Promise.resolve(); clearAuthSession(); reply.resolve(payload('old')); await assertion;
    expect(readAuthSession()?.status).toBe('signed-out'); expect(localStorage.getItem('access_token')).toBeNull();
  });
  it('preserves omitted refresh only for checked same-session refresh', () => {
    commitAuthSession(payload('current')); const stamp = captureAuthSession()!;
    commitAuthSession({access_token:token('current')},stamp); expect(readAuthSession()?.refreshToken).toBe('refresh-current');
    expect(() => commitAuthSession({access_token:token('other')})).toThrow('AUTH_SESSION_PAYLOAD_INVALID');
  });
  it('invalidates only the current session on refresh 401', async () => {
    commitAuthSession(payload('current'));
    await expect(refreshAuthSession(async () => { throw {status:401}; })).rejects.toMatchObject({status:401});
    expect(readAuthSession()?.status).toBe('signed-out');
  });
  it('keeps state on transient failure and rejects stale commits', async () => {
    const current = commitAuthSession(payload('current'));
    await expect(refreshAuthSession(async () => { throw {status:503}; })).rejects.toMatchObject({status:503});
    expect(readAuthSession()).toEqual(current); const stamp = captureAuthSession()!; clearAuthSession();
    expect(() => commitAuthSession(payload('current'),stamp)).toThrow(AuthSessionChangedError);
  });
});
