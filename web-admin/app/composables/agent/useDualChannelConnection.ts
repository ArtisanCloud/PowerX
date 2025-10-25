// app/composables/agent/useDualChannelConnection.ts
import {
  ref,
  computed,
  watchEffect,
  watch,
  type Ref,
  type ComputedRef,
} from "vue";
import { useMessageStore } from "~/stores/message";
import { SSE_EVENT_TYPES } from "~/types/message";
import { BaseFlowKey } from "../api/types/agent";
import { useStreamingThinkParser } from "./useThinkParser";

export interface DualChannelConnection {
  sseActive: Ref<boolean>;
  wsActive: Ref<boolean>;
  currentRequestId: Ref<string | null>;
  reconnectSSE: () => Promise<void>;
  reconnectWS: () => Promise<void>;
  cancel: () => void;
  sendMessage: (
    message: string,
    flowId?: string,
    meta?: Record<string, any>
  ) => Promise<void>;
  sendCommand: (command: any) => boolean;
  messages: Ref<any[]>;
  isGenerating: ComputedRef<boolean>;
  clearMessages: () => void;
  disconnect: () => void;
  onMessage?: (data: any) => void;
  onError?: (error: any) => void;
}

export function useDualChannelConnection(
  agentId?: Ref<number | null>,
  sessionId?: Ref<string | null>
): DualChannelConnection {
  const config = useRuntimeConfig();
  const apiBase = config.public.apiBase;

  const sseActive = ref(false);
  const wsActive = ref(false);
  const currentRequestId = ref<string | null>(null);
  const messages = ref<any[]>([]);
  const isGenerating = computed(() => !!currentRequestId.value);
  const messageStore = useMessageStore();

  let pendingAssistantId: string | null = null;

  watch(sessionId, (newSessionId, oldSessionId) => {
    if (oldSessionId && messages.value.length > 0) {
      messageStore.setMessages(String(oldSessionId), messages.value);
    }
    if (newSessionId) {
      const cached = messageStore.getMessagesBySession(String(newSessionId));
      if (cached.length > 0) messages.value = cached;
    }
  });

  let wsConnection: WebSocket | null = null;
  let onMessageCallback: ((data: any) => void) | undefined;
  let onErrorCallback: ((error: any) => void) | undefined;

  const getEnv = () => {
    if (typeof window === "undefined") return "dev";
    try {
      const envStore = localStorage.getItem("env-store");
      if (envStore) return JSON.parse(envStore)?.currentEnv || "dev";
    } catch {}
    return "dev";
  };
  const getAuthToken = () =>
    typeof window === "undefined"
      ? ""
      : localStorage.getItem("access_token") || "";
  const getTokenType = () =>
    typeof window === "undefined"
      ? "Bearer"
      : localStorage.getItem("token_type") || "Bearer";
  const toBase64Url = (s: string) =>
    btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");

  const buildWSUrl = (path: string, params?: Record<string, any>) => {
    const protocol = location.protocol === "https:" ? "wss:" : "ws:";
    const host = location.host;
    let url = `${protocol}//${host}${apiBase}${path}`;
    if (params) {
      const qs = Object.entries(params)
        .filter(([, v]) => v != null)
        .map(
          ([k, v]) =>
            `${encodeURIComponent(k)}=${encodeURIComponent(
              typeof v === "string" ? v : JSON.stringify(v)
            )}`
        )
        .join("&");
      if (qs) url += (url.includes("?") ? "&" : "?") + qs;
    }
    return url;
  };

  const buildHttpUrl = (path: string, params?: Record<string, any>) => {
    let url = `${apiBase}${path}`;
    if (params) {
      const qs = Object.entries(params)
        .filter(([, v]) => v != null)
        .map(
          ([k, v]) =>
            `${encodeURIComponent(k)}=${encodeURIComponent(
              typeof v === "string" ? v : JSON.stringify(v)
            )}`
        )
        .join("&");
      if (qs) url += (url.includes("?") ? "&" : "?") + qs;
    }
    return url;
  };

  const reconnectSSE = async () => {
    try {
      const res = await fetch(
        buildHttpUrl("/agents/stream/sse", { probe: 1 }),
        {
          method: "GET",
          headers: {
            Accept: "application/json",
            Authorization: `${getTokenType()} ${getAuthToken()}`,
          },
        }
      );
      sseActive.value = res.ok;
    } catch {
      sseActive.value = false;
    }
  };
  const reconnectWS = async () => {
    const token = getAuthToken();
    if (!token) {
      wsActive.value = false;
      return;
    }
    return new Promise<void>((resolve) => {
      try {
        const protocols = [`bearer.${toBase64Url(token)}`];
        const url = buildWSUrl("/agents/stream/ws", {
          probe: 1,
          authorization: `${getTokenType()} ${token}`,
        });
        const ws = new WebSocket(url, protocols);
        let done = false;
        const finish = (ok: boolean) => {
          if (done) return;
          done = true;
          wsActive.value = ok;
          try {
            ws.close();
          } catch {}
          resolve();
        };
        ws.onopen = () => finish(true);
        ws.onmessage = () => finish(true);
        ws.onerror = () => finish(false);
        ws.onclose = () => finish(false);
        setTimeout(() => finish(false), 5000);
      } catch {
        wsActive.value = false;
        resolve();
      }
    });
  };

  function pickText(payload: any, kind?: string): string {
    if (kind === SSE_EVENT_TYPES.TOKEN || kind === SSE_EVENT_TYPES.CHUNK) {
      return (
        payload?.delta ??
        payload?.text ??
        payload?.data?.delta ??
        payload?.data?.text ??
        ""
      );
    }
    const c1 =
      payload?.data?.data?.result?.content ??
      payload?.data?.result?.content ??
      payload?.data?.content ??
      payload?.text ??
      payload?.delta ??
      "";
    return typeof c1 === "string" ? c1 : JSON.stringify(c1);
  }

  function bumpMessagesRef() {
    messages.value = [...messages.value];
    syncMessagesToCache();
  }
  function syncMessagesToCache() {
    if (sessionId?.value && messages.value.length > 0) {
      messageStore.setMessages(String(sessionId.value), messages.value);
    }
  }

  // ✅ 文本去重替换策略：杜绝 FINAL/快照把正文重复
  function applyMainContent(prev: string, next: string) {
    const a = (prev || "").trim();
    const b = (next || "").trim();
    if (!a) return b;
    if (!b) return a;
    if (a === b) return a;
    if (b.startsWith(a)) return b; // 快照或增量超集 → 用 b
    if (a.endsWith(b)) return a; // 已包含 → 保持
    return b; // 其他情况以新为准（FINAL 通常权威）
  }

  // 简单去重：防止同一段 think 重复进入 blocks
  function dedupeThinkBlocks(blocks: any[]) {
    const seen = new Set<string>();
    const out: { content: string; index: number }[] = [];
    for (const b of blocks || []) {
      const content = String(b?.content ?? "").trim();
      if (!content) continue;
      if (seen.has(content)) continue;
      seen.add(content);
      out.push({ content, index: out.length });
    }
    return out;
  }

  const sendSSEMessage = async (
    message: string,
    flowId = BaseFlowKey,
    meta?: Record<string, any>
  ) => {
    const requestId = `req_${Date.now()}_${Math.random()
      .toString(36)
      .slice(2, 9)}`;
    currentRequestId.value = requestId;

    const params: Record<string, any> = {
      q: message,
      env: getEnv(),
      flow_id: flowId,
    };
    if (agentId?.value) params.agent_id = agentId.value;
    if (sessionId?.value) params.session_id = sessionId.value;
    if (meta) Object.assign(params, meta);

    // 你现在用 mock 流
    // const url = buildHttpUrl("/agents/stream/mock", params);
    const url = buildHttpUrl("/agents/stream/sse", params);

    try {
      const resp = await fetch(url, {
        method: "GET",
        headers: {
          Accept: "text/event-stream",
          "Cache-Control": "no-cache",
          Authorization: `${getTokenType()} ${getAuthToken()}`,
        },
      });
      if (!resp.ok) throw new Error(`HTTP ${resp.status} ${resp.statusText}`);

      const reader = resp.body?.getReader();
      if (!reader) throw new Error("无法读取 SSE 流");
      const decoder = new TextDecoder();

      const run = async () => {
        let currentEvent: string | null = null;
        let hasReceivedData = false;
        let timeoutId: any = null;

        // 只创建一次解析器
        const thinkParser = useStreamingThinkParser();
        let started = false;

        const getPendingAssistant = () => {
          if (!pendingAssistantId) return { idx: -1, msg: null as any };
          const idx = messages.value.findIndex(
            (m) => m.id === pendingAssistantId
          );
          return { idx, msg: idx >= 0 ? messages.value[idx] : null };
        };

        const connectionTimeout = () => {
          if (!hasReceivedData) {
            const { idx } = getPendingAssistant();
            if (idx >= 0) {
              messages.value[idx] = {
                ...messages.value[idx],
                isThinking: false,
                isStreaming: false,
                isError: true,
                content: "连接超时：服务器未响应确认包。",
              };
              bumpMessagesRef();
            } else {
              messages.value.push({
                id: `error_${Date.now()}`,
                role: "assistant",
                content: "连接超时：服务器未响应确认包。",
                timestamp: Date.now(),
                isError: true,
              });
              bumpMessagesRef();
            }
            currentRequestId.value = null;
          }
        };
        timeoutId = setTimeout(connectionTimeout, 10000);

        try {
          while (true) {
            const { done, value } = await reader.read();
            if (done) break;
            const chunk = decoder.decode(value, { stream: true });

            for (const line of chunk.split(/\r?\n/)) {
              if (!line) continue;

              if (line.startsWith("event:")) {
                currentEvent = line.slice(6).trim();
                continue;
              }
              if (!line.startsWith("data:")) continue;

              const raw = line.slice(5).trim();
              if (raw === "[DONE]") {
                currentRequestId.value = null;
                continue;
              }

              let payload: any;
              try {
                payload = JSON.parse(raw);
              } catch {
                payload = { raw };
              }
              if (!payload.type && currentEvent) payload.type = currentEvent;

              if (!hasReceivedData) {
                hasReceivedData = true;
                if (timeoutId) {
                  clearTimeout(timeoutId);
                  timeoutId = null;
                }
              }

              onMessageCallback?.(payload);
              const type = String(
                payload.type || currentEvent || ""
              ).toLowerCase();

              // 控制事件略过
              if (
                type === SSE_EVENT_TYPES.ACK ||
                type === SSE_EVENT_TYPES.START ||
                type === SSE_EVENT_TYPES.INTENT ||
                type === SSE_EVENT_TYPES.PLAN ||
                type === SSE_EVENT_TYPES.META ||
                type === SSE_EVENT_TYPES.HEARTBEAT ||
                type === SSE_EVENT_TYPES.ACTION
              ) {
                continue;
              }

              // 内容事件
              if (
                type === SSE_EVENT_TYPES.TOKEN ||
                type === SSE_EVENT_TYPES.CHUNK ||
                type === SSE_EVENT_TYPES.DATA ||
                type === SSE_EVENT_TYPES.FINAL
              ) {
                // 1) 找到/创建唯一的 assistant 占位
                const idx = messages.value.findIndex(
                  (m) => m.id === pendingAssistantId
                );
                if (idx < 0) {
                  pendingAssistantId = `a_${Date.now()}`;
                  messages.value.push({
                    id: pendingAssistantId,
                    role: "assistant",
                    content: "",
                    timestamp: Date.now(),
                    isStreaming: true,
                    done: false,
                    isThinking: true,
                    isError: false,
                    meta: {
                      think: {
                        blocks: [],
                        current: "",
                        hasActiveThink: false,
                        hasThink: false,
                      },
                    },
                  });
                }
                const answerIdx = messages.value.findIndex(
                  (m) => m.id === pendingAssistantId
                );
                const answer = messages.value[answerIdx];

                // 2) 取本次文本片段
                const piece = pickText(payload, type) || "";

                // 3) 关键：DATA/FINAL = snapshot（覆盖），TOKEN/CHUNK = delta（追加）
                const mode =
                  type === SSE_EVENT_TYPES.DATA ||
                  type === SSE_EVENT_TYPES.FINAL
                    ? "snapshot"
                    : "delta";

                // ⚠️ 注意：thinkParser 要在 while 循环外提前 const thinkParser = useStreamingThinkParser();
                const {
                  completedThinks,
                  currentThinkContent,
                  mainContent,
                  hasActiveThink,
                  hasThink,
                } = thinkParser.parseStreamingContent(piece, mode);

                // 4) 覆盖正文（不要 +=）
                answer.content = mainContent; // 覆盖！不要改成 +=

                // 5) 更新状态 & Think 元数据
                answer.isStreaming = type !== SSE_EVENT_TYPES.FINAL;
                answer.done = type === SSE_EVENT_TYPES.FINAL;
                answer.isThinking = hasActiveThink;
                answer.meta = {
                  ...(answer.meta || {}),
                  think: {
                    blocks: completedThinks,
                    current: currentThinkContent,
                    hasActiveThink,
                    hasThink:
                      hasThink ||
                      completedThinks.length > 0 ||
                      !!currentThinkContent,
                  },
                };

                // 6) FINAL 收尾
                if (type === SSE_EVENT_TYPES.FINAL) {
                  pendingAssistantId = null;
                }

                bumpMessagesRef();
                continue;
              }

              if (type === SSE_EVENT_TYPES.END) {
                const { idx, msg } = getPendingAssistant();
                if (idx >= 0 && msg) {
                  msg.done = true;
                  msg.isStreaming = false;
                  msg.isThinking = false;
                  bumpMessagesRef();
                }
                currentRequestId.value = null;
                pendingAssistantId = null;
                continue;
              }

              if (type === SSE_EVENT_TYPES.ERROR) {
                const { idx, msg } = getPendingAssistant();
                if (idx >= 0 && msg) {
                  msg.isThinking = false;
                  msg.isStreaming = false;
                  msg.isError = true;
                  msg.content =
                    payload?.message ||
                    payload?.error ||
                    "发生错误：未知的服务端错误。";
                  bumpMessagesRef();
                } else {
                  messages.value.push({
                    id: `error_${Date.now()}`,
                    role: "assistant",
                    content:
                      payload?.message ||
                      payload?.error ||
                      "发生错误：未知的服务端错误。",
                    timestamp: Date.now(),
                    isError: true,
                  });
                  bumpMessagesRef();
                }
                currentRequestId.value = null;
                pendingAssistantId = null;
                continue;
              }
            }
          }
        } catch (err) {
          const { idx, msg } = (() => {
            if (!pendingAssistantId) return { idx: -1, msg: null as any };
            const i = messages.value.findIndex(
              (m) => m.id === pendingAssistantId
            );
            return { idx: i, msg: i >= 0 ? messages.value[i] : null };
          })();

          if (idx >= 0 && msg) {
            msg.isThinking = false;
            msg.isStreaming = false;
            msg.isError = true;
            msg.content = `发生异常：${(err as any)?.message ?? "未知错误"}`;
            bumpMessagesRef();
          } else {
            messages.value.push({
              id: `error_${Date.now()}`,
              role: "assistant",
              content: `发生异常：${(err as any)?.message ?? "未知错误"}`,
              timestamp: Date.now(),
              isError: true,
            });
            bumpMessagesRef();
          }

          onErrorCallback?.(err);
        } finally {
          if (timeoutId) clearTimeout(timeoutId);
          currentRequestId.value = null;
          try {
            reader.releaseLock();
          } catch {}
        }
      };

      run();
    } catch (err) {
      const { idx, msg } = (() => {
        if (!pendingAssistantId) return { idx: -1, msg: null as any };
        const i = messages.value.findIndex((m) => m.id === pendingAssistantId);
        return { idx: i, msg: i >= 0 ? messages.value[i] : null };
      })();

      if (idx >= 0 && msg) {
        msg.isThinking = false;
        msg.isStreaming = false;
        msg.isError = true;
        msg.content = `发送失败：${(err as any)?.message ?? "未知错误"}`;
        bumpMessagesRef();
      } else {
        messages.value.push({
          id: `error_${Date.now()}`,
          role: "assistant",
          content: `发送失败：${(err as any)?.message ?? "未知错误"}`,
          timestamp: Date.now(),
          isError: true,
        });
        bumpMessagesRef();
      }

      onErrorCallback?.(err);
      currentRequestId.value = null;
      pendingAssistantId = null;
    }
  };

  const sendWSMessage = (_message: string) => {
    return;
  };
  const sendCommand = (_command: any) => {
    if (wsConnection && wsConnection.readyState === WebSocket.OPEN) {
      wsConnection.send(JSON.stringify(_command));
      return true;
    }
    return false;
  };

  const sendMessage = async (
    message: string,
    flowId = BaseFlowKey,
    meta?: Record<string, any>
  ) => {
    messages.value.push({
      id: `u_${Date.now()}`,
      role: "user",
      content: message,
      timestamp: Date.now(),
    });

    pendingAssistantId = `thinking_${Date.now()}`;
    messages.value.push({
      id: pendingAssistantId,
      role: "assistant",
      content: "",
      timestamp: Date.now(),
      isThinking: true,
      isStreaming: true,
      done: false,
      isError: false,
      meta: {
        think: {
          blocks: [],
          current: "",
          hasActiveThink: false,
          hasThink: false,
        },
      },
    });
    bumpMessagesRef();

    await sendSSEMessage(message, flowId, meta);
  };

  const cancel = () => {
    try {
      wsConnection?.close();
    } catch {}
    wsConnection = null;
    currentRequestId.value = null;
  };

  const clearMessages = () => {
    messages.value = [];
    pendingAssistantId = null;
    if (sessionId?.value) {
      messageStore.setMessages(String(sessionId.value), []);
    }
  };

  const disconnect = () => cancel();

  const connection: DualChannelConnection = {
    sseActive,
    wsActive,
    currentRequestId,
    reconnectSSE,
    reconnectWS,
    cancel,
    sendMessage,
    sendCommand,
    messages,
    isGenerating,
    clearMessages,
    disconnect,
    get onMessage() {
      return onMessageCallback;
    },
    set onMessage(cb) {
      onMessageCallback = cb;
    },
    get onError() {
      return onErrorCallback;
    },
    set onError(cb) {
      onErrorCallback = cb;
    },
  };

  if (typeof window !== "undefined") {
    reconnectSSE();
    reconnectWS();
    watchEffect(() => {
      const token = getAuthToken();
      if (token) {
        reconnectSSE();
        reconnectWS();
      }
    });
  }

  return connection;
}
