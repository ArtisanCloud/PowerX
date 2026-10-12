import { AUTH_SESSION_EVENT, AUTH_SESSION_KEY, readAuthSession } from './auth-session';
import type { HostBridgeBody } from '../types/pluginBridge';

export const PLUGIN_BRIDGE_VERSION = 2;
export type FrameStatus = { state: 'loading' | 'ready' | 'error'; code?: string };
type FrameBinding = {
  frame: HTMLIFrameElement; pluginId: string; instanceId: string; origin: string;
  onStatus: (status: FrameStatus) => void; ready: boolean; navigationReady: boolean; sequence: number;
  fingerprint: string; sessionId?: string; authSequence: number; authApplied: boolean; desiredPath?: string; navigationId?: string; navigationPath?: string; completedPath?: string;
  timer?: ReturnType<typeof setTimeout>; onLoad: () => void;
};

/** Host-owned lifecycle. Message payloads cannot select a different window or origin. */
export class PluginBridgeHost {
  private frames = new Map<HTMLIFrameElement, FrameBinding>();
  constructor(private metadata: () => { locale: string; theme: string }, private timeoutMs = 8000, private refreshSession?: () => Promise<unknown>) {
    window.addEventListener('message', this.receive);
    window.addEventListener(AUTH_SESSION_EVENT, this.sessionChanged);
    window.addEventListener('storage', this.storageChanged);
  }
  private storageChanged = (event: StorageEvent) => { if (event.key === AUTH_SESSION_KEY) this.sessionChanged(); };
  private sessionChanged = () => { this.frames.forEach(binding => this.sendAuth(binding, true)); };
  register(frame: HTMLIFrameElement, pluginId: string, instanceId: string, onStatus: FrameBinding['onStatus']) {
    this.unregister(frame);
    let url: URL;
    try { url = new URL(frame.getAttribute('src') || '', window.location.href); }
    catch { onStatus({ state:'error', code:'PLUGIN_BRIDGE_ORIGIN_UNSUPPORTED' }); return; }
    if (!['http:', 'https:'].includes(url.protocol) || frame.hasAttribute('sandbox') && !String(frame.getAttribute('sandbox')).split(/\s+/).includes('allow-same-origin')) {
      onStatus({ state: 'error', code: 'PLUGIN_BRIDGE_ORIGIN_UNSUPPORTED' }); return;
    }
    const binding: FrameBinding = { frame, pluginId, instanceId, origin: url.origin, onStatus,
      ready: false, navigationReady: false, sequence: 0, fingerprint: '', authSequence: 0, authApplied: false, onLoad: () => {} };
    binding.onLoad = () => {
      binding.ready = false; binding.navigationReady = false; binding.authApplied = false; binding.fingerprint = '';
      binding.navigationId = undefined; binding.navigationPath = undefined; binding.completedPath = undefined;
      this.wait(binding, 'PLUGIN_BRIDGE_TIMEOUT'); this.sync(binding);
    };
    this.frames.set(frame, binding);
    frame.addEventListener('load', binding.onLoad);
    this.wait(binding, 'PLUGIN_BRIDGE_TIMEOUT'); this.sync(binding);
  }
  private post(binding: FrameBinding, payload: HostBridgeBody) {
    binding.frame.contentWindow?.postMessage({ ...payload, source: 'powerx', protocolVersion: PLUGIN_BRIDGE_VERSION,
      pluginId: binding.pluginId, instanceId: binding.instanceId }, binding.origin);
  }
  private wait(binding: FrameBinding, code: string) {
    clearTimeout(binding.timer); binding.onStatus({ state: 'loading' });
    binding.timer = setTimeout(() => {
      binding.onStatus({ state: 'error', code: binding.ready && (!binding.desiredPath || binding.completedPath === binding.desiredPath) && !binding.authApplied ? 'PLUGIN_AUTH_ADAPTER_TIMEOUT' : code });
      binding.navigationId = undefined; binding.navigationPath = undefined;
    }, this.timeoutMs);
  }
  private sync(binding: FrameBinding) {
    this.post(binding, { type: 'sync', ...this.metadata(), hostOrigin: window.location.origin });
  }
  private sendAuth(binding: FrameBinding, force = false) {
    if (!binding.ready) return;
    const session = readAuthSession();
    const fingerprint = session ? `${session.sessionId}:${session.revision}:${session.status}` : 'none';
    if (!force && binding.fingerprint === fingerprint) return;
    const generationChanged = binding.sessionId !== session?.sessionId;
    binding.sessionId = session?.sessionId;
    binding.fingerprint = fingerprint; binding.sequence += 1;
    binding.authSequence = binding.sequence; binding.authApplied = false;
    if (!session || session.status !== 'active' || session.expiresAt <= Date.now()) {
      this.post(binding, { type: 'auth-cleared', sequence: binding.sequence,
        sessionId: session?.sessionId, revision: session?.revision });
      clearTimeout(binding.timer); binding.navigationId = undefined;
      binding.onStatus({ state: 'error', code: 'PLUGIN_AUTH_REQUIRED' }); return;
    }
    if (generationChanged) binding.onStatus({ state:'loading' });
    clearTimeout(binding.timer);
    binding.timer = setTimeout(() => {
      if (!binding.authApplied) binding.onStatus({ state:'error', code:'PLUGIN_AUTH_ADAPTER_TIMEOUT' });
    }, this.timeoutMs);
    this.post(binding, { type: 'auth-token', sequence: binding.sequence,
      sessionId: session.sessionId, revision: session.revision,
      accessToken: session.accessToken, tokenType: session.tokenType,
      expiresAt: session.expiresAt, expiresIn: Math.max(1, Math.floor((session.expiresAt - Date.now()) / 1000)),
      scope: session.scope, tenantUuid: session.tenantUuid });
  }
  private async refreshAndSend(binding: FrameBinding) {
    const current = readAuthSession();
    if (current?.status === 'active' && current.expiresAt <= Date.now() && this.refreshSession) {
      try { await this.refreshSession(); }
      catch (error: any) {
        if (this.frames.get(binding.frame) !== binding) return;
        if (error?.code !== 'AUTH_SESSION_CHANGED' && readAuthSession()?.status === 'active') {
          binding.onStatus({state:'error',code:'PLUGIN_HOST_AUTH_UNAVAILABLE'}); return;
        }
      }
    }
    if (this.frames.get(binding.frame) === binding) this.sendAuth(binding, true);
  }
  navigate(frame: HTMLIFrameElement, path: string) {
    const binding = this.frames.get(frame);
    if (!binding) return;
    binding.desiredPath = path;
    if (!binding.ready || binding.navigationId && binding.navigationPath === path) return;
    if (!binding.navigationId && binding.completedPath === path) { this.settle(binding); return; }
    this.sendNavigation(binding);
  }
  private sendNavigation(binding: FrameBinding) {
    if (!binding.desiredPath) { this.settle(binding); return; }
    if (binding.navigationId && binding.navigationPath === binding.desiredPath) return;
    // 新导航开始后，旧成功路径不再能证明 iframe 的当前位置。
    binding.completedPath = undefined;
    binding.navigationPath = binding.desiredPath;
    binding.navigationId = crypto.randomUUID();
    this.wait(binding, 'PLUGIN_NAVIGATION_TIMEOUT');
    this.post(binding, { type: 'navigate', path: binding.desiredPath, requestId: binding.navigationId });
  }
  private receive = (event: MessageEvent) => {
    const data = event.data;
    if (!data || !['plugin', 'powerx-plugin'].includes(data.source)) return;
    const binding = Array.from(this.frames.values()).find(item => item.frame.contentWindow === event.source);
    if (!binding || event.origin !== binding.origin || data.pluginId !== binding.pluginId) return;
    if (data.protocolVersion !== PLUGIN_BRIDGE_VERSION) return;
    if (data.type === 'bridge:hello') { this.sync(binding); return; }
    if (data.instanceId !== binding.instanceId) return;
    if (data.type === 'bridge:ready') {
      const first = !binding.ready;
      const navigatorInstalled = data.navigationReady === true && !binding.navigationReady;
      binding.ready = true; binding.navigationReady = data.navigationReady === true;
      if (first) { this.sendAuth(binding, true); this.sendNavigation(binding); }
      else if (navigatorInstalled) this.sendNavigation(binding);
      return;
    }
    if (data.type === 'auth:applied' && data.sequence === binding.authSequence) {
      const session = readAuthSession();
      if (session?.status !== 'active' || data.sessionId !== session.sessionId || data.revision !== session.revision) return;
      binding.authApplied = true; this.settle(binding); return;
    }
    if (data.type === 'auth:rejected' && data.sequence === binding.authSequence) {
      clearTimeout(binding.timer); binding.onStatus({state:'error',code:'PLUGIN_AUTH_CONTEXT_FAILED'}); return;
    }
    if (data.type === 'auth-token:request') { void this.refreshAndSend(binding); return; }
    if (data.type === 'request-sync') { this.sync(binding); this.sendAuth(binding, true); return; }
    if (data.type === 'navigation:result' && data.requestId === binding.navigationId) {
      const navigationPath = binding.navigationPath;
      binding.navigationId = undefined;
      binding.navigationPath = undefined;
      if (data.ok === true) {
        binding.completedPath = navigationPath; this.settle(binding);
      } else {
        binding.completedPath = undefined;
        const codes = ['PLUGIN_NAVIGATION_UNSUPPORTED', 'PLUGIN_NAVIGATION_FAILED', 'PLUGIN_NAVIGATION_INVALID', 'PLUGIN_NAVIGATION_ROUTE_MISMATCH'];
        const code = codes.includes(data.code) ? data.code : binding.navigationReady ? 'PLUGIN_NAVIGATION_FAILED' : 'PLUGIN_NAVIGATION_UNSUPPORTED';
        clearTimeout(binding.timer); binding.onStatus({ state: 'error', code });
      }
    }
  };
  private settle(binding: FrameBinding) {
    if (binding.authApplied && (!binding.desiredPath || binding.completedPath === binding.desiredPath)) {
      clearTimeout(binding.timer); binding.onStatus({ state: 'ready' });
    }
  }
  broadcastMetadata() { this.frames.forEach(binding => this.sync(binding)); }
  unregister(frame: HTMLIFrameElement) {
    const binding = this.frames.get(frame);
    if (!binding) return;
    clearTimeout(binding.timer); frame.removeEventListener('load', binding.onLoad); this.frames.delete(frame);
  }
  dispose() {
    Array.from(this.frames.keys()).forEach(frame => this.unregister(frame));
    window.removeEventListener('message', this.receive);
    window.removeEventListener(AUTH_SESSION_EVENT, this.sessionChanged);
    window.removeEventListener('storage', this.storageChanged);
  }
}
