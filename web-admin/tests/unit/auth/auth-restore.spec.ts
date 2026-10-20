import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { readonly, ref } from 'vue';
import { __resetCookies, useCookie } from '../../support/nuxt-app-shim';
import { useAuth } from '~/composables/useAuth';
import { AUTH_SESSION_KEY, clearAuthSession, commitAuthSession, readAuthSession } from '~/utils/auth-session';

vi.mock('~/composables/api/services/authService', () => ({ useAuthService: () => ({}) }));

const startSession = () => commitAuthSession({
  access_token: `header.${btoa(JSON.stringify({ tid: 'tenant', exp: Math.floor(Date.now() / 1000) + 3600 }))}.signature`,
  refresh_token: 'refresh-current',
  expires_in: 3600,
});
const removeCookies = () => {
  for (const name of ['access_token', 'refresh_token', 'px_current_tenant_uuid']) {
    document.cookie = `${name}=; Max-Age=0; path=/`;
  }
};

beforeEach(() => {
  (process as any).client = true;
  localStorage.clear();
  removeCookies();
  __resetCookies();
  const states = new Map<string, ReturnType<typeof ref>>();
  vi.stubGlobal('useState', (key: string, init: () => unknown) => {
    if (!states.has(key)) states.set(key, ref(init()));
    return states.get(key);
  });
  vi.stubGlobal('readonly', readonly);
  vi.spyOn(window, 'addEventListener');
});

afterEach(() => {
  for (const [type, listener, options] of vi.mocked(window.addEventListener).mock.calls) {
    window.removeEventListener(type, listener, options);
  }
  delete (window as any).__powerxAuthSessionListener;
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  removeCookies();
});

describe('host authentication restoration', () => {
  it('restores iframe credentials from the current session after session cookies disappear', () => {
    const session = startSession();
    removeCookies();
    __resetCookies();
    const dispatch = vi.spyOn(window, 'dispatchEvent');
    const auth = useAuth();
    auth.initAuth();

    expect(auth.isAuthenticated.value).toBe(true);
    expect(auth.getToken()).toBe(session.accessToken);
    expect(document.cookie).toContain(`access_token=${encodeURIComponent(session.accessToken)}`);
    expect(document.cookie).toContain('refresh_token=refresh-current');
    expect(document.cookie).toContain('px_current_tenant_uuid=tenant');
    expect(useCookie('access_token').value).toBe(session.accessToken);
    expect(useCookie('refresh_token').value).toBe(session.refreshToken);
    expect(readAuthSession()).toEqual(session);
    expect(dispatch).not.toHaveBeenCalled();
  });

  it('replaces stale cookies using the authoritative session instead of scalar mirrors', () => {
    const session = startSession();
    document.cookie = 'access_token=old-cookie; path=/';
    document.cookie = 'refresh_token=old-refresh; path=/';
    localStorage.setItem('access_token', 'old-plugin-token');
    useAuth().initAuth();

    expect(useCookie('access_token').value).toBe(session.accessToken);
    expect(document.cookie).not.toContain('old-cookie');
    expect(readAuthSession()).toEqual(session);
  });

  it('clears expired cookie credentials while preserving the session for host renewal', () => {
    const session = { ...startSession(), expiresAt: Date.now() - 1 };
    localStorage.setItem(AUTH_SESSION_KEY, JSON.stringify(session));
    const auth = useAuth();
    auth.initAuth();

    expect(auth.getToken()).toBeNull();
    expect(document.cookie).not.toContain('access_token=');
    expect(document.cookie).not.toContain('refresh_token=');
    expect(document.cookie).not.toContain('px_current_tenant_uuid=');
    expect(readAuthSession()).toEqual(session);
  });

  it('removes stale cookies when restoring a signed-out session', () => {
    startSession();
    clearAuthSession();
    const session = readAuthSession();
    document.cookie = 'access_token=old-cookie; path=/';
    document.cookie = 'refresh_token=old-refresh; path=/';
    const auth = useAuth();
    auth.initAuth();

    expect(auth.isAuthenticated.value).toBe(false);
    expect(document.cookie).not.toContain('access_token=');
    expect(document.cookie).not.toContain('refresh_token=');
    expect(readAuthSession()).toEqual(session);
  });

  it('synchronizes another host tab and ignores the snapshot carried by a delayed storage event', () => {
    const previous = startSession();
    const auth = useAuth();
    auth.initAuth();
    const current = {
      ...previous,
      sessionId: 'another-host-login',
      accessToken: `header.${btoa(JSON.stringify({ tid: 'other-tenant', exp: Math.floor(Date.now() / 1000) + 3600 }))}.signature`,
      refreshToken: 'another-host-refresh',
      tenantUuid: 'other-tenant',
    };
    localStorage.setItem(AUTH_SESSION_KEY, JSON.stringify(current));
    window.dispatchEvent(new StorageEvent('storage', {
      key: AUTH_SESSION_KEY, oldValue: JSON.stringify(previous), newValue: JSON.stringify(current),
    }));
    expect(auth.getToken()).toBe(current.accessToken);
    expect(document.cookie).toContain('px_current_tenant_uuid=other-tenant');
    expect(useCookie('access_token').value).toBe(current.accessToken);

    window.dispatchEvent(new StorageEvent('storage', { key: AUTH_SESSION_KEY, newValue: JSON.stringify(previous) }));
    expect(useCookie('access_token').value).toBe(current.accessToken);
    expect(useCookie('refresh_token').value).toBe(current.refreshToken);
    expect(readAuthSession()).toEqual(current);
  });

  it('does not restore an incomplete legacy login from cookies or scalar storage', () => {
    localStorage.setItem('access_token', 'legacy-token');
    localStorage.setItem('refresh_token', 'legacy-refresh');
    document.cookie = 'access_token=legacy-cookie; path=/';
    const auth = useAuth();
    auth.initAuth();

    expect(auth.isAuthenticated.value).toBe(false);
    expect(auth.getToken()).toBeNull();
    expect(document.cookie).not.toContain('access_token=');
    expect(readAuthSession()?.status).toBe('signed-out');
  });
});
