import { beforeEach, describe, expect, it } from 'vitest';
import { __resetCookies, useCookie } from '../../support/nuxt-app-shim';
import { clearAuthCookies, syncAuthCookies } from '~/utils/auth-cookie';

describe('auth cookie synchronization', () => {
  beforeEach(() => {
    (process as any).client = true;
    __resetCookies();
    document.cookie = 'access_token=old; Max-Age=0; path=/';
    document.cookie = 'refresh_token=old; Max-Age=0; path=/';
  });
  it('updates document cookies synchronously before Nuxt watchers run', () => {
    document.cookie = 'access_token=old; path=/';
    syncAuthCookies('new-token', 'new-refresh');
    expect(document.cookie).toContain('access_token=new-token');
    expect(document.cookie).toContain('refresh_token=new-refresh');
    expect(useCookie('access_token').value).toBe('new-token');
  });
  it('clears both cookie stores on logout', () => {
    syncAuthCookies('new-token', 'new-refresh');
    clearAuthCookies();
    expect(document.cookie).not.toContain('access_token=');
    expect(document.cookie).not.toContain('refresh_token=');
    expect(useCookie('access_token').value).toBeNull();
  });
});
