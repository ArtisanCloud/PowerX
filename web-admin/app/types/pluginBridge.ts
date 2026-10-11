export interface PluginBridgeEnvelope {
  protocolVersion: 2;
  pluginId: string;
  instanceId: string;
}

export type HostBridgeMessage = PluginBridgeEnvelope & { source: 'powerx' } & (
  | { type: 'sync'; locale: string; theme: string; hostOrigin: string }
  | { type: 'auth-token'; sequence: number; sessionId: string; revision: number;
      accessToken: string; tokenType: string; expiresAt: number; expiresIn: number;
      scope: string; tenantUuid: string | null }
  | { type: 'auth-cleared'; sequence: number; sessionId?: string; revision?: number }
  | { type: 'navigate'; requestId: string; path: string }
);

export type PluginBridgeMessage = PluginBridgeEnvelope & { source: 'plugin' | 'powerx-plugin' } & (
  | { type: 'bridge:hello' }
  | { type: 'bridge:ready'; navigationReady: boolean }
  | { type: 'request-sync' | 'auth-token:request' }
  | { type: 'auth:applied'; sequence: number; sessionId: string; revision: number }
  | { type: 'auth:rejected'; sequence: number }
  | { type: 'navigation:result'; requestId: string; ok: boolean }
);

type Body<T> = T extends unknown ? Omit<T, keyof PluginBridgeEnvelope | 'source'> : never;
export type HostBridgeBody = Body<HostBridgeMessage>;

export interface PluginNavigationRequest {
  signal: AbortSignal;
  requestId: string;
  instanceId: string;
}

/** Implemented by Core's injected consumer in the plugin frame, not by the host window. */
export interface PowerXFrameBridgeClient {
  readonly protocolVersion: 2;
  onAuthToken(handler: (message: Extract<HostBridgeMessage, {type: 'auth-token' | 'auth-cleared'}>) => void | Promise<void>): () => void;
  setNavigator(handler: (path: string, request: PluginNavigationRequest) => void | Promise<void>): () => void;
  requestSync(): void;
  requestAuth(): void;
  getLastSyncAt(): number;
}
