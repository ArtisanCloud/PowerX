import { decodeJwtPayload } from './auth-token';
import { clearAuthCookies, syncAuthCookies } from './auth-cookie';

export const AUTH_SESSION_KEY = 'powerx.session';
export const AUTH_SESSION_EVENT = 'powerx:auth-session-changed';

export interface AuthSessionStamp { sessionId: string; revision: number }
export interface AuthSession extends AuthSessionStamp {
  version: 1;
  status: 'active' | 'signed-out';
  accessToken: string;
  refreshToken: string;
  tokenType: string;
  expiresAt: number;
  scope: string;
  tenantUuid: string | null;
}
export interface AuthPayload {
  access_token: string;
  refresh_token?: string;
  token_type?: string;
  expires_in?: number;
  scope?: string;
}

export class AuthSessionChangedError extends Error {
  readonly code = 'AUTH_SESSION_CHANGED';
  constructor() { super('AUTH_SESSION_CHANGED'); }
}

const mirrorKeys = ['access_token', 'refresh_token', 'token_type', 'expires_in', 'expires_at', 'scope', 'px_current_tenant_uuid'];

export function readAuthSession(): AuthSession | null {
  if (!process.client) return null;
  try {
    const session = JSON.parse(localStorage.getItem(AUTH_SESSION_KEY) || 'null');
    if (session?.version !== 1 || typeof session.sessionId !== 'string' || !session.sessionId ||
      !Number.isSafeInteger(session.revision) || session.revision < 1 ||
      !['active', 'signed-out'].includes(session.status) ||
      typeof session.accessToken !== 'string' || typeof session.refreshToken !== 'string' ||
      typeof session.tokenType !== 'string' || typeof session.scope !== 'string' ||
      !Number.isFinite(session.expiresAt) ||
      !(session.tenantUuid === null || typeof session.tenantUuid === 'string')) return null;
    if (session.status === 'active' && (!session.accessToken || !session.refreshToken || session.expiresAt <= 0)) return null;
    return session;
  } catch { return null; }
}

export function captureAuthSession(): AuthSessionStamp | null {
  const session = readAuthSession();
  return session ? { sessionId: session.sessionId, revision: session.revision } : null;
}

export function isAuthSessionCurrent(stamp: AuthSessionStamp | null): boolean {
  const current = captureAuthSession();
  return current?.sessionId === stamp?.sessionId && current?.revision === stamp?.revision;
}

function publish(session: AuthSession) {
  // One authoritative write. The scalar keys are consumer mirrors, never a restore source.
  localStorage.setItem(AUTH_SESSION_KEY, JSON.stringify(session));
  if (session.status === 'active') {
    localStorage.setItem('access_token', session.accessToken);
    localStorage.setItem('refresh_token', session.refreshToken);
    localStorage.setItem('token_type', session.tokenType);
    localStorage.setItem('expires_at', String(session.expiresAt));
    localStorage.setItem('expires_in', String(Math.max(0, Math.floor((session.expiresAt - Date.now()) / 1000))));
    localStorage.setItem('scope', session.scope);
    if (session.tenantUuid) localStorage.setItem('px_current_tenant_uuid', session.tenantUuid);
    else localStorage.removeItem('px_current_tenant_uuid');
    syncAuthCookies(session.accessToken, session.refreshToken);
  } else {
    mirrorKeys.forEach(key => localStorage.removeItem(key));
    clearAuthCookies();
  }
  document.cookie = session.tenantUuid
    ? `px_current_tenant_uuid=${encodeURIComponent(session.tenantUuid)}; path=/; SameSite=Lax`
    : 'px_current_tenant_uuid=; Max-Age=0; path=/; SameSite=Lax';
  window.dispatchEvent(new CustomEvent(AUTH_SESSION_EVENT, { detail: captureAuthSession() }));
}

export function commitAuthSession(payload: AuthPayload, expected?: AuthSessionStamp): AuthSession {
  if (!process.client) throw new Error('AUTH_SESSION_CLIENT_REQUIRED');
  if (expected && !isAuthSessionCurrent(expected)) throw new AuthSessionChangedError();
  const current = readAuthSession();
  const accessToken = String(payload.access_token || '').trim();
  // An omitted refresh token can only be preserved within a checked refresh transaction.
  const refreshToken = typeof payload.refresh_token === 'string'
    ? payload.refresh_token.trim() : expected ? current?.refreshToken || '' : '';
  const claims = decodeJwtPayload(accessToken);
  const expiresAt = Number(claims?.exp) * 1000;
  if (!accessToken || !refreshToken || !Number.isFinite(expiresAt) || expiresAt <= Date.now()) {
    throw new Error('AUTH_SESSION_PAYLOAD_INVALID');
  }
  const session: AuthSession = {
    version: 1, status: 'active',
    sessionId: expected?.sessionId || crypto.randomUUID(),
    revision: expected ? expected.revision + 1 : 1,
    accessToken, refreshToken, tokenType: payload.token_type || 'Bearer',
    expiresAt, scope: payload.scope || 'access',
    tenantUuid: typeof claims?.tid === 'string' ? claims.tid : null,
  };
  publish(session);
  return session;
}

export function clearAuthSession(expected?: AuthSessionStamp): boolean {
  if (!process.client) return false;
  if (expected && !isAuthSessionCurrent(expected)) return false;
  publish({ version: 1, status: 'signed-out', sessionId: crypto.randomUUID(), revision: 1,
    accessToken: '', refreshToken: '', tokenType: 'Bearer', expiresAt: 0, scope: '', tenantUuid: null });
  return true;
}

let inflight: { stamp: AuthSessionStamp; promise: Promise<string | null> } | null = null;

export async function refreshAuthSession(
  request: (refreshToken: string) => Promise<AuthPayload>,
): Promise<string | null> {
  if (readAuthSession()?.status !== 'active') return null;
  const run = async () => {
    const session = readAuthSession();
    if (!session || session.status !== 'active') return null;
    const stamp = captureAuthSession()!;
    if (inflight && inflight.stamp.sessionId === stamp.sessionId && inflight.stamp.revision === stamp.revision) return inflight.promise;
    const promise = Promise.resolve().then(async () => {
      try {
        const payload = await request(session.refreshToken);
        if (!isAuthSessionCurrent(stamp)) throw new AuthSessionChangedError();
        return commitAuthSession(payload, stamp).accessToken;
      } catch (error: any) {
        if (!isAuthSessionCurrent(stamp)) throw new AuthSessionChangedError();
        const status = Number(error?.status || error?.statusCode || error?.response?.status || 0);
        if (status === 401) clearAuthSession(stamp);
        if (error && typeof error === 'object') error.authSessionStamp = captureAuthSession();
        throw error;
      } finally {
        if (inflight?.promise === promise) inflight = null;
      }
    });
    inflight = { stamp, promise };
    return promise;
  };
  // Serialize refresh across same-origin host tabs as well as callers in this document.
  if (typeof navigator !== 'undefined' && navigator.locks) {
    const before = captureAuthSession();
    return navigator.locks.request('powerx-auth-refresh', async () => {
      const current = readAuthSession();
      if (current?.sessionId !== before?.sessionId) throw new AuthSessionChangedError();
      if (current && current.revision !== before?.revision) return current.accessToken;
      return run();
    });
  }
  throw new Error('AUTH_SESSION_LOCK_UNAVAILABLE');
}
