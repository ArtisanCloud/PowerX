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

const RECONNECT_BASE_DELAY = 1000;
const RECONNECT_MAX_DELAY = 30000;

const buildWSUrl = (token: string) => {
  const protocol = location.protocol === "https:" ? "wss:" : "ws:";
  const auth = encodeURIComponent(`Bearer ${token}`);
  return `${protocol}//${location.host}/api/ws?authorization=${auth}`;
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
  const delay = Math.min(RECONNECT_MAX_DELAY, RECONNECT_BASE_DELAY * Math.pow(2, reconnectAttempts));
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null;
    reconnectAttempts += 1;
    ensureConnection(token);
  }, delay);
};

const ensureConnection = (token: string | null) => {
  if (!process.client) return;
  if (!token) return;
  if (wsInstance && wsConnected.value) return;
  if (wsConnecting.value) return;
  wsConnecting.value = true;
  wsError.value = null;

  const url = buildWSUrl(token);
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
        handlers.forEach((handler) => handler(env.payload, env));
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
  const auth = useAuth();
  const me = useMe();
  const token = computed(() => auth.token.value || auth.getToken());

  if (process.client && !watchersInitialized) {
    watchersInitialized = true;
    watch(
      () => me.currentTenantUuid.value || resolveTenantUUIDForRequest(),
      (nextTenant, prevTenant) => {
        if (!nextTenant) return;
        if (prevTenant && nextTenant !== prevTenant) {
          activeTenant = nextTenant;
          resetConnection("tenant_changed");
          ensureConnection(token.value || null);
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
        ensureConnection(nextToken);
      }
    );
    if (process.client && !networkListenersBound) {
      networkListenersBound = true;
      window.addEventListener("online", () => {
        allowReconnect = true;
        ensureConnection(token.value || null);
      });
      window.addEventListener("offline", () => {
        wsError.value = "网络断开";
      });
    }
  }

  const connect = () => {
    allowReconnect = true;
    ensureConnection(token.value || null);
  };

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
    if (set && handler) {
      set.delete(handler);
      if (set.size === 0) {
        subscriptions.delete(topic);
      }
    } else if (set && !handler) {
      subscriptions.delete(topic);
    }
    sendCommand({ type: WS_BUS_CMD.UNSUBSCRIBE, topic, req_id: reqId });
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
