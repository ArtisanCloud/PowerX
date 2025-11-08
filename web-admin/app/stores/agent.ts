import { defineStore } from "pinia";
import {
  AgentService,
  type ChatMessage,
  type ChatRequest,
  type AgentStatus,
} from "~/composables/api/services/agentService";

export interface AgentState {
  status: AgentStatus | null;
  messages: ChatMessage[];
  isStreaming: boolean;
  isLoading: boolean;
  conversationId: string | null;
  error: string | null;
}

export const useAgentStore = defineStore("agent", {
  state: (): AgentState => ({
    status: null,
    messages: [],
    isStreaming: false,
    isLoading: false,
    conversationId: null,
    error: null,
  }),

  getters: {
    /**
     * 获取最后一条消息
     */
    lastMessage: (state) => {
      return state.messages[state.messages.length - 1] || null;
    },

    /**
     * 检查是否可以发送消息
     */
    canSendMessage: (state) => {
      return !state.isStreaming && !state.isLoading;
    },
  },

  actions: {
    /**
     * 初始化Agent状态
     */
    async initialize() {
      this.isLoading = true;
      try {
        this.status = await AgentService.getStatus();
        this.error = null;
      } catch (error) {
        console.error("获取Agent状态失败:", error);
        this.error = error instanceof Error ? error.message : "未知错误";
      } finally {
        this.isLoading = false;
      }
    },

    /**
     * 发送消息
     */
    async sendMessage(content: string, useStream = true) {
      if (!this.canSendMessage) {
        throw new Error("当前无法发送消息");
      }

      // 添加用户消息
      const userMessage: ChatMessage = {
        role: "user",
        content,
        timestamp: Date.now(),
      };
      this.messages.push(userMessage);

      const request: ChatRequest = {
        message: content,
        history: this.messages.slice(0, -1), // 不包含刚添加的用户消息
        stream: useStream,
      };

      try {
        if (useStream) {
          await this.handleStreamResponse(request);
        } else {
          await this.handleNormalResponse(request);
        }
        this.error = null;
      } catch (error) {
        console.error("发送消息失败:", error);
        this.error = error instanceof Error ? error.message : "发送失败";
        // 移除失败的用户消息
        this.messages.pop();
        throw error;
      }
    },

    /**
     * 处理流式响应
     */
    async handleStreamResponse(request: ChatRequest) {
      this.isStreaming = true;

      // 添加空的助手消息
      const assistantMessage: ChatMessage = {
        role: "assistant",
        content: "",
        timestamp: Date.now(),
      };
      this.messages.push(assistantMessage);

      try {
        const stream = await AgentService.streamChat(request);
        const reader = stream.getReader();
        const decoder = new TextDecoder();

        while (true) {
          const { done, value } = await reader.read();
          if (done) break;

          const chunk = decoder.decode(value);
          const lines = chunk.split("\n");

          for (const line of lines) {
            if (line.startsWith("data: ")) {
              const data = line.slice(6);
              if (data === "[DONE]") {
                return;
              }

              try {
                const parsed = JSON.parse(data);
                if (parsed.content) {
                  // 更新最后一条助手消息
                  const lastMessage = this.messages[this.messages.length - 1];
                  if (lastMessage && lastMessage.role === "assistant") {
                    lastMessage.content += parsed.content;
                  }
                }
                if (parsed.conversation_id) {
                  this.conversationId = parsed.conversation_id;
                }
              } catch (e) {
                console.warn("解析流数据失败:", e);
              }
            }
          }
        }
      } finally {
        this.isStreaming = false;
      }
    },

    /**
     * 处理普通响应
     */
    async handleNormalResponse(request: ChatRequest) {
      this.isLoading = true;

      try {
        const response = await AgentService.chat(request);

        const assistantMessage: ChatMessage = {
          role: "assistant",
          content: response.message,
          timestamp: Date.now(),
        };
        this.messages.push(assistantMessage);

        if (response.conversation_id) {
          this.conversationId = response.conversation_id;
        }
      } finally {
        this.isLoading = false;
      }
    },

    /**
     * 清空对话
     */
    clearMessages() {
      this.messages = [];
      this.conversationId = null;
      this.error = null;
    },

    /**
     * 清除错误
     */
    clearError() {
      this.error = null;
    },
  },
});
