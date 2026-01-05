/**
 * Agent服务 - 处理用户与Agent的交互
 * 对应后端路由: /agents/*
 */

import { ApiEndpoints } from "../config";

// 类型定义
export interface AgentStatus {
  status: string;
  message?: string;
}

export interface ProcessIntentRequest {
  intent: string;
  context?: any;
}

export interface ProcessIntentResponse {
  success: boolean;
  result?: any;
  error?: string;
}

export interface PlanPreviewRequest {
  query: string;
  context?: any;
}

export interface PlanPreviewResponse {
  plan: string;
  steps: string[];
  estimated_time?: number;
}

export interface ChatMessage {
  role: "user" | "assistant";
  content: string;
  timestamp?: number;
}

export interface ChatRequest {
  message: string;
  history?: ChatMessage[];
  stream?: boolean;
}

export interface ChatResponse {
  message: string;
  conversation_id?: string;
  metadata?: any;
}

/**
 * Agent服务类
 */
export class AgentService {
  /**
   * 获取Agent状态
   */
  static async getStatus(): Promise<AgentStatus> {
    const response: any = await $fetch<any>(ApiEndpoints.AGENTS.STATUS);
    // 兼容后端统一响应体：{ code, message, data, timestamp }
    if (response && typeof response === "object" && "code" in response) {
      return (response.data || {}) as AgentStatus;
    }
    return (response || {}) as AgentStatus;
  }

  /**
   * 处理意图
   */
  static async processIntent(
    request: ProcessIntentRequest
  ): Promise<ProcessIntentResponse> {
    const response = await $fetch<ProcessIntentResponse>(
      ApiEndpoints.AGENTS.INTENT,
      {
        method: "POST",
        body: request,
      }
    );
    return response;
  }

  /**
   * 获取计划预览
   */
  static async getPlanPreview(
    request: PlanPreviewRequest
  ): Promise<PlanPreviewResponse> {
    const response = await $fetch<PlanPreviewResponse>(
      ApiEndpoints.AGENTS.PLAN_PREVIEW,
      {
        method: "POST",
        body: request,
      }
    );
    return response;
  }

  /**
   * 流式聊天
   */
  static async streamChat(request: ChatRequest): Promise<ReadableStream> {
    const response = await fetch(ApiEndpoints.AGENTS.CHAT_STREAM, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify(request),
    });

    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }

    return response.body!;
  }

  /**
   * 普通聊天
   */
  static async chat(request: ChatRequest): Promise<ChatResponse> {
    const response = await $fetch<ChatResponse>(ApiEndpoints.AGENTS.CHAT, {
      method: "POST",
      body: request,
    });
    return response;
  }
}
