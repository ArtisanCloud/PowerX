import { computed, watch, type Ref } from "vue";
import { useUserStore } from "~/stores/user";
import {
  sessionScopeParams,
  sessionWorkspaceKey,
  type ChatSessionScope,
} from "~/utils/agent/sessionScope";
import { useApiClient } from "~/composables/api";
import { useAgentSessionStore } from "~/stores/agentSession";
import { useMessageStore } from "~/stores/message";
import { useI18n } from "vue-i18n";
import { useEnvStore } from "~/stores/envStore";
import { normalizeHistoryMessageMeta } from "~/utils/agent/historyMessageMeta";

// 后端会话数据结构
interface SessionDTO {
  id: number;
  createdAt: string;
  updatedAt: string;
  DeletedAt: string | null;
  agentId: number;
  userId: number;
  title: string;
  sessionType: ChatSessionScope["sessionType"];
  teamId: number;
  singleton: boolean;
  ttlDays: number;
  maxKB: number;
  maxTokens: number;
  summary: string;
  status: string;
  latestAt: string;
  expiredAt: string;
  meta: Record<string, any>;
}

// 后端消息数据结构
interface MessageDTO {
  id: number;
  createdAt: string;
  updatedAt: string;
  DeletedAt: string | null;
  sessionId: number;
  agentId: number;
  role: "user" | "assistant" | "system" | "tool" | "summary";
  content: string;
  contentType: string;
  format: string;
  tokens: number;
  sizeBytes: number;
  pinned: boolean;
  isError: boolean;
  meta: Record<string, any>;
}

// 后端响应结构
interface ApiResponse<T> {
  code: number;
  message: string;
  data: {
    items: T[];
  };
  timestamp: number;
}

export interface ChatSession {
  id: number | string;
  sessionType?: ChatSessionScope["sessionType"];
  teamId?: string;
  title?: string;
  lastMessage?: string;
  updatedAt?: Date;
  unread?: number;
  pinned?: boolean;
}

export interface ChatMessage {
  id: number | string;
  role: "user" | "assistant" | "system" | "tool" | "summary";
  content: string;
  timestamp: number;
  isError?: boolean;
  meta?: Record<string, any>;
  isStreaming?: boolean;
  done?: boolean;
  isThinking?: boolean;
}

export function useChatSessions(
  opts: { pageSize?: number; scope?: Ref<ChatSessionScope> } = {},
) {
  const pageSize = opts.pageSize ?? 50;
  const apiClient = useApiClient();
  const sessionStore = useAgentSessionStore();
  const messageStore = useMessageStore();
  const { t } = useI18n();
  const envStore = useEnvStore();
  const ENV = computed(() => envStore.currentEnv || "dev");

  const userStore = useUserStore();
  const scope = computed<ChatSessionScope>(
    () => opts.scope?.value || { sessionType: "agent" },
  );
  const scopeKey = computed(() =>
    opts.scope
      ? sessionWorkspaceKey(
          String(userStore.user?.id || ""),
          String(userStore.currentTenantUuid || ""),
          ENV.value,
          scope.value,
        )
      : sessionStore.activeScopeKey,
  );
  const workspace = computed(() => sessionStore.workspace(scopeKey.value));
  if (opts.scope)
    watch(scopeKey, (key) => sessionStore.activateScope(key), {
      immediate: true,
      flush: "sync",
    });
  const capture = () => ({
    key: scopeKey.value,
    env: ENV.value,
    params: sessionScopeParams(scope.value),
  });
  const LAST_AGENT_ID_KEY = computed(
    () => `powerx.agent.lastSelectedAgentUuid:v2:${scopeKey.value}`,
  );
  const readLastAgentId = (): string | null => {
    if (typeof window === "undefined") return null;
    try {
      const v = localStorage.getItem(LAST_AGENT_ID_KEY.value);
      if (!v) return null;
      return String(v || "").trim() || null;
    } catch {
      return null;
    }
  };
  const writeLastAgentId = (agentId: string | null) => {
    if (typeof window === "undefined") return;
    try {
      if (!agentId) localStorage.removeItem(LAST_AGENT_ID_KEY.value);
      else localStorage.setItem(LAST_AGENT_ID_KEY.value, String(agentId));
    } catch {}
  };

  /**
   * 将后端数据转换为前端格式
   */
  function mapSessionDTO(dto: SessionDTO): ChatSession {
    const id =
      (dto as any)?.id ??
      (dto as any)?.ID ??
      (dto as any)?.sessionId ??
      (dto as any)?.session_id;
    return {
      id,
      sessionType: dto.sessionType || "legacy",
      teamId: String(dto.teamId || ""),
      title: (dto as any)?.title || t("agent.chat.untitledSession"),
      lastMessage: (dto as any)?.summary || "",
      updatedAt: new Date((dto as any)?.latestAt || (dto as any)?.updatedAt),
      unread: 0, // 后端暂无此字段，默认为0
      pinned: false, // 后端暂无此字段，默认为false
    };
  }

  function unwrapApiData<T = any>(resp: any): T | null {
    if (!resp) return null;
    // 常见后端形态：{ code, data, message }
    if (typeof resp === "object" && "data" in resp) {
      return (resp as any).data as T;
    }
    return resp as T;
  }

  /**
   * 将后端消息数据转换为前端格式
   */
  function mapMessageDTO(dto: MessageDTO): ChatMessage {
    const meta = normalizeHistoryMessageMeta({
      ...(dto.meta || {}),
      session_id: (dto as any).sessionId ?? (dto as any).session_id,
      agent_id: (dto as any).agentId ?? (dto as any).agent_id,
    });
    return {
      id: dto.id,
      role: dto.role,
      content: dto.content,
      timestamp: new Date(dto.createdAt).getTime(),
      isError: dto.isError,
      meta,
      // 历史消息不应该有流式状态
      isStreaming: false,
      done: true,
      isThinking: false, // 历史消息不是思考状态
    };
  }

  /**
   * 加载指定 agent 的会话列表
   */
  async function listSessions(agentId: string, force = false) {
    const request = capture();
    const inFlight = useState<Record<string, Promise<void>>>(
      "px-agent-session-list-inflight",
      () => ({}),
    );
    const key = `${request.key}:${agentId}`;
    if (!force && inFlight.value[key]) {
      return inFlight.value[key];
    }

    // 如果已有缓存且不强制刷新，则跳过
    if (
      !force &&
      (sessionStore.workspace(request.key).sessionsByAgent[agentId] || [])
        .length > 0
    ) {
      return;
    }

    sessionStore.setLoading(agentId, true, request.key);
    sessionStore.clearError(request.key);

    const run = async () => {
      const response = await apiClient.get<ApiResponse<SessionDTO>>(
        `/agents/sessions`,
        {
          params: {
            agent_uuid: agentId,
            status: "active",
            limit: pageSize,
            env: request.env,
            ...request.params,
          },
        },
      );

      if (response.code === 200) {
        const sessions = response.data.items
          .map(mapSessionDTO)
          .filter(
            (item) =>
              item.sessionType === request.params.session_type &&
              (item.sessionType !== "team" ||
                item.teamId === request.params.team_id),
          );
        sessionStore.setSessions(agentId, sessions, request.key);
        sessionStore.setHasMore(agentId, false, request.key); // 暂时设为false，后续可根据分页信息调整
      }
    };

    const promise = run();
    inFlight.value = { ...inFlight.value, [key]: promise };
    try {
      await promise;
    } catch (error: any) {
      console.error("加载会话列表失败:", error);
      sessionStore.setError(
        error?.message || t("agent.chat.errors.loadSessionsFailed"),
        request.key,
      );
      sessionStore.setSessions(agentId, [], request.key); // 设置空数组避免重复请求
      sessionStore.setHasMore(agentId, false, request.key);
      throw error;
    } finally {
      sessionStore.setLoading(agentId, false, request.key);
      const next = { ...inFlight.value };
      delete next[key];
      inFlight.value = next;
    }
  }

  /**
   * 加载更多会话（分页）
   */
  async function loadMore(agentId: string) {
    const request = capture();
    if (
      !sessionStore.workspace(request.key).hasMoreByAgent[agentId] ||
      sessionStore.workspace(request.key).sessionsLoadingByAgent[agentId]
    ) {
      return;
    }

    sessionStore.setLoading(agentId, true, request.key);

    try {
      // TODO: 实现分页逻辑，当前后端暂不支持游标分页
      const currentSessions =
        sessionStore.workspace(request.key).sessionsByAgent[agentId] || [];
      const offset = currentSessions.length;

      const response = await apiClient.get<ApiResponse<SessionDTO>>(
        `/agents/sessions`,
        {
          params: {
            agent_uuid: agentId,
            status: "active",
            limit: pageSize,
            offset: offset,
            env: request.env,
            ...request.params,
          },
        },
      );

      if (response.code === 200) {
        const newSessions = response.data.items
          .map(mapSessionDTO)
          .filter(
            (item) =>
              item.sessionType === request.params.session_type &&
              (item.sessionType !== "team" ||
                item.teamId === request.params.team_id),
          );
        const allSessions = [...currentSessions, ...newSessions];
        sessionStore.setSessions(agentId, allSessions, request.key);
        sessionStore.setHasMore(
          agentId,
          newSessions.length >= pageSize,
          request.key,
        );
      }
    } catch (error: any) {
      console.error("加载更多会话失败:", error);
      sessionStore.setError(
        error?.message || t("agent.chat.errors.loadMoreSessionsFailed"),
        request.key,
      );
      throw error;
    } finally {
      sessionStore.setLoading(agentId, false, request.key);
    }
  }

  /**
   * 创建新会话
   */
  async function createSession(
    agentId: string,
    title?: string,
  ): Promise<ChatSession> {
    const request = capture();
    try {
      const response = await apiClient.post<any>(`/agents/sessions`, {
        env: request.env,
        sessionType: request.params.session_type,
        teamId: request.params.team_id ? Number(request.params.team_id) : 0,
        agentUuid: agentId,
        // 置空：让后端在首条消息时自动生成标题（ChatGPT 风格）
        title: title || "",
      });

      const payload = unwrapApiData<SessionDTO>(response);
      if (payload) {
        const newSession = mapSessionDTO(payload);
        if (newSession?.id == null || String(newSession.id) === "") {
          throw new Error(t("agent.chat.errors.createSessionFailedNoData"));
        }
        if (
          newSession.sessionType !== request.params.session_type ||
          (newSession.sessionType === "team" &&
            newSession.teamId !== request.params.team_id)
        ) {
          throw new Error(t("agent.chat.sessionScopeMismatch"));
        }
        sessionStore.addSession(agentId, newSession, request.key);
        if (request.key === scopeKey.value) writeLastAgentId(agentId);
        return newSession;
      }
      throw new Error(t("agent.chat.errors.createSessionFailedNoData"));
    } catch (error: any) {
      console.error("创建会话失败:", error);
      sessionStore.setError(
        error?.message || t("agent.chat.errors.createSessionFailed"),
        request.key,
      );
      throw error;
    }
  }

  /**
   * 删除会话
   */
  async function deleteSession(agentId: string, sessionId: number | string) {
    const request = capture();
    try {
      await apiClient.delete(`/agents/sessions/${sessionId}`, {
        params: {
          env: request.env,
          ...request.params,
        },
        timeout: 60000,
        useGlobalLoading: false,
      });

      sessionStore.removeSession(agentId, sessionId, request.key);
    } catch (error: any) {
      console.error("删除会话失败:", error);
      sessionStore.setError(
        error?.message || t("agent.chat.errors.deleteSessionFailed"),
        request.key,
      );
      throw error;
    }
  }

  /**
   * 重命名会话
   */
  async function renameSession(
    agentId: string,
    sessionId: number | string,
    title: string,
  ) {
    const request = capture();
    try {
      await apiClient.patch(
        `/agents/sessions/${sessionId}`,
        { title },
        {
          params: {
            env: request.env,
            ...request.params,
          },
        },
      );

      sessionStore.updateSession(agentId, sessionId, { title }, request.key);
    } catch (error: any) {
      console.error("重命名会话失败:", error);
      sessionStore.setError(
        error?.message || t("agent.chat.errors.renameSessionFailed"),
        request.key,
      );
      throw error;
    }
  }

  /**
   * 归档会话
   */
  async function archiveSession(agentId: string, sessionId: number | string) {
    const request = capture();
    try {
      await apiClient.post(`/agents/sessions/${sessionId}/archive`, undefined, {
        params: {
          env: request.env,
          ...request.params,
        },
      });

      // 归档后从当前列表中移除
      sessionStore.removeSession(agentId, sessionId, request.key);
    } catch (error: any) {
      console.error("归档会话失败:", error);
      sessionStore.setError(
        error?.message || t("agent.chat.errors.archiveSessionFailed"),
        request.key,
      );
      throw error;
    }
  }

  /**
   * 加载会话消息（带缓存）
   */
  async function loadSessionMessages(
    sessionId: number | string,
    force = false,
  ): Promise<ChatMessage[]> {
    const request = capture();
    const sessionIdStr = JSON.stringify([request.key, String(sessionId)]);

    // 如果有缓存且不强制刷新，则返回缓存的消息
    if (!force && messageStore.getMessagesBySession(sessionIdStr).length > 0) {
      // console.info(
      //   "使用缓存的消息:",
      //   messageStore.getMessagesBySession(sessionIdStr)
      // );
      return messageStore.getMessagesBySession(sessionIdStr);
    }

    messageStore.setLoading(sessionIdStr, true);
    messageStore.clearError();

    try {
      const response = await apiClient.get<ApiResponse<MessageDTO>>(
        `/agents/sessions/${sessionId}/messages`,
        {
          params: {
            env: request.env,
            ...request.params,
            limit: 200,
          },
          // 历史消息加载不应阻塞全局启动/页面渲染
          useGlobalLoading: false,
          // 大会话在本地 DB 冷启动/慢查询时可能超过默认超时
          timeout: 60000,
        },
      );

      if (response.code === 200) {
        const messages = response.data.items.map(mapMessageDTO);
        // console.info("[useChatSessions] 加载会话消息成功:", messages);
        messageStore.setMessages(sessionIdStr, messages);
        return messages;
      }
      return [];
    } catch (error: any) {
      console.error("加载会话消息失败:", error);
      messageStore.setError(
        error?.message || t("agent.chat.errors.loadMessagesFailed"),
      );
      throw error;
    } finally {
      messageStore.setLoading(sessionIdStr, false);
    }
  }

  /**
   * 加载更多消息（分页）
   */
  async function loadMoreMessages(
    sessionId: number | string,
  ): Promise<ChatMessage[]> {
    const request = capture();
    const sessionIdStr = JSON.stringify([request.key, String(sessionId)]);
    const lastMessageId = messageStore.lastMessageIdBySession[sessionIdStr];

    if (!lastMessageId || messageStore.isLoadingBySession(sessionIdStr)) {
      return [];
    }

    messageStore.setLoading(sessionIdStr, true);

    try {
      const response = await apiClient.get<ApiResponse<MessageDTO>>(
        `/agents/sessions/${sessionId}/messages`,
        {
          params: {
            env: request.env,
            ...request.params,
            after_id: lastMessageId,
            limit: 50,
          },
          useGlobalLoading: false,
          timeout: 60000,
        },
      );

      if (response.code === 200) {
        const newMessages = response.data.items.map(mapMessageDTO);
        messageStore.appendMessages(sessionIdStr, newMessages);
        messageStore.setHasMore(sessionIdStr, newMessages.length >= 50);
        return newMessages;
      }
      return [];
    } catch (error: any) {
      console.error("加载更多消息失败:", error);
      messageStore.setError(
        error?.message || t("agent.chat.errors.loadMoreMessagesFailed"),
      );
      throw error;
    } finally {
      messageStore.setLoading(sessionIdStr, false);
    }
  }

  /**
   * 清空指定 Agent 的全部会话（逐个调用删除接口）
   */
  async function clearAllSessions(agentId: string) {
    const request = capture();
    if (!agentId) return;
    // 确保列表是最新的
    await listSessions(agentId, true);
    const sessions =
      sessionStore.workspace(request.key).sessionsByAgent[agentId] || [];
    // 先清掉本地缓存（即使后端部分失败也不影响 UI 卡死）
    for (const s of sessions) {
      messageStore.clearMessages(JSON.stringify([request.key, String(s.id)]));
    }
    sessionStore.clearSessionsForAgent(agentId, request.key);

    // 再逐个删除后端会话
    for (const s of sessions) {
      try {
        await apiClient.delete(`/agents/sessions/${s.id}`, {
          params: { env: request.env, ...request.params },
          timeout: 60000,
          useGlobalLoading: false,
        });
      } catch (e) {
        // 忽略单个失败，避免“一键清空”半途卡死
        console.warn("清空会话失败:", s.id, e);
      }
    }
  }

  return {
    scopeKey,
    // 状态（从 store 获取）
    sessionsByAgent: computed(() => workspace.value.sessionsByAgent),
    sessionsLoadingByAgent: computed(
      () => workspace.value.sessionsLoadingByAgent,
    ),
    hasMoreByAgent: computed(() => workspace.value.hasMoreByAgent),
    currentSessionId: computed(() => workspace.value.currentSessionId),
    currentAgentId: computed(() => workspace.value.currentAgentId),
    error: computed(() => workspace.value.error),

    // 方法
    listSessions,
    loadMore,
    createSession,
    deleteSession,
    renameSession,
    archiveSession,
    clearAllSessions,
    loadSessionMessages,
    loadMoreMessages,

    // Store 方法的直接暴露
    selectSession: (agentId: string, sessionId: number | string) => {
      sessionStore.selectSession(agentId, sessionId, scopeKey.value);
      writeLastAgentId(agentId);
    },
    selectAgent: (agentId: string) => {
      sessionStore.selectAgent(agentId, scopeKey.value);
      writeLastAgentId(agentId);
    },
    getLastSelectedAgentId: readLastAgentId,
    clearError: () => sessionStore.clearError(scopeKey.value),
    clear: () => sessionStore.clear(scopeKey.value),
  };
}
