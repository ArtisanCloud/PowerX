import { AuthSessionChangedError, captureAuthSession, clearAuthSession, isAuthSessionCurrent, readAuthSession, type AuthSession, type AuthSessionStamp } from './auth-session';
import { authFailureCode, authFailureStatus, isTerminalAuthFailure } from './auth-failure';

export type HostSessionRecovery = (expected: AuthSessionStamp, signal?: AbortSignal) => Promise<'active' | 'signed-out'>;
type Dependencies = {
  refresh: () => Promise<unknown>;
  probe: (session: AuthSession, signal: AbortSignal) => Promise<unknown>;
  login: (signedOut: AuthSessionStamp) => Promise<unknown> | unknown;
};

/** 插件消息只是核验提示；只有宿主 API 确认终止才清理当前代际并进入登录。 */
export function createHostSessionRecovery(deps: Dependencies): HostSessionRecovery {
  let probe: { key: string; promise: Promise<unknown>; abort: AbortController; consumers: number } | null = null;
  let loginKey = '';
  return async (expected, signal) => {
    let stamp = expected;
    const current = () => {
      if (signal?.aborted || !isAuthSessionCurrent(stamp)) throw new AuthSessionChangedError();
      const snapshot = readAuthSession();
      if (snapshot?.status !== 'active') throw new AuthSessionChangedError();
      return snapshot;
    };
    let snapshot = current();
    try {
      let renewed = false;
      const renew = async () => {
        await deps.refresh();
        if (signal?.aborted || readAuthSession()?.sessionId !== expected.sessionId) throw new AuthSessionChangedError();
        stamp = captureAuthSession()!;
        snapshot = current(); renewed = true;
      };
      const verify = async () => {
        const key = `${stamp.sessionId}:${stamp.revision}`;
        if (!probe || probe.key !== key) {
          const abort = new AbortController();
          probe = { key, abort, consumers: 0, promise: Promise.resolve().then(() => deps.probe(snapshot, abort.signal)) };
        }
        const flight = probe;
        flight.consumers += 1;
        let released = false;
        const release = () => {
          if (released) return;
          released = true; flight.consumers -= 1;
          if (!flight.consumers) { flight.abort.abort(); if (probe === flight) probe = null; }
        };
        signal?.addEventListener('abort', release, { once: true });
        if (signal?.aborted) release();
        try { await flight.promise; }
        finally { signal?.removeEventListener('abort', release); release(); }
      };
      if (snapshot.expiresAt <= Date.now()) await renew();
      try { await verify(); }
      catch (error) {
        // 服务器明确确认 access 过期时兼容本机时钟偏差；会话终止绝不走续期。
        current();
        if (renewed || authFailureStatus(error) !== 401 || authFailureCode(error) !== 'AUTH_ACCESS_EXPIRED') throw error;
        await renew(); await verify();
      }
      current();
      return 'active';
    } catch (error: any) {
      if (signal?.aborted || error instanceof AuthSessionChangedError) throw new AuthSessionChangedError();
      if (!isTerminalAuthFailure(error)) { current(); throw error; }
      const after = readAuthSession();
      // refreshAuthSession 已在权威终止响应后清理；只接受它记录的 signed-out 代际。
      if (after?.status === 'signed-out' && error.authSessionStamp && isAuthSessionCurrent(error.authSessionStamp)) stamp = error.authSessionStamp;
      else { current(); if (!clearAuthSession(stamp)) throw new AuthSessionChangedError(); stamp = captureAuthSession()!; }
      if (!isAuthSessionCurrent(stamp) || readAuthSession()?.status !== 'signed-out') throw new AuthSessionChangedError();
      const key = `${stamp.sessionId}:${stamp.revision}`;
      if (loginKey !== key) { loginKey = key; await deps.login(stamp); }
      return 'signed-out';
    }
  };
}
