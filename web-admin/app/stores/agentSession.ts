import { defineStore } from "pinia";
import type { ChatSession } from "~/composables/agent/useChatSessions";

export interface AgentSessionState {
  // 按 agentId 分组的会话列表
  sessionsByAgent: Record<string, ChatSession[]>;
  // 加载状态
  sessionsLoadingByAgent: Record<string, boolean>;
  // 是否还有更多数据
  hasMoreByAgent: Record<string, boolean>;
  // 当前选中的会话ID
  currentSessionId: number | string | null;
  // 当前选中的agentId
  currentAgentId: string | null;
  // 错误信息
  error: string | null;
}

export const useAgentSessionStore = defineStore("agentSession", {
  state: (): AgentSessionState => ({
    sessionsByAgent: {},
    sessionsLoadingByAgent: {},
    hasMoreByAgent: {},
    currentSessionId: null,
    currentAgentId: null,
    error: null,
  }),

  getters: {
    /**
     * 获取指定 agent 的会话列表
     */
    getSessionsByAgent: (state) => (agentId: string) => {
      return state.sessionsByAgent[agentId] || [];
    },

    /**
     * 获取指定 agent 的加载状态
     */
    isLoadingByAgent: (state) => (agentId: string) => {
      return state.sessionsLoadingByAgent[agentId] || false;
    },

    /**
     * 获取指定 agent 是否还有更多数据
     */
    getHasMoreByAgent: (state) => (agentId: string) => {
      return state.hasMoreByAgent[agentId] || false;
    },

    /**
     * 获取当前选中的会话
     */
    currentSession: (state) => {
      if (!state.currentAgentId || !state.currentSessionId) return null;
      const sessions = state.sessionsByAgent[state.currentAgentId] || [];
      return sessions.find((s) => s.id === state.currentSessionId) || null;
    },
  },

  actions: {
    /**
     * 设置加载状态
     */
    setLoading(agentId: string, loading: boolean) {
      this.sessionsLoadingByAgent = {
        ...this.sessionsLoadingByAgent,
        [agentId]: loading,
      };
    },

    /**
     * 设置会话列表
     */
    setSessions(agentId: string, sessions: ChatSession[]) {
      this.sessionsByAgent = {
        ...this.sessionsByAgent,
        [agentId]: sessions,
      };
    },

    /**
     * 添加会话到列表开头
     */
    addSession(agentId: string, session: ChatSession) {
      const currentSessions = this.sessionsByAgent[agentId] || [];
      this.sessionsByAgent = {
        ...this.sessionsByAgent,
        [agentId]: [session, ...currentSessions],
      };

      // 自动选中新会话
      this.currentSessionId = session.id;
      this.currentAgentId = agentId;
    },

    /**
     * 从列表中移除会话
     */
    removeSession(agentId: string, sessionId: number | string) {
      const sessions = this.sessionsByAgent[agentId] || [];
      const filteredSessions = sessions.filter((s) => s.id !== sessionId);

      this.sessionsByAgent = {
        ...this.sessionsByAgent,
        [agentId]: filteredSessions,
      };

      // 如果删除的是当前选中的会话，自动选中第一个
      if (this.currentSessionId === sessionId) {
        this.currentSessionId =
          filteredSessions.length > 0 ? filteredSessions[0].id : null;
      }
    },

    /**
     * 更新会话信息
     */
    updateSession(
      agentId: string,
      sessionId: number | string,
      updates: Partial<ChatSession>
    ) {
      const sessions = this.sessionsByAgent[agentId] || [];
      const session = sessions.find((s) => s.id === sessionId);
      if (session) {
        Object.assign(session, updates);
      }
    },

    /**
     * 设置是否还有更多数据
     */
    setHasMore(agentId: string, hasMore: boolean) {
      this.hasMoreByAgent = {
        ...this.hasMoreByAgent,
        [agentId]: hasMore,
      };
    },

    /**
     * 设置错误信息
     */
    setError(error: string | null) {
      this.error = error;
    },

    /**
     * 选择会话
     */
    selectSession(agentId: string, sessionId: number | string) {
      this.currentAgentId = agentId;
      this.currentSessionId = sessionId;
    },

    /**
     * 选择 Agent
     */
    selectAgent(agentId: string) {
      this.currentAgentId = agentId;
      this.currentSessionId = null;
    },

    /**
     * 清除错误
     */
    clearError() {
      this.error = null;
    },

    /**
     * 清除所有数据
     */
    clear() {
      this.sessionsByAgent = {};
      this.sessionsLoadingByAgent = {};
      this.hasMoreByAgent = {};
      this.currentSessionId = null;
      this.currentAgentId = null;
      this.error = null;
    },

    /**
     * 清空某个 Agent 的会话列表
     */
    clearSessionsForAgent(agentId: string) {
      this.sessionsByAgent = { ...this.sessionsByAgent, [agentId]: [] };
      this.sessionsLoadingByAgent = {
        ...this.sessionsLoadingByAgent,
        [agentId]: false,
      };
      this.hasMoreByAgent = { ...this.hasMoreByAgent, [agentId]: false };
      if (this.currentAgentId === agentId) {
        this.currentSessionId = null;
      }
    },
  },
});
