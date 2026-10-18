import { computed, onScopeDispose, ref, watch } from "vue";
import { useAuth } from "~/composables/useAuth";
import { AUTH_SESSION_EVENT, AUTH_SESSION_KEY, captureAuthSession, readAuthSession, AuthSessionChangedError } from "~/utils/auth-session";
import { WS_BUS_CMD, WS_BUS_TYPE, type WSBusCommand, type WSBusEnvelope } from "~/composables/wsBus";

type TopicHandler = (payload: any, envelope: WSBusEnvelope) => void;

const wsConnected = ref(false);
const wsConnecting = ref(false);
const wsError = ref<string | null>(null);

let wsInstance: WebSocket | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let activeTenant: string | null = null;
let connectionIdentity: string | null = null;
const subscriptions = new Map<string, Set<TopicHandler>>();
let reconnectAttempts = 0;
let allowReconnect = true;

const RECONNECT_DELAY = 3000;
const MAX_RECONNECT_ATTEMPTS = 6;
let notifyReconnectFailed: (() => void) | null = null;
const UUID_RE =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

const hasActiveSubscriptions = () => subscriptions.size > 0;
const isValidTenantUUID = (tenantUUID?: string | null) =>
  UUID_RE.test(String(tenantUUID || "").trim());
const shouldBlockReconnectForCloseCode = (code: number) => code === 1008 || code === 1003;

const refreshConnectionToken = async (auth: ReturnType<typeof useAuth>): Promise<string | null> => {
  if (!process.client) return null;
  try { return await auth.refreshAuth(); }
  catch (error) { if (error instanceof AuthSessionChangedError) throw error; return null; }
};

const isLoopbackHost = (host?: string | null) => {
  const h = String(host || "").trim().toLowerCase();
  return h === "127.0.0.1" || h === "localhost" || h === "::1";
};

const buildWSUrl = (token: string, tenantUUID?: string | null) => {
  const auth = encodeURIComponent(`Bearer ${token}`);
  const cfg = useRuntimeConfig();
  const origin = String((cfg.public as any)?.wsOrigin || "").trim();
  const wsPath = String((cfg.public as any)?.wsPath || "/api/ws").trim() || "/api/ws";
  const tenant = encodeURIComponent(String(tenantUUID || ""));
  const tenantQuery = tenant ? `tenant_uuid=${tenant}` : "";
  const appendAuth = (base: string) => {
    const sep = base.includes("?") ? "&" : "?";
    const q = [`authorization=${auth}`, tenantQuery].filter(Boolean).join("&");
    return `${base}${sep}${q}`;
  };

  // 优先支持显式 wsUrl（可配绝对地址或相对路径）
  if (wsPath.startsWith("ws://") || wsPath.startsWith("wss://")) {
    return appendAuth(wsPath);
  }
  if (wsPath.startsWith("/")) {
    // 开发态优先直连 wsOrigin，避免 dev server 代理 WS 时出现“握手成功但无业务帧”。
    if (import.meta.dev && (origin.startsWith("ws://") || origin.startsWith("wss://"))) {
      try {
        const u = new URL(origin);
        if (isLoopbackHost(u.hostname) && !isLoopbackHost(location.hostname)) {
          const protocol = location.protocol === "https:" ? "wss:" : "ws:";
          return appendAuth(`${protocol}//${location.host}${wsPath}`);
        }
        const base = origin.replace(/\/+$/, "");
        return appendAuth(`${base}${wsPath}`);
      } catch {
        // ignore: fallback below
      }
    }
    const protocol = location.protocol === "https:" ? "wss:" : "ws:";
    return appendAuth(`${protocol}//${location.host}${wsPath}`);
  }

  if (origin.startsWith("ws://") || origin.startsWith("wss://")) {
    // 页面在公网域名访问时，禁止使用 loopback 上游（浏览器会连到访问者自己的本机）。
    try {
      const u = new URL(origin);
      if (isLoopbackHost(u.hostname) && !isLoopbackHost(location.hostname)) {
        const protocol = location.protocol === "https:" ? "wss:" : "ws:";
        return appendAuth(`${protocol}//${location.host}/api/ws`);
      }
    } catch {
      // ignore
    }
    let base = origin.replace(/\/+$/, "");
    if (base.endsWith("/api/ws")) return appendAuth(base);
    if (base.endsWith("/ws")) {
      base = `${base.slice(0, -3)}/api/ws`;
      return appendAuth(base);
    }
    if (base.endsWith("/api")) return appendAuth(`${base}/ws`);
    return appendAuth(`${base}/api/ws`);
  }
  const protocol = location.protocol === "https:" ? "wss:" : "ws:";
  return appendAuth(`${protocol}//${location.host}/api/ws`);
};

const normalizePayload = (payload: unknown) => {
  if (typeof payload !== "string") return payload;
  const trimmed = payload.trim();
  if (!trimmed) return payload;
  const looksJSON =
    (trimmed.startsWith("{") && trimmed.endsWith("}")) ||
    (trimmed.startsWith("[") && trimmed.endsWith("]"));
  if (!looksJSON) return payload;
  try {
    return JSON.parse(trimmed);
  } catch {
    return payload;
  }
};

const resetConnection = (reason?: string, keepReconnect = true) => {
  if (reason) wsError.value = reason;
  if (reconnectTimer) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
  if (!keepReconnect) {
    allowReconnect = false;
  }
  const previous = wsInstance;
  wsInstance = null;
  connectionIdentity = null;
  if (previous) {
    try {
      previous.close();
    } catch {
      // ignore
    }
  }
  wsConnected.value = false;
  wsConnecting.value = false;
};

const scheduleReconnect = (token: string | null, refreshTokenOnHandshakeFailure?: () => Promise<string | null>) => {
  if (!process.client) return;
  if (!allowReconnect) return;
  if (!token) return;
  if (!hasActiveSubscriptions()) return;
  if (reconnectTimer) return;
  if (reconnectAttempts >= MAX_RECONNECT_ATTEMPTS) {
    allowReconnect = false;
    wsError.value = "reconnect_exhausted";
    if (notifyReconnectFailed && hasActiveSubscriptions()) {
      notifyReconnectFailed();
    }
    return;
  }
  const delay = RECONNECT_DELAY;
  const sessionId = captureAuthSession()?.sessionId;
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null;
    if (captureAuthSession()?.sessionId !== sessionId) return;
    reconnectAttempts += 1;
    ensureConnection(readAuthSession()?.accessToken || token, activeTenant, refreshTokenOnHandshakeFailure);
  }, delay);
};

const ensureConnection = (
  token: string | null,
  tenantUUID?: string | null,
  refreshTokenOnHandshakeFailure?: () => Promise<string | null>
) => {
  if (!process.client) return;
  if (!token) {
    wsConnecting.value = false;
    wsConnected.value = false;
    wsError.value = "token_missing";
    return;
  }
  const normalizedTenant = String(tenantUUID || "").trim();
  // WS Bus 当前依赖 tenant_uuid；无租户上下文时不建立连接，避免无意义重试噪音。
  if (!isValidTenantUUID(normalizedTenant)) {
    wsConnecting.value = false;
    wsConnected.value = false;
    wsError.value = normalizedTenant ? "tenant_invalid" : "tenant_missing";
    return;
  }
  if (!hasActiveSubscriptions()) return;
  const identity = `${captureAuthSession()?.sessionId || ""}:${normalizedTenant}:${token}`;
  if (wsInstance && connectionIdentity === identity && (wsConnected.value || wsConnecting.value)) return;
  if (wsInstance) resetConnection();
  connectionIdentity = identity;
  wsConnecting.value = true;
  wsError.value = null;

  const url = buildWSUrl(token, normalizedTenant);
  const ws = new WebSocket(url);
  wsInstance = ws;
  const connectionSessionId = captureAuthSession()?.sessionId;
  let opened = false;

  ws.onopen = () => {
    if (wsInstance !== ws) return;
    opened = true;
    wsConnected.value = true;
    wsConnecting.value = false;
    wsError.value = null;
    reconnectAttempts = 0;
    subscriptions.forEach((_handlers, topic) => {
      sendCommand({ type: WS_BUS_CMD.SUBSCRIBE, topic });
    });
  };
  ws.onclose = (event) => {
    if (wsInstance !== ws) return;
    wsConnected.value = false;
    wsConnecting.value = false;
    wsInstance = null;
    if (shouldBlockReconnectForCloseCode(event.code)) {
      allowReconnect = false;
      wsError.value = "tenant_invalid";
      return;
    }

    if (!opened && refreshTokenOnHandshakeFailure) {
      void refreshTokenOnHandshakeFailure().then((refreshedToken) => {
        if (!allowReconnect || !hasActiveSubscriptions() || captureAuthSession()?.sessionId !== connectionSessionId) return;
        if (refreshedToken) {
          ensureConnection(refreshedToken, readAuthSession()?.tenantUuid || null, refreshTokenOnHandshakeFailure);
          return;
        }
        wsError.value = "authentication_refresh_failed";
        scheduleReconnect(token, refreshTokenOnHandshakeFailure);
      }).catch(error => { if (!(error instanceof AuthSessionChangedError)) wsError.value = "authentication_refresh_failed"; });
      return;
    }

    wsError.value = "connection_closed";
    scheduleReconnect(token, refreshTokenOnHandshakeFailure);
  };
  ws.onerror = () => {
    if (wsInstance !== ws) return;
    wsConnected.value = false;
    wsConnecting.value = false;
    wsError.value = "connection_failed";
  };
  ws.onmessage = (evt) => {
    if (wsInstance !== ws || activeTenant !== normalizedTenant || captureAuthSession()?.sessionId !== connectionSessionId) return;
    try {
      const env = JSON.parse(String(evt.data || "")) as WSBusEnvelope;
      if (!env || !env.type) return;
      if (env.type === WS_BUS_TYPE.ERROR) {
        const payload = normalizePayload(env.payload) as any;
        wsError.value = String(payload?.message || "订阅失败");
        return;
      }
      if (env.type === WS_BUS_TYPE.EVENT && env.topic && subscriptions.has(env.topic)) {
        const handlers = Array.from(subscriptions.get(env.topic) || []);
        const payload = normalizePayload(env.payload);
        handlers.forEach((handler) => handler(payload, env));
      }
    } catch {
      // ignore
    }
  };
};

const sendCommand = (cmd: WSBusCommand) => {
  if (!wsInstance || wsInstance.readyState !== WebSocket.OPEN) return;
  wsInstance.send(JSON.stringify(cmd));
};

export const useWSBus = () => {
  const toast = useToast();
  const { t } = useI18n();
  const auth = useAuth();
  const session = ref(readAuthSession());
  const synchronizeSession = () => { session.value = readAuthSession(); };
  const synchronizeStorage = (event: StorageEvent) => {
    if (event.key === AUTH_SESSION_KEY || event.key === null) synchronizeSession();
  };
  const token = computed(() => {
    // auth.token is the reactive invalidation signal; the snapshot is authoritative.
    void auth.token.value;
    void session.value;
    return auth.getToken();
  });
  const getTenantForConnection = () => { void auth.token.value; void session.value; return readAuthSession()?.tenantUuid || null; };
  const reconnectWithFreshToken = () => refreshConnectionToken(auth);

  // The socket is shared, but each consumer owns its reactive watches/listeners.
  // A component unmount must not leave a global "initialized" latch behind.
  if (process.client) {
    window.addEventListener(AUTH_SESSION_EVENT, synchronizeSession);
    window.addEventListener("storage", synchronizeStorage);
    watch(
      () => [token.value, getTenantForConnection(), session.value?.sessionId] as const,
      ([nextToken, nextTenant]) => {
        activeTenant = nextTenant;
        if (!nextToken || !isValidTenantUUID(nextTenant)) {
          resetConnection(!nextToken ? "token_missing" : "tenant_missing", false);
          return;
        }
        allowReconnect = true;
        ensureConnection(nextToken, nextTenant, reconnectWithFreshToken);
      },
      { immediate: true, flush: "sync" }
    );
  }

  const connect = (tenantOverride?: string | null) => {
    if (!hasActiveSubscriptions()) return;
    reconnectAttempts = 0;
    allowReconnect = true;
    const tenantNow = String(tenantOverride || "").trim() || getTenantForConnection();
    activeTenant = tenantNow || null;
    const persistedRefreshToken = readAuthSession()?.refreshToken || "";
    const connectionSession = captureAuthSession();
    const currentToken = token.value || null;
    if (!currentToken && persistedRefreshToken) {
      void refreshConnectionToken(auth).then((refreshedToken) => {
        if (captureAuthSession()?.sessionId !== connectionSession?.sessionId) return;
        if (!refreshedToken) {
          wsError.value = "authentication_refresh_failed";
          return;
        }
        ensureConnection(refreshedToken, tenantNow, reconnectWithFreshToken);
      }).catch(error => { if (!(error instanceof AuthSessionChangedError)) wsError.value = "authentication_refresh_failed"; });
      return;
    }
    ensureConnection(currentToken, tenantNow, reconnectWithFreshToken);
  };

  if (!notifyReconnectFailed) {
    notifyReconnectFailed = () => {
      toast.add({
        title: t("workflow.runtimeError.reconnectFailedTitle"),
        description: t("workflow.runtimeError.reconnectExhausted"),
        color: "error",
      });
    };
  }

  const subscribe = (topic: string, handler: TopicHandler, reqId?: string) => {
    if (!process.client || !topic) return () => {};
    if (!subscriptions.has(topic)) {
      subscriptions.set(topic, new Set());
    }
    subscriptions.get(topic)!.add(handler);
    connect(activeTenant);
    sendCommand({ type: WS_BUS_CMD.SUBSCRIBE, topic, req_id: reqId });
    const release = () => {
      if (!ownedSubscriptions.delete(release)) return;
      unsubscribe(topic, handler, reqId);
    };
    ownedSubscriptions.add(release);
    return release;
  };

  const unsubscribe = (topic: string, handler?: TopicHandler, reqId?: string) => {
    const set = subscriptions.get(topic);
    let shouldSend = false;
    if (set && handler) {
      set.delete(handler);
      if (set.size === 0) {
        subscriptions.delete(topic);
        shouldSend = true;
      }
    } else if (set && !handler) {
      subscriptions.delete(topic);
      shouldSend = true;
    }
    if (shouldSend) {
      sendCommand({ type: WS_BUS_CMD.UNSUBSCRIBE, topic, req_id: reqId });
    }
    if (!hasActiveSubscriptions()) resetConnection("no_subscriptions", false);
  };

  const disconnect = () => {
    resetConnection("manual_disconnect", false);
  };

  const ownedSubscriptions = new Set<() => void>();
  const onOnline = () => connect();
  const onOffline = () => resetConnection("network_offline", false);
  if (process.client) {
    window.addEventListener("online", onOnline);
    window.addEventListener("offline", onOffline);
  }
  onScopeDispose(() => {
    ownedSubscriptions.forEach(release => release());
    if (!process.client) return;
    window.removeEventListener(AUTH_SESSION_EVENT, synchronizeSession);
    window.removeEventListener("storage", synchronizeStorage);
    window.removeEventListener("online", onOnline);
    window.removeEventListener("offline", onOffline);
  });

  return {
    connected: wsConnected,
    connecting: wsConnecting,
    lastError: wsError,
    activeTenant: computed(() => activeTenant),
    connect,
    subscribe,
    unsubscribe,
    disconnect,
  };
};
