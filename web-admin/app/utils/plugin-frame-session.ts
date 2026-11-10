import { captureAuthSession, readAuthSession, syncAuthSessionCookies, type AuthSessionStamp } from './auth-session';

/** A document rejection may repair its cookies once; only local expiry renews the host session. */
export async function recoverPluginFrameSession(renewHostSession: () => Promise<unknown>, verifyHostSession?: () => Promise<unknown>): Promise<AuthSessionStamp | null> {
  const before = readAuthSession();
  if (before?.status !== 'active') return null;
  if (before.expiresAt <= Date.now()) await renewHostSession();
  if (verifyHostSession) await verifyHostSession();
  const current = readAuthSession();
  if (current?.status !== 'active' || current.sessionId !== before.sessionId || current.expiresAt <= Date.now()) return null;
  syncAuthSessionCookies();
  return captureAuthSession();
}
