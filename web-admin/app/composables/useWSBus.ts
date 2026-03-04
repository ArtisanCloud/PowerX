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

const buildWSUrl = (token: string, tenantUUID?: string | null) => {
  const auth = encodeURIComponent(`Bearer ${token}`);
  const cfg = useRuntimeConfig();
  const upstream = String(cfg.public?.wsUpstream || "").trim();
  const tenant = encodeURIComponent(String(tenantUUID || ""));
  const tenantQuery = tenant ? `&tenant_uuid=${tenant}` : "";
  if (upstream.startsWith("ws://") || upstream.startsWith("wss://")) {
    let base = upstream.replace(/\/+$/, "");
    if (base.endsWith("/api/ws")) {
      return `${base}?authorization=${auth}${tenantQuery}`;
    }
    if (base.endsWith("/ws")) {
      base = `${base.slice(0, -3)}/api/ws`;
      return `${base}?authorization=${auth}${tenantQuery}`;
    }
    if (base.endsWith("/api")) {
      return `${base}/ws?authorization=${auth}${tenantQuery}`;
    }
    return `${base}/api/ws?authorization=${auth}${tenantQuery}`;
  }
  const protocol = location.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${location.host}/api/ws?authorization=${auth}${tenantQuery}`;
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
  if (reconnectTimer) return;
  if (reconnectAttempts >= MAX_RECONNECT_ATTEMPTS) {
    allowReconnect = false;
    wsError.value = "连接失败，请检查配置或联系管理员";
    if (notifyReconnectFailed) {
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
  if (wsInstance && wsConnected.value) return;
  if (wsConnecting.value) return;
  wsConnecting.value = true;
  wsError.value = null;

  const url = buildWSUrl(token, tenantUUID);
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
  ws.onclose = () => {
    wsConnected.value = false;
    wsConnecting.value = false;
    wsInstance = null;
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
        if (!nextTenant) return;
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
