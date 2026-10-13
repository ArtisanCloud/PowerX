import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { AUTH_SESSION_KEY, captureAuthSession, clearAuthSession, commitAuthSession, readAuthSession } from '~/utils/auth-session';
import { recoverPluginFrameSession } from '~/utils/plugin-frame-session';

const login = () => commitAuthSession({
  access_token: `h.${btoa(JSON.stringify({ tid: 'tenant', exp: Math.floor(Date.now() / 1000) + 3600 }))}.s`,
  refresh_token: 'refresh-current',
});
const expire = () => {
  const session = { ...login(), expiresAt: Date.now() - 1 };
  localStorage.setItem(AUTH_SESSION_KEY, JSON.stringify(session));
  return session;
};
beforeEach(() => { (process as any).client = true; localStorage.clear(); });
afterEach(() => {
  for (const name of ['access_token', 'refresh_token', 'px_current_tenant_uuid']) document.cookie = `${name}=; Max-Age=0; path=/`;
});

describe('host-owned plugin document recovery', () => {
  it('repairs missing cookies without rotating a still-valid host session', async () => {
    const session = login();
    document.cookie = 'access_token=; Max-Age=0; path=/';
    const renew = vi.fn();
    expect(await recoverPluginFrameSession(renew)).toEqual(captureAuthSession());
    expect(renew).not.toHaveBeenCalled();
    expect(document.cookie).toContain(`access_token=${encodeURIComponent(session.accessToken)}`);
    expect(readAuthSession()).toEqual(session);
  });
  it('renews only an expired host snapshot and binds recovery to the resulting revision', async () => {
    const previous = expire();
    const stamp = captureAuthSession()!;
    const renew = vi.fn(async () => commitAuthSession({
      access_token: `h.${btoa(JSON.stringify({ tid: 'tenant', exp: Math.floor(Date.now() / 1000) + 3600 }))}.s`,
      refresh_token: 'rotated-refresh',
    }, stamp));
    expect(await recoverPluginFrameSession(renew)).toEqual({ sessionId: previous.sessionId, revision: previous.revision + 1 });
    expect(renew).toHaveBeenCalledTimes(1);
    expect(document.cookie).toContain('refresh_token=rotated-refresh');
  });
  it('cannot restore a signed-out session from a scalar mirror', async () => {
    login(); clearAuthSession(); localStorage.setItem('access_token', 'old-plugin');
    const renew = vi.fn();
    expect(await recoverPluginFrameSession(renew)).toBeNull();
    expect(renew).not.toHaveBeenCalled();
    expect(readAuthSession()?.status).toBe('signed-out');
  });
  it('keeps an expired host session on a transient renewal failure', async () => {
    const session = expire();
    await expect(recoverPluginFrameSession(async () => { throw { status: 503 }; })).rejects.toMatchObject({ status: 503 });
    expect(readAuthSession()).toEqual(session);
  });
  it('cannot repair cookies for an old login after the host identity changes', async () => {
    expire();
    let resolve!:() => void;
    const recovery = recoverPluginFrameSession(() => new Promise<void>(yes => { resolve = yes; }));
    const current = login();
    resolve();
    expect(await recovery).toBeNull();
    expect(readAuthSession()).toEqual(current);
    expect(document.cookie).toContain(`access_token=${encodeURIComponent(current.accessToken)}`);
  });
  it('cannot reload a signed-out generation after a delayed renewal completes', async () => {
    expire();
    let resolve!:() => void;
    const recovery = recoverPluginFrameSession(() => new Promise<void>(yes => { resolve = yes; }));
    clearAuthSession();
    resolve();
    expect(await recovery).toBeNull();
    expect(document.cookie).not.toContain('access_token=');
    expect(readAuthSession()?.status).toBe('signed-out');
  });
});
