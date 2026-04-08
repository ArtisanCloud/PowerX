import { computed, ref, watch } from "vue";
import { useAuth } from "~/composables/useAuth";
import { useMe } from "~/composables/useMe";
import { resolveTenantUUIDForRequest } from "~/utils/tenant-context";
import { WS_BUS_CMD, WS_BUS_TYPE, type WSBusCommand, type WSBusEnvelope } from "~/composables/wsBus";

type TopicHandler = (payload: any, envelope: WSBusEnvelope) => void;

const wsConnected = ref(false);
const wsConnecting = ref(false);
const wsError = ref<string | null>(null);

let wsInstance: WebSocket | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let activeTenant: string | null = null;
const subscriptions = new Map<string, Set<TopicHandler>>();
let reconnectAttempts = 0;
let allowReconnect = true;
let watchersInitialized = false;
let networkListenersBound = false;

const RECONNECT_DELAY = 3000;
const MAX_RECONNECT_ATTEMPTS = 6;
let notifyReconnectFailed: (() => void) | null = null;
const UUID_RE =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

const hasActiveSubscriptions = () => subscriptions.size > 0;
const isValidTenantUUID = (tenantUUID?: string | null) =>
  UUID_RE.test(String(tenantUUID || "").trim());

const isLoopbackHost = (host?: string | null) => {
  const h = String(host || "").trim().toLowerCase();
  return h === "127.0.0.1" || h === "localhost" || h === "::1";
};

const buildWSUrl = (token: string, tenantUUID?: string | null) => {
  const auth = encodeURIComponent(`Bearer ${token}`);
  const cfg = useRuntimeConfig();
  const upstream = String(cfg.public?.wsUpstream || "").trim();
  const wsPath = String(cfg.public?.wsUrl || "/api/ws").trim() || "/api/ws";
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
    const protocol = location.protocol === "https:" ? "wss:" : "ws:";
    return appendAuth(`${protocol}//${location.host}${wsPath}`);
  }

  if (upstream.startsWith("ws://") || upstream.startsWith("wss://")) {
    // 页面在公网域名访问时，禁止使用 loopback 上游（浏览器会连到访问者自己的本机）。
    try {
      const u = new URL(upstream);
      if (isLoopbackHost(u.hostname) && !isLoopbackHost(location.hostname)) {
        const protocol = location.protocol === "https:" ? "wss:" : "ws:";
        return appendAuth(`${protocol}//${location.host}/api/ws`);
      }
    } catch {
      // ignore
    }
    let base = upstream.replace(/\/+$/, "");
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
  if (wsInstance) {
    try {
      wsInstance.close();
    } catch {
      // ignore
    }
  }
  wsInstance = null;
  wsConnected.value = false;
  wsConnecting.value = false;
};

const scheduleReconnect = (token: string | null) => {
  if (!process.client) return;
  if (!allowReconnect) return;
  if (!token) return;
  if (!hasActiveSubscriptions()) return;
  if (reconnectTimer) return;
  if (reconnectAttempts >= MAX_RECONNECT_ATTEMPTS) {
    allowReconnect = false;
    wsError.value = "连接失败，请检查配置或联系管理员";
    if (notifyReconnectFailed && hasActiveSubscriptions()) {
      notifyReconnectFailed();
    }
    return;
  }
  const delay = RECONNECT_DELAY;
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null;
    reconnectAttempts += 1;
    ensureConnection(token, resolveTenantUUIDForRequest());
  }, delay);
};

const ensureConnection = (token: string | null, tenantUUID?: string | null) => {
  if (!process.client) return;
  if (!token) return;
  const normalizedTenant = String(tenantUUID || "").trim();
  // WS Bus 当前依赖 tenant_uuid；无租户上下文时不建立连接，避免无意义重试噪音。
  if (!isValidTenantUUID(normalizedTenant)) {
    wsConnecting.value = false;
    wsConnected.value = false;
    wsError.value = null;
    allowReconnect = false;
    return;
  }
  if (wsInstance && wsConnected.value) return;
  if (wsConnecting.value) return;
  wsConnecting.value = true;
  wsError.value = null;

  const url = buildWSUrl(token, normalizedTenant);
  const ws = new WebSocket(url);
  wsInstance = ws;

  ws.onopen = () => {
    wsConnected.value = true;
    wsConnecting.value = false;
    wsError.value = null;
    reconnectAttempts = 0;
    subscriptions.forEach((_handlers, topic) => {
      sendCommand({ type: WS_BUS_CMD.SUBSCRIBE, topic });
    });
  };
  ws.onclose = (event) => {
    wsConnected.value = false;
    wsConnecting.value = false;
    wsInstance = null;
    if (event.code === 1008 || event.code === 1003) {
      allowReconnect = false;
      wsError.value = "租户上下文无效";
      return;
    }
    scheduleReconnect(token);
  };
  ws.onerror = () => {
    wsConnected.value = false;
    wsConnecting.value = false;
    wsError.value = "连接失败";
    wsInstance = null;
    scheduleReconnect(token);
  };
  ws.onmessage = (evt) => {
    try {
      const env = JSON.parse(String(evt.data || "")) as WSBusEnvelope;
      if (!env || !env.type) return;
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
  const auth = useAuth();
  const me = useMe();
  const token = computed(() => {
    const fresh = auth.getToken();
    if (fresh && fresh !== auth.token.value) {
      auth.token.value = fresh;
    }
    return fresh || auth.token.value;
  });
  const getTenantForConnection = () => me.currentTenantUuid.value || resolveTenantUUIDForRequest();

  if (process.client && !watchersInitialized) {
    watchersInitialized = true;
    watch(
      () => getTenantForConnection(),
      (nextTenant, prevTenant) => {
        if (!nextTenant) {
          activeTenant = null;
          resetConnection("tenant_missing", false);
          return;
        }
        if (prevTenant && nextTenant !== prevTenant) {
          activeTenant = nextTenant;
          resetConnection("tenant_changed");
          ensureConnection(token.value || null, nextTenant);
        } else {
          activeTenant = nextTenant;
        }
      },
      { immediate: true }
    );
    watch(
      () => token.value,
      (nextToken, prevToken) => {
        if (nextToken === prevToken) return;
        if (!nextToken) {
          resetConnection("token_missing", false);
          return;
        }
        allowReconnect = true;
        resetConnection("token_changed");
        ensureConnection(nextToken, activeTenant);
      }
    );
    if (process.client && !networkListenersBound) {
      networkListenersBound = true;
      window.addEventListener("online", () => {
        allowReconnect = true;
        ensureConnection(token.value || null, getTenantForConnection());
      });
      window.addEventListener("offline", () => {
        wsError.value = "网络断开";
      });
    }
  }

  const connect = () => {
    if (!hasActiveSubscriptions()) return;
    reconnectAttempts = 0;
    allowReconnect = true;
    const tenantNow = getTenantForConnection();
    activeTenant = tenantNow || null;
    ensureConnection(token.value || null, tenantNow);
  };

  if (!notifyReconnectFailed) {
    notifyReconnectFailed = () => {
      toast.add({
        title: "实时连接失败",
        description: "WebSocket 重试已达上限，请检查网络或联系管理员。",
        color: "error",
      });
    };
  }

  const subscribe = (topic: string, handler: TopicHandler, reqId?: string) => {
    if (!topic) return () => {};
    if (!subscriptions.has(topic)) {
      subscriptions.set(topic, new Set());
    }
    subscriptions.get(topic)!.add(handler);
    connect();
    sendCommand({ type: WS_BUS_CMD.SUBSCRIBE, topic, req_id: reqId });
    return () => {
      unsubscribe(topic, handler, reqId);
    };
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
  };

  const disconnect = () => {
    resetConnection("manual_disconnect", false);
  };

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
