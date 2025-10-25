import { defineStore } from "pinia";
import type { ChatMessage } from "~/composables/agent/useChatSessions";

export interface MessageState {
  /** 按会话ID缓存的消息列表 */
  messagesBySession: Record<string, ChatMessage[]>;
  /** 消息加载状态 */
  loadingBySession: Record<string, boolean>;
  /** 是否还有更多消息 */
  hasMoreBySession: Record<string, boolean>;
  /** 最后加载的消息ID（用于分页） */
  lastMessageIdBySession: Record<string, number>;
  /** 错误信息 */
  error: string | null;
}

export const useMessageStore = defineStore("message", {
  state: (): MessageState => ({
    messagesBySession: {},
    loadingBySession: {},
    hasMoreBySession: {},
    lastMessageIdBySession: {},
    error: null,
  }),

  getters: {
    /**
     * 获取指定会话的消息列表
     */
    getMessagesBySession: (state) => (sessionId: string | number) => {
      const key = String(sessionId);
      return state.messagesBySession[key] || [];
    },

    /**
     * 获取指定会话的加载状态
     */
    getIsLoadingBySession: (state) => (sessionId: string | number) => {
      const key = String(sessionId);
      return state.loadingBySession[key] || false;
    },

    /**
     * 获取指定会话是否还有更多消息
     */
    getHasMoreBySession: (state) => (sessionId: string | number) => {
      const key = String(sessionId);
      return state.hasMoreBySession[key] || false;
    },
  },

  actions: {
    /** 工具函数：统一 sessionId 为 string key */
    _key(sessionId: string | number) {
      return String(sessionId);
    },

    /**
     * 设置加载状态
     */
    setLoading(sessionId: string | number, loading: boolean) {
      const key = this._key(sessionId);
      this.loadingBySession = {
        ...this.loadingBySession,
        [key]: loading,
      };
    },

    /**
     * 设置会话消息（可选传入 pageSize 以便同时设置 hasMore）
     */
    setMessages(
      sessionId: string | number,
      messages: ChatMessage[],
      pageSize?: number
    ) {
      const key = this._key(sessionId);

      this.messagesBySession = {
        ...this.messagesBySession,
        [key]: messages,
      };

      // 更新最后消息ID
      if (messages.length > 0) {
        const lastMessage = messages[messages.length - 1];
        if (typeof lastMessage.id === "number") {
          this.lastMessageIdBySession = {
            ...this.lastMessageIdBySession,
            [key]: lastMessage.id,
          };
        }
      } else {
        // 没有消息则重置 lastMessageId
        this.lastMessageIdBySession = {
          ...this.lastMessageIdBySession,
          [key]: 0,
        };
      }

      // 可选：根据 pageSize 设定 hasMore
      if (typeof pageSize === "number") {
        this.hasMoreBySession = {
          ...this.hasMoreBySession,
          [key]: messages.length >= pageSize,
        };
      }
    },

    /**
     * 添加消息到会话（用于实时消息）
     */
    addMessage(sessionId: string | number, message: ChatMessage) {
      const key = this._key(sessionId);
      const currentMessages = this.messagesBySession[key] || [];
      const nextMessages = [...currentMessages, message];

      this.messagesBySession = {
        ...this.messagesBySession,
        [key]: nextMessages,
      };

      if (typeof message.id === "number") {
        this.lastMessageIdBySession = {
          ...this.lastMessageIdBySession,
          [key]: message.id,
        };
      }
    },

    /**
     * 更新消息（用于流式消息更新）
     */
    updateMessage(
      sessionId: string | number,
      messageId: string | number,
      updates: Partial<ChatMessage>
    ) {
      const key = this._key(sessionId);
      const messages = this.messagesBySession[key] || [];
      const idx = messages.findIndex((m) => m.id === messageId);

      if (idx !== -1) {
        const next = [...messages];
        next[idx] = { ...next[idx], ...updates };
        this.messagesBySession = {
          ...this.messagesBySession,
          [key]: next,
        };
      }
    },

    /**
     * 追加更多消息（分页加载）
     */
    appendMessages(sessionId: string | number, newMessages: ChatMessage[]) {
      const key = this._key(sessionId);
      const current = this.messagesBySession[key] || [];
      const next = [...current, ...newMessages];

      this.messagesBySession = {
        ...this.messagesBySession,
        [key]: next,
      };

      // 更新最后消息ID
      if (newMessages.length > 0) {
        const lastMessage = newMessages[newMessages.length - 1];
        if (typeof lastMessage.id === "number") {
          this.lastMessageIdBySession = {
            ...this.lastMessageIdBySession,
            [key]: lastMessage.id,
          };
        }
      }
    },

    /**
     * 设置是否还有更多消息
     */
    setHasMore(sessionId: string | number, hasMore: boolean) {
      const key = this._key(sessionId);
      this.hasMoreBySession = {
        ...this.hasMoreBySession,
        [key]: hasMore,
      };
    },

    /**
     * 清除会话消息
     */
    clearMessages(sessionId: string | number) {
      const key = this._key(sessionId);
      this.messagesBySession = {
        ...this.messagesBySession,
        [key]: [],
      };
      this.lastMessageIdBySession = {
        ...this.lastMessageIdBySession,
        [key]: 0,
      };
      this.hasMoreBySession = {
        ...this.hasMoreBySession,
        [key]: false,
      };
    },

    /**
     * 设置错误信息
     */
    setError(error: string | null) {
      this.error = error;
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
      this.messagesBySession = {};
      this.loadingBySession = {};
      this.hasMoreBySession = {};
      this.lastMessageIdBySession = {};
      this.error = null;
    },
  },
});
