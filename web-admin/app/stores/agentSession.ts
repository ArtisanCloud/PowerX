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

const emptyWorkspace = (): AgentSessionState => ({
  sessionsByAgent: {},
  sessionsLoadingByAgent: {},
  hasMoreByAgent: {},
  currentSessionId: null,
  currentAgentId: null,
  error: null,
});

export const useAgentSessionStore = defineStore("agentSession", {
  state: () => ({
    activeScopeKey: "",
    workspaces: {} as Record<string, AgentSessionState>,
  }),
  getters: {
    activeWorkspace: (state): AgentSessionState =>
      state.workspaces[state.activeScopeKey] || emptyWorkspace(),
    sessionsByAgent(): AgentSessionState["sessionsByAgent"] {
      return this.activeWorkspace.sessionsByAgent;
    },
    sessionsLoadingByAgent(): AgentSessionState["sessionsLoadingByAgent"] {
      return this.activeWorkspace.sessionsLoadingByAgent;
    },
    hasMoreByAgent(): AgentSessionState["hasMoreByAgent"] {
      return this.activeWorkspace.hasMoreByAgent;
    },
    currentSessionId(): AgentSessionState["currentSessionId"] {
      return this.activeWorkspace.currentSessionId;
    },
    currentAgentId(): AgentSessionState["currentAgentId"] {
      return this.activeWorkspace.currentAgentId;
    },
    error(): AgentSessionState["error"] {
      return this.activeWorkspace.error;
    },

    /**
     * 获取指定 agent 的会话列表
     */
    getSessionsByAgent() {
      return (agentId: string) => this.sessionsByAgent[agentId] || [];
    },

    /**
     * 获取指定 agent 的加载状态
     */
    isLoadingByAgent() {
      return (agentId: string) => this.sessionsLoadingByAgent[agentId] || false;
    },

    /**
     * 获取指定 agent 是否还有更多数据
     */
    getHasMoreByAgent() {
      return (agentId: string) => this.hasMoreByAgent[agentId] || false;
    },

    /**
     * 获取当前选中的会话
     */
    currentSession() {
      if (!this.currentAgentId || !this.currentSessionId) return null;
      return (
        (this.sessionsByAgent[this.currentAgentId] || []).find(
          (s) => s.id === this.currentSessionId,
        ) || null
      );
    },
  },

  actions: {
    workspace(scopeKey = this.activeScopeKey): AgentSessionState {
      return (this.workspaces[scopeKey] ||= emptyWorkspace());
    },
    activateScope(scopeKey: string) {
      this.workspace(scopeKey);
      this.activeScopeKey = scopeKey;
    },
    /**
     * 设置加载状态
     */
    setLoading(
      agentId: string,
      loading: boolean,
      scopeKey = this.activeScopeKey,
    ) {
      this.workspace(scopeKey).sessionsLoadingByAgent = {
        ...this.workspace(scopeKey).sessionsLoadingByAgent,
        [agentId]: loading,
      };
    },

    /**
     * 设置会话列表
     */
    setSessions(
      agentId: string,
      sessions: ChatSession[],
      scopeKey = this.activeScopeKey,
    ) {
      this.workspace(scopeKey).sessionsByAgent = {
        ...this.workspace(scopeKey).sessionsByAgent,
        [agentId]: sessions,
      };
    },

    /**
     * 添加会话到列表开头
     */
    addSession(
      agentId: string,
      session: ChatSession,
      scopeKey = this.activeScopeKey,
    ) {
      const currentSessions =
        this.workspace(scopeKey).sessionsByAgent[agentId] || [];
      this.workspace(scopeKey).sessionsByAgent = {
        ...this.workspace(scopeKey).sessionsByAgent,
        [agentId]: [session, ...currentSessions],
      };

      // 自动选中新会话
      this.workspace(scopeKey).currentSessionId = session.id;
      this.workspace(scopeKey).currentAgentId = agentId;
    },

    /**
     * 从列表中移除会话
     */
    removeSession(
      agentId: string,
      sessionId: number | string,
      scopeKey = this.activeScopeKey,
    ) {
      const sessions = this.workspace(scopeKey).sessionsByAgent[agentId] || [];
      const filteredSessions = sessions.filter((s) => s.id !== sessionId);

      this.workspace(scopeKey).sessionsByAgent = {
        ...this.workspace(scopeKey).sessionsByAgent,
        [agentId]: filteredSessions,
      };

      // 如果删除的是当前选中的会话，自动选中第一个
      if (this.workspace(scopeKey).currentSessionId === sessionId) {
        this.workspace(scopeKey).currentSessionId =
          filteredSessions.length > 0 ? filteredSessions[0].id : null;
      }
    },

    /**
     * 更新会话信息
     */
    updateSession(
      agentId: string,
      sessionId: number | string,
      updates: Partial<ChatSession>,
      scopeKey = this.activeScopeKey,
    ) {
      const sessions = this.workspace(scopeKey).sessionsByAgent[agentId] || [];
      const session = sessions.find((s) => s.id === sessionId);
      if (session) {
        Object.assign(session, updates);
      }
    },

    /**
     * 设置是否还有更多数据
     */
    setHasMore(
      agentId: string,
      hasMore: boolean,
      scopeKey = this.activeScopeKey,
    ) {
      this.workspace(scopeKey).hasMoreByAgent = {
        ...this.workspace(scopeKey).hasMoreByAgent,
        [agentId]: hasMore,
      };
    },

    /**
     * 设置错误信息
     */
    setError(error: string | null, scopeKey = this.activeScopeKey) {
      this.workspace(scopeKey).error = error;
    },

    /**
     * 选择会话
     */
    selectSession(
      agentId: string,
      sessionId: number | string,
      scopeKey = this.activeScopeKey,
    ) {
      this.workspace(scopeKey).currentAgentId = agentId;
      this.workspace(scopeKey).currentSessionId = sessionId;
    },

    /**
     * 选择 Agent
     */
    selectAgent(agentId: string, scopeKey = this.activeScopeKey) {
      if (this.workspace(scopeKey).currentAgentId !== agentId) {
        this.workspace(scopeKey).currentAgentId = agentId;
        this.workspace(scopeKey).currentSessionId = null;
      }
    },

    /**
     * 清除错误
     */
    clearError(scopeKey = this.activeScopeKey) {
      this.workspace(scopeKey).error = null;
    },

    /**
     * 清除所有数据
     */
    clear(scopeKey = this.activeScopeKey) {
      this.workspace(scopeKey).sessionsByAgent = {};
      this.workspace(scopeKey).sessionsLoadingByAgent = {};
      this.workspace(scopeKey).hasMoreByAgent = {};
      this.workspace(scopeKey).currentSessionId = null;
      this.workspace(scopeKey).currentAgentId = null;
      this.workspace(scopeKey).error = null;
    },

    /**
     * 清空某个 Agent 的会话列表
     */
    clearSessionsForAgent(agentId: string, scopeKey = this.activeScopeKey) {
      this.workspace(scopeKey).sessionsByAgent = {
        ...this.workspace(scopeKey).sessionsByAgent,
        [agentId]: [],
      };
      this.workspace(scopeKey).sessionsLoadingByAgent = {
        ...this.workspace(scopeKey).sessionsLoadingByAgent,
        [agentId]: false,
      };
      this.workspace(scopeKey).hasMoreByAgent = {
        ...this.workspace(scopeKey).hasMoreByAgent,
        [agentId]: false,
      };
      if (this.workspace(scopeKey).currentAgentId === agentId) {
        this.workspace(scopeKey).currentSessionId = null;
      }
    },
  },
});
