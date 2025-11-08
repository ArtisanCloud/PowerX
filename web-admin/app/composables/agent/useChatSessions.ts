import { computed } from "vue";
import { useApiClient } from "~/composables/api";
import { useAgentSessionStore } from "~/stores/agentSession";
import { useMessageStore } from "~/stores/message";
import { useI18n } from "vue-i18n";

// 后端会话数据结构
interface SessionDTO {
  id: number;
  createdAt: string;
  updatedAt: string;
  DeletedAt: string | null;
  agentId: number;
  userId: number;
  title: string;
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

export function useChatSessions(opts: { pageSize?: number } = {}) {
  const pageSize = opts.pageSize ?? 50;
  const apiClient = useApiClient();
  const sessionStore = useAgentSessionStore();
  const messageStore = useMessageStore();
  const { t } = useI18n();

  /**
   * 将后端数据转换为前端格式
   */
  function mapSessionDTO(dto: SessionDTO): ChatSession {
    return {
      id: dto.id,
      title: dto.title || t("agent.chat.untitledSession"),
      lastMessage: dto.summary || "",
      updatedAt: new Date(dto.latestAt || dto.updatedAt),
      unread: 0, // 后端暂无此字段，默认为0
      pinned: false, // 后端暂无此字段，默认为false
    };
  }

  /**
   * 将后端消息数据转换为前端格式
   */
  function mapMessageDTO(dto: MessageDTO): ChatMessage {
    return {
      id: dto.id,
      role: dto.role,
      content: dto.content,
      timestamp: new Date(dto.createdAt).getTime(),
      isError: dto.isError,
      meta: dto.meta,
      // 历史消息不应该有流式状态
      isStreaming: false,
      done: true,
      isThinking: false, // 历史消息不是思考状态
    };
  }

  /**
   * 加载指定 agent 的会话列表
   */
  async function listSessions(agentId: number, force = false) {
    // 如果已有缓存且不强制刷新，则跳过
    if (!force && sessionStore.getSessionsByAgent(agentId).length > 0) {
      return;
    }

    sessionStore.setLoading(agentId, true);
    sessionStore.clearError();

    try {
      const response = await apiClient.get<ApiResponse<SessionDTO>>(
        `/agents/sessions`,
        {
          params: {
            agent_id: agentId,
            status: "active",
            limit: pageSize,
            env: "dev",
          },
        }
      );

      if (response.code === 200) {
        const sessions = response.data.items.map(mapSessionDTO);
        sessionStore.setSessions(agentId, sessions);
        sessionStore.setHasMore(agentId, false); // 暂时设为false，后续可根据分页信息调整
      }
    } catch (error: any) {
      console.error("加载会话列表失败:", error);
      sessionStore.setError(
        error?.message || t("agent.chat.errors.loadSessionsFailed")
      );
      sessionStore.setSessions(agentId, []); // 设置空数组避免重复请求
      sessionStore.setHasMore(agentId, false);
      throw error;
    } finally {
      sessionStore.setLoading(agentId, false);
    }
  }

  /**
   * 加载更多会话（分页）
   */
  async function loadMore(agentId: number) {
    if (
      !sessionStore.getHasMoreByAgent(agentId) ||
      sessionStore.isLoadingByAgent(agentId)
    ) {
      return;
    }

    sessionStore.setLoading(agentId, true);

    try {
      // TODO: 实现分页逻辑，当前后端暂不支持游标分页
      const currentSessions = sessionStore.getSessionsByAgent(agentId);
      const offset = currentSessions.length;

      const response = await apiClient.get<ApiResponse<SessionDTO>>(
        `/agents/sessions`,
        {
          params: {
            agent_id: agentId,
            status: "active",
            limit: pageSize,
            offset: offset,
            env: "dev",
          },
        }
      );

      if (response.code === 200) {
        const newSessions = response.data.items.map(mapSessionDTO);
        const allSessions = [...currentSessions, ...newSessions];
        sessionStore.setSessions(agentId, allSessions);
        sessionStore.setHasMore(agentId, newSessions.length >= pageSize);
      }
    } catch (error: any) {
      console.error("加载更多会话失败:", error);
      sessionStore.setError(
        error?.message || t("agent.chat.errors.loadMoreSessionsFailed")
      );
      throw error;
    } finally {
      sessionStore.setLoading(agentId, false);
    }
  }

  /**
   * 创建新会话
   */
  async function createSession(
    agentId: number,
    title?: string
  ): Promise<ChatSession> {
    try {
      const response = await apiClient.post<SessionDTO>(`/agents/sessions`, {
        env: "dev",
        agentId: agentId,
        title: title || t("agent.chat.newSession"),
      });

      if (response) {
        const newSession = mapSessionDTO(response);
        sessionStore.addSession(agentId, newSession);
        return newSession;
      }
      throw new Error(t("agent.chat.errors.createSessionFailedNoData"));
    } catch (error: any) {
      console.error("创建会话失败:", error);
      sessionStore.setError(
        error?.message || t("agent.chat.errors.createSessionFailed")
      );
      throw error;
    }
  }

  /**
   * 删除会话
   */
  async function deleteSession(agentId: number, sessionId: number | string) {
    try {
      await apiClient.delete(`/agents/sessions/${sessionId}`, {
        params: {
          env: "dev",
        },
      });

      sessionStore.removeSession(agentId, sessionId);
    } catch (error: any) {
      console.error("删除会话失败:", error);
      sessionStore.setError(
        error?.message || t("agent.chat.errors.deleteSessionFailed")
      );
      throw error;
    }
  }

  /**
   * 重命名会话
   */
  async function renameSession(
    agentId: number,
    sessionId: number | string,
    title: string
  ) {
    try {
      await apiClient.patch(
        `/agents/sessions/${sessionId}`,
        { title },
        {
          params: {
            env: "dev",
          },
        }
      );

      sessionStore.updateSession(agentId, sessionId, { title });
    } catch (error: any) {
      console.error("重命名会话失败:", error);
      sessionStore.setError(
        error?.message || t("agent.chat.errors.renameSessionFailed")
      );
      throw error;
    }
  }

  /**
   * 归档会话
   */
  async function archiveSession(agentId: number, sessionId: number | string) {
    try {
      await apiClient.post(`/agents/sessions/${sessionId}/archive`, undefined, {
        params: {
          env: "dev",
        },
      });

      // 归档后从当前列表中移除
      sessionStore.removeSession(agentId, sessionId);
    } catch (error: any) {
      console.error("归档会话失败:", error);
      sessionStore.setError(
        error?.message || t("agent.chat.errors.archiveSessionFailed")
      );
      throw error;
    }
  }

  /**
   * 加载会话消息（带缓存）
   */
  async function loadSessionMessages(
    sessionId: number | string,
    force = false
  ): Promise<ChatMessage[]> {
    const sessionIdStr = String(sessionId);

    // 如果有缓存且不强制刷新，则返回缓存的消息
    if (!force && messageStore.getMessagesBySession(sessionIdStr).length > 0) {
      // console.log(
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
            env: "dev",
            limit: 200,
          },
        }
      );

      if (response.code === 200) {
        const messages = response.data.items.map(mapMessageDTO);
        // console.log("[useChatSessions] 加载会话消息成功:", messages);
        messageStore.setMessages(sessionIdStr, messages);
        return messages;
      }
      return [];
    } catch (error: any) {
      console.error("加载会话消息失败:", error);
      messageStore.setError(
        error?.message || t("agent.chat.errors.loadMessagesFailed")
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
    sessionId: number | string
  ): Promise<ChatMessage[]> {
    const sessionIdStr = String(sessionId);
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
            env: "dev",
            after_id: lastMessageId,
            limit: 50,
          },
        }
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
        error?.message || t("agent.chat.errors.loadMoreMessagesFailed")
      );
      throw error;
    } finally {
      messageStore.setLoading(sessionIdStr, false);
    }
  }

  return {
    // 状态（从 store 获取）
    sessionsByAgent: computed(() => sessionStore.sessionsByAgent),
    sessionsLoadingByAgent: computed(() => sessionStore.sessionsLoadingByAgent),
    hasMoreByAgent: computed(() => sessionStore.hasMoreByAgent),
    currentSessionId: computed(() => sessionStore.currentSessionId),
    currentAgentId: computed(() => sessionStore.currentAgentId),
    error: computed(() => sessionStore.error),

    // 方法
    listSessions,
    loadMore,
    createSession,
    deleteSession,
    renameSession,
    archiveSession,
    loadSessionMessages,
    loadMoreMessages,

    // Store 方法的直接暴露
    selectSession: sessionStore.selectSession,
    selectAgent: sessionStore.selectAgent,
    clearError: sessionStore.clearError,
    clear: sessionStore.clear,
  };
}
