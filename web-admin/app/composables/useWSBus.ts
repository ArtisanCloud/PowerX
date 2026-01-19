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

const buildWSUrl = (token: string) => {
  const protocol = location.protocol === "https:" ? "wss:" : "ws:";
  const auth = encodeURIComponent(`Bearer ${token}`);
  return `${protocol}//${location.host}/api/ws?authorization=${auth}`;
};

const resetConnection = (reason?: string) => {
  if (reason) wsError.value = reason;
  if (reconnectTimer) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
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

const clearSubscriptions = () => {
  subscriptions.clear();
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
    subscriptions.forEach((_handlers, topic) => {
      sendCommand({ type: WS_BUS_CMD.SUBSCRIBE, topic });
    });
  };
  ws.onclose = () => {
    wsConnected.value = false;
    wsConnecting.value = false;
  };
  ws.onerror = () => {
    wsConnected.value = false;
    wsConnecting.value = false;
    wsError.value = "连接失败";
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

  if (process.client) {
    watch(
      () => me.currentTenantUuid.value || resolveTenantUUIDForRequest(),
      (nextTenant, prevTenant) => {
        if (!nextTenant) return;
        if (prevTenant && nextTenant !== prevTenant) {
          activeTenant = nextTenant;
          resetConnection("tenant_changed");
          clearSubscriptions();
        } else {
          activeTenant = nextTenant;
        }
      },
      { immediate: true }
    );
  }

  const connect = () => {
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
    resetConnection();
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
