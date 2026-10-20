/** Revoke the independent administrator session when the normal user logs out.
 * The CSRF value is transient; no recovery credential is stored in JS storage. */
export async function clearIndependentRecoverySession(): Promise<void> {
  if (typeof window === 'undefined') return;
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 3000);
  try {
    const response = await fetch('/__powerx_recovery/session', { credentials: 'same-origin', cache: 'no-store', signal: controller.signal });
    if (!response.ok) return;
    const session = await response.json();
    if (typeof session.csrf !== 'string' || !session.csrf) return;
    const revoked = await fetch('/__powerx_recovery/session', { method: 'DELETE', credentials: 'same-origin', cache: 'no-store', headers: { 'X-PowerX-Recovery-CSRF': session.csrf }, signal: controller.signal });
    if (revoked.ok && typeof BroadcastChannel !== 'undefined') {
      const channel = new BroadcastChannel('powerx-recovery-auth');
      channel.postMessage({ event: 'logout' });
      channel.close();
    }
  } catch { /* A normal unmanaged site has no recovery session endpoint. */ }
  finally { clearTimeout(timeout); }
}
