// app/composables/agent/useDualChannelConnection.ts
import {
  ref,
  computed,
  watchEffect,
  watch,
  onMounted,
  onUnmounted,
  type Ref,
  type ComputedRef,
} from "vue";
import { useMessageStore } from "~/stores/message";
import { SSE_EVENT_TYPES } from "~/types/message";
import { BaseFlowKey } from "../api/types/agent";
import { useStreamingThinkParser } from "./useThinkParser";
import { useEnvStore } from "~/stores/envStore";

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
  regenerateFrom: (
    messageId: number,
    flowId?: string,
    editedMessage?: string
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
  agentId?: Ref<string | null>,
  sessionId?: Ref<string | null>
): DualChannelConnection {
  const config = useRuntimeConfig();
  const apiBase = config.public.apiBase;
  const wsAgentPrefix = String((config.public as any).wsAgentPrefix || "/ws").trim() || "/ws";
  const envStore = useEnvStore();

  const sseActive = ref(false);
  const wsActive = ref(false);
  const currentRequestId = ref<string | null>(null);
  const messages = ref<any[]>([]);
  const isGenerating = computed(() => !!currentRequestId.value);
  const messageStore = useMessageStore();

  let pendingAssistantId: string | null = null;
  let currentSseAbort: AbortController | null = null;
  let sseProbeAbort: AbortController | null = null;
  let lastSseProbeAt = 0;
  let currentTraceMeta: Record<string, any> = {};

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
    if (envStore.currentEnv) return envStore.currentEnv;
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
  const getCurrentTenantUuid = () => {
    if (typeof window === "undefined") return "";
    try {
      return (
        localStorage.getItem("px_current_tenant_uuid") ||
        JSON.parse(localStorage.getItem("user-store") || "{}")?.context?.current_tenant_uuid ||
        ""
      );
    } catch {
      return localStorage.getItem("px_current_tenant_uuid") || "";
    }
  };
  const toBase64Url = (s: string) =>
    btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");

  const normalizePathPrefix = (input: string, fallback: string) => {
    const raw = String(input || "").trim();
    if (!raw) return fallback;
    const withLeading = raw.startsWith("/") ? raw : `/${raw}`;
    return withLeading.replace(/\/+$/, "") || fallback;
  };

  const resolveWSOrigin = () => {
    const protocol = location.protocol === "https:" ? "wss:" : "ws:";
    return `${protocol}//${location.host}`;
  };

  const buildWSUrl = (path: string, params?: Record<string, any>) => {
    const prefix = normalizePathPrefix(wsAgentPrefix, "/ws");
    const origin = resolveWSOrigin();
    let url = `${origin}${prefix}${path}`;
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
    const token = getAuthToken();
    if (!token) {
      sseActive.value = false;
      return;
    }

    // 防抖：避免 focus/visibility 等触发过于频繁，导致堆积大量 pending 请求
    const now = Date.now();
    if (now - lastSseProbeAt < 1500) return;
    lastSseProbeAt = now;

    // 重要：SSE 探针必须“读取并关闭”响应体，否则浏览器会把连接长期显示为 pending，甚至耗尽并发连接数
    try {
      sseProbeAbort?.abort();
    } catch {}
    const abortController = new AbortController();
    sseProbeAbort = abortController;

    const timeoutId = setTimeout(() => {
      try {
        abortController.abort();
      } catch {}
    }, 2000);

    try {
      const res = await fetch(buildHttpUrl("/agents/stream/sse", { probe: 1 }), {
        method: "GET",
        headers: {
          Accept: "text/event-stream",
          "Cache-Control": "no-cache",
          Authorization: `${getTokenType()} ${token}`,
        },
        signal: abortController.signal,
      });
      sseActive.value = res.ok;
      if (!res.ok) return;

      // 读取一个 chunk 即可判定链路可用，然后立刻关闭连接，避免 pending
      try {
        const reader = res.body?.getReader();
        if (reader) {
          await reader.read();
          await reader.cancel();
        } else {
          // fallback：消费一次文本
          await res.text();
        }
      } catch {}
    } catch {
      sseActive.value = false;
    } finally {
      clearTimeout(timeoutId);
      if (sseProbeAbort === abortController) sseProbeAbort = null;
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

  function cleanTraceMeta(meta?: Record<string, any> | null) {
    const out: Record<string, any> = {};
    if (!meta || typeof meta !== "object") return out;
    for (const [key, value] of Object.entries(meta)) {
      if (value == null) continue;
      const text = String(value).trim();
      if (!text) continue;
      out[key] = value;
    }
    return out;
  }

  function hasMessageTraceMeta(meta?: Record<string, any> | null) {
    const t = cleanTraceMeta(meta);
    return !!String(t.tenant_uuid || "").trim()
      && !!String(t.session_id || t.session_id_num || "").trim()
      && !!String(t.message_id || t.user_message_id || "").trim();
  }

  function getMessageTraceMeta(message: any) {
    return cleanTraceMeta(message?.meta?.trace || message?.metadata?.trace);
  }

  function getLastUserTraceMeta() {
    for (let i = messages.value.length - 1; i >= 0; i--) {
      const message = messages.value[i] as any;
      if (message?.role !== "user") continue;
      const trace = getMessageTraceMeta(message);
      if (hasMessageTraceMeta(trace)) return trace;
      const id = String(message?.id || "").trim();
      if (/^\d+$/.test(id)) {
        return cleanTraceMeta({
          ...trace,
          tenant_uuid: trace.tenant_uuid || getCurrentTenantUuid(),
          session_id: trace.session_id || sessionId?.value,
          message_id: trace.message_id || id,
        });
      }
    }
    return {};
  }

  function resolveTraceMetaForAssistantError() {
    const current = cleanTraceMeta(currentTraceMeta);
    if (hasMessageTraceMeta(current)) return current;
    return cleanTraceMeta({
      ...getLastUserTraceMeta(),
      ...current,
    });
  }

  function attachTraceMeta(message: any, trace: Record<string, any>) {
    const normalized = cleanTraceMeta(trace);
    if (Object.keys(normalized).length === 0) return false;
    message.meta = {
      ...(message.meta || {}),
      trace: {
        ...((message.meta || {}).trace || {}),
        ...normalized,
      },
    };
    return true;
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
    // 避免旧请求残留导致一直“生成中”
    try {
      currentSseAbort?.abort();
    } catch {}

    const abortController = new AbortController();
    currentSseAbort = abortController;

    const requestId = `req_${Date.now()}_${Math.random()
      .toString(36)
      .slice(2, 9)}`;
    currentRequestId.value = requestId;

    const params: Record<string, any> = {
      q: message,
      env: getEnv(),
      flow_id: flowId,
    };
    if (agentId?.value) params.agent_uuid = agentId.value;
    if (sessionId?.value) params.session_id = sessionId.value;
    if (meta) {
      // 兼容前端历史字段：sessionId/agentId -> session_id/agent_uuid
      if ((meta as any).session_id == null && (meta as any).sessionId != null) {
        (meta as any).session_id = (meta as any).sessionId;
      }
      if ((meta as any).agent_uuid == null && (meta as any).agentId != null) {
        (meta as any).agent_uuid = (meta as any).agentId;
      }
      Object.assign(params, meta);
    }

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
        signal: abortController.signal,
      });
      if (!resp.ok) throw new Error(`HTTP ${resp.status} ${resp.statusText}`);

      const reader = resp.body?.getReader();
      if (!reader) throw new Error("无法读取 SSE 流");
      const decoder = new TextDecoder();

      const run = async () => {
        // SSE 是“以空行分隔事件块”的协议；网络分片可能把一行 JSON 拆开，
        // 不能用 chunk.split("\n") 逐行即时 JSON.parse，否则会丢事件导致“前端无反馈”。
        let sseBuffer = "";
        let hasReceivedData = false;
        let timeoutId: any = null;
        let abortedByClient = false;

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

        const ensureProcessState = (msg: any) => {
          if (!msg.meta) msg.meta = {};
          if (!msg.meta.process) {
            msg.meta.process = {
              intent: null,
              plan: null,
              nodes: [],
              updatedAt: Date.now(),
            };
          }
          return msg.meta.process as {
            intent: any;
            plan: any;
            nodes: any[];
            updatedAt: number;
          };
        };

        const ensureRunState = (msg: any) => {
          if (!msg.meta) msg.meta = {};
          if (!msg.meta.runState) {
            msg.meta.runState = {
              run: {},
              responsePlan: null,
              intent: null,
              plan: null,
              tasks: [],
              pendingParams: [],
              results: [],
              errors: [],
              traceLinks: [],
              updatedAt: Date.now(),
            };
          }
          return msg.meta.runState as any;
        };

        const mergeRunIdentity = (runState: any, payload: any) => {
          const source = payload?.payload && typeof payload.payload === "object"
            ? { ...payload, ...payload.payload }
            : payload;
          for (const key of ["run_id", "session_id", "message_id", "trace_id"]) {
            const value = source?.[key];
            if (value != null && String(value).trim() !== "") {
              runState.run[key] = value;
            }
          }
        };

        const upsertRunTask = (runState: any, task: any, fallbackStatus?: string) => {
          const normalized = {
            ...(task || {}),
            status: String(task?.status || fallbackStatus || "pending").trim(),
            updated_at: task?.updated_at || new Date().toISOString(),
          };
          const taskId = String(normalized.task_id || normalized.node_id || "").trim();
          if (!taskId) normalized.task_id = `task_${Date.now()}_${runState.tasks.length}`;
          const key = String(normalized.task_id || taskId);
          const idx = runState.tasks.findIndex((item: any) => String(item?.task_id || "") === key);
          if (idx < 0) runState.tasks.push(normalized);
          else runState.tasks[idx] = { ...(runState.tasks[idx] || {}), ...normalized };
          if (normalized.status === "awaiting_params") {
            runState.pendingParams = runState.tasks.filter((item: any) => item?.status === "awaiting_params");
          }
          if (normalized.status === "completed") {
            runState.results = runState.tasks.filter((item: any) => item?.status === "completed" && (item?.result || item?.links?.length));
          }
          if (normalized.status === "failed") {
            runState.errors = runState.tasks.filter((item: any) => item?.status === "failed");
          }
        };

        const applyRunStateEvent = (eventType: string, payload: any) => {
          const { idx, msg } = getPendingAssistant();
          if (idx < 0 || !msg) return;
          const runState = ensureRunState(msg);
          mergeRunIdentity(runState, payload);
          const inner = payload?.payload && typeof payload.payload === "object" ? payload.payload : payload;
          if (eventType === SSE_EVENT_TYPES.AGENT_RUN_RESPONSE_PLAN) {
            runState.responsePlan = inner;
          } else if (eventType === SSE_EVENT_TYPES.AGENT_RUN_INTENT_DETECTED) {
            runState.intent = inner;
          } else if (eventType === SSE_EVENT_TYPES.AGENT_RUN_PLAN_CREATED) {
            runState.plan = inner;
            const planTasks = inner?.plan?.tasks || inner?.payload?.plan?.tasks;
            if (Array.isArray(planTasks)) {
              for (const task of planTasks) upsertRunTask(runState, task, "pending");
            }
          } else if (
            eventType === SSE_EVENT_TYPES.AGENT_RUN_TASK_STATUS ||
            eventType === SSE_EVENT_TYPES.AGENT_RUN_TASK_STARTED ||
            eventType === SSE_EVENT_TYPES.AGENT_RUN_TASK_COMPLETED ||
            eventType === SSE_EVENT_TYPES.AGENT_RUN_TASK_FAILED
          ) {
            upsertRunTask(runState, inner);
          } else if (eventType === SSE_EVENT_TYPES.AGENT_RUN_AWAITING_PARAMS) {
            const missing = Array.isArray(inner?.missing_fields) ? inner.missing_fields : [];
            upsertRunTask(runState, {
              ...inner,
              task_id: inner?.task_id || "pending_params",
              status: "awaiting_params",
              missing_fields: missing,
            });
          } else if (eventType === SSE_EVENT_TYPES.AGENT_RUN_FINAL) {
            runState.final = inner;
          } else if (eventType === SSE_EVENT_TYPES.AGENT_RUN_ENDED) {
            runState.ended = true;
          }
          runState.updatedAt = Date.now();
          bumpMessagesRef();
        };

        const upsertNodeState = (nodes: any[], eventType: string, payload: any) => {
          const nodeId = String(
            payload?.node_id ?? payload?.task_id ?? payload?.flow_id ?? ""
          ).trim();
          const statusFromEvent =
            eventType === SSE_EVENT_TYPES.NODE_START
              ? "running"
              : String(payload?.status || "pending");
          const patch = {
            node_id: nodeId,
            task_id: payload?.task_id,
            flow_id: payload?.flow_id,
            node_kind: payload?.node_kind,
            node_ref: payload?.node_ref,
            node_name: payload?.node_name,
            node_desc: payload?.node_desc,
            status: statusFromEvent,
            error: payload?.error || "",
            updated_at: Date.now(),
          };
          if (!nodeId) {
            nodes.push(patch);
            return;
          }
          const idx = nodes.findIndex(
            (n) =>
              String(n?.node_id || "").trim() === nodeId ||
              String(n?.task_id || "").trim() === nodeId
          );
          if (idx < 0) {
            nodes.push(patch);
          } else {
            nodes[idx] = { ...(nodes[idx] || {}), ...patch };
          }
        };

        const applyProcessEvent = (eventType: string, payload: any) => {
          const { idx, msg } = getPendingAssistant();
          if (idx < 0 || !msg) return;
          const process = ensureProcessState(msg);
          if (eventType === SSE_EVENT_TYPES.INTENT) {
            process.intent = payload;
          } else if (eventType === SSE_EVENT_TYPES.PLAN) {
            process.plan = payload;
          } else if (
            eventType === SSE_EVENT_TYPES.NODE_START ||
            eventType === SSE_EVENT_TYPES.NODE_END
          ) {
            upsertNodeState(process.nodes, eventType, payload);
          }
          process.updatedAt = Date.now();
          bumpMessagesRef();
        };

        const mergeTraceMetaIntoPending = (payload: any) => {
          const data =
            payload?.data && typeof payload.data === "object"
              ? payload.data
              : {};
          const patch: Record<string, any> = {};
          const pick = (key: string, ...aliases: string[]) => {
            for (const name of [key, ...aliases]) {
              const value = payload?.[name] ?? data?.[name];
              if (value != null && String(value).trim() !== "") {
                patch[key] = value;
                return;
              }
            }
          };
          pick("tenant_uuid");
          pick("trace_id");
          pick("session_id", "session_id_num");
          pick("session_uuid");
          pick("message_id", "user_message_id");
          pick("agent_id");
          if (Object.keys(patch).length === 0) return;
          currentTraceMeta = { ...currentTraceMeta, ...patch };
          const messageID = String(currentTraceMeta.message_id || "").trim();
          const clientMsgID = String(
            payload?.client_msg_id ?? data?.client_msg_id ?? ""
          ).trim();
          if (messageID || clientMsgID) {
            const userIdx = messages.value.findIndex((m) => {
              const id = String(m.id);
              return id === messageID || (clientMsgID && id === clientMsgID);
            });
            if (userIdx >= 0) {
              const updated = { ...messages.value[userIdx] };
              attachTraceMeta(updated, currentTraceMeta);
              messages.value[userIdx] = updated;
            }
          }
          const { idx, msg } = getPendingAssistant();
          if (idx >= 0 && msg) {
            attachTraceMeta(msg, currentTraceMeta);
            bumpMessagesRef();
          }
        };

        const finalize = (opts?: {
          errorMessage?: string;
          abort?: boolean;
        }) => {
          const { idx, msg } = getPendingAssistant();
          if (idx >= 0 && msg) {
            msg.done = true;
            msg.isStreaming = false;
            msg.isThinking = false;
            if (opts?.errorMessage) {
              msg.isError = true;
              msg.content = opts.errorMessage;
            }
            attachTraceMeta(
              msg,
              opts?.errorMessage
                ? resolveTraceMetaForAssistantError()
                : currentTraceMeta
            );
            bumpMessagesRef();
          }

          currentRequestId.value = null;
          pendingAssistantId = null;
          if (currentSseAbort === abortController) {
            currentSseAbort = null;
          }

          if (opts?.abort) {
            abortedByClient = true;
            try {
              abortController.abort();
            } catch {}
          }
        };

        const connectionTimeout = () => {
          if (!hasReceivedData) {
            const errorTrace = resolveTraceMetaForAssistantError();
            const { idx } = getPendingAssistant();
            if (idx >= 0) {
              const updated = {
                ...messages.value[idx],
                isThinking: false,
                isStreaming: false,
                isError: true,
                content: "连接超时：服务器未响应确认包。",
              };
              attachTraceMeta(updated, errorTrace);
              messages.value[idx] = updated;
              bumpMessagesRef();
            } else {
              const errorMessage: any = {
                id: `error_${Date.now()}`,
                role: "assistant",
                content: "连接超时：服务器未响应确认包。",
                timestamp: Date.now(),
                isError: true,
              };
              attachTraceMeta(errorMessage, errorTrace);
              messages.value.push(errorMessage);
              bumpMessagesRef();
            }
            finalize({ abort: true });
          }
        };
        timeoutId = setTimeout(connectionTimeout, 10000);

        const dispatchSSEBlock = (block: string) => {
          // 解析单个 SSE block（以空行分隔）
          let eventName = "";
          const dataLines: string[] = [];
          for (const line of block.split("\n")) {
            if (!line) continue;
            if (line.startsWith(":")) continue; // comment/keepalive
            if (line.startsWith("event:")) {
              eventName = line.slice(6).trim();
              continue;
            }
            if (line.startsWith("data:")) {
              dataLines.push(line.slice(5).trimStart());
              continue;
            }
          }

          const raw = dataLines.join("\n").trim();
          if (raw === "[DONE]") {
            finalize({ abort: true });
            return;
          }

          // 只要收到任何 block，都算“服务器已响应”，停止“确认包超时”计时
          if (!hasReceivedData) {
            hasReceivedData = true;
            if (timeoutId) {
              clearTimeout(timeoutId);
              timeoutId = null;
            }
          }

          // 允许没有 data 的事件（例如纯心跳/控制帧）
          let payload: any = {};
          if (raw) {
            try {
              payload = JSON.parse(raw);
            } catch {
              payload = { raw };
            }
          }
          if (!payload.type && eventName) payload.type = eventName;

          onMessageCallback?.(payload);
          const type = String(payload.type || eventName || "").toLowerCase();

          if (type.startsWith("agent_run.")) {
            applyRunStateEvent(type, payload);
            return;
          }

          // meta：用于把“前端临时消息 id”映射到“DB message id”（支持立即重新生成）
          if (type === SSE_EVENT_TYPES.META) {
            mergeTraceMetaIntoPending(payload);
            const clientMsgId =
              payload?.client_msg_id ?? payload?.data?.client_msg_id;
            const userMessageId =
              payload?.user_message_id ?? payload?.data?.user_message_id;
            if (clientMsgId && userMessageId) {
              const idx = messages.value.findIndex((m) => m.id === clientMsgId);
              if (idx >= 0) {
                const updated = { ...messages.value[idx], id: userMessageId };
                attachTraceMeta(updated, currentTraceMeta);
                messages.value[idx] = updated;
                bumpMessagesRef();
              }
            }
            return;
          }

          // 控制事件略过
          if (
            type === SSE_EVENT_TYPES.ACK ||
            type === SSE_EVENT_TYPES.START ||
            type === SSE_EVENT_TYPES.INTENT ||
            type === SSE_EVENT_TYPES.PLAN ||
            type === SSE_EVENT_TYPES.NODE_START ||
            type === SSE_EVENT_TYPES.NODE_END ||
            type === SSE_EVENT_TYPES.HEARTBEAT ||
            type === SSE_EVENT_TYPES.ACTION
          ) {
            if (
              type === SSE_EVENT_TYPES.INTENT ||
              type === SSE_EVENT_TYPES.PLAN ||
              type === SSE_EVENT_TYPES.NODE_START ||
              type === SSE_EVENT_TYPES.NODE_END
            ) {
              applyProcessEvent(type, payload);
              if (type === SSE_EVENT_TYPES.NODE_START || type === SSE_EVENT_TYPES.NODE_END) {
                const { idx, msg } = getPendingAssistant();
                if (idx >= 0 && msg) {
                  const runState = ensureRunState(msg);
                  const status =
                    type === SSE_EVENT_TYPES.NODE_START
                      ? "running"
                      : String(payload?.status || "").trim().toLowerCase();
                  if (status) {
                    upsertRunTask(runState, {
                      ...payload,
                      status,
                      task_id: payload?.task_id || payload?.node_id || payload?.flow_id,
                    });
                  }
                }
              }
            }
            return;
          }

          // 内容事件
          if (
            type === SSE_EVENT_TYPES.TOKEN ||
            type === SSE_EVENT_TYPES.CHUNK ||
            type === SSE_EVENT_TYPES.DATA ||
            type === SSE_EVENT_TYPES.FINAL
          ) {
            // 后端会先发一个 EventData（step_id=start, data.message=start）作为“流开始”占位；
            // 这条不应触发 UI 创建空的 assistant 气泡，否则会出现“空白回复 + 没有思考提示”。
            if (
              type === SSE_EVENT_TYPES.DATA &&
              (String(payload?.step_id || "") === "start" ||
                String(payload?.data?.message || "") === "start")
            ) {
              return;
            }

            // 1) 找到/创建唯一的 assistant 占位
            const idx = messages.value.findIndex((m) => m.id === pendingAssistantId);
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
                  trace: currentTraceMeta,
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
              type === SSE_EVENT_TYPES.DATA || type === SSE_EVENT_TYPES.FINAL
                ? "snapshot"
                : "delta";

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
              trace: {
                ...((answer.meta || {}).trace || {}),
                ...currentTraceMeta,
              },
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
              // 后端可能不会发送 [DONE]/END，FINAL 视为一次响应结束
              finalize({ abort: true });
              return;
            }

            bumpMessagesRef();
            return;
          }

          if (type === SSE_EVENT_TYPES.END) {
            finalize({ abort: true });
            return;
          }

          if (type === SSE_EVENT_TYPES.ERROR) {
            mergeTraceMetaIntoPending(payload);
            const errorTrace = resolveTraceMetaForAssistantError();
            const { idx, msg } = getPendingAssistant();
          if (idx >= 0 && msg) {
            msg.isThinking = false;
            msg.isStreaming = false;
            msg.isError = true;
            msg.content =
              payload?.message ||
              payload?.error ||
              "发生错误：未知的服务端错误。";
            attachTraceMeta(msg, errorTrace);
            bumpMessagesRef();
          } else {
            const errorMessage: any = {
              id: `error_${Date.now()}`,
              role: "assistant",
              content:
                payload?.message ||
                payload?.error ||
                "发生错误：未知的服务端错误。",
              timestamp: Date.now(),
              isError: true,
            };
            attachTraceMeta(errorMessage, errorTrace);
            messages.value.push(errorMessage);
            bumpMessagesRef();
          }
            finalize({ abort: true });
            return;
          }
        };

        try {
          while (true) {
            const { done, value } = await reader.read();
            if (done) {
              // flush 残留：有些实现可能在断开前未补齐空行分隔
              if (sseBuffer.trim()) {
                dispatchSSEBlock(sseBuffer);
                sseBuffer = "";
              }
              // 连接正常结束但未显式发送 [DONE]/END/FINAL：也要收尾，避免 UI 永久“生成中”
              finalize();
              break;
            }
            const chunk = decoder.decode(value, { stream: true })
              .replace(/\r\n/g, "\n")
              .replace(/\r/g, "\n");
            sseBuffer += chunk;

            // SSE 事件块：以空行分隔（\n\n）
            // 这里必须缓冲处理，避免网络分片导致 data 行被拆开而解析失败。
            while (true) {
              const sep = sseBuffer.indexOf("\n\n");
              if (sep < 0) break;
              const block = sseBuffer.slice(0, sep);
              sseBuffer = sseBuffer.slice(sep + 2);
              if (!block.trim()) continue;
              dispatchSSEBlock(block);
            }
          }
        } catch (err) {
          // 主动 abort 结束流：不算错误
          if (abortedByClient && (err as any)?.name === "AbortError") {
            return;
          }
          const { idx, msg } = (() => {
            if (!pendingAssistantId) return { idx: -1, msg: null as any };
            const i = messages.value.findIndex(
              (m) => m.id === pendingAssistantId
            );
            return { idx: i, msg: i >= 0 ? messages.value[i] : null };
          })();
          const errorTrace = resolveTraceMetaForAssistantError();

      if (idx >= 0 && msg) {
        msg.isThinking = false;
        msg.isStreaming = false;
        msg.isError = true;
        msg.content = `发生异常：${(err as any)?.message ?? "未知错误"}`;
        attachTraceMeta(msg, errorTrace);
        bumpMessagesRef();
      } else {
        const errorMessage: any = {
          id: `error_${Date.now()}`,
          role: "assistant",
          content: `发生异常：${(err as any)?.message ?? "未知错误"}`,
          timestamp: Date.now(),
          isError: true,
        };
        attachTraceMeta(errorMessage, errorTrace);
        messages.value.push(errorMessage);
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
      const errorTrace = resolveTraceMetaForAssistantError();

      if (idx >= 0 && msg) {
        msg.isThinking = false;
        msg.isStreaming = false;
        msg.isError = true;
        msg.content = `发送失败：${(err as any)?.message ?? "未知错误"}`;
        attachTraceMeta(msg, errorTrace);
        bumpMessagesRef();
      } else {
        const errorMessage: any = {
          id: `error_${Date.now()}`,
          role: "assistant",
          content: `发送失败：${(err as any)?.message ?? "未知错误"}`,
          timestamp: Date.now(),
          isError: true,
        };
        attachTraceMeta(errorMessage, errorTrace);
        messages.value.push(errorMessage);
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
    const noUserEcho = !!(meta as any)?.noUserEcho;
    const clientMsgId = `u_${Date.now()}`;
    const initialTraceMeta: Record<string, any> = {
      tenant_uuid: String((meta as any)?.tenant_uuid || getCurrentTenantUuid() || "").trim(),
      session_id: String((meta as any)?.session_id || (meta as any)?.sessionId || sessionId?.value || "").trim(),
    };
    Object.keys(initialTraceMeta).forEach((key) => {
      if (!initialTraceMeta[key]) delete initialTraceMeta[key];
    });
    if (!noUserEcho) {
      messages.value.push({
        id: clientMsgId,
        role: "user",
        content: message,
        timestamp: Date.now(),
        meta: Object.keys(initialTraceMeta).length > 0 ? { trace: initialTraceMeta } : undefined,
      });
    }

    pendingAssistantId = `thinking_${Date.now()}`;
    currentTraceMeta = { ...initialTraceMeta };
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
        trace: currentTraceMeta,
        think: {
          blocks: [],
          current: "",
          hasActiveThink: false,
          hasThink: false,
        },
      },
    });
    bumpMessagesRef();

    await sendSSEMessage(message, flowId, {
      ...(meta || {}),
      // 仅在本地生成的 user 消息需要映射
      ...(noUserEcho ? {} : { client_msg_id: clientMsgId }),
    });
  };

  const regenerateFrom = async (
    messageId: number,
    flowId = BaseFlowKey,
    editedMessage?: string
  ) => {
    if (!messageId) return;

    // 裁剪本地消息：保留到该 user message（含）为止
    const cutoff = messages.value.findIndex((m) => m.id === messageId);
    if (cutoff >= 0) {
      if (typeof editedMessage === "string") {
        const trimmed = editedMessage.trim();
        if (trimmed) {
          const m: any = messages.value[cutoff];
          if (m?.role === "user") {
            messages.value[cutoff] = { ...m, content: trimmed };
          }
        }
      }
      messages.value = messages.value.slice(0, cutoff + 1);
    }

    // 放一个新的 assistant 占位
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

    await sendSSEMessage((editedMessage ?? "").trim(), flowId, {
      noUserEcho: true,
      regen_from_message_id: messageId,
    });
  };

  const cancel = () => {
    try {
      wsConnection?.close();
    } catch {}
    wsConnection = null;
    try {
      currentSseAbort?.abort();
    } catch {}
    currentSseAbort = null;
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
    regenerateFrom,
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

    // 参考 ChatGPT 体验：仅在页面恢复活跃时重连；切换会话/离开页面再中断
    const handleFocus = () => {
      reconnectSSE();
      reconnectWS();
    };
    const handleVisibility = () => {
      if (document.visibilityState === "visible") {
        reconnectSSE();
        reconnectWS();
      }
    };

    onMounted(() => {
      window.addEventListener("focus", handleFocus);
      document.addEventListener("visibilitychange", handleVisibility);
      window.addEventListener("beforeunload", cancel);
    });

    onUnmounted(() => {
      window.removeEventListener("focus", handleFocus);
      document.removeEventListener("visibilitychange", handleVisibility);
      window.removeEventListener("beforeunload", cancel);
      cancel();
    });

    // 切换会话/智能体时：中断当前流，避免跨会话串流导致 UI 异常
    watch(
      [agentId ?? ref(null), sessionId ?? ref(null)],
      ([newAgentId, newSessionId], [oldAgentId, oldSessionId]) => {
        if (currentRequestId.value == null) return;
        if (newAgentId !== oldAgentId || newSessionId !== oldSessionId) cancel();
      }
    );
  }

  return connection;
}
